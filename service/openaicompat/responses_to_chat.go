package openaicompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

type ResponsesToolName struct {
	Namespace string
	Name      string
}

func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	out, _, err := ResponsesRequestToChatCompletionsRequestWithToolMap(req)
	return out, err
}

func ResponsesRequestToChatCompletionsRequestWithToolMap(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, map[string]ResponsesToolName, error) {
	if req == nil {
		return nil, nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, nil, errors.New("model is required")
	}

	messages, err := responsesInputToChatMessages(req.Instructions, req.Input)
	if err != nil {
		return nil, nil, err
	}

	tools, toolNameMap, err := responsesToolsToChatTools(req.Tools)
	if err != nil {
		return nil, nil, err
	}

	toolChoice, err := responsesToolChoiceToChatToolChoice(req.ToolChoice)
	if err != nil {
		return nil, nil, err
	}

	responseFormat, err := responsesTextToChatResponseFormat(req.Text)
	if err != nil {
		return nil, nil, err
	}

	out := &dto.GeneralOpenAIRequest{
		Model:            req.Model,
		Messages:         normalizeResponsesChatMessages(messages),
		Stream:           req.Stream,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		FrequencyPenalty: req.FrequencyPenalty,
		PresencePenalty:  req.PresencePenalty,
		Stop:             req.Stop,
		N:                req.N,
		Seed:             req.Seed,
		TopLogProbs:      req.TopLogProbs,
		Tools:            tools,
		ResponseFormat:   responseFormat,
		User:             req.User,
		Store:            req.Store,
		ServiceTier:      req.ServiceTier,
		LogitBias:        req.LogitBias,
		Metadata:         req.Metadata,
	}

	if req.MaxOutputTokens != nil {
		out.MaxTokens = common.GetPointer(*req.MaxOutputTokens)
	}
	if req.MaxTokens != nil {
		out.MaxTokens = common.GetPointer(*req.MaxTokens)
	}
	if req.MaxCompletionTokens != nil {
		out.MaxCompletionTokens = common.GetPointer(*req.MaxCompletionTokens)
	}
	if out.ResponseFormat == nil {
		out.ResponseFormat = req.ResponseFormat
	}
	if req.StreamOptions != nil {
		out.StreamOptions = &dto.StreamOptions{
			IncludeUsage: req.StreamOptions.IncludeUsage,
		}
	}
	if req.Stream != nil && *req.Stream {
		if out.StreamOptions == nil {
			out.StreamOptions = &dto.StreamOptions{}
		}
		out.StreamOptions.IncludeUsage = true
	}
	if req.Reasoning != nil {
		out.ReasoningEffort = req.Reasoning.Effort
	}
	if len(tools) > 0 {
		out.ToolChoice = toolChoice
	}
	if len(tools) > 0 && len(req.ParallelToolCalls) > 0 {
		var parallel bool
		if err := common.Unmarshal(req.ParallelToolCalls, &parallel); err == nil {
			out.ParallelTooCalls = &parallel
		}
	}
	if req.LogProbs != nil {
		out.LogProbs = req.LogProbs
	} else if req.TopLogProbs != nil {
		out.LogProbs = common.GetPointer(true)
	}

	return out, toolNameMap, nil
}

func normalizeResponsesChatMessages(messages []dto.Message) []dto.Message {
	systemChunks := make([]string, 0)
	rest := make([]dto.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == "system" {
			if text := strings.TrimSpace(message.StringContent()); text != "" {
				systemChunks = append(systemChunks, text)
			}
			continue
		}
		if message.Role == "assistant" && message.Content == nil && len(message.ToolCalls) == 0 {
			message.Content = ""
		}
		rest = append(rest, message)
	}
	if len(systemChunks) == 0 {
		return rest
	}
	out := make([]dto.Message, 0, len(rest)+1)
	out = append(out, dto.Message{
		Role:    "system",
		Content: strings.Join(systemChunks, "\n\n"),
	})
	out = append(out, rest...)
	return out
}

