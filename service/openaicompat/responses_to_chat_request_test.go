package openaicompat

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestResponsesRequestToChatCompletionsRequestConvertsCoreFields(t *testing.T) {
	stream := true
	maxOutputTokens := uint(0)
	temperature := 0.7
	topP := 0.9
	input := common.StringToByteSlice(`[
		{"role":"user","content":[
			{"type":"input_text","text":"hi"},
			{"type":"input_image","image_url":"data:image/png;base64,abc","detail":"low"}
		]},
		{"type":"function_call_output","call_id":"call_1","output":"42"}
	]`)
	instructions := common.StringToByteSlice(`"be concise"`)
	tools := common.StringToByteSlice(`[{"type":"function","name":"lookup","description":"Lookup things","parameters":{"type":"object"}}]`)
	toolChoice := common.StringToByteSlice(`{"type":"function","name":"lookup"}`)
	text := common.StringToByteSlice(`{"format":{"type":"json_object"}}`)

	chatReq, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model:           "deepseek-chat",
		Input:           input,
		Instructions:    instructions,
		MaxOutputTokens: &maxOutputTokens,
		Stream:          &stream,
		StreamOptions:   &dto.StreamOptions{IncludeUsage: true, IncludeObfuscation: true},
		Temperature:     &temperature,
		TopP:            &topP,
		Tools:           tools,
		ToolChoice:      toolChoice,
		Text:            text,
		Reasoning:       &dto.Reasoning{Effort: "low"},
	})

	require.NoError(t, err)
	require.Equal(t, "deepseek-chat", chatReq.Model)
	require.True(t, *chatReq.Stream)
	require.True(t, chatReq.StreamOptions.IncludeUsage)
	require.False(t, chatReq.StreamOptions.IncludeObfuscation)
	require.NotNil(t, chatReq.MaxTokens)
	require.Equal(t, uint(0), *chatReq.MaxTokens)
	require.Equal(t, "low", chatReq.ReasoningEffort)
	require.Equal(t, temperature, *chatReq.Temperature)
	require.Equal(t, topP, *chatReq.TopP)
	require.Equal(t, "json_object", chatReq.ResponseFormat.Type)

	require.Len(t, chatReq.Messages, 3)
	require.Equal(t, "system", chatReq.Messages[0].Role)
	require.Equal(t, "be concise", chatReq.Messages[0].Content)
	require.Equal(t, "user", chatReq.Messages[1].Role)
	require.IsType(t, []map[string]any{}, chatReq.Messages[1].Content)
	require.Equal(t, "user", chatReq.Messages[2].Role)
	require.Contains(t, chatReq.Messages[2].Content, "Function call output (call_1): 42")

	require.Len(t, chatReq.Tools, 1)
	require.Equal(t, "function", chatReq.Tools[0].Type)
	require.Equal(t, "lookup", chatReq.Tools[0].Function.Name)
	require.Equal(t, "Lookup things", chatReq.Tools[0].Function.Description)

	choice, ok := chatReq.ToolChoice.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function", choice["type"])
	function, ok := choice["function"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "lookup", function["name"])
}

func TestResponsesRequestToChatCompletionsRequestFlattensNamespaceTools(t *testing.T) {
	tools := common.StringToByteSlice(`[{
		"type":"namespace",
		"name":"mcp__idea__",
		"description":"IDE tools",
		"tools":[{
			"type":"function",
			"name":"read_file",
			"description":"Read a file",
			"parameters":{"type":"object","properties":{"path":{"type":"string"}}}
		}]
	}]`)

	chatReq, toolMap, err := ResponsesRequestToChatCompletionsRequestWithToolMap(&dto.OpenAIResponsesRequest{
		Model: "deepseek-chat",
		Input: common.StringToByteSlice(`"hello"`),
		Tools: tools,
	})

	require.NoError(t, err)
	require.Len(t, chatReq.Tools, 1)
	require.Equal(t, "function", chatReq.Tools[0].Type)
	require.Equal(t, "mcp__idea__read_file", chatReq.Tools[0].Function.Name)
	require.Contains(t, chatReq.Tools[0].Function.Description, "IDE tools")
	require.Contains(t, chatReq.Tools[0].Function.Description, "Read a file")
	require.Equal(t, ResponsesToolName{
		Namespace: "mcp__idea__",
		Name:      "read_file",
	}, toolMap["mcp__idea__read_file"])
}

