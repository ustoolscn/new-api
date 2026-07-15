package moonshot

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertOpenAIResponsesRequestUsesChatCompletionsCompat(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://api.moonshot.cn",
			UpstreamModelName: "kimi-k2",
		},
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "kimi-k2",
		Input: common.StringToByteSlice(`"hello"`),
	})

	require.NoError(t, err)
	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "kimi-k2", chatReq.Model)
	require.Len(t, chatReq.Messages, 1)
	require.Equal(t, "user", chatReq.Messages[0].Role)
	require.Equal(t, "hello", chatReq.Messages[0].Content)

	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://api.moonshot.cn/v1/chat/completions", url)
}

func TestConvertOpenAIResponsesRequestFiltersUnsupportedCustomTools(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://api.moonshot.cn",
			UpstreamModelName: "kimi-k2.6",
		},
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "kimi-k2.6",
		Input: common.StringToByteSlice(`[
			{"role":"user","content":"next turn"},
			{"type":"custom_tool_call","call_id":"call_custom","name":"apply_patch","input":"patch body"},
			{"type":"custom_tool_call_output","call_id":"call_custom","output":"ok"},
			{"type":"function_call","call_id":"call_function","name":"lookup","arguments":"{\"q\":\"x\"}"},
			{"type":"function_call_output","call_id":"call_function","output":"found"}
		]`),
		Tools: common.StringToByteSlice(`[
			{"type":"custom","name":"apply_patch"},
			{"type":"function","name":"lookup","parameters":{"type":"object"}},
			{"type":"namespace","name":"repo","tools":[{"type":"function","name":"search","parameters":{"type":"object"}}]}
		]`),
		ToolChoice: common.StringToByteSlice(`{"type":"custom","name":"apply_patch"}`),
	})
	require.NoError(t, err)

	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, chatReq.Tools, 2)
	require.Equal(t, "function", chatReq.Tools[0].Type)
	require.Equal(t, "lookup", chatReq.Tools[0].Function.Name)
	require.Equal(t, "function", chatReq.Tools[1].Type)
	require.Equal(t, "repo__search", chatReq.Tools[1].Function.Name)
	require.Nil(t, chatReq.ToolChoice)
	require.Len(t, chatReq.Messages, 3)
	require.Equal(t, "user", chatReq.Messages[0].Role)
	require.Equal(t, "assistant", chatReq.Messages[1].Role)
	require.Equal(t, "tool", chatReq.Messages[2].Role)
	require.Equal(t, "call_function", chatReq.Messages[2].ToolCallId)
	require.False(t, gjson.GetBytes(chatReq.Messages[1].ToolCalls, "#(type==\"custom\")").Exists())
	require.Equal(t, "function", gjson.GetBytes(chatReq.Messages[1].ToolCalls, "0.type").String())
}
