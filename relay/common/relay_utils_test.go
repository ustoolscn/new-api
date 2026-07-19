package common

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeURLForLogMasksSensitiveQueryValues(t *testing.T) {
	rawURL := "https://example.test/v1beta/models/gemini:streamGenerateContent?alt=sse&key=sk-secret&access_token=ya29-secret&api-version=2024-02-01"

	got := SanitizeURLForLog(rawURL)

	assert.NotContains(t, got, "sk-secret")
	assert.NotContains(t, got, "ya29-secret")
	parsedURL, err := url.Parse(got)
	require.NoError(t, err)
	query := parsedURL.Query()
	assert.Equal(t, "***masked***", query.Get("key"))
	assert.Equal(t, "***masked***", query.Get("access_token"))
	assert.Equal(t, "sse", query.Get("alt"))
	assert.Equal(t, "2024-02-01", query.Get("api-version"))
}

func TestSanitizeURLForLogMasksAWSAndSecretLikeQueryKeys(t *testing.T) {
	rawURL := "https://example.test/path?X-Amz-Credential=credential&X-Amz-Signature=signature&session_token=session&client_secret=secret&model=gpt-test"

	got := SanitizeURLForLog(rawURL)

	assert.NotContains(t, got, "X-Amz-Credential=credential")
	assert.NotContains(t, got, "X-Amz-Signature=signature")
	assert.NotContains(t, got, "session_token=session")
	assert.NotContains(t, got, "client_secret=secret")
	parsedURL, err := url.Parse(got)
	require.NoError(t, err)
	query := parsedURL.Query()
	assert.Equal(t, "***masked***", query.Get("X-Amz-Credential"))
	assert.Equal(t, "***masked***", query.Get("X-Amz-Signature"))
	assert.Equal(t, "***masked***", query.Get("session_token"))
	assert.Equal(t, "***masked***", query.Get("client_secret"))
	assert.Equal(t, "gpt-test", query.Get("model"))
}

func TestSanitizeURLForLogKeepsURLWithoutSensitiveQuery(t *testing.T) {
	rawURL := "https://example.test/v1/chat/completions?api-version=2024-02-01&alt=sse"

	got := SanitizeURLForLog(rawURL)

	assert.Equal(t, rawURL, got)
}

func TestValidateMultipartDirectNormalizesImageField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := strings.NewReader(`{"model":"wan2.7-i2v","prompt":"animate","image":" https://example.com/first.png "}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	info := &RelayInfo{
		TaskRelayInfo: &TaskRelayInfo{},
	}

	taskErr := ValidateMultipartDirect(context, info)

	require.Nil(t, taskErr)
	storedReq, err := GetTaskRequest(context)
	require.NoError(t, err)
	require.Equal(t, []string{"https://example.com/first.png"}, storedReq.Images)
	require.Equal(t, constant.TaskActionGenerate, info.Action)
}

func TestTaskSubmitRequestNormalizesVideoAliases(t *testing.T) {
	var req TaskSubmitReq
	err := rootcommon.Unmarshal([]byte(`{
		"model":"veo-3.1-generate-preview",
		"prompt":"animate this",
		"images":{"url":"https://example.com/first.png"},
		"video":"https://example.com/input.mp4",
		"duration":"8",
		"input_video_duration":"3.5",
		"resolution":"1280x720",
		"frame_rate":48,
		"seed":0,
		"generate_audio":false,
		"metadata":"{\"personGeneration\":\"allow_adult\"}"
	}`), &req)

	require.NoError(t, err)
	assert.Equal(t, "8", req.Seconds)
	assert.Equal(t, 8, req.Duration)
	assert.Equal(t, "1280x720", req.Size)
	require.NotNil(t, req.FPS)
	assert.Equal(t, 48, *req.FPS)
	require.NotNil(t, req.InputVideoSeconds)
	assert.InDelta(t, 3.5, *req.InputVideoSeconds, 0.0001)
	assert.Equal(t, []string{"https://example.com/first.png"}, req.Images)
	assert.Equal(t, "https://example.com/input.mp4", req.InputVideo)
	assert.Equal(t, []string{"https://example.com/input.mp4"}, req.InputVideos)
	require.NotNil(t, req.Seed)
	assert.Equal(t, 0, *req.Seed)
	require.NotNil(t, req.GenerateAudio)
	assert.False(t, *req.GenerateAudio)
	assert.Equal(t, "allow_adult", req.Metadata["personGeneration"])
}

func TestTaskSubmitRequestCanonicalFieldsOverrideMetadata(t *testing.T) {
	var req TaskSubmitReq
	err := rootcommon.Unmarshal([]byte(`{
		"model":"veo-3.1-generate-preview",
		"prompt":"animate this",
		"fps":48,
		"seed":0,
		"negative_prompt":"canonical negative prompt",
		"generate_audio":false,
		"input_video_seconds":3.5,
		"metadata":{
			"fps":12,
			"seed":99,
			"negative_prompt":"metadata negative prompt",
			"generate_audio":true,
			"input_video_seconds":30
		}
	}`), &req)

	require.NoError(t, err)
	assert.Equal(t, 48, req.Metadata["fps"])
	assert.Equal(t, 0, req.Metadata["seed"])
	assert.Equal(t, "canonical negative prompt", req.Metadata["negative_prompt"])
	assert.Equal(t, false, req.Metadata["generate_audio"])
	assert.Equal(t, 3.5, req.Metadata["input_video_seconds"])
}

func TestValidateBasicTaskRequestNormalizesMultipartVideoFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "veo-3.1-generate-preview"))
	require.NoError(t, writer.WriteField("prompt", "animate this"))
	require.NoError(t, writer.WriteField("duration", "8"))
	require.NoError(t, writer.WriteField("resolution", "1280x720"))
	require.NoError(t, writer.WriteField("framesPerSecond", "48"))
	require.NoError(t, writer.WriteField("inputVideoDuration", "3.5"))
	require.NoError(t, writer.WriteField("generate_audio", "false"))
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	info := &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}

	taskErr := ValidateBasicTaskRequest(context, info, constant.TaskActionGenerate)

	require.Nil(t, taskErr)
	req, err := GetTaskRequest(context)
	require.NoError(t, err)
	assert.Equal(t, "8", req.Seconds)
	assert.Equal(t, "1280x720", req.Size)
	require.NotNil(t, req.FPS)
	assert.Equal(t, 48, *req.FPS)
	require.NotNil(t, req.InputVideoSeconds)
	assert.InDelta(t, 3.5, *req.InputVideoSeconds, 0.0001)
	require.NotNil(t, req.GenerateAudio)
	assert.False(t, *req.GenerateAudio)
}

func TestValidateBasicTaskRequestRejectsInvalidMultipartFPS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "veo-3.1-generate-preview"))
	require.NoError(t, writer.WriteField("prompt", "animate this"))
	require.NoError(t, writer.WriteField("fps", "fast"))
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	info := &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}

	taskErr := ValidateBasicTaskRequest(context, info, constant.TaskActionGenerate)

	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_multipart_form", taskErr.Code)
}

func TestValidateNoTaskInputVideoRejectsCanonicalAndMetadataInputs(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "canonical input video",
			body: `{"model":"veo-3.1-generate-preview","prompt":"animate","input_video":"https://example.com/input.mp4"}`,
		},
		{
			name: "metadata input video",
			body: `{"model":"veo-3.1-generate-preview","prompt":"animate","metadata":{"content":[{"type":"video_url","video_url":{"url":"https://example.com/input.mp4"}}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request
			info := &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}

			require.Nil(t, ValidateBasicTaskRequest(context, info, constant.TaskActionGenerate))
			taskErr := ValidateNoTaskInputVideo(context, "test provider")

			require.NotNil(t, taskErr)
			assert.Equal(t, "unsupported_input_video", taskErr.Code)
		})
	}
}

