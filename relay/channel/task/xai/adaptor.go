package xai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	pkgerrors "github.com/pkg/errors"
)

const videoRequestContextKey = "xai_video_request"

var videoModelList = []string{
	"grok-imagine-video",
	"grok-imagine-video-1.5",
}

type videoMedia struct {
	URL    string `json:"url,omitempty"`
	FileID string `json:"file_id,omitempty"`
}

type videoRequest struct {
	Model           string          `json:"model"`
	Prompt          string          `json:"prompt,omitempty"`
	Duration        *int            `json:"duration,omitempty"`
	AspectRatio     string          `json:"aspect_ratio,omitempty"`
	Resolution      string          `json:"resolution,omitempty"`
	Image           *videoMedia     `json:"image,omitempty"`
	ReferenceImages []videoMedia    `json:"reference_images,omitempty"`
	Video           *videoMedia     `json:"video,omitempty"`
	Output          json.RawMessage `json:"output,omitempty"`
	StorageOptions  json.RawMessage `json:"storage_options,omitempty"`
	User            json.RawMessage `json:"user,omitempty"`
	Operation       string          `json:"-"`
}

type incomingVideoRequest struct {
	Model             string                     `json:"model"`
	Prompt            string                     `json:"prompt"`
	Mode              string                     `json:"mode,omitempty"`
	Operation         string                     `json:"operation,omitempty"`
	Duration          *dto.IntValue              `json:"duration,omitempty"`
	Seconds           *dto.IntValue              `json:"seconds,omitempty"`
	InputVideoSeconds *float64                   `json:"input_video_seconds,omitempty"`
	AspectRatio       string                     `json:"aspect_ratio,omitempty"`
	Resolution        string                     `json:"resolution,omitempty"`
	Size              string                     `json:"size,omitempty"`
	Image             json.RawMessage            `json:"image,omitempty"`
	Images            json.RawMessage            `json:"images,omitempty"`
	ReferenceImages   json.RawMessage            `json:"reference_images,omitempty"`
	Video             json.RawMessage            `json:"video,omitempty"`
	InputVideo        json.RawMessage            `json:"input_video,omitempty"`
	InputVideos       json.RawMessage            `json:"input_videos,omitempty"`
	Output            json.RawMessage            `json:"output,omitempty"`
	StorageOptions    json.RawMessage            `json:"storage_options,omitempty"`
	User              json.RawMessage            `json:"user,omitempty"`
	Metadata          map[string]json.RawMessage `json:"metadata,omitempty"`
}

type submitResponse struct {
	RequestID string `json:"request_id"`
}

