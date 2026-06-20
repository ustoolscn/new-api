package gemini

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

func TestConvertOpenAIResponsesRequestUsesGeminiRequest(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://generativelanguage.googleapis.com",
			UpstreamModelName: "gemini-2.5-pro",
		},
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "gemini-2.5-pro",
		Input: common.StringToByteSlice(`"hello"`),
	})

	require.NoError(t, err)
	geminiReq, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, geminiReq.Contents, 1)
	require.Equal(t, "user", geminiReq.Contents[0].Role)
	require.Len(t, geminiReq.Contents[0].Parts, 1)
	require.Equal(t, "hello", geminiReq.Contents[0].Parts[0].Text)
}

func TestGeminiResponsesHandlerWrapsGeminiResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-2.5-pro",
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(bytes.NewReader(common.StringToByteSlice(`{
			"candidates":[{
				"index":0,
				"content":{"role":"model","parts":[{"text":"hello"}]},
				"finishReason":"STOP",
				"safetyRatings":[]
			}],
			"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2,"totalTokenCount":5}
		}`))),
	}

	usage, err := GeminiResponsesHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 3, usage.PromptTokens)
	require.Equal(t, 2, usage.CompletionTokens)

	var responsesResp dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &responsesResp))
	require.Equal(t, "response", responsesResp.Object)
	require.Equal(t, "gemini-2.5-pro", responsesResp.Model)
	require.Len(t, responsesResp.Output, 1)
	require.Equal(t, "hello", responsesResp.Output[0].Content[0].Text)
	require.Equal(t, 3, responsesResp.Usage.InputTokens)
	require.Equal(t, 2, responsesResp.Usage.OutputTokens)
}