func TestResponsesRequestToChatCompletionsRequestKeepsToolOutputContextValid(t *testing.T) {
	input := common.StringToByteSlice(`[
		{"type":"function_call","call_id":"call_a","name":"lookup","arguments":{"q":"a"}},
		{"type":"function_call","call_id":"call_b","name":"lookup","arguments":"{\"q\":\"b\"}"},
		{"type":"function_call_output","call_id":"call_a","output":{"ok":true}},
		{"type":"function_call_output","call_id":"missing","output":"late"}
	]`)

	chatReq, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "deepseek-chat",
		Input: input,
	})

	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 3)
	require.Equal(t, "assistant", chatReq.Messages[0].Role)

	var toolCalls []dto.ToolCallRequest
	require.NoError(t, common.Unmarshal(chatReq.Messages[0].ToolCalls, &toolCalls))
	require.Len(t, toolCalls, 2)
	require.Equal(t, "call_a", toolCalls[0].ID)
	require.Equal(t, "lookup", toolCalls[0].Function.Name)
	require.JSONEq(t, `{"q":"a"}`, toolCalls[0].Function.Arguments)
	require.Equal(t, "call_b", toolCalls[1].ID)
	require.JSONEq(t, `{"q":"b"}`, toolCalls[1].Function.Arguments)

	require.Equal(t, "tool", chatReq.Messages[1].Role)
	require.Equal(t, "call_a", chatReq.Messages[1].ToolCallId)
	require.JSONEq(t, `{"ok":true}`, chatReq.Messages[1].Content.(string))

	require.Equal(t, "user", chatReq.Messages[2].Role)
	require.Contains(t, chatReq.Messages[2].Content, "Function call output (missing): late")
}

func TestResponsesRequestToChatCompletionsRequestMapsResponsesRolesToChatRoles(t *testing.T) {
	input := common.StringToByteSlice(`[
		{"role":"developer","content":"developer note"},
		{"role":"latest_reminder","content":"remember this"},
		{"role":"unknown_role","content":"fallback"}
	]`)

	chatReq, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "deepseek-chat",
		Input: input,
	})

	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 3)
	require.Equal(t, "system", chatReq.Messages[0].Role)
	require.Equal(t, "developer note", chatReq.Messages[0].Content)
	require.Equal(t, "user", chatReq.Messages[1].Role)
	require.Equal(t, "remember this", chatReq.Messages[1].Content)
	require.Equal(t, "user", chatReq.Messages[2].Role)
	require.Equal(t, "fallback", chatReq.Messages[2].Content)
}

func TestResponsesRequestToChatCompletionsRequestNormalizesInstructionsAndSystemMessages(t *testing.T) {
	input := common.StringToByteSlice(`[
		{"role":"developer","content":"developer note"},
		{"role":"system","content":"system note"},
		{"role":"user","content":[
			{"type":"input_text","text":"hello"},
			{"type":"refusal","refusal":"not allowed"},
			{"type":"output_text","text":"again"}
		]}
	]`)

	chatReq, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model:        "deepseek-chat",
		Instructions: common.StringToByteSlice(`[{"text":"first instruction"},"second instruction"]`),
		Input:        input,
	})

	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 2)
	require.Equal(t, "system", chatReq.Messages[0].Role)
	require.Equal(t, "first instruction\n\nsecond instruction\n\ndeveloper note\n\nsystem note", chatReq.Messages[0].Content)
	require.Equal(t, "user", chatReq.Messages[1].Role)
	require.Equal(t, "hello\nnot allowed\nagain", chatReq.Messages[1].Content)
}

