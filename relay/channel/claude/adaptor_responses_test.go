package claude

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
	claudeReq, ok := converted.(*kitdto.ClaudeRequest)
	require.True(t, ok)
	require.Equal(t, "claude-sonnet-4", claudeReq.Model)
	require.Len(t, claudeReq.Messages, 1)
	require.Equal(t, "user", claudeReq.Messages[0].Role)
	parts, err := claudeReq.Messages[0].ParseContent()
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, "hello", parts[0].GetText())
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

func TestOfficialClaudeResponsesReasoning(t *testing.T) {
	for _, tc := range []struct{ model, effort, want string }{
		{"claude-opus-5-5", "minimal", "low"},
		{"claude-opus-5-5", "xhigh", "xhigh"},
		{"claude-opus-4-8", "high", "high"},
		{"claude-sonnet-5-5-20260901", "high", "high"},
	} {
		t.Run(tc.model+tc.effort, func(t *testing.T) {
			var req dto.OpenAIResponsesRequest
			body, err := common.Marshal(map[string]any{"model": tc.model, "input": "hello", "max_output_tokens": 1024, "temperature": 0.7, "top_p": 0.9, "reasoning": map[string]string{"effort": tc.effort}})
			require.NoError(t, err)
			require.NoError(t, common.Unmarshal(body, &req))
			out, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
			require.NoError(t, err)
			result, ok := out.(*kitdto.ClaudeRequest)
			require.True(t, ok)
			assert.Nil(t, result.Temperature)
			assert.Nil(t, result.TopP)
			require.NotNil(t, result.Thinking)
			assert.Equal(t, "adaptive", result.Thinking.Type)
			assert.Empty(t, result.Thinking.Display)
			assert.Nil(t, result.Thinking.BudgetTokens)
			assert.Equal(t, uint(1024), *result.MaxTokens)
			assert.Equal(t, tc.want, result.GetEfforts())
		})
	}
}

func TestOfficialClaudeNativeAndMappedControls(t *testing.T) {
	input := &dto.ClaudeRequest{Model: "claude-opus-5-5", MaxTokens: common.GetPointer(uint(8192)), Temperature: common.GetPointer(0.7), Thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(2048)}, Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}}}
	out, err := (&Adaptor{}).ConvertClaudeRequest(nil, nil, input)
	require.NoError(t, err)
	got, err := common.Marshal(out)
	require.NoError(t, err)
	want, err := common.Marshal(input)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
	info := &relaycommon.RelayInfo{OriginModelName: "claude-opus-5-5", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "vendor-alias"}}
	out, err = (&Adaptor{}).ConvertOpenAIRequest(nil, info, &dto.GeneralOpenAIRequest{Model: "vendor-alias", ReasoningEffort: "high", Temperature: common.GetPointer(0.7), TopK: common.GetPointer(40), Messages: []dto.Message{{Role: "user", Content: "hello"}}})
	require.NoError(t, err)
	result, ok := out.(*kitdto.ClaudeRequest)
	require.True(t, ok)
	assert.Equal(t, "vendor-alias", result.Model)
	assert.Nil(t, result.Temperature)
	assert.Nil(t, result.TopK)
	require.NotNil(t, result.Thinking)
	assert.Equal(t, "adaptive", result.Thinking.Type)
}

func TestOfficialClaudeResponsesStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fail := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-opus-5-5"}}
		events := []string{
			`{"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"usage":{"input_tokens":7,"output_tokens":1}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
			`{"type":"content_block_stop","index":0}`,
		}
		if fail {
			events = append(events, `{"type":"error","error":{"type":"overloaded_error","message":"upstream overloaded"}}`)
		} else {
			events = append(events, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`, `{"type":"message_stop"}`)
		}
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + strings.Join(events, "\n\ndata: ") + "\n\n"))}
		usage, apiErr := ClaudeResponsesStreamHandler(c, resp, info)
		require.Nil(t, apiErr)
		require.NotNil(t, usage)
		assert.Equal(t, 7, usage.PromptTokens)
		assert.Contains(t, recorder.Body.String(), "response.output_text.delta")
		assert.Contains(t, recorder.Body.String(), "hello")
		if fail {
			assert.Contains(t, recorder.Body.String(), "response.failed")
			assert.NotContains(t, recorder.Body.String(), "response.completed")
		} else {
			assert.Equal(t, 2, usage.CompletionTokens)
			assert.Contains(t, recorder.Body.String(), "response.completed")
		}
	}
}

func TestOfficialClaudeResponsesToolRoundTrip(t *testing.T) {
	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.UnmarshalJsonStr(`{"model":"claude-opus-5-5","input":[{"role":"user","content":"weather?"},{"type":"function_call","call_id":"call_weather","name":"weather","arguments":"{\"city\":\"Shanghai\"}"},{"type":"function_call_output","call_id":"call_weather","output":"sunny"}],"tools":[{"type":"function","name":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]}`, &request))
	out, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, request)
	require.NoError(t, err)
	result, ok := out.(*kitdto.ClaudeRequest)
	require.True(t, ok)
	require.Len(t, result.Messages, 3)
	parts, err := result.Messages[1].ParseContent()
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "tool_use", parts[0].Type)
	assert.Equal(t, "call_weather", parts[0].Id)
	assert.Equal(t, map[string]any{"city": "Shanghai"}, parts[0].Input)
	parts, err = result.Messages[2].ParseContent()
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "tool_result", parts[0].Type)
	assert.Equal(t, "call_weather", parts[0].ToolUseId)
	assert.Equal(t, "sunny", parts[0].Content)
}
