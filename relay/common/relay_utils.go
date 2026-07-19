package common

import (
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type HasPrompt interface {
	GetPrompt() string
}

type HasImage interface {
	HasImage() bool
}

func GetFullRequestURL(baseURL string, requestURL string, channelType int) string {
	fullRequestURL := fmt.Sprintf("%s%s", baseURL, requestURL)

	if strings.HasPrefix(baseURL, "https://gateway.ai.cloudflare.com") {
		switch channelType {
		case constant.ChannelTypeOpenAI:
			fullRequestURL = fmt.Sprintf("%s%s", baseURL, strings.TrimPrefix(requestURL, "/v1"))
		case constant.ChannelTypeAzure:
			fullRequestURL = fmt.Sprintf("%s%s", baseURL, strings.TrimPrefix(requestURL, "/openai/deployments"))
		}
	}
	return fullRequestURL
}

func SanitizeURLForLog(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	query := parsedURL.Query()
	if len(query) == 0 {
		return rawURL
	}

	changed := false
	for key := range query {
		if isSensitiveURLQueryKey(key) {
			query.Set(key, "***masked***")
			changed = true
		}
	}
	if !changed {
		return rawURL
	}

	parsedURL.RawQuery = query.Encode()
	return parsedURL.String()
}

func isSensitiveURLQueryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "key",
		"api_key",
		"api-key",
		"apikey",
		"x-api-key",
		"access_token",
		"refresh_token",
		"id_token",
		"token",
		"authorization",
		"auth",
		"client_secret",
		"secret",
		"password",
		"passwd",
		"signature",
		"sig",
		"awsaccesskeyid",
		"x-amz-credential",
		"x-amz-security-token",
		"x-amz-signature":
		return true
	}
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "signature")
}

func GetAPIVersion(c *gin.Context) string {
	query := c.Request.URL.Query()
	apiVersion := query.Get("api-version")
	if apiVersion == "" {
		apiVersion = c.GetString("api_version")
	}
	return apiVersion
}

func createTaskError(err error, code string, statusCode int, localError bool) *dto.TaskError {
	return &dto.TaskError{
		Code:       code,
		Message:    err.Error(),
		StatusCode: statusCode,
		LocalError: localError,
		Error:      err,
	}
}

func storeTaskRequest(c *gin.Context, info *RelayInfo, action string, requestObj TaskSubmitReq) {
	info.Action = action
	c.Set("task_request", requestObj)
}
func GetTaskRequest(c *gin.Context) (TaskSubmitReq, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return TaskSubmitReq{}, fmt.Errorf("request not found in context")
	}
	req, ok := v.(TaskSubmitReq)
	if !ok {
		return TaskSubmitReq{}, fmt.Errorf("invalid task request type")
	}
	return req, nil
}

func validatePrompt(prompt string) *dto.TaskError {
	if strings.TrimSpace(prompt) == "" {
		return createTaskError(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest, true)
	}
	return nil
}

// MaxTaskDurationSeconds caps user-supplied video duration. Duration is used
// as a billing multiplier (OtherRatio "seconds"); an unbounded value could
// overflow quota calculation into a negative charge.
const MaxTaskDurationSeconds = 3600

const MaxTaskFPS = 120

const MaxTaskInputVideos = 4

// NormalizeVideoResolution converts dimensions such as 1280x720 to the
// provider-neutral short-side form (720p).
func NormalizeVideoResolution(size string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(size), " ", ""))
	parts := strings.FieldsFunc(normalized, func(r rune) bool {
		return r == 'x' || r == '*' || r == '×'
	})
	if len(parts) == 2 {
		width, widthErr := strconv.Atoi(parts[0])
		height, heightErr := strconv.Atoi(parts[1])
		if widthErr == nil && heightErr == nil && width > 0 && height > 0 {
			return fmt.Sprintf("%dp", min(width, height))
		}
	}
	if normalized != "" && !strings.HasSuffix(normalized, "p") {
		if _, err := strconv.Atoi(normalized); err == nil {
			return normalized + "p"
		}
	}
	return normalized
}

