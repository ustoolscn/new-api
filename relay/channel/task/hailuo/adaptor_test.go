package hailuo

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func TestH3RequestURLAndPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", common.TaskSubmitReq{
		Prompt:   "a cat fly in sea",
		Size:     "1280x720",
		Duration: 5,
	})

	adaptor := &TaskAdaptor{baseURL: "https://cp.compshare.cn", apiKey: "secret"}
	info := &common.RelayInfo{
		OriginModelName: "MiniMax-H3",
		ChannelMeta:     &common.ChannelMeta{UpstreamModelName: "MiniMax-H3"},
		TaskRelayInfo:   &common.TaskRelayInfo{},
	}

	url, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://cp.compshare.cn/minimax/v2/video_generation", url)

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"MiniMax-H3","content":[{"type":"text","text":"a cat fly in sea"}],"resolution":"768P","duration":5,"ratio":"16:9"}`, string(data))

}

func TestH3RequestPayloadMapsMediaAndProviderOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	useContextIR := true
	aigcWatermark := false
	c.Set("task_request", common.TaskSubmitReq{
		Prompt: "人物走向镜头",
		Images: []string{"https://example.com/first.png", "https://example.com/last.png"},
		ImageInputs: []common.TaskImageInput{
			{URL: "https://example.com/first.png", Role: "first_frame"},
			{URL: "https://example.com/last.png", Role: "last_frame"},
		},
		Duration: 5,
		Ratio:    "adaptive",
		Metadata: map[string]interface{}{
			"callback_url":   "https://example.com/callback",
			"callback_token": "callback-secret",
			"use_context_ir": &useContextIR,
			"aigc_watermark": &aigcWatermark,
		},
	})

	adaptor := &TaskAdaptor{baseURL: "https://cp.compshare.cn"}
	info := &common.RelayInfo{ChannelMeta: &common.ChannelMeta{UpstreamModelName: "MiniMax-H3"}}
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"MiniMax-H3","content":[{"type":"text","text":"人物走向镜头"},{"type":"image_url","image_url":{"url":"https://example.com/first.png"},"role":"first_frame"},{"type":"image_url","image_url":{"url":"https://example.com/last.png"},"role":"last_frame"}],"resolution":"768P","duration":5,"ratio":"adaptive","callback_url":"https://example.com/callback","callback_token":"callback-secret","use_context_ir":true,"aigc_watermark":false}`, string(data))
}

func TestH3ParseTaskResult(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{
		"task":{"id":"01a024fd-36d2-762a-9965-264ebd75a737","model":"MiniMax-H3","status":"succeeded","content":{"url":"https://files.example/video.mp4","prompt":"a cat fly in sea"},"resolution":"768P","duration":5,"ratio":"16:9"}
	}`))
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", string(result.Status))
	assert.Equal(t, "100%", result.Progress)
	assert.Equal(t, "https://files.example/video.mp4", result.Url)
}

func TestH3FetchTaskUsesPathEndpoint(t *testing.T) {
	server := httptest.NewServer(nil)
	defer server.Close()
	assert.Equal(t, server.URL+"/minimax/v2/query/video_generation/task-1", h3EndpointForBase(server.URL, H3QueryTaskEndpoint)+"/task-1")
}

func TestH3ModelMatchingIsCaseInsensitive(t *testing.T) {
	assert.True(t, isH3Model(" minimax-h3 "))
	assert.False(t, isH3Model(strings.TrimSpace("MiniMax-Hailuo-02")))
}

func TestH3ResolutionNormalization(t *testing.T) {
	assert.Equal(t, Resolution768P, h3ResolutionFromSize("768P"))
	assert.Equal(t, Resolution1080P, h3ResolutionFromSize("1920x1080"))
	assert.Equal(t, Resolution2K, h3ResolutionFromSize("2K"))
	assert.Empty(t, h3ResolutionFromSize("4K"))
}