// TestTaskDurationBounds guards the billing invariant that user-supplied
// video duration (a quota multiplier via OtherRatio "seconds") is bounded, so
// it can never overflow quota calculation into a negative charge.
func TestTaskDurationBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newContext := func(t *testing.T, body string) (*gin.Context, *RelayInfo) {
		request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = request
		return context, &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}
	}

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "huge duration is rejected",
			body:    `{"model":"sora-2","prompt":"a cat","duration":9999999999}`,
			wantErr: true,
		},
		{
			name:    "huge seconds string is rejected",
			body:    `{"model":"sora-2","prompt":"a cat","seconds":"9999999999"}`,
			wantErr: true,
		},
		{
			name:    "negative duration is rejected",
			body:    `{"model":"sora-2","prompt":"a cat","duration":-8}`,
			wantErr: true,
		},
		{
			name:    "input video duration is bounded",
			body:    `{"model":"sora-2","prompt":"a cat","seconds":8,"input_video":"https://example.com/in.mp4","input_video_seconds":3601}`,
			wantErr: true,
		},
		{
			name:    "fps is bounded",
			body:    `{"model":"sora-2","prompt":"a cat","seconds":8,"fps":121}`,
			wantErr: true,
		},
		{
			name: "normal duration is accepted",
			body: `{"model":"sora-2","prompt":"a cat","seconds":"8"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" (multipart direct)", func(t *testing.T) {
			context, info := newContext(t, tt.body)
			taskErr := ValidateMultipartDirect(context, info)
			if tt.wantErr {
				require.NotNil(t, taskErr)
				require.Contains(t, []string{"invalid_seconds", "invalid_input_video_seconds", "invalid_fps"}, taskErr.Code)
			} else {
				require.Nil(t, taskErr)
			}
		})
		t.Run(tt.name+" (basic task request)", func(t *testing.T) {
			context, info := newContext(t, tt.body)
			taskErr := ValidateBasicTaskRequest(context, info, constant.TaskActionGenerate)
			if tt.wantErr {
				require.NotNil(t, taskErr)
				require.Contains(t, []string{"invalid_seconds", "invalid_input_video_seconds", "invalid_fps"}, taskErr.Code)
			} else {
				require.Nil(t, taskErr)
			}
		})
	}
}
