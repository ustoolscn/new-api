package openaicompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}

	messages, err := responsesInputToChatMessages(req.Instructions, req.Input)
	if err != nil {
		return nil, err
	}

	tools, err := responsesToolsToChatTools(req.Tools)
	if err != nil {
		return nil, err
	}

	toolChoice, err := responsesToolChoiceToChatToolChoice(req.ToolChoice)
	if err != nil {
		return nil, err
	}

	responseFormat, err := responsesTextToChatResponseFormat(req.Text)
	if err != nil {
		return nil, err
	}

	out := &dto.GeneralOpenAIRequest{
		Model:          req.Model,
		Messages:       messages,
		Stream:         req.Stream,
		Temperature:    req.Temperature,
		TopP:           req.TopP,
		TopLogProbs:    req.TopLogProbs,
		Tools:          tools,
		ToolChoice:     toolChoice,
		ResponseFormat: responseFormat,
		User:           req.User,
		Store:          req.Store,
		Metadata:       req.Metadata,
	}

	if req.MaxOutputTokens != nil {
		out.MaxTokens = common.GetPointer(*req.MaxOutputTokens)
	}
	if req.StreamOptions != nil {
		out.StreamOptions = &dto.StreamOptions{
			IncludeUsage: req.StreamOptions.IncludeUsage,
		}
	}
	if req.Reasoning != nil {
		out.ReasoningEffort = req.Reasoning.Effort
	}
	if len(req.ParallelToolCalls) > 0 {
		var parallel bool
		if err := common.Unmarshal(req.ParallelToolCalls, &parallel); err == nil {
			out.ParallelTooCalls = &parallel
		}
	}
	if req.TopLogProbs != nil {
		out.LogProbs = common.GetPointer(true)
	}

	return out, nil
}

func responsesInputToChatMessages(instructions json.RawMessage, input json.RawMessage) ([]dto.Message, error) {
	messages := make([]dto.Message, 0)

	if len(instructions) > 0 {
		if content := rawMessageToChatString(instructions); strings.TrimSpace(content) != "" {
			messages = append(messages, dto.Message{Role: "system", Content: content})
		}
	}

	if len(input) == 0 {
		return messages, nil
	}

	switch common.GetJsonType(input) {
	case "null":
		return messages, nil
	case "string":
		var text string
		if err := common.Unmarshal(input, &text); err != nil {
			return nil, err
		}
		messages = append(messages, dto.Message{Role: "user", Content: text})
		return messages, nil
	case "array":
		var items []map[string]any
		if err := common.Unmarshal(input, &items); err != nil {
			return nil, err
		}
		for _, item := range items {
			converted, err := responsesInputItemToChatMessages(item)
			if err != nil {
				return nil, err
			}
			messages = append(messages, converted...)
		}
		return messages, nil
	default:
		return nil, fmt.Errorf("unsupported responses input type %q", common.GetJsonType(input))
	}
}

func responsesInputItemToChatMessages(item map[string]any) ([]dto.Message, error) {
	itemType := strings.TrimSpace(common.Interface2String(item["type"]))
	switch itemType {
	case "function_call_output":
		return []dto.Message{{
			Role:       "tool",
			ToolCallId: firstNonEmptyString(item["call_id"], item["id"]),
			Content:    anyToChatString(item["output"]),
		}}, nil
	case "function_call":
		msg := dto.Message{Role: "assistant", Content: ""}
		toolCall := dto.ToolCallRequest{
			ID:   firstNonEmptyString(item["call_id"], item["id"]),
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      common.Interface2String(item["name"]),
				Arguments: anyToChatString(item["arguments"]),
			},
		}
		toolCallsRaw, _ := common.Marshal([]dto.ToolCallRequest{toolCall})
		msg.ToolCalls = toolCallsRaw
		return []dto.Message{msg}, nil
	case "reasoning":
		return nil, nil
	}

	role := strings.TrimSpace(common.Interface2String(item["role"]))
	if role == "" && (itemType == "" || itemType == "message") {
		role = "user"
	}
	if role == "" {
		return nil, nil
	}

	content, err := responsesContentToChatContent(item["content"], role)
	if err != nil {
		return nil, err
	}
	return []dto.Message{{Role: role, Content: content}}, nil
}

