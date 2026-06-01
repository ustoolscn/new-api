package openaicompat

import (
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
	require.Equal(t, "tool", chatReq.Messages[2].Role)
	require.Equal(t, "call_1", chatReq.Messages[2].ToolCallId)
	require.Equal(t, "42", chatReq.Messages[2].Content)

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

func TestChatCompletionsResponseToResponsesResponseConvertsOutputAndUsage(t *testing.T) {
	toolCallsRaw := common.StringToByteSlice(`[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"hi\"}"}}]`)
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

	responsesResp, usage, err := ChatCompletionsResponseToResponsesResponse(chatResp)

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
	require.Equal(t, 10, responsesResp.Usage.InputTokens)
	require.Equal(t, 5, responsesResp.Usage.OutputTokens)
	require.Equal(t, 15, responsesResp.Usage.TotalTokens)
	require.Equal(t, 3, responsesResp.Usage.InputTokensDetails.CachedTokens)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 5, usage.CompletionTokens)
	require.Equal(t, 15, usage.TotalTokens)
	require.Equal(t, 3, usage.PromptTokensDetails.CachedTokens)
}
