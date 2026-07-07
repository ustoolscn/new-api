package service

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service/relayconvert"
)

type ResponsesToolName struct {
	Namespace string
	Name      string
}

func ChatCompletionsRequestToResponsesRequest(req *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error) {
	return relayconvert.ChatCompletionsRequestToResponsesRequest(req)
}

func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	return relayconvert.ResponsesRequestToChatCompletionsRequest(req)
}

func ResponsesRequestToChatCompletionsRequestWithToolMap(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, map[string]ResponsesToolName, error) {
	out, err := relayconvert.ResponsesRequestToChatCompletionsRequest(req)
	if err != nil {
		return nil, nil, err
	}
	tools, toolNameMap, ok := responsesNamespaceToolsToChatTools(req.Tools)
	if ok {
		out.Tools = tools
	}
	if len(toolNameMap) > 0 {
		out.ToolChoice = flattenResponsesNamespaceToolChoice(req.ToolChoice, out.ToolChoice)
	}
	return out, toolNameMap, nil
}

func ChatCompletionsResponseToResponsesResponse(resp *dto.OpenAITextResponse, id string) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	return relayconvert.ChatCompletionsResponseToResponsesResponse(resp, id)
}

func ResponsesResponseToChatCompletionsResponse(resp *dto.OpenAIResponsesResponse, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	return relayconvert.ResponsesResponseToChatCompletionsResponse(resp, id)
}

func ResponsesFinishReasonFromStatus(resp *dto.OpenAIResponsesResponse) (string, bool) {
	return relayconvert.ResponsesFinishReasonFromStatus(resp)
}

func ChatCompletionsResponseToResponsesResponseWithToolMap(resp *dto.OpenAITextResponse, toolNameMap map[string]ResponsesToolName) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	if resp == nil {
		return relayconvert.ChatCompletionsResponseToResponsesResponse(resp, "")
	}
	responsesResponse, usage, err := relayconvert.ChatCompletionsResponseToResponsesResponse(resp, resp.Id)
	if err != nil {
		return nil, nil, err
	}
	if len(toolNameMap) == 0 || responsesResponse == nil {
		return responsesResponse, usage, nil
	}
	for index := range responsesResponse.Output {
		output := &responsesResponse.Output[index]
		if output.Type != "function_call" {
			continue
		}
		if nameInfo, ok := toolNameMap[output.Name]; ok && nameInfo.Name != "" {
			output.Name = nameInfo.Name
			output.Namespace = nameInfo.Namespace
		}
	}
	return responsesResponse, usage, nil
}

func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	return relayconvert.ExtractOutputTextFromResponses(resp)
}

func SplitLeadingThinkBlock(text string) (string, string, bool) {
	leadingWsLen := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
	afterWs := text[leadingWsLen:]
	if !strings.HasPrefix(afterWs, "<think>") {
		return "", "", false
	}
	bodyStart := leadingWsLen + len("<think>")
	closeRelative := strings.Index(text[bodyStart:], "</think>")
	if closeRelative < 0 {
		return "", "", false
	}
	closeStart := bodyStart + closeRelative
	answerStart := closeStart + len("</think>")
	return strings.TrimSpace(text[bodyStart:closeStart]), strings.TrimLeft(text[answerStart:], "\r\n\t "), true
}

func responsesNamespaceToolsToChatTools(raw json.RawMessage) ([]dto.ToolCallRequest, map[string]ResponsesToolName, bool) {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return nil, nil, false
	}

	var tools []map[string]any
	if err := common.Unmarshal(raw, &tools); err != nil {
		return nil, nil, false
	}

	out := make([]dto.ToolCallRequest, 0, len(tools))
	toolNameMap := make(map[string]ResponsesToolName)
	hasNamespaceTool := false
	for _, tool := range tools {
		switch strings.TrimSpace(common.Interface2String(tool["type"])) {
		case "function":
			if chatTool, ok := responsesFunctionToolToChatTool(tool, "", ""); ok {
				out = append(out, chatTool)
			}
		case "namespace":
			hasNamespaceTool = true
			namespace := firstNonEmptyString(tool["name"], tool["namespace"])
			namespaceDescription := common.Interface2String(tool["description"])
			for _, namespaceTool := range normalizeNamespaceTools(tool["tools"]) {
				name := firstNonEmptyString(namespaceTool["name"], functionMapName(namespaceTool))
				chatTool, ok := responsesFunctionToolToChatTool(namespaceTool, namespace, namespaceDescription)
				if !ok {
					continue
				}
				out = append(out, chatTool)
				if namespace != "" && name != "" {
					toolNameMap[chatTool.Function.Name] = ResponsesToolName{
						Namespace: namespace,
						Name:      name,
					}
				}
			}
		}
	}
	return out, toolNameMap, hasNamespaceTool
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
	description := firstNonEmptyString(common.Interface2String(tool["description"]), functionMapDescription(tool))
	parameters := firstNonNil(tool["parameters"], tool["input_schema"], functionMapParameters(tool))
	if namespace != "" {
		name = flattenResponsesToolName(namespace, name)
		description = combineToolDescriptions(namespaceDescription, description)
	}
	return dto.ToolCallRequest{
		Type: "function",
		Function: dto.FunctionRequest{
			Name:        name,
			Description: description,
			Parameters:  normalizeChatToolParameters(parameters),
		},
	}, true
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

func flattenResponsesNamespaceToolChoice(raw json.RawMessage, fallback any) any {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return fallback
	}
	var choice map[string]any
	if err := common.Unmarshal(raw, &choice); err != nil {
		return fallback
	}
	if common.Interface2String(choice["type"]) != "function" {
		return fallback
	}
	name := common.Interface2String(choice["name"])
	namespace := common.Interface2String(choice["namespace"])
	if fn, ok := choice["function"].(map[string]any); ok {
		if name == "" {
			name = common.Interface2String(fn["name"])
		}
		if namespace == "" {
			namespace = common.Interface2String(fn["namespace"])
		}
	}
	if namespace == "" || name == "" {
		return fallback
	}
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": flattenResponsesToolName(namespace, name),
		},
	}
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

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s := strings.TrimSpace(common.Interface2String(value)); s != "" {
			return s
		}
	}
	return ""
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
