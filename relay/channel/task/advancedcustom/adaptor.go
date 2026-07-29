package advancedcustom

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const ChannelName = "advanced_custom_video"

type TaskAdaptor struct {
	taskcommon.BaseBilling
	config  *dto.AdvancedCustomVideoTaskConfig
	baseURL string
	apiKey  string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.config = nil
	a.baseURL = ""
	a.apiKey = ""
	if info == nil {
		return
	}
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
	if info.ChannelOtherSettings.AdvancedCustom != nil {
		a.config = info.ChannelOtherSettings.AdvancedCustom.VideoTask
	}
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if a.config == nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("advanced_custom.video_task is required"), "invalid_request", http.StatusBadRequest)
	}
	if err := a.config.Validate(); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if taskErr := relaycommon.ValidateMultipartDirect(c, info); taskErr != nil {
		return taskErr
	}
	if strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "multipart/") {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_multipart_form", http.StatusBadRequest)
		}
		if len(form.File) > 0 {
			return service.TaskErrorWrapperLocal(fmt.Errorf("advanced custom video tasks require image and video URLs; file uploads are not supported"), "unsupported_file_upload", http.StatusBadRequest)
		}
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestMethod(_ *relaycommon.RelayInfo) string {
	if a.config == nil {
		return http.MethodPost
	}
	return endpointMethod(a.config.Submit.Method, http.MethodPost)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if a.config == nil {
		return "", fmt.Errorf("advanced_custom.video_task is required")
	}
	originTaskID := ""
	if info.TaskRelayInfo != nil {
		originTaskID = info.OriginTaskID
	}
	values := map[string]any{
		"api_key":        a.apiKey,
		"model":          info.UpstreamModelName,
		"origin_task_id": originTaskID,
	}
	return buildEndpointURL(a.baseURL, a.config.Submit, values)
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	if a.config == nil {
		return fmt.Errorf("advanced_custom.video_task is required")
	}
	channel.SetupApiRequestHeader(info, c, &req.Header)
	req.Header.Set("Content-Type", "application/json")
	originTaskID := ""
	if info.TaskRelayInfo != nil {
		originTaskID = info.OriginTaskID
	}
	values := map[string]any{
		"api_key":        a.apiKey,
		"model":          info.UpstreamModelName,
		"origin_task_id": originTaskID,
	}
	applyEndpointHeaders(req.Header, a.config.Submit, values, a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	if a.config == nil {
		return nil, fmt.Errorf("advanced_custom.video_task is required")
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	originTaskID := ""
	if info.TaskRelayInfo != nil {
		originTaskID = info.OriginTaskID
	}
	values := buildSubmitTemplateValues(req, info.UpstreamModelName, originTaskID, a.apiKey)
	body, present, err := renderTemplateValue(a.config.Submit.Body, values)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, fmt.Errorf("advanced_custom.video_task.submit.body rendered empty")
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal advanced custom video request: %w", err)
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

	taskID := mappedString(responseBody, a.config.Response.TaskIDPath)
	if taskID == "" {
		return "", responseBody, service.TaskErrorWrapper(fmt.Errorf("task id is empty at response path %s", a.config.Response.TaskIDPath), "invalid_response", http.StatusBadGateway)
	}

	video := dto.NewOpenAIVideo()
	video.ID = info.PublicTaskID
	video.TaskID = info.PublicTaskID
	video.Model = info.OriginModelName
	video.CreatedAt = time.Now().Unix()
	c.JSON(http.StatusOK, video)
	return taskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	if snapshot, ok := body["advanced_custom_video_task"].(*dto.AdvancedCustomVideoTaskConfig); ok && snapshot != nil {
		a.config = snapshot
	}
	if a.config == nil {
		return nil, fmt.Errorf("advanced_custom.video_task is required")
	}
	taskID, _ := body["task_id"].(string)
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	values := map[string]any{
		"api_key": key,
		"task_id": taskID,
		"action":  body["action"],
		"model":   body["model"],
	}
	requestURL, err := buildEndpointURL(baseURL, a.config.Query, values)
	if err != nil {
		return nil, err
	}

	var requestBody io.Reader
	if a.config.Query.Body != nil {
		rendered, present, renderErr := renderTemplateValue(a.config.Query.Body, values)
		if renderErr != nil {
			return nil, renderErr
		}
		if present {
			data, marshalErr := common.Marshal(rendered)
			if marshalErr != nil {
				return nil, marshalErr
			}
			requestBody = bytes.NewReader(data)
		}
	}

	req, err := http.NewRequest(endpointMethod(a.config.Query.Method, http.MethodGet), requestURL, requestBody)
	if err != nil {
		return nil, err
	}
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	applyEndpointHeaders(req.Header, a.config.Query, values, key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	if a.config == nil {
		return nil, fmt.Errorf("advanced_custom.video_task is required")
	}
	statusValue := mappedString(respBody, a.config.Response.StatusPath)
	if statusValue == "" {
		return nil, fmt.Errorf("task status is empty at response path %s", a.config.Response.StatusPath)
	}
	status, ok := mapTaskStatus(statusValue, a.config.Response.StatusMap)
	if !ok {
		return nil, fmt.Errorf("unmapped advanced custom video task status: %s", statusValue)
	}

	result := &relaycommon.TaskInfo{
		TaskID: mappedString(respBody, a.config.Response.TaskIDPath),
		Status: string(status),
		Url:    mappedString(respBody, a.config.Response.ResultURLPath),
		Reason: mappedString(respBody, a.config.Response.ErrorPath),
	}
	if progress := mappedString(respBody, a.config.Response.ProgressPath); progress != "" {
		result.Progress = normalizeProgress(progress)
	}
	if status == model.TaskStatusSuccess && result.Progress == "" {
		result.Progress = taskcommon.ProgressComplete
	}
	if status == model.TaskStatusFailure {
		result.Progress = taskcommon.ProgressComplete
		if result.Reason == "" {
			result.Reason = "task failed"
		}
	}
	return result, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	video := dto.NewOpenAIVideo()
	video.ID = task.TaskID
	video.TaskID = task.TaskID
	video.Model = task.Properties.OriginModelName
	video.Status = task.Status.ToVideoStatus()
	video.SetProgressStr(task.Progress)
	video.CreatedAt = task.CreatedAt
	video.CompletedAt = task.FinishTime
	if resultURL := task.GetResultURL(); resultURL != "" {
		video.SetMetadata("url", resultURL)
	}
	if task.Status == model.TaskStatusFailure {
		video.Error = &dto.OpenAIVideoError{Message: task.FailReason, Code: "video_generation_failed"}
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string {
	return nil
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func endpointMethod(method string, fallback string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return fallback
	}
	return method
}

func buildEndpointURL(baseURL string, endpoint dto.AdvancedCustomVideoTaskEndpoint, values map[string]any) (string, error) {
	renderedPath, _, err := renderTemplateString(endpoint.Path, values)
	if err != nil {
		return "", err
	}
	parsed, err := resolveEndpointURL(baseURL, renderedPath)
	if err != nil {
		return "", err
	}
	if endpoint.Auth != nil && strings.TrimSpace(endpoint.Auth.Type) == dto.AdvancedCustomAuthTypeQuery {
		value, _, renderErr := renderTemplateString(endpoint.Auth.Value, values)
		if renderErr != nil {
			return "", renderErr
		}
		query := parsed.Query()
		query.Set(strings.TrimSpace(endpoint.Auth.Name), value)
		parsed.RawQuery = query.Encode()
	}
	return parsed.String(), nil
}

func resolveEndpointURL(baseURL string, endpointPath string) (*url.URL, error) {
	endpointPath = strings.TrimSpace(endpointPath)
	if strings.HasPrefix(endpointPath, "/") {
		if strings.HasPrefix(endpointPath, "//") {
			return nil, fmt.Errorf("advanced custom video path must be a full URL or a path starting with /")
		}
		base, err := url.Parse(strings.TrimSpace(baseURL))
		if err != nil || base.Scheme == "" || base.Host == "" {
			return nil, fmt.Errorf("channel base URL must be a full URL when advanced custom video path is relative")
		}
		relative, parseErr := url.Parse(endpointPath)
		if parseErr != nil {
			return nil, parseErr
		}
		base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(relative.Path, "/")
		base.RawPath = ""
		base.RawQuery = relative.RawQuery
		base.Fragment = relative.Fragment
		return base, nil
	}
	parsed, err := url.Parse(endpointPath)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("advanced custom video path must be a full URL or a path starting with /")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("advanced custom video path must use http or https")
	}
	return parsed, nil
}

func applyEndpointHeaders(headers http.Header, endpoint dto.AdvancedCustomVideoTaskEndpoint, values map[string]any, apiKey string) {
	for name, template := range endpoint.Headers {
		value, _, err := renderTemplateString(template, values)
		if err == nil {
			headers.Set(strings.TrimSpace(name), value)
		}
	}
	if endpoint.Auth == nil {
		headers.Set("Authorization", "Bearer "+apiKey)
		return
	}
	if strings.TrimSpace(endpoint.Auth.Type) != dto.AdvancedCustomAuthTypeHeader {
		return
	}
	value, _, err := renderTemplateString(endpoint.Auth.Value, values)
	if err == nil {
		headers.Set(strings.TrimSpace(endpoint.Auth.Name), value)
	}
}

func buildSubmitTemplateValues(req relaycommon.TaskSubmitReq, modelName string, originTaskID string, apiKey string) map[string]any {
	request := make(map[string]any, len(req.Metadata)+16)
	for key, value := range req.Metadata {
		request[key] = value
	}
	request["model"] = modelName
	request["prompt"] = req.Prompt
	if req.Mode != "" {
		request["mode"] = req.Mode
	}
	if req.Image != "" {
		request["image"] = req.Image
		request["input_reference"] = req.Image
	}
	if len(req.Images) > 0 {
		request["images"] = req.Images
	}
	if req.InputVideo != "" {
		request["input_video"] = req.InputVideo
	}
	if len(req.InputVideos) > 0 {
		request["input_videos"] = req.InputVideos
	}
	if req.Size != "" {
		request["size"] = req.Size
	}
	if req.Width != nil {
		request["width"] = *req.Width
	}
	if req.Height != nil {
		request["height"] = *req.Height
	}
	if seconds := req.OutputSeconds(); seconds > 0 {
		request["seconds"] = seconds
		request["duration"] = seconds
	}
	if req.FPS != nil {
		request["fps"] = *req.FPS
	}
	if req.Seed != nil {
		request["seed"] = *req.Seed
	}
	if req.NegativePrompt != "" {
		request["negative_prompt"] = req.NegativePrompt
	}
	if req.GenerateAudio != nil {
		request["generate_audio"] = *req.GenerateAudio
	}
	if len(req.Metadata) > 0 {
		request["metadata"] = req.Metadata
	}
	values := make(map[string]any, len(request)+2)
	for key, value := range request {
		values[key] = value
	}
	values["request"] = request
	values["api_key"] = apiKey
	if originTaskID != "" {
		values["origin_task_id"] = originTaskID
	}
	return values
}

func renderTemplateValue(template any, values map[string]any) (any, bool, error) {
	switch value := template.(type) {
	case string:
		if placeholder, ok := exactPlaceholder(value); ok {
			resolved, exists := lookupTemplateValue(values, placeholder)
			if !exists || isEmptyTemplateValue(resolved) {
				return nil, false, nil
			}
			return resolved, true, nil
		}
		rendered, _, err := renderTemplateString(value, values)
		return rendered, true, err
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, nested := range value {
			rendered, present, err := renderTemplateValue(nested, values)
			if err != nil {
				return nil, false, err
			}
			if present {
				result[key] = rendered
			}
		}
		return result, true, nil
	case []any:
		result := make([]any, 0, len(value))
		for _, nested := range value {
			rendered, present, err := renderTemplateValue(nested, values)
			if err != nil {
				return nil, false, err
			}
			if present {
				result = append(result, rendered)
			}
		}
		return result, true, nil
	default:
		return value, true, nil
	}
}

func renderTemplateString(template string, values map[string]any) (string, bool, error) {
	var builder strings.Builder
	builder.Grow(len(template))
	found := false
	remaining := template
	for {
		start := strings.Index(remaining, "{")
		if start < 0 {
			builder.WriteString(remaining)
			break
		}
		builder.WriteString(remaining[:start])
		endOffset := strings.Index(remaining[start+1:], "}")
		if endOffset < 0 {
			builder.WriteString(remaining[start:])
			break
		}
		end := start + 1 + endOffset
		placeholder := strings.TrimSpace(remaining[start+1 : end])
		if placeholder == "" {
			return "", false, fmt.Errorf("empty template placeholder")
		}
		value, exists := lookupTemplateValue(values, placeholder)
		replacement := ""
		if exists {
			replacement = templateStringValue(value)
			found = true
		}
		builder.WriteString(replacement)
		remaining = remaining[end+1:]
	}
	return builder.String(), found, nil
}

func exactPlaceholder(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 3 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return "", false
	}
	placeholder := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if placeholder == "" || strings.ContainsAny(placeholder, "{}") {
		return "", false
	}
	return placeholder, true
}

func lookupTemplateValue(values map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = values
	for _, part := range parts {
		switch typed := current.(type) {
		case map[string]any:
			current, _ = typed[part]
			if current == nil {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func templateStringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	data, err := common.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func isEmptyTemplateValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []string:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func mappedString(body []byte, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	result := gjson.GetBytes(body, path)
	if !result.Exists() || result.Type == gjson.Null {
		return ""
	}
	return strings.TrimSpace(result.String())
}

func mapTaskStatus(upstream string, configured map[string]string) (model.TaskStatus, bool) {
	upstream = strings.TrimSpace(upstream)
	for value, target := range configured {
		if strings.EqualFold(strings.TrimSpace(value), upstream) {
			return canonicalTaskStatus(target)
		}
	}
	switch strings.ToLower(upstream) {
	case "submitted", "created":
		return model.TaskStatusSubmitted, true
	case "queued", "pending", "waiting":
		return model.TaskStatusQueued, true
	case "processing", "running", "in_progress", "in-progress":
		return model.TaskStatusInProgress, true
	case "success", "succeeded", "completed", "done":
		return model.TaskStatusSuccess, true
	case "failure", "failed", "error", "cancelled", "canceled":
		return model.TaskStatusFailure, true
	default:
		return "", false
	}
}

func canonicalTaskStatus(value string) (model.TaskStatus, bool) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "SUBMITTED":
		return model.TaskStatusSubmitted, true
	case "QUEUED":
		return model.TaskStatusQueued, true
	case "IN_PROGRESS":
		return model.TaskStatusInProgress, true
	case "SUCCESS":
		return model.TaskStatusSuccess, true
	case "FAILURE":
		return model.TaskStatusFailure, true
	default:
		return "", false
	}
}

func normalizeProgress(progress string) string {
	progress = strings.TrimSpace(progress)
	if progress == "" || strings.HasSuffix(progress, "%") {
		return progress
	}
	if _, err := strconv.ParseFloat(progress, 64); err == nil {
		return progress + "%"
	}
	return progress
}
