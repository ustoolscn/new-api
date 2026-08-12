package deepseek

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIResponsesRequestPassesThroughNative(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://api.deepseek.com",
			UpstreamModelName: "deepseek-chat",
		},
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "deepseek-chat",
		Input: common.StringToByteSlice(`"hello"`),
	})

	require.NoError(t, err)
	req, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, "deepseek-chat", req.Model)
	require.Equal(t, `"hello"`, string(req.Input))

	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://api.deepseek.com/responses", url)
}

func TestConvertOpenAIResponsesRequestMapsV4ReasoningSuffix(t *testing.T) {
	adaptor := &Adaptor{}
	tests := []struct {
		name       string
		model      string
		wantModel  string
		wantEffort string
	}{
		{name: "max", model: "deepseek-v4-flash-max", wantModel: "deepseek-v4-flash", wantEffort: "max"},
		{name: "none", model: "deepseek-v4-flash-none", wantModel: "deepseek-v4-flash", wantEffort: "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				RelayMode:   relayconstant.RelayModeResponses,
				ChannelMeta: &relaycommon.ChannelMeta{},
			}
			converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
				Model: tt.model,
				Input: common.StringToByteSlice(`"hello"`),
			})

			require.NoError(t, err)
			req, ok := converted.(dto.OpenAIResponsesRequest)
			require.True(t, ok)
			require.Equal(t, tt.wantModel, req.Model)
			require.NotNil(t, req.Reasoning)
			require.Equal(t, tt.wantEffort, req.Reasoning.Effort)
			require.Equal(t, tt.wantModel, info.UpstreamModelName)
			if tt.wantEffort == "none" {
				require.Equal(t, "", info.ReasoningEffort)
			} else {
				require.Equal(t, tt.wantEffort, info.ReasoningEffort)
			}
		})
	}
}

func TestChatCompletionsToResponsesHandlerWrapsChatResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-chat",
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(bytes.NewReader(common.StringToByteSlice(`{
			"id":"chatcmpl_123",
			"object":"chat.completion",
			"created":123,
			"model":"deepseek-chat",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`))),
	}

	usage, err := chatCompletionsToResponsesHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 5, usage.CompletionTokens)
	require.Equal(t, http.StatusOK, recorder.Code)

	var responsesResp dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &responsesResp))
	require.Equal(t, "chatcmpl_123", responsesResp.ID)
	require.Equal(t, "response", responsesResp.Object)
	require.Len(t, responsesResp.Output, 1)
	require.Equal(t, "message", responsesResp.Output[0].Type)
	require.Equal(t, "hello", responsesResp.Output[0].Content[0].Text)
	require.Equal(t, 10, responsesResp.Usage.InputTokens)
	require.Equal(t, 5, responsesResp.Usage.OutputTokens)
}

func TestChatCompletionsToResponsesStreamHandlerEmitsToolCallEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		IsStream:    true,
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-chat",
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(bytes.NewReader(common.StringToByteSlice(
			"data: {\"id\":\"chatcmpl_123\",\"created\":123,\"model\":\"deepseek-chat\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"q\\\"\"}}]}}]}\n\n" +
				"data: {\"id\":\"chatcmpl_123\",\"created\":123,\"model\":\"deepseek-chat\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\":\\\"hi\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":3,\"total_tokens\":13}}\n\n" +
				"data: [DONE]\n\n"))),
	}

	usage, err := chatCompletionsToResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)
	body := recorder.Body.String()
	require.Contains(t, body, "event: response.output_item.added")
	require.Contains(t, body, `"type":"function_call"`)
	require.Contains(t, body, `"call_id":"call_1"`)
	require.Contains(t, body, `"name":"lookup"`)
	require.Contains(t, body, "event: response.function_call_arguments.delta")
	require.Contains(t, body, `\"q\"`)
	require.Contains(t, body, "event: response.function_call_arguments.done")
	require.Contains(t, body, `"arguments":"{\"q\":\"hi\"}"`)
	require.Contains(t, body, "event: response.output_item.done")
	require.Contains(t, body, "event: response.completed")
}

func TestChatCompletionsToResponsesStreamHandlerSeparatesReasoningEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		IsStream:    true,
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-reasoner",
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(bytes.NewReader(common.StringToByteSlice(
			"data: {\"id\":\"chatcmpl_reasoning\",\"created\":123,\"model\":\"deepseek-reasoner\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"think \"}}]}\n\n" +
				"data: {\"id\":\"chatcmpl_reasoning\",\"created\":123,\"model\":\"deepseek-reasoner\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"more\"}}]}\n\n" +
				"data: {\"id\":\"chatcmpl_reasoning\",\"created\":123,\"model\":\"deepseek-reasoner\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4,\"total_tokens\":14}}\n\n" +
				"data: [DONE]\n\n"))),
	}

	usage, err := chatCompletionsToResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	body := recorder.Body.String()
	require.Contains(t, body, "event: response.reasoning_summary_part.added")
	require.Contains(t, body, "event: response.reasoning_summary_text.delta")
	require.Contains(t, body, `"delta":"think "`)
	require.Contains(t, body, `"delta":"more"`)
	require.Contains(t, body, "event: response.reasoning_summary_text.done")
	require.Contains(t, body, `"text":"think more"`)
	require.Contains(t, body, "event: response.output_text.delta")
	require.Contains(t, body, `"delta":"answer"`)
	require.Contains(t, body, `"type":"response.output_text.done","text":"answer"`)
	require.NotContains(t, body, `"type":"response.output_text.delta","delta":"think "`)
	require.Contains(t, body, `"type":"reasoning"`)
	require.Contains(t, body, `"reasoning_content":"think more"`)
	require.Contains(t, body, `"text":"answer"`)
}

func TestChatCompletionsToResponsesStreamHandlerSplitsInlineThinkBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		IsStream:    true,
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-reasoner",
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(bytes.NewReader(common.StringToByteSlice(
			"data: {\"id\":\"chatcmpl_think\",\"created\":123,\"model\":\"deepseek-reasoner\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"<think>hidden\"}}]}\n\n" +
				"data: {\"id\":\"chatcmpl_think\",\"created\":123,\"model\":\"deepseek-reasoner\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"</think>\\nvisible\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4,\"total_tokens\":14}}\n\n" +
				"data: [DONE]\n\n"))),
	}

	_, err := chatCompletionsToResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	body := recorder.Body.String()
	require.Contains(t, body, "event: response.reasoning_summary_text.delta")
	require.Contains(t, body, `"delta":"hidden"`)
	require.Contains(t, body, "event: response.output_text.delta")
	require.Contains(t, body, `"delta":"visible"`)
	require.NotContains(t, body, `<think>`)
	require.NotContains(t, body, `</think>`)
}