func responsesInputToChatMessages(instructions json.RawMessage, input json.RawMessage) ([]dto.Message, error) {
	messages := make([]dto.Message, 0)

	if len(instructions) > 0 {
		if content := rawInstructionsToChatString(instructions); strings.TrimSpace(content) != "" {
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
		converted, err := responsesInputItemsToChatMessages(items)
		if err != nil {
			return nil, err
		}
		messages = append(messages, converted...)
		return messages, nil
	case "object":
		var item map[string]any
		if err := common.Unmarshal(input, &item); err != nil {
			return nil, err
		}
		converted, err := responsesInputItemsToChatMessages([]map[string]any{item})
		if err != nil {
			return nil, err
		}
		messages = append(messages, converted...)
		return messages, nil
	default:
		return nil, fmt.Errorf("unsupported responses input type %q", common.GetJsonType(input))
	}
}

func responsesInputItemsToChatMessages(items []map[string]any) ([]dto.Message, error) {
	messages := make([]dto.Message, 0, len(items))
	pendingToolCalls := make([]dto.ToolCallRequest, 0)
	seenToolCallIDs := make(map[string]struct{})

	flushToolCalls := func() error {
		if len(pendingToolCalls) == 0 {
			return nil
		}
		if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
			if err := appendToolCallsToAssistantMessage(&messages[len(messages)-1], pendingToolCalls); err != nil {
				return err
			}
		} else {
			msg := dto.Message{Role: "assistant", Content: ""}
			toolCallsRaw, err := common.Marshal(pendingToolCalls)
			if err != nil {
				return err
			}
			msg.ToolCalls = toolCallsRaw
			messages = append(messages, msg)
		}
		pendingToolCalls = pendingToolCalls[:0]
		return nil
	}

	for _, item := range items {
		itemType := strings.TrimSpace(common.Interface2String(item["type"]))
		switch itemType {
		case "function_call":
			toolCall, ok := responsesFunctionCallItemToChatToolCall(item)
			if !ok {
				continue
			}
			seenToolCallIDs[toolCall.ID] = struct{}{}
			pendingToolCalls = append(pendingToolCalls, toolCall)
		case "function_call_output":
			callID := firstNonEmptyString(item["call_id"], item["id"])
			if callID == "" {
				continue
			}
			if _, ok := seenToolCallIDs[callID]; !ok {
				if err := flushToolCalls(); err != nil {
					return nil, err
				}
				messages = append(messages, orphanToolOutputMessage(callID, item["output"]))
				continue
			}
			if err := flushToolCalls(); err != nil {
				return nil, err
			}
			messages = append(messages, dto.Message{
				Role:       "tool",
				ToolCallId: callID,
				Content:    anyToChatString(item["output"]),
			})
		case "reasoning":
			continue
		default:
			if err := flushToolCalls(); err != nil {
				return nil, err
			}
			converted, err := responsesInputItemToChatMessages(item)
			if err != nil {
				return nil, err
			}
			messages = append(messages, converted...)
		}
	}
	if err := flushToolCalls(); err != nil {
		return nil, err
	}
	return messages, nil
}

func appendToolCallsToAssistantMessage(message *dto.Message, incoming []dto.ToolCallRequest) error {
	if message == nil || len(incoming) == 0 {
		return nil
	}
	existing := message.ParseToolCalls()
	existing = append(existing, incoming...)
	toolCallsRaw, err := common.Marshal(existing)
	if err != nil {
		return err
	}
	message.ToolCalls = toolCallsRaw
	if message.Content == nil {
		message.Content = ""
	}
	return nil
}

func responsesFunctionCallItemToChatToolCall(item map[string]any) (dto.ToolCallRequest, bool) {
	name := common.Interface2String(item["name"])
	if namespace := common.Interface2String(item["namespace"]); namespace != "" {
		name = flattenResponsesToolName(namespace, name)
	}
	callID := firstNonEmptyString(item["call_id"], item["id"])
	if name == "" || callID == "" {
		return dto.ToolCallRequest{}, false
	}
	return dto.ToolCallRequest{
		ID:   callID,
		Type: "function",
		Function: dto.FunctionRequest{
			Name:      name,
			Arguments: anyToChatString(item["arguments"]),
		},
	}, true
}

func orphanToolOutputMessage(callID string, output any) dto.Message {
	return dto.Message{
		Role:    "user",
		Content: fmt.Sprintf("Function call output (%s): %s", callID, anyToChatString(output)),
	}
}

