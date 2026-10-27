package sd99

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const Model = "video-sd-2.0(TC)"

// SubmissionAttempted prevents replay of a potentially billable POST, even when
// its response was lost or malformed. It is set only immediately before sending.
const SubmissionAttempted = "sd99_submission_attempted"

var taskIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type mediaURL struct {
	URL string `json:"url"`
}

type contentItem struct {
	Type     string    `json:"type"`
	Role     string    `json:"role"`
	ImageURL *mediaURL `json:"image_url,omitempty"`
	VideoURL *mediaURL `json:"video_url,omitempty"`
	AudioURL *mediaURL `json:"audio_url,omitempty"`
}

type metadata struct {
	Ratio         string        `json:"ratio"`
	Resolution    string        `json:"resolution"`
	GenerateAudio *bool         `json:"generate_audio,omitempty"`
	Content       []contentItem `json:"content,omitempty"`
}

type requestPayload struct {
	Model    string   `json:"model"`
	Prompt   string   `json:"prompt"`
	Duration int      `json:"duration"`
	Metadata metadata `json:"metadata"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	baseURL       string
	apiKey        string
	probeDuration func(context.Context, string) (float64, error)
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		return service.TaskErrorWrapperLocal(fmt.Errorf("SD99 requires application/json and media URLs"), "invalid_request", http.StatusBadRequest)
	}
	if info.Action == constant.TaskActionRemix {
		return service.TaskErrorWrapperLocal(fmt.Errorf("SD99 editing requires duration=-1 and a source video, not remix"), "unsupported_action", http.StatusBadRequest)
	}
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	var raw struct {
		Metadata metadata `json:"metadata"`
	}
	if err := common.UnmarshalBodyReusable(c, &raw); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_metadata", http.StatusBadRequest)
	}
	m := raw.Metadata
	if m.Resolution == "" {
		m.Resolution = taskcommon.NormalizeVideoResolution(req.Size)
	}
	if m.Resolution == "" {
		m.Resolution = "480p"
	}
	if m.Ratio == "" {
		m.Ratio = req.Ratio
	}
	if m.Ratio == "" {
		m.Ratio = "16:9"
	}
	if req.GenerateAudio != nil {
		m.GenerateAudio = req.GenerateAudio
	}
	for _, image := range req.Images {
		m.Content = append(m.Content, contentItem{Type: "image_url", Role: "reference_image", ImageURL: &mediaURL{URL: image}})
	}
	for _, video := range req.InputVideos {
		m.Content = append(m.Content, contentItem{Type: "video_url", Role: "reference_video", VideoURL: &mediaURL{URL: video}})
	}
	for _, audio := range req.InputAudios {
		m.Content = append(m.Content, contentItem{Type: "audio_url", Role: "reference_audio", AudioURL: &mediaURL{URL: audio}})
	}
	payload := requestPayload{Model: req.Model, Prompt: req.Prompt, Metadata: m}
	seconds, err := strconv.ParseFloat(req.Seconds, 64)
	if req.Seconds == "" {
		seconds, err = 5, nil
	}
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || math.Trunc(seconds) != seconds || (seconds != -1 && (seconds < 5 || seconds > 15)) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("SD99 duration must be an integer from 5 to 15, or -1 for editing"), "invalid_seconds", http.StatusBadRequest)
	}
	payload.Duration = int(seconds) // bounded to -1 or 5..15 above
	if err := payload.validate(); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if seconds == -1 {
		probe := a.probeDuration
		if probe == nil {
			probe = service.ProbeVideoDurationURL
		}
		var source string
		for _, item := range payload.Metadata.Content {
			if item.VideoURL != nil {
				source = item.VideoURL.URL
			}
		}
		duration, err := probe(c.Request.Context(), source)
		if err != nil {
			return service.TaskErrorWrapperLocal(fmt.Errorf("cannot detect SD99 source video duration: %w", err), "invalid_input_video", http.StatusBadRequest)
		}
		if math.IsNaN(duration) || math.IsInf(duration, 0) || duration < 4 || duration > 15 {
			return service.TaskErrorWrapperLocal(fmt.Errorf("SD99 editing requires a source video from 4 to 15 seconds"), "invalid_input_video", http.StatusBadRequest)
		}
		seconds = math.Ceil(duration)
	}
	// Billing sees only positive, validated seconds. The upstream sentinel remains
	// in the private context payload and can never enter quota multiplication.
	req.Seconds = strconv.Itoa(int(seconds))
	req.Duration = int(seconds)
	req.Size = payload.Metadata.Resolution
	req.FPS = nil // this protocol does not expose FPS; do not let it change billing
	encoded, err := common.Marshal(payload.Metadata)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_metadata", http.StatusBadRequest)
	}
	req.Metadata = nil
	if err := common.Unmarshal(encoded, &req.Metadata); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_metadata", http.StatusBadRequest)
	}
	// Preserve normalized metadata as the sole media source for billing probes.
	req.Images, req.InputVideos, req.InputAudios = nil, nil, nil
	req.Image, req.InputVideo, req.InputAudio, req.InputReference = "", "", "", ""
	req.InputVideoSeconds = nil
	if taskErr := relaycommon.ValidateParsedTaskRequest(c, info, constant.TaskActionGenerate, req); taskErr != nil {
		return taskErr
	}
	c.Set("sd99_payload", payload)
	return nil
}

func (r *requestPayload) validate() error {
	if strings.TrimSpace(r.Model) == "" || strings.TrimSpace(r.Prompt) == "" {
		return fmt.Errorf("model and prompt are required")
	}
	if r.Metadata.Resolution != "480p" && r.Metadata.Resolution != "720p" && r.Metadata.Resolution != "1080p" {
		return fmt.Errorf("SD99 resolution must be 480p, 720p or 1080p")
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	items := make([]contentItem, 0, len(r.Metadata.Content))
	for _, item := range r.Metadata.Content {
		var media *mediaURL
		var role string
		switch item.Type {
		case "image_url":
			media, role = item.ImageURL, "reference_image"
		case "video_url":
			media, role = item.VideoURL, "reference_video"
		case "audio_url":
			media, role = item.AudioURL, "reference_audio"
		default:
			return fmt.Errorf("unsupported SD99 content type")
		}
		if media == nil || item.Role != role {
			return fmt.Errorf("SD99 content requires matching media URL and reference role")
		}
		media.URL = strings.TrimSpace(media.URL)
		parsed, err := url.Parse(media.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return fmt.Errorf("SD99 references must be HTTPS media URLs")
		}
		if (item.ImageURL != nil && item.Type != "image_url") || (item.VideoURL != nil && item.Type != "video_url") || (item.AudioURL != nil && item.Type != "audio_url") {
			return fmt.Errorf("SD99 content must contain exactly one media URL")
		}
		if seen[media.URL] {
			continue
		}
		seen[media.URL] = true
		counts[item.Type]++
		items = append(items, item)
	}
	if counts["image_url"] > 9 || counts["video_url"] > 3 || counts["audio_url"] > 3 {
		return fmt.Errorf("SD99 supports at most 9 images, 3 videos and 3 audio references")
	}
	if counts["audio_url"] > 0 && counts["image_url"]+counts["video_url"] == 0 {
		return fmt.Errorf("SD99 audio references require an image or video")
	}
	r.Metadata.Content = items
	if r.Duration == -1 {
		if counts["video_url"] != 1 || r.Metadata.Ratio != "adaptive" {
			return fmt.Errorf("SD99 editing requires exactly one source video and ratio=adaptive")
		}
		return nil
	}
	switch r.Metadata.Ratio {
	case "16:9", "9:16", "1:1", "4:3", "3:4", "21:9":
		return nil
	default:
		return fmt.Errorf("unsupported SD99 ratio")
	}
}

// APIURL accepts both an origin and a base ending in /v1.
func APIURL(baseURL, suffix string) (string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", fmt.Errorf("invalid SD99 base URL")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(u.Path, "/v1") {
		u.Path += "/v1"
	}
	u.Path += suffix
	u.RawPath, u.Fragment = "", ""
	return u.String(), nil
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return APIURL(a.baseURL, "/videos")
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	value, ok := c.Get("sd99_payload")
	payload, valid := value.(requestPayload)
	if !ok || !valid {
		return nil, fmt.Errorf("SD99 request was not validated")
	}
	payload.Model = info.UpstreamModelName
	data, err := common.Marshal(payload)
	return bytes.NewReader(data), err
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (*http.Response, error) {
	endpoint, err := a.BuildRequestURL(info)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	if err := a.BuildRequestHeader(c, req, info); err != nil {
		return nil, err
	}
	client, err := service.GetHttpClientWithProxy(info.ChannelSetting.Proxy)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	submitClient := *client
	submitClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.Set(SubmissionAttempted, true)
	return submitClient.Do(req)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || !gjson.ValidBytes(body) || responseError(body) != "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("SD99 submission result requires verification; do not resubmit automatically"), "submission_unconfirmed", http.StatusBadGateway)
	}
	id := ""
	for _, path := range []string{"id", "task_id", "job_id"} {
		value := gjson.GetBytes(body, path)
		if value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
			id = strings.TrimSpace(value.String())
			break
		}
	}
	if !taskIDPattern.MatchString(id) {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("SD99 returned no valid task ID; verify submission before retrying"), "submission_unconfirmed", http.StatusBadGateway)
	}
	video := dto.NewOpenAIVideo()
	video.ID, video.TaskID, video.Model = info.PublicTaskID, info.PublicTaskID, info.OriginModelName
	video.CreatedAt = time.Now().Unix()
	c.JSON(http.StatusOK, video)
	return id, body, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	id, _ := body["task_id"].(string)
	if !taskIDPattern.MatchString(id) {
		return nil, fmt.Errorf("invalid SD99 task ID")
	}
	a.baseURL = baseURL
	endpoint, err := APIURL(baseURL, "/videos/"+id)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	queryClient := *client
	queryClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := queryClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("SD99 task query returned HTTP %d; keep polling original task", resp.StatusCode)
	}
	return resp, nil
}

func responseError(body []byte) string {
	for _, path := range []string{"error.code", "code", "error.message", "message", "reason", "error_msg", "fail_reason", "error"} {
		value := gjson.GetBytes(body, path)
		if strings.Contains(strings.ToLower(value.String()), "fail_to_fetch_task") {
			return "fail_to_fetch_task"
		}
	}
	if value := gjson.GetBytes(body, "error"); value.Exists() && value.Type != gjson.Null && value.String() != "" {
		return "upstream_error"
	}
	if value := gjson.GetBytes(body, "code"); value.Exists() && value.Type != gjson.Null {
		switch strings.ToLower(strings.TrimSpace(value.String())) {
		case "", "0", "200", "success":
		default:
			return "upstream_error"
		}
	}
	return ""
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	if !gjson.ValidBytes(body) || responseError(body) == "fail_to_fetch_task" {
		return nil, fmt.Errorf("SD99 query failed; keep polling original task")
	}
	result := &relaycommon.TaskInfo{}
	switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String())) {
	case "created", "queued", "pending", "submitted", "pending_review":
		result.Status = model.TaskStatusQueued
	case "in_progress", "processing", "running":
		result.Status = model.TaskStatusInProgress
	case "completed", "succeeded", "success":
		result.Status = model.TaskStatusSuccess
		result.Progress = "100%"
		if responseError(body) != "" {
			return nil, fmt.Errorf("SD99 returned a query error with a success status")
		}
		rawURL := resultURL(body)
		if rawURL != "" {
			_, err := resolveMediaURL(a.baseURL, rawURL)
			if err != nil {
				return nil, err
			}
			// Leave Url empty so the poller publishes our authenticated content
			// proxy for both direct links and the upstream /content fallback.
		}
	case "failed", "error", "cancelled", "canceled", "expired", "refunded":
		result.Status = model.TaskStatusFailure
		result.Progress = "100%"
		for _, path := range []string{"error.message", "message", "reason", "error_msg", "fail_reason", "error"} {
			value := gjson.GetBytes(body, path)
			if value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
				result.Reason = value.String()
				break
			}
		}
		if result.Reason == "" {
			result.Reason = "SD99 video generation failed"
		}
	default:
		return nil, fmt.Errorf("unknown SD99 task status; keep polling original task")
	}
	return result, nil
}

func resultURL(body []byte) string {
	for _, path := range []string{"url", "video_url", "result_url", "metadata.url", "metadata.video_url", "content.video_url", "result.url", "result.video_url", "output.video_url", "data.url", "data.video_url"} {
		value := gjson.GetBytes(body, path)
		if value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
			return strings.TrimSpace(value.String())
		}
	}
	for _, path := range []string{"data", "output.assets"} {
		for _, item := range gjson.GetBytes(body, path).Array() {
			kind := strings.ToLower(item.Get("type").String())
			role := strings.ToLower(item.Get("role").String())
			if role != "" && role != "output" && role != "output_video" {
				continue
			}
			if kind != "video" && kind != "output_video" && kind != "video_url" {
				continue
			}
			if value := item.Get("url"); value.Type == gjson.String && value.String() != "" {
				return value.String()
			}
		}
	}
	for _, path := range []string{"download_url", "content_url"} {
		value := gjson.GetBytes(body, path)
		if value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
			return strings.TrimSpace(value.String())
		}
	}
	return ""
}

func resolveMediaURL(baseURL, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid SD99 result URL")
	}
	if !u.IsAbs() {
		base, err := url.Parse(baseURL)
		if err != nil || base.Host == "" {
			return "", fmt.Errorf("invalid SD99 base URL")
		}
		base.Path, base.RawPath, base.RawQuery, base.Fragment = "/", "", "", ""
		u = base.ResolveReference(u)
	}
	if u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return "", fmt.Errorf("invalid SD99 result URL")
	}
	return u.String(), nil
}

// DownloadURL selects an output URL or the authenticated content fallback.
// Credentials belong only on this task's content endpoint, never media links.
func DownloadURL(baseURL, taskID string, body []byte) (string, bool, error) {
	if !taskIDPattern.MatchString(taskID) {
		return "", false, fmt.Errorf("invalid SD99 task ID")
	}
	contentURL, err := APIURL(baseURL, "/videos/"+taskID+"/content")
	if err != nil {
		return "", false, err
	}
	if raw := resultURL(body); raw != "" {
		resolved, err := resolveMediaURL(baseURL, raw)
		if err != nil {
			return "", false, err
		}
		candidate, _ := url.Parse(resolved)
		content, _ := url.Parse(contentURL)
		// Some responses advertise the provider's internal localhost URL.
		// Only recover this task's content endpoint, using the configured origin.
		host := candidate.Hostname()
		ip := net.ParseIP(host)
		if candidate.Path == content.Path && (strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())) {
			return contentURL, true, nil
		}
		auth := candidate.Scheme == content.Scheme && strings.EqualFold(candidate.Host, content.Host) && candidate.Path == content.Path
		return resolved, auth, nil
	}
	return contentURL, true, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	video := dto.NewOpenAIVideo()
	video.ID, video.TaskID, video.Model = task.TaskID, task.TaskID, task.Properties.OriginModelName
	video.Status = task.Status.ToVideoStatus()
	video.CreatedAt, video.CompletedAt = task.CreatedAt, task.FinishTime
	video.SetProgressStr(task.Progress)
	if task.Status == model.TaskStatusSuccess {
		video.SetMetadata("url", taskcommon.BuildProxyURL(task.TaskID))
	}
	if task.Status == model.TaskStatusFailure {
		video.Error = &dto.OpenAIVideoError{Code: "video_generation_failed", Message: task.FailReason}
	}
	return common.Marshal(video)
}

func (*TaskAdaptor) GetModelList() []string { return []string{Model} }
func (*TaskAdaptor) GetChannelName() string { return "sd99" }
