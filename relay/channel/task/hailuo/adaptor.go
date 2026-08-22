package hailuo

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

// https://platform.minimaxi.com/docs/api-reference/video-generation-intro
type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if taskErr = relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}
	if isH3Model(taskModel(c, info)) {
		return validateH3Request(c)
	}
	return relaycommon.ValidateNoTaskInputVideo(c, "Hailuo")
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if isH3Model(info.UpstreamModelName) {
		return a.h3Endpoint(H3TextToVideoEndpoint), nil
	}
	return fmt.Sprintf("%s%s", a.baseURL, TextToVideoEndpoint), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	if idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key")); idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("invalid request type in context")
	}

	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()
	if isH3Model(info.UpstreamModelName) {
		var h3Resp H3VideoResponse
		if err := common.Unmarshal(responseBody, &h3Resp); err != nil {
			taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
			return
		}
		if h3Resp.TaskID == "" {
			var errorResp H3ErrorResponse
			_ = common.Unmarshal(responseBody, &errorResp)
			message := strings.TrimSpace(errorResp.Error.Message)
			if message == "" {
				message = "minimax H3 response has empty task_id"
			}
			taskErr = service.TaskErrorWrapper(fmt.Errorf("%s", message), "minimax_h3_error", http.StatusBadGateway)
			return
		}
		ov := dto.NewOpenAIVideo()
		ov.ID = info.PublicTaskID
		ov.TaskID = info.PublicTaskID
		ov.CreatedAt = time.Now().Unix()
		ov.Model = info.OriginModelName
		c.JSON(http.StatusOK, ov)
		return h3Resp.TaskID, responseBody, nil
	}

	var hResp VideoResponse
	if err := common.Unmarshal(responseBody, &hResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if hResp.BaseResp.StatusCode != StatusSuccess {
		taskErr = service.TaskErrorWrapper(
			fmt.Errorf("hailuo api error: %s", hResp.BaseResp.StatusMsg),
			strconv.Itoa(hResp.BaseResp.StatusCode),
			http.StatusBadRequest,
		)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return hResp.TaskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	modelName, _ := body["model"].(string)
	uri := fmt.Sprintf("%s%s?task_id=%s", baseUrl, QueryTaskEndpoint, taskID)
	if isH3Model(modelName) {
		uri = fmt.Sprintf("%s/%s", h3EndpointForBase(baseUrl, H3QueryTaskEndpoint), taskID)
	}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (any, error) {
	if isH3Model(info.UpstreamModelName) {
		return a.convertToH3RequestPayload(req, info)
	}
	modelConfig := GetModelConfig(info.UpstreamModelName)
	duration := DefaultDuration
	if req.Duration > 0 {
		duration = req.Duration
	}
	resolution := modelConfig.DefaultResolution
	if req.Size != "" {
		resolution = a.parseResolutionFromSize(req.Size, modelConfig)
	}

	videoRequest := &VideoRequest{
		Model:      info.UpstreamModelName,
		Prompt:     req.Prompt,
		Duration:   &duration,
		Resolution: resolution,
	}
	if err := req.UnmarshalMetadata(&videoRequest); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata to video request failed")
	}
	videoRequest.Model = info.UpstreamModelName
	videoRequest.Prompt = req.Prompt
	if req.HasImage() {
		videoRequest.FirstFrameImage = req.Images[0]
		videoRequest.LastFrameImage = ""
		if len(req.Images) > 1 {
			videoRequest.LastFrameImage = req.Images[1]
		}
	}
	if req.Duration > 0 {
		videoRequest.Duration = &req.Duration
	}
	if req.Size != "" {
		videoRequest.Resolution = a.parseResolutionFromSize(req.Size, modelConfig)
	}

	return videoRequest, nil
}

func (a *TaskAdaptor) convertToH3RequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*H3VideoRequest, error) {
	duration := req.Duration
	resolution := h3ResolutionFromSize(req.Size)
	if resolution == "" {
		resolution = Resolution768P
	}
	payload := &H3VideoRequest{
		Model:      info.UpstreamModelName,
		Content:    []H3VideoContent{{Type: "text", Text: req.Prompt}},
		Resolution: resolution,
		Duration:   duration,
		Ratio:      "16:9",
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, payload); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata to H3 video request failed")
	}
	payload.Model = info.UpstreamModelName
	payload.Content = h3ContentFromRequest(req)
	payload.Resolution = resolution
	payload.Duration = req.Duration
	if req.Ratio != "" {
		payload.Ratio = req.Ratio
	}
	if ratio := ratioFromSize(req.Size); ratio != "" && req.Ratio == "" {
		payload.Ratio = ratio
	}
	return payload, nil
}