func responsesInputItemToChatMessages(item map[string]any) ([]dto.Message, error) {
	itemType := strings.TrimSpace(common.Interface2String(item["type"]))
	switch itemType {
	case "function_call_output":
		callID := firstNonEmptyString(item["call_id"], item["id"])
		if callID == "" {
			return nil, nil
		}
		return []dto.Message{orphanToolOutputMessage(callID, item["output"])}, nil
	case "function_call":
		toolCall, ok := responsesFunctionCallItemToChatToolCall(item)
		if !ok {
			return nil, nil
		}
		toolCallsRaw, _ := common.Marshal([]dto.ToolCallRequest{toolCall})
		msg := dto.Message{Role: "assistant", Content: ""}
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
	role = responsesRoleToChatRole(role)

	content, err := responsesContentToChatContent(item["content"], role)
	if err != nil {
		return nil, err
	}
	return []dto.Message{{Role: role, Content: content}}, nil
}

func responsesRoleToChatRole(role string) string {
	switch strings.TrimSpace(role) {
	case "developer", "system":
		return "system"
	case "assistant":
		return "assistant"
	case "tool":
		return "tool"
	case "latest_reminder", "user", "":
		return "user"
	default:
		return "user"
	}
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
	appendTextOnly := func(text string) {
		if text == "" {
			return
		}
		if textOnly.Len() > 0 {
			textOnly.WriteString("\n")
		}
		textOnly.WriteString(text)
	}

	for _, partAny := range parts {
		part, ok := partAny.(map[string]any)
		if !ok {
			continue
		}
		partType := strings.TrimSpace(common.Interface2String(part["type"]))
		switch partType {
		case "input_text", "output_text", "text":
			text := common.Interface2String(part["text"])
			appendTextOnly(text)
			chatParts = append(chatParts, map[string]any{
				"type": dto.ContentTypeText,
				"text": text,
			})
		case "refusal":
			text := common.Interface2String(part["refusal"])
			appendTextOnly(text)
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

func responsesToolsToChatTools(raw json.RawMessage) ([]dto.ToolCallRequest, map[string]ResponsesToolName, error) {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return nil, nil, nil
	}
	tools, err := parseResponsesToolObjects(raw)
	if err != nil {
		return nil, nil, err
	}
	out := make([]dto.ToolCallRequest, 0, len(tools))
	toolContext := buildResponsesToolContext(tools)
	toolNameMap := make(map[string]ResponsesToolName)
	for _, tool := range tools {
		toolType := strings.TrimSpace(common.Interface2String(tool["type"]))
		switch toolType {
		case "function":
			chatTool, ok := responsesFunctionToolToChatTool(tool, "", "")
			if ok {
				out = append(out, chatTool)
			}
		case "namespace":
			namespace := firstNonEmptyString(tool["name"], tool["namespace"])
			namespaceDescription := common.Interface2String(tool["description"])
			namespaceTools := normalizeNamespaceTools(tool["tools"])
			for _, namespaceTool := range namespaceTools {
				name := firstNonEmptyString(namespaceTool["name"], functionMapName(namespaceTool))
				flatName := flattenResponsesToolName(namespace, name)
				if namespace != "" {
					if nameInfo, ok := toolContext[flatName]; ok && nameInfo.Namespace == "" {
						continue
					}
				}
				chatTool, ok := responsesFunctionToolToChatTool(namespaceTool, namespace, namespaceDescription)
				if !ok {
					continue
				}
				out = append(out, chatTool)
				if nameInfo, ok := toolContext[chatTool.Function.Name]; ok && nameInfo.Namespace != "" {
					toolNameMap[chatTool.Function.Name] = nameInfo
				}
			}
		default:
			continue
		}
	}
	return out, toolNameMap, nil
}

func parseResponsesToolObjects(raw json.RawMessage) ([]map[string]any, error) {
	var items []any
	if err := common.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	tools := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if tool, ok := item.(map[string]any); ok {
			tools = append(tools, tool)
		}
	}
	return tools, nil
}

func buildResponsesToolContext(tools []map[string]any) map[string]ResponsesToolName {
	context := make(map[string]ResponsesToolName)
	for _, tool := range tools {
		switch strings.TrimSpace(common.Interface2String(tool["type"])) {
		case "function":
			name := firstNonEmptyString(tool["name"], functionMapName(tool))
			if name != "" {
				context[name] = ResponsesToolName{Name: name}
			}
		case "namespace":
			namespace := firstNonEmptyString(tool["name"], tool["namespace"])
			for _, namespaceTool := range normalizeNamespaceTools(tool["tools"]) {
				toolType := strings.TrimSpace(common.Interface2String(namespaceTool["type"]))
				if toolType != "" && toolType != "function" {
					continue
				}
				name := firstNonEmptyString(namespaceTool["name"], functionMapName(namespaceTool))
				if name == "" {
					continue
				}
				flatName := flattenResponsesToolName(namespace, name)
				if namespace == "" {
					context[flatName] = ResponsesToolName{Name: name}
					continue
				}
				if existing, ok := context[flatName]; !ok || existing.Namespace != "" {
					context[flatName] = ResponsesToolName{
						Namespace: namespace,
						Name:      name,
					}
				}
			}
		}
	}
	return context
}

func normalizeNamespaceTools(raw any) []map[string]any {
	switch tools := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(tools))
		for _, item := range tools {
			if tool, ok := item.(map[string]any); ok {
				out = append(out, tool)
			}
		}
		return out
	case map[string]any:
		out := make([]map[string]any, 0, len(tools))
		for name, item := range tools {
			tool, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if common.Interface2String(tool["name"]) == "" {
				tool["name"] = name
			}
			out = append(out, tool)
		}
		return out
	default:
		return nil
	}
}

