package claude

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
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaude55ResponsesReasoningConversion(t *testing.T) {
	settings := model_setting.GetClaudeSettings()
	original := *settings
	t.Cleanup(func() { *settings = original })
	settings.DefaultMaxTokens = map[string]int{"default": 8192}
	settings.ThinkingAdapterEnabled = true

	for _, tc := range []struct {
		name   string
		input  string
		model  string
		effort string
		limit  uint
	}{
		{"opus high with sampling", `{"model":"claude-opus-5-5","input":"hello","temperature":0.7,"top_p":0.9,"reasoning":{"effort":"high"}}`, "claude-opus-5-5", "high", 8192},
		{"opus xhigh", `{"model":"claude-opus-5-5","input":"hello","reasoning":{"effort":"xhigh"}}`, "claude-opus-5-5", "xhigh", 8192},
		{"opus max", `{"model":"claude-opus-5-5","input":"hello","reasoning":{"effort":"max"}}`, "claude-opus-5-5", "max", 8192},
		{"small output limit", `{"model":"claude-opus-5-5","input":"hello","max_output_tokens":1024,"reasoning":{"effort":"high"}}`, "claude-opus-5-5", "high", 1024},
		{"default effort", `{"model":"claude-opus-5-5","input":"hello","temperature":0,"top_p":0}`, "claude-opus-5-5", "", 8192},
		{"effort suffix", `{"model":"claude-opus-5-5-high","input":"hello"}`, "claude-opus-5-5", "high", 8192},
		{"explicit effort wins", `{"model":"claude-opus-5-5-high","input":"hello","reasoning":{"effort":"low"}}`, "claude-opus-5-5", "low", 8192},
		{"thinking suffix preserves limit", `{"model":"claude-opus-5-5-thinking","input":"hello","max_output_tokens":1024}`, "claude-opus-5-5", "high", 1024},
		{"sonnet medium", `{"model":"claude-sonnet-5-5","input":"hello","temperature":0.7,"reasoning":{"effort":"medium"}}`, "claude-sonnet-5-5", "medium", 8192},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, route := range []string{"anthropic adaptor", "direct converter"} {
				t.Run(route, func(t *testing.T) {
					var input dto.OpenAIResponsesRequest
					require.NoError(t, common.UnmarshalJsonStr(tc.input, &input))
					var converted any
					var err error
					if route == "anthropic adaptor" {
						converted, err = (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, &relaycommon.RelayInfo{}, input)
					} else {
						converted, err = relayconvert.OpenAIResponsesRequestToClaudeMessages(nil, &input)
					}
					require.NoError(t, err)
					request, ok := converted.(*dto.ClaudeRequest)
					require.True(t, ok)
					assert.Equal(t, tc.model, request.Model)
					require.NotNil(t, request.MaxTokens)
					assert.Equal(t, tc.limit, *request.MaxTokens)
					require.Len(t, request.Messages, 1)
					if request.Messages[0].IsStringContent() {
						assert.Equal(t, "hello", request.Messages[0].GetStringContent())
					} else {
						parts, err := request.Messages[0].ParseContent()
						require.NoError(t, err)
						require.Len(t, parts, 1)
						assert.Equal(t, "hello", parts[0].GetText())
					}
					body, err := common.Marshal(request)
					require.NoError(t, err)
					var payload map[string]any
					require.NoError(t, common.Unmarshal(body, &payload))
					assert.NotContains(t, payload, "temperature")
					assert.NotContains(t, payload, "top_p")
					assert.NotContains(t, payload, "top_k")
					assert.Equal(t, map[string]any{"type": "adaptive", "display": "summarized"}, payload["thinking"])
					if tc.effort == "" {
						assert.NotContains(t, payload, "output_config")
					} else {
						assert.Equal(t, map[string]any{"effort": tc.effort}, payload["output_config"])
					}
				})
			}
		})
	}
}

func TestClaudeResponsesRejectsUnsupportedClaude55Effort(t *testing.T) {
	for _, effort := range []string{"none", "minimal", "invalid"} {
		t.Run(effort, func(t *testing.T) {
			var request dto.OpenAIResponsesRequest
			require.NoError(t, common.UnmarshalJsonStr(`{"model":"claude-opus-5-5","input":"hello","reasoning":{"effort":"`+effort+`"}}`, &request))
			_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, &relaycommon.RelayInfo{}, request)
			require.ErrorContains(t, err, "unsupported reasoning effort")
			_, err = relayconvert.OpenAIResponsesRequestToClaudeMessages(nil, &request)
			require.ErrorContains(t, err, "unsupported reasoning effort")
		})
	}
}

func TestClaudeChatSamplingCompatibility(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-4-5", "custom-claude-model"} {
		t.Run(model, func(t *testing.T) {
			request, err := relayconvert.OpenAIChatRequestToClaudeMessages(nil, dto.GeneralOpenAIRequest{
				Model: model, Temperature: common.GetPointer(0.0), TopP: common.GetPointer(0.9), TopK: common.GetPointer(40),
				Messages: []dto.Message{{Role: "user", Content: "hello"}},
			})
			require.NoError(t, err)
			if model == "claude-opus-5-5" || model == "claude-sonnet-5-5" {
				assert.Nil(t, request.Temperature)
				assert.Nil(t, request.TopP)
				assert.Nil(t, request.TopK)
			} else {
				require.NotNil(t, request.Temperature)
				assert.Equal(t, 0.0, *request.Temperature)
				require.NotNil(t, request.TopP)
				assert.Equal(t, 0.9, *request.TopP)
				require.NotNil(t, request.TopK)
				assert.Equal(t, 40, *request.TopK)
				assert.Nil(t, request.Thinking)
			}
		})
	}
}

func TestConvertOpenAIResponsesRequestUsesClaudeMessages(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "claude-sonnet-4",
		},
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "claude-sonnet-4",
		Input: common.StringToByteSlice(`"hello"`),
	})

	require.NoError(t, err)
	claudeReq, ok := converted.(*dto.ClaudeRequest)
	require.True(t, ok)
	require.Equal(t, "claude-sonnet-4", claudeReq.Model)
	require.Len(t, claudeReq.Messages, 1)
	require.Equal(t, "user", claudeReq.Messages[0].Role)
	require.Equal(t, "hello", claudeReq.Messages[0].GetStringContent())
}

func TestClaudeResponsesHandlerWrapsMessageResponseAndKeepsAnthropicUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "claude-sonnet-4",
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(bytes.NewReader(common.StringToByteSlice(`{
			"id":"msg_123",
			"type":"message",
			"role":"assistant",
			"model":"claude-sonnet-4",
			"content":[{"type":"text","text":"hello"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":7,"output_tokens":2}
		}`))),
	}

	usage, err := ClaudeResponsesHandler(c, resp, info)

	require.Nil(t, err)
	require.Equal(t, "anthropic", usage.UsageSemantic)
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, 2, usage.CompletionTokens)

	var responsesResp dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &responsesResp))
	require.Equal(t, "msg_123", responsesResp.ID)
	require.Equal(t, "response", responsesResp.Object)
	require.Len(t, responsesResp.Output, 1)
	require.Equal(t, "hello", responsesResp.Output[0].Content[0].Text)
	require.Equal(t, 7, responsesResp.Usage.InputTokens)
	require.Equal(t, 2, responsesResp.Usage.OutputTokens)
}
