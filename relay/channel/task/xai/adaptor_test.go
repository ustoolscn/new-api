package xai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newVideoContext(body string) *gin.Context {
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	return context
}

func newVideoRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: "grok-imagine-video",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://api.x.ai",
			ApiKey:         "xai-key",
		},
	}
}

func TestTextVideoRequestMapsOfficialParameters(t *testing.T) {
	context := newVideoContext(`{
		"model":"grok-imagine-video",
		"prompt":"a rocket launching from Mars",
		"duration":"10",
		"aspect_ratio":"16:9",
		"resolution":"720p"
	}`)
	info := newVideoRelayInfo()
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	assert.Equal(t, constant.TaskActionTextGenerate, info.Action)
	info.UpstreamModelName = "grok-imagine-video"
	body, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, int64(10), gjson.GetBytes(data, "duration").Int())
	assert.Equal(t, "16:9", gjson.GetBytes(data, "aspect_ratio").String())
	assert.Equal(t, "720p", gjson.GetBytes(data, "resolution").String())
	assert.False(t, gjson.GetBytes(data, "seconds").Exists())
	url, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.x.ai/v1/videos/generations", url)
}

func TestImageVideoAllowsPromptOmissionAndFileID(t *testing.T) {
	context := newVideoContext(`{
		"model":"grok-imagine-video-1.5",
		"image":{"file_id":"file_image"},
		"duration":8,
		"resolution":"1080p"
	}`)
	info := newVideoRelayInfo()
	info.OriginModelName = "grok-imagine-video-1.5"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	assert.Equal(t, constant.TaskActionGenerate, info.Action)
	info.UpstreamModelName = "grok-imagine-video-1.5"
	body, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "file_image", gjson.GetBytes(data, "image.file_id").String())
	assert.False(t, gjson.GetBytes(data, "prompt").Exists())
}

func TestReferenceVideoUsesReferenceImages(t *testing.T) {
	context := newVideoContext(`{
		"model":"grok-imagine-video",
		"prompt":"use both references",
		"images":[
			{"url":"https://example.com/one.png"},
			{"file_id":"file_two"}
		]
	}`)
	info := newVideoRelayInfo()
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	info.UpstreamModelName = "grok-imagine-video"
	body, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, int64(2), gjson.GetBytes(data, "reference_images.#").Int())
	assert.Equal(t, "file_two", gjson.GetBytes(data, "reference_images.1.file_id").String())
	assert.False(t, gjson.GetBytes(data, "image").Exists())
}

func TestVideoExtensionUsesExtensionEndpoint(t *testing.T) {
	context := newVideoContext(`{
		"model":"grok-imagine-video",
		"prompt":"continue into the sunset",
		"input_video":"https://example.com/input.mp4",
		"mode":"extension",
		"duration":6
	}`)
	info := newVideoRelayInfo()
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	assert.Equal(t, constant.TaskActionVideoExtend, info.Action)
	url, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.x.ai/v1/videos/extensions", url)
	info.UpstreamModelName = "grok-imagine-video"
	body, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/input.mp4", gjson.GetBytes(data, "video.url").String())
	assert.Equal(t, int64(6), gjson.GetBytes(data, "duration").Int())
}

func TestVideoRequestRejectsUnsupportedReferenceMode(t *testing.T) {
	context := newVideoContext(`{
		"model":"grok-imagine-video-1.5",
		"prompt":"use references",
		"reference_images":[{"url":"https://example.com/one.png"}]
	}`)
	info := newVideoRelayInfo()
	info.OriginModelName = "grok-imagine-video-1.5"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	taskErr := adaptor.ValidateRequestAndSetAction(context, info)

	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "does not support reference_images")
}

func TestParseTaskResultMapsXAIStatusAndURL(t *testing.T) {
	progress := 100
	body, err := common.Marshal(taskResponse{
		Status:   "done",
		Progress: &progress,
		Video: &struct {
			URL               string `json:"url,omitempty"`
			Duration          int    `json:"duration,omitempty"`
			RespectModeration bool   `json:"respect_moderation,omitempty"`
		}{URL: "https://example.com/result.mp4", Duration: 8, RespectModeration: true},
	})
	require.NoError(t, err)

	result, err := (&TaskAdaptor{}).ParseTaskResult(body)

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "100%", result.Progress)
	assert.Equal(t, "https://example.com/result.mp4", result.Url)
}