func TestResponsesRequestToChatCompletionsRequestPassesChatCompatFields(t *testing.T) {
	stream := true
	maxOutputTokens := uint(99)
	maxTokens := uint(0)
	maxCompletionTokens := uint(12)
	frequencyPenalty := 0.0
	presencePenalty := 0.0
	n := 0
	seed := 0.0
	logprobs := false

	chatReq, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model:               "deepseek-chat",
		Input:               common.StringToByteSlice(`"hello"`),
		Stream:              &stream,
		StreamOptions:       &dto.StreamOptions{},
		MaxOutputTokens:     &maxOutputTokens,
		MaxTokens:           &maxTokens,
		MaxCompletionTokens: &maxCompletionTokens,
		FrequencyPenalty:    &frequencyPenalty,
		PresencePenalty:     &presencePenalty,
		Stop:                common.StringToByteSlice(`["END"]`),
		N:                   &n,
		Seed:                &seed,
		LogProbs:            &logprobs,
		LogitBias:           common.StringToByteSlice(`{"42":1}`),
		ResponseFormat:      &dto.ResponseFormat{Type: "json_object"},
		ServiceTier:         common.StringToByteSlice(`"default"`),
	})

	require.NoError(t, err)
	require.NotNil(t, chatReq.StreamOptions)
	require.True(t, chatReq.StreamOptions.IncludeUsage)
	require.NotNil(t, chatReq.MaxTokens)
	require.Equal(t, uint(0), *chatReq.MaxTokens)
	require.NotNil(t, chatReq.MaxCompletionTokens)
	require.Equal(t, uint(12), *chatReq.MaxCompletionTokens)
	require.NotNil(t, chatReq.FrequencyPenalty)
	require.Equal(t, 0.0, *chatReq.FrequencyPenalty)
	require.NotNil(t, chatReq.PresencePenalty)
	require.Equal(t, 0.0, *chatReq.PresencePenalty)
	require.JSONEq(t, `["END"]`, string(chatReq.Stop.(json.RawMessage)))
	require.NotNil(t, chatReq.N)
	require.Equal(t, 0, *chatReq.N)
	require.NotNil(t, chatReq.Seed)
	require.Equal(t, 0.0, *chatReq.Seed)
	require.NotNil(t, chatReq.LogProbs)
	require.False(t, *chatReq.LogProbs)
	require.JSONEq(t, `{"42":1}`, string(chatReq.LogitBias))
	require.Equal(t, "json_object", chatReq.ResponseFormat.Type)
	require.JSONEq(t, `"default"`, string(chatReq.ServiceTier))
}

func TestResponsesRequestToChatCompletionsRequestNormalizesToolParameters(t *testing.T) {
	chatReq, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "deepseek-chat",
		Input: common.StringToByteSlice(`"hello"`),
		Tools: common.StringToByteSlice(`[{
			"type":"function",
			"name":"lookup"
		},{
			"type":"namespace",
			"name":"mcp__idea__",
			"tools":[{"type":"function","name":"read_file","parameters":"bad schema"}]
		}]`),
	})

	require.NoError(t, err)
	require.Len(t, chatReq.Tools, 2)
	for _, tool := range chatReq.Tools {
		params, ok := tool.Function.Parameters.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "object", params["type"])
		require.IsType(t, map[string]any{}, params["properties"])
		require.IsType(t, []any{}, params["required"])
	}
}

func TestResponsesRequestToChatCompletionsRequestIgnoresUnsupportedToolShapes(t *testing.T) {
	chatReq, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "deepseek-chat",
		Input: common.StringToByteSlice(`"hello"`),
		Tools: common.StringToByteSlice(`[
			"apply_patch",
			{"type":"custom","name":"freeform"},
			{"type":"web_search","name":"web_search"},
			{"type":"function","name":"lookup","parameters":{"type":"object"}}
		]`),
	})

	require.NoError(t, err)
	require.Len(t, chatReq.Tools, 1)
	require.Equal(t, "lookup", chatReq.Tools[0].Function.Name)
}

func TestResponsesRequestToChatCompletionsRequestOmitsToolChoiceWithoutTools(t *testing.T) {
	chatReq, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model:      "deepseek-chat",
		Input:      common.StringToByteSlice(`"hello"`),
		ToolChoice: common.StringToByteSlice(`{"type":"function","name":"lookup"}`),
	})

	require.NoError(t, err)
	require.Nil(t, chatReq.ToolChoice)
}