func responsesFunctionToolToChatTool(tool map[string]any, namespace string, namespaceDescription string) (dto.ToolCallRequest, bool) {
	toolType := strings.TrimSpace(common.Interface2String(tool["type"]))
	if toolType != "" && toolType != "function" {
		return dto.ToolCallRequest{}, false
	}
	name := firstNonEmptyString(tool["name"], functionMapName(tool))
	if name == "" {
		return dto.ToolCallRequest{}, false
	}
	description := firstNonEmptyString(tool["description"], functionMapDescription(tool))
	parameters := normalizeChatToolParameters(firstNonNil(tool["parameters"], tool["input_schema"], functionMapParameters(tool)))
	if namespace != "" {
		name = flattenResponsesToolName(namespace, name)
		description = combineToolDescriptions(namespaceDescription, description)
	}
	return dto.ToolCallRequest{
		Type: "function",
		Function: dto.FunctionRequest{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}, true
}

func normalizeChatToolParameters(parameters any) map[string]any {
	normalized := make(map[string]any)
	if parameterMap, ok := parameters.(map[string]any); ok {
		for key, value := range parameterMap {
			normalized[key] = value
		}
	}
	if normalized["type"] == nil {
		normalized["type"] = "object"
	}
	if normalized["properties"] == nil {
		normalized["properties"] = map[string]any{}
	}
	if normalized["required"] == nil {
		normalized["required"] = []any{}
	}
	return normalized
}

func functionMapName(tool map[string]any) string {
	if fn, ok := tool["function"].(map[string]any); ok {
		return common.Interface2String(fn["name"])
	}
	return ""
}

func functionMapDescription(tool map[string]any) string {
	if fn, ok := tool["function"].(map[string]any); ok {
		return common.Interface2String(fn["description"])
	}
	return ""
}

func functionMapParameters(tool map[string]any) any {
	if fn, ok := tool["function"].(map[string]any); ok {
		return firstNonNil(fn["parameters"], fn["input_schema"])
	}
	return nil
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func combineToolDescriptions(namespaceDescription string, toolDescription string) string {
	namespaceDescription = strings.TrimSpace(namespaceDescription)
	toolDescription = strings.TrimSpace(toolDescription)
	switch {
	case namespaceDescription == "":
		return toolDescription
	case toolDescription == "":
		return namespaceDescription
	default:
		return namespaceDescription + "\n" + toolDescription
	}
}

func flattenResponsesToolName(namespace string, name string) string {
	namespace = sanitizeChatToolName(namespace)
	name = sanitizeChatToolName(name)
	if namespace == "" {
		return name
	}
	if name == "" {
		return namespace
	}
	if strings.HasSuffix(namespace, "__") || strings.HasPrefix(name, "__") {
		return namespace + name
	}
	return namespace + "__" + name
}

func sanitizeChatToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
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
		namespace := common.Interface2String(choice["namespace"])
		if namespace == "" {
			if fn, ok := choice["function"].(map[string]any); ok {
				namespace = common.Interface2String(fn["namespace"])
			}
		}
		if namespace != "" {
			name = flattenResponsesToolName(namespace, name)
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

func rawInstructionsToChatString(raw json.RawMessage) string {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return ""
	}
	if common.GetJsonType(raw) == "string" {
		var s string
		if err := common.Unmarshal(raw, &s); err == nil {
			return s
		}
		return ""
	}
	if common.GetJsonType(raw) != "array" {
		return ""
	}
	var parts []any
	if err := common.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		switch typed := part.(type) {
		case string:
			if typed != "" {
				texts = append(texts, typed)
			}
		case map[string]any:
			text := common.Interface2String(typed["text"])
			if text != "" {
				texts = append(texts, text)
			}
		}
	}
	return strings.Join(texts, "\n\n")
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
	return ChatCompletionsResponseToResponsesResponseWithToolMap(resp, nil)
}

func ChatCompletionsResponseToResponsesResponseWithToolMap(resp *dto.OpenAITextResponse, toolNameMap map[string]ResponsesToolName) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}

	usage := normalizeChatUsageForResponses(resp.Usage)
	statusRaw, _ := common.Marshal("completed")
	output := chatChoicesToResponsesOutput(resp.Choices, resp.Id, toolNameMap)

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

func chatChoicesToResponsesOutput(choices []dto.OpenAITextResponseChoice, responseID string, toolNameMap map[string]ResponsesToolName) []dto.ResponsesOutput {
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
			nameInfo := toolNameMap[toolCall.Function.Name]
			name := toolCall.Function.Name
			namespace := ""
			if nameInfo.Name != "" {
				name = nameInfo.Name
				namespace = nameInfo.Namespace
			}
			output = append(output, dto.ResponsesOutput{
				Type:      "function_call",
				ID:        firstNonEmptyString(toolCall.ID, fmt.Sprintf("fc_%s_%d", responseID, choice.Index)),
				Status:    "completed",
				CallId:    firstNonEmptyString(toolCall.ID, fmt.Sprintf("call_%s_%d", responseID, choice.Index)),
				Name:      name,
				Namespace: namespace,
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
