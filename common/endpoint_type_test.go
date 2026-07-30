package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestVideoGenerationModelDetectionAndDefaultEndpoint(t *testing.T) {
	assert.True(t, IsVideoGenerationModel("sora-2"))
	assert.True(t, IsVideoGenerationModel("veo-3.1-generate-preview"))
	assert.True(t, IsVideoGenerationModel("doubao-seedance-2-0-260128"))
	assert.True(t, IsVideoGenerationModel("grok-imagine-video-1.5"))
	assert.True(t, IsImageGenerationModel("grok-imagine-image-quality"))
	assert.False(t, IsVideoGenerationModel("video-embedding-model"))

	endpoint, ok := GetDefaultEndpointInfo(constant.EndpointTypeOpenAIVideo)
	assert.True(t, ok)
	assert.Equal(t, "/v1/video/generations", endpoint.Path)
}