func validateTaskDurationBounds(req TaskSubmitReq) *dto.TaskError {
	seconds := req.OutputSeconds()
	if req.Seconds != "" && (seconds <= 0 || seconds > MaxTaskDurationSeconds || math.Trunc(seconds) != seconds || math.IsNaN(seconds) || math.IsInf(seconds, 0)) {
		return createTaskError(fmt.Errorf("seconds must be a whole number between 1 and %d", MaxTaskDurationSeconds), "invalid_seconds", http.StatusBadRequest, true)
	}
	if req.InputVideoSeconds != nil {
		inputSeconds := *req.InputVideoSeconds
		if inputSeconds <= 0 || inputSeconds > MaxTaskDurationSeconds || math.IsNaN(inputSeconds) || math.IsInf(inputSeconds, 0) {
			return createTaskError(fmt.Errorf("input_video_seconds must be between 1 and %d", MaxTaskDurationSeconds), "invalid_input_video_seconds", http.StatusBadRequest, true)
		}
	}
	if req.FPS != nil && (*req.FPS <= 0 || *req.FPS > MaxTaskFPS) {
		return createTaskError(fmt.Errorf("fps must be between 1 and %d", MaxTaskFPS), "invalid_fps", http.StatusBadRequest, true)
	}
	return nil
}

func validateTaskInputVideoURLs(req TaskSubmitReq) *dto.TaskError {
	if !req.HasAnyInputVideo() {
		return nil
	}
	if len(req.InputVideos) > MaxTaskInputVideos {
		return createTaskError(fmt.Errorf("input_videos supports at most %d URLs", MaxTaskInputVideos), "invalid_input_video", http.StatusBadRequest, true)
	}
	for _, rawURL := range req.InputVideos {
		if strings.TrimSpace(rawURL) == "" {
			return createTaskError(fmt.Errorf("input_videos must not contain empty values"), "invalid_input_video", http.StatusBadRequest, true)
		}
	}
	if len(req.InputVideos) > 0 && strings.TrimSpace(req.InputVideo) != "" && strings.TrimSpace(req.InputVideo) != strings.TrimSpace(req.InputVideos[0]) {
		return createTaskError(fmt.Errorf("input_video conflicts with input_videos"), "invalid_input_video", http.StatusBadRequest, true)
	}
	urls := req.InputVideoURLs()
	if len(urls) == 0 {
		return createTaskError(fmt.Errorf("input video must contain an HTTP or HTTPS URL"), "invalid_input_video", http.StatusBadRequest, true)
	}
	if len(urls) > MaxTaskInputVideos {
		return createTaskError(fmt.Errorf("input_videos supports at most %d URLs", MaxTaskInputVideos), "invalid_input_video", http.StatusBadRequest, true)
	}
	for _, rawURL := range urls {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return createTaskError(fmt.Errorf("input video must be an HTTP or HTTPS URL"), "invalid_input_video", http.StatusBadRequest, true)
		}
	}
	return nil
}

func validateMultipartTaskVideoFiles(form *multipart.Form) error {
	if form == nil {
		return nil
	}
	for field, files := range form.File {
		for _, file := range files {
			contentType := strings.ToLower(file.Header.Get("Content-Type"))
			fileName := strings.ToLower(file.Filename)
			if strings.Contains(strings.ToLower(field), "video") || strings.HasPrefix(contentType, "video/") ||
				strings.HasSuffix(fileName, ".mp4") || strings.HasSuffix(fileName, ".mov") || strings.HasSuffix(fileName, ".webm") {
				return fmt.Errorf("input video files are not supported; provide an HTTP or HTTPS URL")
			}
		}
	}
	return nil
}