func taskModel(c *gin.Context, info *relaycommon.RelayInfo) string {
	if info != nil && info.UpstreamModelName != "" {
		return info.UpstreamModelName
	}
	if req, err := relaycommon.GetTaskRequest(c); err == nil {
		return req.Model
	}
	return ""
}

func validateH3Request(c *gin.Context) *dto.TaskError {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if req.Duration < 4 || req.Duration > 15 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("MiniMax-H3 duration must be between 4 and 15 seconds"), "invalid_duration", http.StatusBadRequest)
	}
	if req.Size != "" && h3ResolutionFromSize(req.Size) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("MiniMax-H3 resolution must be 768P, 1080P, or 2K"), "invalid_resolution", http.StatusBadRequest)
	}
	if req.Ratio != "" && !h3Ratios[req.Ratio] {
		return service.TaskErrorWrapperLocal(fmt.Errorf("MiniMax-H3 ratio is invalid"), "invalid_ratio", http.StatusBadRequest)
	}
	return nil
}

var h3Ratios = map[string]bool{
	"adaptive": true, "21:9": true, "16:9": true, "4:3": true,
	"1:1": true, "3:4": true, "9:16": true,
}

func h3ContentFromRequest(req *relaycommon.TaskSubmitReq) []H3VideoContent {
	content := []H3VideoContent{{Type: "text", Text: req.Prompt}}
	inputs := req.ImageInputs
	if len(inputs) == 0 {
		for _, image := range req.Images {
			inputs = append(inputs, relaycommon.TaskImageInput{URL: image})
		}
	}
	for _, input := range inputs {
		if strings.TrimSpace(input.URL) == "" {
			continue
		}
		role := strings.TrimSpace(input.Role)
		if role == "" {
			role = "first_frame"
		}
		content = append(content, H3VideoContent{
			Type:     "image_url",
			ImageURL: &H3MediaURL{URL: input.URL},
			Role:     role,
		})
	}
	for _, video := range req.InputVideoURLs() {
		content = append(content, H3VideoContent{
			Type:     "video_url",
			VideoURL: &H3MediaURL{URL: video},
			Role:     "reference_video",
		})
	}
	return content
}

func (a *TaskAdaptor) parseResolutionFromSize(size string, modelConfig ModelConfig) string {
	switch {
	case strings.Contains(size, "1080"):
		return Resolution1080P
	case strings.Contains(size, "768"):
		return Resolution768P
	case strings.Contains(size, "720"):
		return Resolution720P
	case strings.Contains(size, "512"):
		return Resolution512P
	default:
		return modelConfig.DefaultResolution
	}
}