func responsesContentToChatContent(content any, role string) (any, error) {
	if content == nil {
		return "", nil
	}
	if text, ok := content.(string); ok {
		return text, nil
	}

	parts, ok := content.([]any)
	if !ok {
		return anyToChatString(content), nil
	}

	chatParts := make([]map[string]any, 0, len(parts))
	var textOnly strings.Builder
	nonText := false

	for _, partAny := range parts {
		part, ok := partAny.(map[string]any)
		if !ok {
			continue
		}
		partType := strings.TrimSpace(common.Interface2String(part["type"]))
		switch partType {
		case "input_text", "output_text", "text":
			text := common.Interface2String(part["text"])
			textOnly.WriteString(text)
			chatParts = append(chatParts, map[string]any{
				"type": dto.ContentTypeText,
				"text": text,
			})
		case "input_image", "image_url":
			nonText = true
			imageURL := normalizedImageURLPart(part)
			chatParts = append(chatParts, map[string]any{
				"type":      dto.ContentTypeImageURL,
				"image_url": imageURL,
			})
		case "input_file", "file":
			nonText = true
			chatParts = append(chatParts, map[string]any{
				"type": dto.ContentTypeFile,
				"file": normalizedFilePart(part),
			})
		}
	}

	if !nonText {
		return textOnly.String(), nil
	}
	if role == "assistant" && textOnly.Len() > 0 {
		return textOnly.String(), nil
	}
	return chatParts, nil
}

func normalizedImageURLPart(part map[string]any) any {
	imageURL := part["image_url"]
	if imageURL == nil {
		imageURL = part["url"]
	}
	detail := common.Interface2String(part["detail"])
	if detail == "" {
		return imageURL
	}
	if url, ok := imageURL.(string); ok {
		return map[string]any{"url": url, "detail": detail}
	}
	if m, ok := imageURL.(map[string]any); ok {
		m["detail"] = detail
		return m
	}
	return imageURL
}

func normalizedFilePart(part map[string]any) any {
	if file := part["file"]; file != nil {
		return file
	}
	if fileID := common.Interface2String(part["file_id"]); fileID != "" {
		return map[string]any{"file_id": fileID}
	}
	if fileData := common.Interface2String(part["file_data"]); fileData != "" {
		return map[string]any{"file_data": fileData}
	}
	if fileURL := common.Interface2String(part["file_url"]); fileURL != "" {
		return map[string]any{"file_data": fileURL}
	}
	return part
}

func responsesToolsToChatTools(raw json.RawMessage) ([]dto.ToolCallRequest, error) {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return nil, nil
	}
	var tools []map[string]any
	if err := common.Unmarshal(raw, &tools); err != nil {
		return nil, err
	}
	out := make([]dto.ToolCallRequest, 0, len(tools))
	for _, tool := range tools {
		toolType := strings.TrimSpace(common.Interface2String(tool["type"]))
		if toolType != "function" {
			return nil, fmt.Errorf("unsupported responses tool type %q for chat completions compatibility", toolType)
		}
		name := common.Interface2String(tool["name"])
		description := common.Interface2String(tool["description"])
		parameters := tool["parameters"]
		if fn, ok := tool["function"].(map[string]any); ok {
			if name == "" {
				name = common.Interface2String(fn["name"])
			}
			if description == "" {
				description = common.Interface2String(fn["description"])
			}
			if parameters == nil {
				parameters = fn["parameters"]
			}
		}
		out = append(out, dto.ToolCallRequest{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        name,
				Description: description,
				Parameters:  parameters,
			},
		})
	}
	return out, nil
}