func validateMultipartTaskRequest(c *gin.Context, info *RelayInfo, action string) (TaskSubmitReq, error) {
	var req TaskSubmitReq
	form, err := c.MultipartForm()
	if err != nil {
		return req, err
	}
	if err := validateMultipartTaskVideoFiles(form); err != nil {
		return req, err
	}

	formData := c.Request.PostForm
	req = TaskSubmitReq{
		Prompt:         formData.Get("prompt"),
		Model:          formData.Get("model"),
		Mode:           formData.Get("mode"),
		Image:          formData.Get("image"),
		InputVideo:     formData.Get("input_video"),
		Size:           formData.Get("size"),
		NegativePrompt: formData.Get("negative_prompt"),
		InputReference: formData.Get("input_reference"),
		Metadata:       make(map[string]interface{}),
	}
	if req.InputVideo == "" {
		req.InputVideo = formData.Get("video")
	}
	if req.Size == "" {
		req.Size = formData.Get("resolution")
	}

	secondsValue := formData.Get("seconds")
	if secondsValue == "" {
		secondsValue = formData.Get("duration")
	}
	if secondsValue != "" {
		seconds, err := strconv.ParseFloat(strings.TrimSpace(secondsValue), 64)
		if err != nil {
			return req, fmt.Errorf("seconds must be a number or numeric string")
		}
		req.Seconds = strconv.FormatFloat(seconds, 'f', -1, 64)
		if math.Trunc(seconds) == seconds && seconds <= float64(math.MaxInt) && seconds >= float64(math.MinInt) {
			req.Duration = int(seconds)
		}
	}

	inputVideoSecondsValue := formData.Get("input_video_seconds")
	if inputVideoSecondsValue == "" {
		inputVideoSecondsValue = formData.Get("input_video_duration")
	}
	if inputVideoSecondsValue == "" {
		inputVideoSecondsValue = formData.Get("inputVideoDuration")
	}
	if inputVideoSecondsValue != "" {
		inputVideoSeconds, err := strconv.ParseFloat(strings.TrimSpace(inputVideoSecondsValue), 64)
		if err != nil {
			return req, fmt.Errorf("input_video_seconds must be a number or numeric string")
		}
		req.InputVideoSeconds = &inputVideoSeconds
	}

	if widthValue := strings.TrimSpace(formData.Get("width")); widthValue != "" {
		width, err := strconv.Atoi(widthValue)
		if err != nil || width <= 0 {
			return req, fmt.Errorf("width must be a positive integer")
		}
		req.Width = &width
	}
	if heightValue := strings.TrimSpace(formData.Get("height")); heightValue != "" {
		height, err := strconv.Atoi(heightValue)
		if err != nil || height <= 0 {
			return req, fmt.Errorf("height must be a positive integer")
		}
		req.Height = &height
	}
	if req.Size == "" && req.Width != nil && req.Height != nil {
		req.Size = fmt.Sprintf("%dx%d", *req.Width, *req.Height)
	}
	fpsValue := formData.Get("fps")
	if fpsValue == "" {
		fpsValue = formData.Get("frame_rate")
	}
	if fpsValue == "" {
		fpsValue = formData.Get("framespersecond")
	}
	if fpsValue == "" {
		fpsValue = formData.Get("framesPerSecond")
	}
	if fpsValue = strings.TrimSpace(fpsValue); fpsValue != "" {
		fps, err := strconv.Atoi(fpsValue)
		if err != nil {
			return req, fmt.Errorf("fps must be an integer")
		}
		req.FPS = &fps
	}
	if seedValue := strings.TrimSpace(formData.Get("seed")); seedValue != "" {
		seed, err := strconv.Atoi(seedValue)
		if err != nil {
			return req, fmt.Errorf("seed must be an integer")
		}
		req.Seed = &seed
	}
	if generateAudioValue := strings.TrimSpace(formData.Get("generate_audio")); generateAudioValue != "" {
		generateAudio, err := strconv.ParseBool(generateAudioValue)
		if err != nil {
			return req, fmt.Errorf("generate_audio must be a boolean")
		}
		req.GenerateAudio = &generateAudio
	}

	if images := formData["images"]; len(images) > 0 {
		req.Images = images
	}
	if inputVideos := formData["input_videos"]; len(inputVideos) > 0 {
		req.InputVideos = inputVideos
	}
	if metadataValue := strings.TrimSpace(formData.Get("metadata")); metadataValue != "" {
		if err := common.UnmarshalJsonStr(metadataValue, &req.Metadata); err != nil {
			return req, fmt.Errorf("metadata must be a JSON object: %w", err)
		}
	}

	for key, values := range formData {
		if len(values) > 0 && !isKnownTaskField(key) {
			if intVal, err := strconv.Atoi(values[0]); err == nil {
				req.Metadata[key] = intVal
			} else if floatVal, err := strconv.ParseFloat(values[0], 64); err == nil {
				req.Metadata[key] = floatVal
			} else {
				req.Metadata[key] = values[0]
			}
		}
	}
	req.Normalize()
	return req, nil
}

func ValidateMultipartDirect(c *gin.Context, info *RelayInfo) *dto.TaskError {
	var req TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return createTaskError(err, "invalid_json", http.StatusBadRequest, true)
	}
	if strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "multipart/") {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return createTaskError(err, "invalid_multipart_form", http.StatusBadRequest, true)
		}
		defer form.RemoveAll()
		if err := validateMultipartTaskVideoFiles(form); err != nil {
			return createTaskError(err, "invalid_input_video", http.StatusBadRequest, true)
		}
	}

	req.Normalize()

	if strings.TrimSpace(req.Model) == "" {
		return createTaskError(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest, true)
	}

	if taskErr := validatePrompt(req.Prompt); taskErr != nil {
		return taskErr
	}

	if taskErr := validateTaskDurationBounds(req); taskErr != nil {
		return taskErr
	}
	if taskErr := validateTaskInputVideoURLs(req); taskErr != nil {
		return taskErr
	}

	action := constant.TaskActionTextGenerate
	if req.HasImage() || req.HasAnyInputVideo() {
		action = constant.TaskActionGenerate
	}
	if strings.HasPrefix(req.Model, "sora-2") {

		if req.Size == "" {
			req.Size = "720x1280"
		}

		if req.OutputSeconds() <= 0 {
			req.Seconds = "4"
			req.Duration = 4
		}

		if req.Model == "sora-2" && !lo.Contains([]string{"720x1280", "1280x720"}, req.Size) {
			return createTaskError(fmt.Errorf("sora-2 size is invalid"), "invalid_size", http.StatusBadRequest, true)
		}
		if req.Model == "sora-2-pro" && !lo.Contains([]string{"720x1280", "1280x720", "1792x1024", "1024x1792"}, req.Size) {
			return createTaskError(fmt.Errorf("sora-2 size is invalid"), "invalid_size", http.StatusBadRequest, true)
		}
		// OtherRatios 已移到 Sora adaptor 的 EstimateBilling 中设置
	}

	storeTaskRequest(c, info, action, req)

	return nil
}

