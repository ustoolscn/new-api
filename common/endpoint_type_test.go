package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestVideoGenerationModelsUseUnifiedEndpoint(t *testing.T) {
	assert.True(t, IsVideoGenerationModel("sora-2"))
	assert.True(t, IsVideoGenerationModel("veo-3.1-generate-preview"))
	assert.True(t, IsVideoGenerationModel("doubao-seedance-2-0-260128"))
	assert.True(t, IsVideoGenerationModel("grok-imagine-video-1.5"))
	assert.True(t, IsImageGenerationModel("grok-imagine-image-quality"))
	assert.False(t, IsVideoGenerationModel("video-embedding-model"))

	endpointTypes := GetEndpointTypesByChannelType(
		constant.ChannelTypeGemini,
		"veo-3.1-generate-preview",
	)
	assert.Equal(t, constant.EndpointTypeOpenAIVideo, endpointTypes[0])
	endpoint, ok := GetDefaultEndpointInfo(constant.EndpointTypeOpenAIVideo)
	assert.True(t, ok)
	assert.Equal(t, "/v1/video/generations", endpoint.Path)

	xaiVideoEndpoints := GetEndpointTypesByChannelType(constant.ChannelTypeXai, "grok-imagine-video")
	assert.Equal(t, constant.EndpointTypeOpenAIVideo, xaiVideoEndpoints[0])
	xaiImageEndpoints := GetEndpointTypesByChannelType(constant.ChannelTypeXai, "grok-imagine-image-quality")
	assert.Equal(t, constant.EndpointTypeImageGeneration, xaiImageEndpoints[0])
	xaiCompactEndpoints := GetEndpointTypesByChannelType(constant.ChannelTypeXai, "grok-4.5-openai-compact")
	assert.Equal(t, constant.EndpointTypeOpenAIResponseCompact, xaiCompactEndpoints[0])
}