func responsesToolChoiceToChatToolChoice(raw json.RawMessage) (any, error) {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return nil, nil
	}
	if common.GetJsonType(raw) == "string" {
		var choice string
		if err := common.Unmarshal(raw, &choice); err != nil {
			return nil, err
		}
		return choice, nil
	}
	var choice map[string]any
	if err := common.Unmarshal(raw, &choice); err != nil {
		return nil, err
	}
	if common.Interface2String(choice["type"]) == "function" {
		name := common.Interface2String(choice["name"])
		if name == "" {
			if fn, ok := choice["function"].(map[string]any); ok {
				name = common.Interface2String(fn["name"])
			}
		}
		if name != "" {
			return map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": name,
				},
			}, nil
		}
	}
	return choice, nil
}

func responsesTextToChatResponseFormat(raw json.RawMessage) (*dto.ResponseFormat, error) {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return nil, nil
	}
	var textPayload map[string]json.RawMessage
	if err := common.Unmarshal(raw, &textPayload); err != nil {
		return nil, err
	}
	formatRaw := textPayload["format"]
	if len(formatRaw) == 0 {
		return nil, nil
	}
	var format map[string]any
	if err := common.Unmarshal(formatRaw, &format); err != nil {
		return nil, err
	}
	formatType := strings.TrimSpace(common.Interface2String(format["type"]))
	if formatType == "" {
		return nil, nil
	}
	responseFormat := &dto.ResponseFormat{Type: formatType}
	if formatType == "json_schema" {
		delete(format, "type")
		schemaRaw, _ := common.Marshal(format)
		responseFormat.JsonSchema = schemaRaw
	}
	return responseFormat, nil
}

func rawMessageToChatString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if common.GetJsonType(raw) == "null" {
		return ""
	}
	if common.GetJsonType(raw) == "string" {
		var s string
		if err := common.Unmarshal(raw, &s); err == nil {
			return s
		}
	}
	return string(raw)
}

func anyToChatString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := common.Marshal(v)
	if err != nil {
		return common.Interface2String(v)
	}
	return string(b)
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s := strings.TrimSpace(common.Interface2String(value)); s != "" {
			return s
		}
	}
	return ""
}

func ChatCompletionsResponseToResponsesResponse(resp *dto.OpenAITextResponse) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}

	usage := normalizeChatUsageForResponses(resp.Usage)
	statusRaw, _ := common.Marshal("completed")
	output := chatChoicesToResponsesOutput(resp.Choices, resp.Id)

	out := &dto.OpenAIResponsesResponse{
		ID:                resp.Id,
		Object:            "response",
		CreatedAt:         chatCreatedToInt(resp.Created),
		Status:            statusRaw,
		Model:             resp.Model,
		Output:            output,
		ParallelToolCalls: true,
		Usage:             responseUsageFromChatUsage(usage),
	}
	return out, usage, nil
}

func chatChoicesToResponsesOutput(choices []dto.OpenAITextResponseChoice, responseID string) []dto.ResponsesOutput {
	output := make([]dto.ResponsesOutput, 0, len(choices))
	for _, choice := range choices {
		content := strings.TrimSpace(choice.Message.StringContent())
		if content != "" {
			output = append(output, dto.ResponsesOutput{
				Type:   "message",
				ID:     fmt.Sprintf("msg_%s_%d", responseID, choice.Index),
				Status: "completed",
				Role:   "assistant",
				Content: []dto.ResponsesOutputContent{
					{
						Type:        "output_text",
						Text:        content,
						Annotations: []interface{}{},
					},
				},
			})
		}

		for _, toolCall := range choice.Message.ParseToolCalls() {
			if toolCall.Type != "" && toolCall.Type != "function" {
				continue
			}
			output = append(output, dto.ResponsesOutput{
				Type:      "function_call",
				ID:        firstNonEmptyString(toolCall.ID, fmt.Sprintf("fc_%s_%d", responseID, choice.Index)),
				Status:    "completed",
				CallId:    firstNonEmptyString(toolCall.ID, fmt.Sprintf("call_%s_%d", responseID, choice.Index)),
				Name:      toolCall.Function.Name,
				Arguments: chatFunctionArgumentsRaw(toolCall.Function.Arguments),
			})
		}
	}
	return output
}

func chatFunctionArgumentsRaw(arguments string) json.RawMessage {
	raw, _ := common.Marshal(arguments)
	return raw
}

