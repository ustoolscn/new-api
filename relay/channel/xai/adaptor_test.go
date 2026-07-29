package xai

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/tidwall/gjson"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertImageGenerationRequestUsesXAIParameters(t *testing.T) {
	request := dto.ImageRequest{
		Model:          "grok-imagine-image-quality",
		Prompt:         "a cinematic city",
		N:              common.GetPointer(uint(2)),
		Size:           "1536x1024",
		ResponseFormat: "b64_json",
		Extra: map[string]json.RawMessage{
			"aspect_ratio":    json.RawMessage(`"16:9"`),
			"resolution":      json.RawMessage(`"2k"`),
			"storage_options": json.RawMessage(`{"filename":"city.png"}`),
		},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(nil, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
	}, request)

	require.NoError(t, err)
	data, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.Equal(t, "grok-imagine-image-quality", gjson.GetBytes(data, "model").String())
	assert.Equal(t, int64(2), gjson.GetBytes(data, "n").Int())
	assert.Equal(t, "16:9", gjson.GetBytes(data, "aspect_ratio").String())
	assert.Equal(t, "2k", gjson.GetBytes(data, "resolution").String())
	assert.Equal(t, "city.png", gjson.GetBytes(data, "storage_options.filename").String())
	assert.False(t, gjson.GetBytes(data, "size").Exists())
}

func TestConvertImageEditRequestNormalizesURLAndFileID(t *testing.T) {
	request := dto.ImageRequest{
		Model:  "grok-imagine-image-quality",
		Prompt: "combine the references",
		Images: json.RawMessage(`[
			"https://example.com/first.png",
			{"file_id":"file_second"}
		]`),
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(nil, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
	}, request)

	require.NoError(t, err)
	data, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/first.png", gjson.GetBytes(data, "images.0.url").String())
	assert.Equal(t, "file_second", gjson.GetBytes(data, "images.1.file_id").String())
	assert.False(t, gjson.GetBytes(data, "image").Exists())
}

func TestConvertImageEditRequestRejectsConflictingInputs(t *testing.T) {
	request := dto.ImageRequest{
		Model:  "grok-imagine-image-quality",
		Prompt: "edit",
		Image:  json.RawMessage(`"https://example.com/image.png"`),
		Images: json.RawMessage(`["https://example.com/reference.png"]`),
	}

	_, err := (&Adaptor{}).ConvertImageRequest(nil, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
	}, request)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestConvertResponsesSearchAliasUsesOfficialTools(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-4.3-search"}}
	request := dto.OpenAIResponsesRequest{Model: "grok-4.3-search"}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)

	require.NoError(t, err)
	responseRequest, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	assert.Equal(t, "grok-4.3", responseRequest.Model)
	assert.Equal(t, "grok-4.3", info.UpstreamModelName)
	assert.Equal(t, "web_search", gjson.GetBytes(responseRequest.Tools, "0.type").String())
	assert.Equal(t, "x_search", gjson.GetBytes(responseRequest.Tools, "1.type").String())
}

func TestModelListIncludesResponsesCompactVariants(t *testing.T) {
	assert.Contains(t, ModelList, "grok-4.5-openai-compact")
	assert.Contains(t, ModelList, "grok-imagine-image-quality")
	assert.NotContains(t, ModelList, "grok-imagine-video-openai-compact")
}