func h3ResolutionFromSize(size string) string {
	normalized := strings.ToUpper(strings.TrimSpace(size))
	switch normalized {
	case "", Resolution768P:
		return Resolution768P
	case Resolution1080P, Resolution2K:
		return normalized
	case "720P", "1280X720", "720X1280":
		return Resolution768P
	case "1920X1080", "1080X1920":
		return Resolution1080P
	case "2048X1152", "1152X2048", "2560X1440", "1440X2560":
		return Resolution2K
	default:
		return ""
	}
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var h3Resp H3TaskResponse
	if err := common.Unmarshal(respBody, &h3Resp); err == nil && h3Resp.Task.ID != "" {
		result := &relaycommon.TaskInfo{Code: 0}
		switch strings.ToLower(h3Resp.Task.Status) {
		case "queued", "pending", "created":
			result.Status = model.TaskStatusQueued
			result.Progress = "20%"
		case "running", "processing":
			result.Status = model.TaskStatusInProgress
			result.Progress = "50%"
		case "succeeded", "success", "completed":
			result.Status = model.TaskStatusSuccess
			result.Progress = "100%"
			result.Url = h3Resp.Task.Content.URL
		case "failed", "failure", "canceled", "cancelled":
			result.Status = model.TaskStatusFailure
			result.Progress = "100%"
			if h3Resp.Task.Error != nil {
				result.Reason = h3Resp.Task.Error.Message
				if result.Reason == "" {
					result.Reason = h3Resp.Task.Error.Code
				}
			}
			if result.Reason == "" {
				result.Reason = h3Resp.Task.Content.Prompt
			}
			if result.Reason == "" {
				result.Reason = "task failed"
			}
		default:
			result.Status = model.TaskStatusInProgress
			result.Progress = "30%"
		}
		return result, nil
	}
	var h3Error H3ErrorResponse
	if err := common.Unmarshal(respBody, &h3Error); err == nil && strings.TrimSpace(h3Error.Error.Message) != "" {
		return &relaycommon.TaskInfo{
			Status:   model.TaskStatusFailure,
			Progress: "100%",
			Reason:   h3Error.Error.Message,
		}, nil
	}
	resTask := QueryTaskResponse{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{}

	if resTask.BaseResp.StatusCode == StatusSuccess {
		taskResult.Code = 0
	} else {
		taskResult.Code = resTask.BaseResp.StatusCode
		taskResult.Reason = resTask.BaseResp.StatusMsg
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
	}

	switch resTask.Status {
	case TaskStatusPreparing, TaskStatusQueueing, TaskStatusProcessing:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
		if resTask.Status == TaskStatusProcessing {
			taskResult.Progress = "50%"
		}
	case TaskStatusSuccess:
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = a.buildVideoURL(resTask.TaskID, resTask.FileID)
	case TaskStatusFailed:
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		if taskResult.Reason == "" {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var h3Resp H3TaskResponse
	if err := common.Unmarshal(originTask.Data, &h3Resp); err == nil && h3Resp.Task.ID != "" {
		openAIVideo := originTask.ToOpenAIVideo()
		if h3Resp.Task.Content.URL != "" {
			openAIVideo.SetMetadata("url", h3Resp.Task.Content.URL)
		}
		if strings.EqualFold(h3Resp.Task.Status, "failed") || strings.EqualFold(h3Resp.Task.Status, "cancelled") || strings.EqualFold(h3Resp.Task.Status, "canceled") {
			message := "task failed"
			code := "minimax_h3_task_failed"
			if h3Resp.Task.Error != nil {
				if h3Resp.Task.Error.Message != "" {
					message = h3Resp.Task.Error.Message
				}
				if h3Resp.Task.Error.Code != "" {
					code = h3Resp.Task.Error.Code
				}
			}
			openAIVideo.Error = &dto.OpenAIVideoError{Message: message, Code: code}
		}
		return common.Marshal(openAIVideo)
	}
	var hailuoResp QueryTaskResponse
	if err := common.Unmarshal(originTask.Data, &hailuoResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal hailuo task data failed")
	}

	openAIVideo := originTask.ToOpenAIVideo()
	if hailuoResp.BaseResp.StatusCode != StatusSuccess {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: hailuoResp.BaseResp.StatusMsg,
			Code:    strconv.Itoa(hailuoResp.BaseResp.StatusCode),
		}
	}

	jsonData, err := common.Marshal(openAIVideo)
	if err != nil {
		return nil, errors.Wrap(err, "marshal openai video failed")
	}

	return jsonData, nil
}

func isH3Model(modelName string) bool {
	return strings.EqualFold(strings.TrimSpace(modelName), "MiniMax-H3")
}

func (a *TaskAdaptor) h3Endpoint(path string) string {
	return h3EndpointForBase(a.baseURL, path)
}

func h3EndpointForBase(baseURL, path string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/minimax/v2") {
		return baseURL + strings.TrimPrefix(path, "/minimax/v2")
	}
	if strings.HasSuffix(baseURL, "/minimax") {
		return baseURL + strings.TrimPrefix(path, "/minimax")
	}
	return baseURL + path
}

func ratioFromSize(size string) string {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(size)), func(r rune) bool { return r == 'x' || r == '*' || r == '×' })
	if len(parts) != 2 {
		return ""
	}
	w, errW := strconv.ParseFloat(parts[0], 64)
	h, errH := strconv.ParseFloat(parts[1], 64)
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return ""
	}
	if math.Abs(w/h-16.0/9.0) < 0.02 {
		return "16:9"
	}
	if math.Abs(w/h-9.0/16.0) < 0.02 {
		return "9:16"
	}
	if math.Abs(w/h-1) < 0.02 {
		return "1:1"
	}
	return ""
}

func (a *TaskAdaptor) buildVideoURL(_, fileID string) string {
	if a.apiKey == "" || a.baseURL == "" {
		return ""
	}

	url := fmt.Sprintf("%s/v1/files/retrieve?file_id=%s", a.baseURL, fileID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var retrieveResp RetrieveFileResponse
	if err := common.Unmarshal(responseBody, &retrieveResp); err != nil {
		return ""
	}

	if retrieveResp.BaseResp.StatusCode != StatusSuccess {
		return ""
	}

	return retrieveResp.File.DownloadURL
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsInt(slice []int, item int) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