func normalizeChatUsageForResponses(usage dto.Usage) *dto.Usage {
	out := usage
	if out.PromptTokens == 0 && out.InputTokens != 0 {
		out.PromptTokens = out.InputTokens
	}
	if out.CompletionTokens == 0 && out.OutputTokens != 0 {
		out.CompletionTokens = out.OutputTokens
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}
	if out.PromptTokensDetails.CachedTokens == 0 && out.PromptCacheHitTokens != 0 {
		out.PromptTokensDetails.CachedTokens = out.PromptCacheHitTokens
	}
	if out.InputTokensDetails != nil && out.PromptTokensDetails.CachedTokens == 0 {
		out.PromptTokensDetails.CachedTokens = out.InputTokensDetails.CachedTokens
	}
	return &out
}

func responseUsageFromChatUsage(usage *dto.Usage) *dto.Usage {
	if usage == nil {
		return nil
	}
	out := *usage
	out.InputTokens = usage.PromptTokens
	out.OutputTokens = usage.CompletionTokens
	inputDetails := usage.PromptTokensDetails
	out.InputTokensDetails = &inputDetails
	return &out
}

func chatCreatedToInt(created any) int {
	switch v := created.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case json.Number:
		i, _ := strconv.Atoi(v.String())
		return i
	case string:
		i, _ := strconv.Atoi(v)
		return i
	default:
		return 0
	}
}

func ResponsesResponseToChatCompletionsResponse(resp *dto.OpenAIResponsesResponse, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}

	text := ExtractOutputTextFromResponses(resp)

	usage := &dto.Usage{}
	if resp.Usage != nil {
		if resp.Usage.InputTokens != 0 {
			usage.PromptTokens = resp.Usage.InputTokens
			usage.InputTokens = resp.Usage.InputTokens
		}
		if resp.Usage.OutputTokens != 0 {
			usage.CompletionTokens = resp.Usage.OutputTokens
			usage.OutputTokens = resp.Usage.OutputTokens
		}
		if resp.Usage.TotalTokens != 0 {
			usage.TotalTokens = resp.Usage.TotalTokens
		} else {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		if resp.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = resp.Usage.InputTokensDetails.CachedTokens
			usage.PromptTokensDetails.ImageTokens = resp.Usage.InputTokensDetails.ImageTokens
			usage.PromptTokensDetails.AudioTokens = resp.Usage.InputTokensDetails.AudioTokens
		}
		if resp.Usage.CompletionTokenDetails.ReasoningTokens != 0 {
			usage.CompletionTokenDetails.ReasoningTokens = resp.Usage.CompletionTokenDetails.ReasoningTokens
		}
	}

	created := resp.CreatedAt

	var toolCalls []dto.ToolCallResponse
	if text == "" && len(resp.Output) > 0 {
		for _, out := range resp.Output {
			if out.Type != "function_call" {
				continue
			}
			name := strings.TrimSpace(out.Name)
			if name == "" {
				continue
			}
			callID := strings.TrimSpace(out.CallId)
			if callID == "" {
				callID = strings.TrimSpace(out.ID)
			}
			toolCalls = append(toolCalls, dto.ToolCallResponse{
				ID:   callID,
				Type: "function",
				Function: dto.FunctionResponse{
					Name:      name,
					Arguments: out.ArgumentsString(),
				},
			})
		}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	msg := dto.Message{
		Role:    "assistant",
		Content: text,
	}
	if len(toolCalls) > 0 {
		msg.SetToolCalls(toolCalls)
		msg.Content = ""
	}

	out := &dto.OpenAITextResponse{
		Id:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   resp.Model,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: *usage,
	}

	return out, usage, nil
}

func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil || len(resp.Output) == 0 {
		return ""
	}

	var sb strings.Builder

	for _, out := range resp.Output {
		if out.Type != "message" {
			continue
		}
		if out.Role != "" && out.Role != "assistant" {
			continue
		}
		for _, c := range out.Content {
			if c.Type == "output_text" && c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	if sb.Len() > 0 {
		return sb.String()
	}
	for _, out := range resp.Output {
		for _, c := range out.Content {
			if c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	return sb.String()
}
