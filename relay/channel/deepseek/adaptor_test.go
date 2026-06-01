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

func TestConvertOpenAIResponsesRequestUsesChatCompletionsCompat(t *testing.T) {
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
	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "deepseek-chat", chatReq.Model)
	require.Len(t, chatReq.Messages, 1)
	require.Equal(t, "user", chatReq.Messages[0].Role)
	require.Equal(t, "hello", chatReq.Messages[0].Content)

	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://api.deepseek.com/v1/chat/completions", url)
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