type taskResponse struct {
	Status   string `json:"status"`
	Model    string `json:"model,omitempty"`
	Progress *int   `json:"progress,omitempty"`
	Error    *struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
	Video *struct {
		URL               string `json:"url,omitempty"`
		Duration          int    `json:"duration,omitempty"`
		RespectModeration bool   `json:"respect_moderation,omitempty"`
	} `json:"video,omitempty"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.apiKey = info.ApiKey
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if c == nil || c.Request == nil || !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		return service.TaskErrorWrapperLocal(errors.New("xAI video requests must use application/json"), "invalid_request", http.StatusBadRequest)
	}

	var incoming incomingVideoRequest
	if err := common.UnmarshalBodyReusable(c, &incoming); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	request, taskRequest, err := normalizeVideoRequest(incoming)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if request.Model == "" {
		request.Model = info.OriginModelName
	}
	if request.Model == "" {
		return service.TaskErrorWrapperLocal(errors.New("model is required"), "invalid_request", http.StatusBadRequest)
	}
	if request.Model == "grok-imagine-video-1.5" && len(request.ReferenceImages) > 0 {
		return service.TaskErrorWrapperLocal(errors.New("grok-imagine-video-1.5 does not support reference_images"), "invalid_request", http.StatusBadRequest)
	}

	switch request.Operation {
	case "edit":
		info.Action = constant.TaskActionVideoEdit
	case "extension":
		info.Action = constant.TaskActionVideoExtend
	default:
		if request.Image != nil || len(request.ReferenceImages) > 0 {
			info.Action = constant.TaskActionGenerate
		} else {
			info.Action = constant.TaskActionTextGenerate
		}
	}
	taskRequest.Model = request.Model
	c.Set("task_request", taskRequest)
	c.Set(videoRequestContextKey, request)
	return nil
}

func normalizeVideoRequest(incoming incomingVideoRequest) (*videoRequest, relaycommon.TaskSubmitReq, error) {
	if incoming.InputVideoSeconds != nil {
		seconds := *incoming.InputVideoSeconds
		if seconds <= 0 || seconds > relaycommon.MaxTaskDurationSeconds || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			return nil, relaycommon.TaskSubmitReq{}, fmt.Errorf("input_video_seconds must be between 1 and %d", relaycommon.MaxTaskDurationSeconds)
		}
	}
	request := &videoRequest{
		Model:          strings.TrimSpace(incoming.Model),
		Prompt:         strings.TrimSpace(incoming.Prompt),
		AspectRatio:    strings.ToLower(strings.TrimSpace(incoming.AspectRatio)),
		Resolution:     strings.ToLower(strings.TrimSpace(incoming.Resolution)),
		Output:         incoming.Output,
		StorageOptions: incoming.StorageOptions,
		User:           incoming.User,
	}

	mode := strings.ToLower(strings.TrimSpace(incoming.Operation))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(incoming.Mode))
	}
	if mode == "" {
		mode = metadataString(incoming.Metadata, "operation")
	}
	if mode == "" {
		mode = metadataString(incoming.Metadata, "mode")
	}

	if request.AspectRatio == "" {
		request.AspectRatio = strings.ToLower(metadataString(incoming.Metadata, "aspect_ratio"))
	}
	if request.Resolution == "" {
		request.Resolution = strings.ToLower(metadataString(incoming.Metadata, "resolution"))
	}
	applyVideoSizeAliases(request, incoming.Size)
	if request.AspectRatio != "" && !containsString([]string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"}, request.AspectRatio) {
		return nil, relaycommon.TaskSubmitReq{}, fmt.Errorf("unsupported aspect_ratio %q", request.AspectRatio)
	}
	if request.Resolution != "" && !containsString([]string{"480p", "720p", "1080p"}, request.Resolution) {
		return nil, relaycommon.TaskSubmitReq{}, fmt.Errorf("unsupported resolution %q", request.Resolution)
	}

	if len(request.Output) == 0 {
		request.Output = incoming.Metadata["output"]
	}
	if len(request.StorageOptions) == 0 {
		request.StorageOptions = incoming.Metadata["storage_options"]
	}
	if len(request.User) == 0 {
		request.User = incoming.Metadata["user"]
	}

	imageInputs, err := parseVideoMediaList(incoming.Image)
	if err != nil {
		return nil, relaycommon.TaskSubmitReq{}, fmt.Errorf("invalid image: %w", err)
	}
	referenceInputs, err := parseVideoMediaList(incoming.ReferenceImages)
	if err != nil {
		return nil, relaycommon.TaskSubmitReq{}, fmt.Errorf("invalid reference_images: %w", err)
	}
	images, err := parseVideoMediaList(incoming.Images)
	if err != nil {
		return nil, relaycommon.TaskSubmitReq{}, fmt.Errorf("invalid images: %w", err)
	}
	if len(imageInputs) > 1 {
		return nil, relaycommon.TaskSubmitReq{}, errors.New("image accepts only one item")
	}
	if len(imageInputs) == 1 {
		request.Image = &imageInputs[0]
	}
	if len(images) > 0 {
		if request.Image != nil || len(referenceInputs) > 0 {
			return nil, relaycommon.TaskSubmitReq{}, errors.New("image, images and reference_images cannot be combined")
		}
		if len(images) == 1 {
			request.Image = &images[0]
		} else {
			referenceInputs = images
		}
	}
	request.ReferenceImages = referenceInputs
	if request.Image != nil {
		if err := validateMediaLocation(*request.Image, "image", true); err != nil {
			return nil, relaycommon.TaskSubmitReq{}, err
		}
	}
	for _, image := range request.ReferenceImages {
		if err := validateMediaLocation(image, "reference image", true); err != nil {
			return nil, relaycommon.TaskSubmitReq{}, err
		}
	}
	if request.Image != nil && len(request.ReferenceImages) > 0 {
		return nil, relaycommon.TaskSubmitReq{}, errors.New("image and reference_images are mutually exclusive")
	}
	if len(request.ReferenceImages) > 7 {
		return nil, relaycommon.TaskSubmitReq{}, errors.New("reference_images supports at most 7 images")
	}

	videoInputs, err := parseVideoMediaList(incoming.Video)
	if err != nil {
		return nil, relaycommon.TaskSubmitReq{}, fmt.Errorf("invalid video: %w", err)
	}
	if len(videoInputs) == 0 {
		videoInputs, err = parseVideoMediaList(incoming.InputVideo)
		if err != nil {
			return nil, relaycommon.TaskSubmitReq{}, fmt.Errorf("invalid input_video: %w", err)
		}
	}
	if len(videoInputs) == 0 {
		videoInputs, err = parseVideoMediaList(incoming.InputVideos)
		if err != nil {
			return nil, relaycommon.TaskSubmitReq{}, fmt.Errorf("invalid input_videos: %w", err)
		}
	}
	if len(videoInputs) > 1 {
		return nil, relaycommon.TaskSubmitReq{}, errors.New("xAI supports only one input video")
	}
	if len(videoInputs) == 1 {
		request.Video = &videoInputs[0]
		if err := validateMediaLocation(*request.Video, "input video", false); err != nil {
			return nil, relaycommon.TaskSubmitReq{}, err
		}
		if request.Image != nil || len(request.ReferenceImages) > 0 {
			return nil, relaycommon.TaskSubmitReq{}, errors.New("video input cannot be combined with image inputs")
		}
	}

	duration := incoming.Duration
	if duration == nil {
		duration = incoming.Seconds
	}
	if request.Video != nil {
		if mode == "extension" || mode == "extend" || mode == "video_extension" {
			request.Operation = "extension"
			value := 6
			if duration != nil {
				value = int(*duration)
			}
			if value < 2 || value > 10 {
				return nil, relaycommon.TaskSubmitReq{}, errors.New("extension duration must be between 2 and 10 seconds")
			}
			request.Duration = &value
		} else {
			request.Operation = "edit"
		}
		if request.Prompt == "" {
			return nil, relaycommon.TaskSubmitReq{}, errors.New("prompt is required for video editing and extension")
		}
	} else {
		request.Operation = "generation"
		value := 8
		if duration != nil {
			value = int(*duration)
		}
		if value < 1 || value > 15 {
			return nil, relaycommon.TaskSubmitReq{}, errors.New("duration must be between 1 and 15 seconds")
		}
		request.Duration = &value
		if request.Prompt == "" && request.Image == nil {
			return nil, relaycommon.TaskSubmitReq{}, errors.New("prompt is required for text-to-video and reference-to-video")
		}
	}

	billingSeconds := 8
	if request.Duration != nil {
		billingSeconds = *request.Duration
	} else if incoming.InputVideoSeconds != nil && *incoming.InputVideoSeconds > 0 {
		billingSeconds = int(math.Ceil(*incoming.InputVideoSeconds))
	}
	taskRequest := relaycommon.TaskSubmitReq{
		Prompt:            request.Prompt,
		Model:             request.Model,
		Mode:              request.Operation,
		Duration:          billingSeconds,
		Seconds:           strconv.Itoa(billingSeconds),
		Size:              request.Resolution,
		InputVideoSeconds: incoming.InputVideoSeconds,
		Metadata:          make(map[string]interface{}),
	}
	if request.AspectRatio != "" {
		taskRequest.Metadata["aspect_ratio"] = request.AspectRatio
	}
	if request.Resolution != "" {
		taskRequest.Metadata["resolution"] = request.Resolution
	}
	if request.Image != nil {
		taskRequest.Metadata["image"] = request.Image
		if request.Image.URL != "" {
			taskRequest.Image = request.Image.URL
			taskRequest.Images = []string{request.Image.URL}
		}
	}
	if len(request.ReferenceImages) > 0 {
		taskRequest.Metadata["reference_images"] = request.ReferenceImages
		for _, image := range request.ReferenceImages {
			if image.URL != "" {
				taskRequest.Images = append(taskRequest.Images, image.URL)
			}
		}
	}
	if request.Video != nil {
		taskRequest.Metadata["video"] = request.Video
		if request.Video.URL != "" {
			taskRequest.InputVideo = request.Video.URL
			taskRequest.InputVideos = []string{request.Video.URL}
		}
	}
	taskRequest.Normalize()
	return request, taskRequest, nil
}

func metadataString(metadata map[string]json.RawMessage, key string) string {
	raw := metadata[key]
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func parseVideoMediaList(raw json.RawMessage) ([]videoMedia, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var items []json.RawMessage
	if err := common.Unmarshal(raw, &items); err != nil {
		items = []json.RawMessage{raw}
	}
	media := make([]videoMedia, 0, len(items))
	for _, item := range items {
		var url string
		if err := common.Unmarshal(item, &url); err == nil {
			url = strings.TrimSpace(url)
			if url == "" {
				return nil, errors.New("media URL is empty")
			}
			media = append(media, videoMedia{URL: url})
			continue
		}
		var value struct {
			URL      string          `json:"url,omitempty"`
			FileID   string          `json:"file_id,omitempty"`
			ImageURL json.RawMessage `json:"image_url,omitempty"`
		}
		if err := common.Unmarshal(item, &value); err != nil {
			return nil, err
		}
		value.URL = strings.TrimSpace(value.URL)
		value.FileID = strings.TrimSpace(value.FileID)
		if value.URL == "" && len(value.ImageURL) > 0 {
			if err := common.Unmarshal(value.ImageURL, &value.URL); err != nil {
				var nested struct {
					URL string `json:"url"`
				}
				if nestedErr := common.Unmarshal(value.ImageURL, &nested); nestedErr == nil {
					value.URL = strings.TrimSpace(nested.URL)
				}
			}
		}
		if (value.URL == "") == (value.FileID == "") {
			return nil, errors.New("exactly one of url or file_id is required")
		}
		media = append(media, videoMedia{URL: value.URL, FileID: value.FileID})
	}
	return media, nil
}

func validateMediaLocation(media videoMedia, label string, allowImageDataURL bool) error {
	if media.FileID != "" {
		return nil
	}
	if allowImageDataURL && strings.HasPrefix(strings.ToLower(media.URL), "data:image/") {
		return nil
	}
	parsed, err := url.Parse(media.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must contain an HTTP or HTTPS URL", label)
	}
	return nil
}

func applyVideoSizeAliases(request *videoRequest, size string) {
	size = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(size, "×", "x")))
	if size == "" {
		return
	}
	if request.Resolution == "" && containsString([]string{"480p", "720p", "1080p"}, size) {
		request.Resolution = size
		return
	}
	if request.AspectRatio == "" && containsString([]string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"}, size) {
		request.AspectRatio = size
		return
	}
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return
	}
	width, widthErr := strconv.Atoi(parts[0])
	height, heightErr := strconv.Atoi(parts[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return
	}
	if request.Resolution == "" {
		request.Resolution = fmt.Sprintf("%dp", min(width, height))
	}
	if request.AspectRatio == "" {
		divisor := greatestCommonDivisor(width, height)
		ratio := fmt.Sprintf("%d:%d", width/divisor, height/divisor)
		if containsString([]string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"}, ratio) {
			request.AspectRatio = ratio
		}
	}
}

func greatestCommonDivisor(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a <= 0 {
		return 1
	}
	return a
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil || req.OutputSeconds() <= 0 {
		return nil
	}
	return map[string]float64{"seconds": req.OutputSeconds()}
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	switch info.Action {
	case constant.TaskActionVideoEdit:
		return a.baseURL + "/v1/videos/edits", nil
	case constant.TaskActionVideoExtend:
		return a.baseURL + "/v1/videos/extensions", nil
	default:
		return a.baseURL + "/v1/videos/generations", nil
	}
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	value, ok := c.Get(videoRequestContextKey)
	if !ok {
		return nil, errors.New("xAI video request is missing")
	}
	request, ok := value.(*videoRequest)
	if !ok || request == nil {
		return nil, errors.New("invalid xAI video request")
	}
	request.Model = info.UpstreamModelName
	data, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var response submitResponse
	if err := common.Unmarshal(responseBody, &response); err != nil {
		return "", nil, service.TaskErrorWrapper(pkgerrors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(response.RequestID) == "" {
		return "", nil, service.TaskErrorWrapper(errors.New("request_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	video := dto.NewOpenAIVideo()
	video.ID = info.PublicTaskID
	video.TaskID = info.PublicTaskID
	video.Model = info.OriginModelName
	video.CreatedAt = time.Now().Unix()
	if req, getErr := relaycommon.GetTaskRequest(c); getErr == nil {
		video.Seconds = req.Seconds
		video.Size = req.Size
	}
	c.JSON(http.StatusOK, video)
	return response.RequestID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, errors.New("invalid task_id")
	}
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/videos/"+taskID, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(request)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var response taskResponse
	if err := common.Unmarshal(respBody, &response); err != nil {
		return nil, pkgerrors.Wrap(err, "unmarshal xAI video task result failed")
	}
	result := &relaycommon.TaskInfo{}
	switch strings.ToLower(strings.TrimSpace(response.Status)) {
	case "pending", "queued":
		result.Status = model.TaskStatusQueued
	case "processing", "in_progress", "running":
		result.Status = model.TaskStatusInProgress
	case "done", "completed", "succeeded":
		result.Status = model.TaskStatusSuccess
		if response.Video != nil {
			result.Url = response.Video.URL
		}
	case "failed", "expired", "cancelled", "canceled":
		result.Status = model.TaskStatusFailure
		if response.Error != nil {
			result.Reason = response.Error.Message
		}
		if result.Reason == "" {
			result.Reason = "xAI video task failed"
		}
	default:
		return nil, fmt.Errorf("unknown xAI video status %q", response.Status)
	}
	if response.Progress != nil {
		progress := *response.Progress
		if progress < 0 {
			progress = 0
		}
		if progress > 100 {
			progress = 100
		}
		result.Progress = strconv.Itoa(progress) + "%"
	}
	return result, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var response taskResponse
	if len(task.Data) > 0 {
		_ = common.Unmarshal(task.Data, &response)
	}
	video := dto.NewOpenAIVideo()
	video.ID = task.TaskID
	video.TaskID = task.TaskID
	video.Model = task.Properties.OriginModelName
	video.Status = task.Status.ToVideoStatus()
	video.SetProgressStr(task.Progress)
	video.CreatedAt = task.CreatedAt
	video.CompletedAt = task.UpdatedAt
	if response.Video != nil {
		video.Seconds = strconv.Itoa(response.Video.Duration)
		video.SetMetadata("duration", response.Video.Duration)
		video.SetMetadata("url", response.Video.URL)
		video.SetMetadata("respect_moderation", response.Video.RespectModeration)
	} else if task.GetResultURL() != "" {
		video.SetMetadata("url", task.GetResultURL())
	}
	if task.Status == model.TaskStatusFailure {
		video.Error = &dto.OpenAIVideoError{Message: task.FailReason}
		if response.Error != nil {
			video.Error.Code = response.Error.Code
			if video.Error.Message == "" {
				video.Error.Message = response.Error.Message
			}
		}
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string {
	return videoModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return "xai"
}

var _ channel.TaskAdaptor = (*TaskAdaptor)(nil)
var _ channel.OpenAIVideoConverter = (*TaskAdaptor)(nil)
