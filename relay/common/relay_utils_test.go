package common

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestValidateBasicTaskRequestDecodesGB18030JSONPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"doubao-seedance-2.0","prompt":"小猫在城市上空急速飞行","duration":5,"width":1280,"height":720}`
	encodedBody, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(body))
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewReader(encodedBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	info := &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}
	taskErr := ValidateBasicTaskRequest(ctx, info, constant.TaskActionGenerate)
	require.Nil(t, taskErr)

	req, err := GetTaskRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "小猫在城市上空急速飞行", req.Prompt)
	require.Equal(t, "doubao-seedance-2.0", req.Model)
	require.Equal(t, 5, req.Duration)
	require.Equal(t, 1280, req.Width)
	require.Equal(t, 720, req.Height)
}

func TestValidateBasicTaskRequestAcceptsImageObjects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{
		"model":"doubao-seedance-2.0",
		"prompt":"hello",
		"images":[
			{"url":"https://example.com/first.jpeg","role":"first_frame"},
			{"image_url":{"url":"https://example.com/last.jpeg"},"role":"last_frame"}
		],
		"duration":5
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewReader([]byte(body)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	info := &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}
	taskErr := ValidateBasicTaskRequest(ctx, info, constant.TaskActionGenerate)
	require.Nil(t, taskErr)

	req, err := GetTaskRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"https://example.com/first.jpeg", "https://example.com/last.jpeg"}, req.Images)
	require.Len(t, req.ImageInputs, 2)
	require.Equal(t, "first_frame", req.ImageInputs[0].Role)
	require.Equal(t, "last_frame", req.ImageInputs[1].Role)
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
				require.Equal(t, "invalid_seconds", taskErr.Code)
			} else {
				require.Nil(t, taskErr)
			}
		})
		t.Run(tt.name+" (basic task request)", func(t *testing.T) {
			context, info := newContext(t, tt.body)
			taskErr := ValidateBasicTaskRequest(context, info, constant.TaskActionGenerate)
			if tt.wantErr {
				require.NotNil(t, taskErr)
				require.Equal(t, "invalid_seconds", taskErr.Code)
			} else {
				require.Nil(t, taskErr)
			}
		})
	}
}