func isKnownTaskField(field string) bool {
	knownFields := map[string]bool{
		"prompt":               true,
		"model":                true,
		"mode":                 true,
		"image":                true,
		"images":               true,
		"input_video":          true,
		"input_videos":         true,
		"video":                true,
		"input_video_seconds":  true,
		"input_video_duration": true,
		"inputVideoDuration":   true,
		"size":                 true,
		"resolution":           true,
		"width":                true,
		"height":               true,
		"seconds":              true,
		"duration":             true,
		"fps":                  true,
		"frame_rate":           true,
		"framespersecond":      true,
		"framesPerSecond":      true,
		"seed":                 true,
		"negative_prompt":      true,
		"generate_audio":       true,
		"metadata":             true,
		"input_reference":      true,
	}
	return knownFields[field]
}

func ValidateBasicTaskRequest(c *gin.Context, info *RelayInfo, action string) *dto.TaskError {
	var err error
	contentType := c.GetHeader("Content-Type")
	var req TaskSubmitReq
	if strings.HasPrefix(contentType, "multipart/form-data") {
		req, err = validateMultipartTaskRequest(c, info, action)
		if err != nil {
			return createTaskError(err, "invalid_multipart_form", http.StatusBadRequest, true)
		}
	} else if strings.HasPrefix(contentType, "application/json") {
		// 为了metadata字段的兼容性，统一读取可复用请求体；额外兼容非 UTF-8 JSON。
		err = unmarshalTaskJSONBody(c, &req)
	} else {
		err = common.UnmarshalBodyReusable(c, &req)
	}
	if err != nil {
		return createTaskError(err, "invalid_request", http.StatusBadRequest, true)
	}

	if taskErr := validatePrompt(req.Prompt); taskErr != nil {
		return taskErr
	}

	if taskErr := validateTaskDurationBounds(req); taskErr != nil {
		return taskErr
	}

	req.Normalize()
	if taskErr := validateTaskInputVideoURLs(req); taskErr != nil {
		return taskErr
	}
	if action == constant.TaskActionTextGenerate && (req.HasImage() || req.HasInputVideo()) {
		action = constant.TaskActionGenerate
	}

	storeTaskRequest(c, info, action, req)
	return nil
}

func ValidateNoTaskInputVideo(c *gin.Context, provider string) *dto.TaskError {
	req, err := GetTaskRequest(c)
	if err != nil {
		return createTaskError(err, "invalid_request", http.StatusBadRequest, true)
	}
	if req.HasAnyInputVideo() {
		return createTaskError(
			fmt.Errorf("%s does not support input_video", provider),
			"unsupported_input_video",
			http.StatusBadRequest,
			true,
		)
	}
	return nil
}

func ValidateTaskInputVideoCount(c *gin.Context, provider string, maxVideos int) *dto.TaskError {
	req, err := GetTaskRequest(c)
	if err != nil {
		return createTaskError(err, "invalid_request", http.StatusBadRequest, true)
	}
	if count := len(req.InputVideoURLs()); count > maxVideos {
		return createTaskError(
			fmt.Errorf("%s supports at most %d input video URL(s)", provider, maxVideos),
			"unsupported_input_video_count",
			http.StatusBadRequest,
			true,
		)
	}
	return nil
}

func unmarshalTaskJSONBody(c *gin.Context, req *TaskSubmitReq) error {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	requestBody, err := storage.Bytes()
	if err != nil {
		return err
	}
	if utf8.Valid(requestBody) {
		return common.Unmarshal(requestBody, req)
	}
	decoded, decodeErr := simplifiedchinese.GB18030.NewDecoder().Bytes(requestBody)
	if decodeErr == nil {
		if err := common.Unmarshal(decoded, req); err == nil {
			return nil
		}
	}
	return common.Unmarshal(requestBody, req)
}