func TestResponsesRequestToChatCompletionsRequestFlattensNestedNamespaceToolChoice(t *testing.T) {
	chatReq, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "deepseek-chat",
		Input: common.StringToByteSlice(`"hello"`),
		Tools: common.StringToByteSlice(`[{
			"type":"namespace",
			"name":"mcp__idea__",
			"tools":[{"type":"function","name":"read_file","parameters":{"type":"object"}}]
		}]`),
		ToolChoice: common.StringToByteSlice(`{
			"type":"function",
			"function":{"namespace":"mcp__idea__","name":"read_file"}
		}`),
	})

	require.NoError(t, err)
	choice, ok := chatReq.ToolChoice.(map[string]any)
	require.True(t, ok)
	function, ok := choice["function"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "mcp__idea__read_file", function["name"])
}

func TestResponsesRequestToChatCompletionsRequestSkipsNamespaceToolNameCollision(t *testing.T) {
	chatReq, toolMap, err := ResponsesRequestToChatCompletionsRequestWithToolMap(&dto.OpenAIResponsesRequest{
		Model: "deepseek-chat",
		Input: common.StringToByteSlice(`"hello"`),
		Tools: common.StringToByteSlice(`[
			{"type":"function","name":"mcp__idea__read_file","parameters":{"type":"object"}},
			{"type":"namespace","name":"mcp__idea__","tools":[
				{"type":"function","name":"read_file","parameters":{"type":"object"}}
			]}
		]`),
	})

	require.NoError(t, err)
	require.Len(t, chatReq.Tools, 1)
	require.Equal(t, "mcp__idea__read_file", chatReq.Tools[0].Function.Name)
	require.NotContains(t, toolMap, "mcp__idea__read_file")
}

func TestChatCompletionsResponseToResponsesResponseConvertsOutputAndUsage(t *testing.T) {
	toolCallsRaw := common.StringToByteSlice(`[{"id":"call_1","type":"function","function":{"name":"mcp__idea__lookup","arguments":"{\"q\":\"hi\"}"}}]`)
	chatResp := &dto.OpenAITextResponse{
		Id:      "chatcmpl_123",
		Object:  "chat.completion",
		Created: int64(123),
		Model:   "deepseek-chat",
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:      "assistant",
					Content:   "hello",
					ToolCalls: toolCallsRaw,
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: dto.Usage{
			PromptTokens:         10,
			CompletionTokens:     5,
			TotalTokens:          15,
			PromptCacheHitTokens: 3,
		},
	}

	responsesResp, usage, err := ChatCompletionsResponseToResponsesResponseWithToolMap(chatResp, map[string]ResponsesToolName{
		"mcp__idea__lookup": {
			Namespace: "mcp__idea__",
			Name:      "lookup",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "chatcmpl_123", responsesResp.ID)
	require.Equal(t, "response", responsesResp.Object)
	require.Equal(t, "deepseek-chat", responsesResp.Model)
	require.JSONEq(t, `"completed"`, string(responsesResp.Status))
	require.Len(t, responsesResp.Output, 2)
	require.Equal(t, "message", responsesResp.Output[0].Type)
	require.Equal(t, "assistant", responsesResp.Output[0].Role)
	require.Equal(t, "output_text", responsesResp.Output[0].Content[0].Type)
	require.Equal(t, "hello", responsesResp.Output[0].Content[0].Text)
	require.Equal(t, "function_call", responsesResp.Output[1].Type)
	require.Equal(t, "call_1", responsesResp.Output[1].CallId)
	require.Equal(t, "lookup", responsesResp.Output[1].Name)
	require.Equal(t, "mcp__idea__", responsesResp.Output[1].Namespace)
	require.Equal(t, 10, responsesResp.Usage.InputTokens)
	require.Equal(t, 5, responsesResp.Usage.OutputTokens)
	require.Equal(t, 15, responsesResp.Usage.TotalTokens)
	require.Equal(t, 3, responsesResp.Usage.InputTokensDetails.CachedTokens)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 5, usage.CompletionTokens)
	require.Equal(t, 15, usage.TotalTokens)
	require.Equal(t, 3, usage.PromptTokensDetails.CachedTokens)
}
