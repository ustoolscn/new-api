package openaicompat

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const ResponsesToolNameMapKey = "openai_compat_responses_tool_name_map"

func ConvertResponsesRequestToChat(c *gin.Context, request dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	chatRequest, toolNameMap, err := service.ResponsesRequestToChatCompletionsRequestWithToolMap(&request)
	if err != nil {
		return nil, err
	}
	if c != nil && len(toolNameMap) > 0 {
		c.Set(ResponsesToolNameMapKey, toolNameMap)
	}
	return chatRequest, nil
}

func ChatCompletionsToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	var chatResponse dto.OpenAITextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	logger.LogDebug(c, "upstream response body: %s", responseBody)

	if err := common.Unmarshal(responseBody, &chatResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := chatResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	return WriteChatCompletionsResponseAsResponses(c, info, resp, &chatResponse)
}

func WriteChatCompletionsResponseAsResponses(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, chatResponse *dto.OpenAITextResponse) (*dto.Usage, *types.NewAPIError) {
	FillMissingChatUsage(c, info, chatResponse)
	responsesResponse, usage, err := service.ChatCompletionsResponseToResponsesResponseWithToolMap(chatResponse, GetResponsesToolNameMap(c))
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	jsonResponse, err := common.Marshal(responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, jsonResponse)
	return usage, nil
}

func FillMissingChatUsage(c *gin.Context, info *relaycommon.RelayInfo, chatResponse *dto.OpenAITextResponse) {
	if chatResponse == nil || chatResponse.Usage.PromptTokens != 0 {
		return
	}

	completionTokens := chatResponse.Usage.CompletionTokens
	if completionTokens == 0 {
		for _, choice := range chatResponse.Choices {
			completionTokens += service.CountTextToken(choice.Message.StringContent()+choice.Message.GetReasoningContent(), info.UpstreamModelName)
			completionTokens += len(choice.Message.ParseToolCalls()) * 7
		}
	}
	chatResponse.Usage.PromptTokens = info.GetEstimatePromptTokens()
	chatResponse.Usage.CompletionTokens = completionTokens
	chatResponse.Usage.TotalTokens = chatResponse.Usage.PromptTokens + completionTokens
	if chatResponse.Usage.PromptTokensDetails.CachedTokens == 0 && chatResponse.Usage.PromptCacheHitTokens != 0 {
		chatResponse.Usage.PromptTokensDetails.CachedTokens = chatResponse.Usage.PromptCacheHitTokens
	}
}

type streamedToolCall struct {
	id          string
	name        string
	namespace   string
	added       bool
	outputIndex int
	itemID      string
	arguments   strings.Builder
}

type inlineThinkMode int

const (
	inlineThinkModeDetecting inlineThinkMode = iota
	inlineThinkModeReasoning
	inlineThinkModeText
)

type thinkPrefixDecision int

const (
	thinkPrefixNeedMore thinkPrefixDecision = iota
	thinkPrefixReasoning
	thinkPrefixText
)

type ResponsesStreamConverter struct {
	c                 *gin.Context
	info              *relaycommon.RelayInfo
	responseID        string
	model             string
	createdAt         int
	started           bool
	nextIndex         int
	textAdded         bool
	textIndex         int
	textBuilder       strings.Builder
	reasoningAdded    bool
	reasoningDone     bool
	reasoningIndex    int
	reasoningItemID   string
	reasoningBuilder  strings.Builder
	inlineThinkMode   inlineThinkMode
	inlineThinkBuffer strings.Builder
	usage             *dto.Usage
	toolCalls         map[int]*streamedToolCall
	toolNameMap       map[string]service.ResponsesToolName
}

func NewResponsesStreamConverter(c *gin.Context, info *relaycommon.RelayInfo) *ResponsesStreamConverter {
	model := ""
	if info != nil {
		model = info.UpstreamModelName
	}
	return &ResponsesStreamConverter{
		c:               c,
		info:            info,
		model:           model,
		createdAt:       int(time.Now().Unix()),
		inlineThinkMode: inlineThinkModeDetecting,
		usage:           &dto.Usage{},
		toolCalls:       map[int]*streamedToolCall{},
		toolNameMap:     GetResponsesToolNameMap(c),
	}
}

func (s *ResponsesStreamConverter) SetUsage(usage *dto.Usage) {
	if usage == nil {
		return
	}
	clone := *usage
	s.usage = &clone
}

func (s *ResponsesStreamConverter) HandleChatChunk(chunk *dto.ChatCompletionsStreamResponse) {
	if chunk == nil {
		return
	}
	if chunk.Id != "" {
		s.responseID = chunk.Id
	}
	if chunk.Model != "" {
		s.model = chunk.Model
	}
	if chunk.Created != 0 {
		s.createdAt = int(chunk.Created)
	}
	s.ensureStarted()

	if chunk.Usage != nil && service.ValidUsage(chunk.Usage) {
		s.SetUsage(chunk.Usage)
		if s.usage.PromptTokensDetails.CachedTokens == 0 && s.usage.PromptCacheHitTokens != 0 {
			s.usage.PromptTokensDetails.CachedTokens = s.usage.PromptCacheHitTokens
		}
	}

	for _, choice := range chunk.Choices {
		if reasoningDelta := choice.Delta.GetReasoningContent(); reasoningDelta != "" {
			s.sendReasoningDelta(reasoningDelta)
		}
		if delta := choice.Delta.GetContentString(); delta != "" {
			s.pushContentDelta(delta)
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			s.flushInlineThinkAtBoundary()
			s.finalizeReasoning()
			index := 0
			if toolCall.Index != nil {
				index = *toolCall.Index
			}
			acc := s.toolCalls[index]
			if acc == nil {
				acc = &streamedToolCall{}
				s.toolCalls[index] = acc
			}
			if toolCall.ID != "" {
				acc.id = toolCall.ID
			}
			if toolCall.Function.Name != "" {
				if nameInfo, ok := s.toolNameMap[toolCall.Function.Name]; ok && nameInfo.Name != "" {
					acc.name = nameInfo.Name
					acc.namespace = nameInfo.Namespace
				} else {
					acc.name = toolCall.Function.Name
				}
			}
			if toolCall.ID != "" || toolCall.Function.Name != "" {
				s.ensureToolAdded(index, acc)
			}
			if toolCall.Function.Arguments != "" {
				s.ensureToolAdded(index, acc)
				acc.arguments.WriteString(toolCall.Function.Arguments)
				sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
					Type:        "response.function_call_arguments.delta",
					Delta:       toolCall.Function.Arguments,
					OutputIndex: common.GetPointer(acc.outputIndex),
					ItemID:      acc.itemID,
				})
			}
		}
	}
}

func (s *ResponsesStreamConverter) Finish() *dto.Usage {
	if s.responseID == "" {
		s.responseID = fmt.Sprintf("resp_%d", time.Now().UnixNano())
	}
	if s.model == "" && s.info != nil {
		s.model = s.info.UpstreamModelName
	}
	if !s.started {
		sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
			Type:     "response.created",
			Response: buildStreamingResponsesResponse(s.responseID, s.model, s.createdAt, nil, nil),
		})
	}

	s.flushInlineThinkAtBoundary()
	text := s.textBuilder.String()
	reasoning := s.reasoningBuilder.String()
	completionText := text + reasoning
	if s.usage.CompletionTokens == 0 && completionText != "" && s.info != nil {
		s.usage.CompletionTokens = service.CountTextToken(completionText, s.info.UpstreamModelName)
	}
	if s.usage.PromptTokens == 0 && s.usage.CompletionTokens != 0 && s.info != nil {
		s.usage.PromptTokens = s.info.GetEstimatePromptTokens()
	}
	if s.usage.TotalTokens == 0 {
		s.usage.TotalTokens = s.usage.PromptTokens + s.usage.CompletionTokens
	}

	output := streamedResponsesOutput(s.responseID, text, s.textAdded, s.textIndex, reasoning, s.reasoningAdded, s.reasoningIndex, s.reasoningItemID, s.toolCalls)
	s.finalizeReasoning()
	if s.textAdded {
		sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
			Type:         "response.output_text.done",
			Text:         text,
			OutputIndex:  common.GetPointer(s.textIndex),
			ContentIndex: common.GetPointer(0),
			ItemID:       responseMessageID(s.responseID),
		})
		if text != "" {
			messageItem := streamedTextOutput(s.responseID, text)
			sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
				Type:        dto.ResponsesOutputTypeItemDone,
				Item:        &messageItem,
				OutputIndex: common.GetPointer(s.textIndex),
			})
		}
	}
	for _, index := range sortedStreamedToolCallIndexes(s.toolCalls) {
		toolCall := s.toolCalls[index]
		if toolCall == nil {
			continue
		}
		if !toolCall.added {
			s.ensureToolAdded(index, toolCall)
		}
		arguments := toolCall.arguments.String()
		sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
			Type:        "response.function_call_arguments.done",
			Arguments:   arguments,
			OutputIndex: common.GetPointer(toolCall.outputIndex),
			ItemID:      toolCall.itemID,
		})
		item := streamedToolCallOutput(s.responseID, index, toolCall)
		sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
			Type:        dto.ResponsesOutputTypeItemDone,
			Item:        &item,
			OutputIndex: common.GetPointer(toolCall.outputIndex),
		})
	}
	sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
		Type:     "response.completed",
		Response: buildStreamingResponsesResponse(s.responseID, s.model, s.createdAt, output, s.usage),
	})
	helper.Done(s.c)

	return s.usage
}

func (s *ResponsesStreamConverter) ensureStarted() {
	if s.started {
		return
	}
	s.started = true
	sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
		Type:     "response.created",
		Response: buildStreamingResponsesResponse(s.responseID, s.model, s.createdAt, nil, nil),
	})
}

func (s *ResponsesStreamConverter) ensureTextAdded() {
	s.ensureStarted()
	if s.textAdded {
		return
	}
	s.textAdded = true
	s.textIndex = s.nextIndex
	s.nextIndex++
	sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
		Type: "response.output_item.added",
		Item: &dto.ResponsesOutput{
			Type:   "message",
			ID:     responseMessageID(s.responseID),
			Status: "in_progress",
			Role:   "assistant",
		},
		OutputIndex: common.GetPointer(s.textIndex),
	})
}

func (s *ResponsesStreamConverter) ensureReasoningAdded() {
	s.ensureStarted()
	if s.reasoningAdded {
		return
	}
	s.reasoningAdded = true
	s.reasoningIndex = s.nextIndex
	s.nextIndex++
	s.reasoningItemID = responseReasoningID(s.responseID)
	sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
		Type: "response.output_item.added",
		Item: &dto.ResponsesOutput{
			Type:             "reasoning",
			ID:               s.reasoningItemID,
			Status:           "in_progress",
			ReasoningContent: "",
		},
		OutputIndex: common.GetPointer(s.reasoningIndex),
	})
	sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
		Type:         "response.reasoning_summary_part.added",
		OutputIndex:  common.GetPointer(s.reasoningIndex),
		SummaryIndex: common.GetPointer(0),
		ItemID:       s.reasoningItemID,
		Part: &dto.ResponsesReasoningSummaryPart{
			Type: "summary_text",
			Text: "",
		},
	})
}

func (s *ResponsesStreamConverter) sendReasoningDelta(delta string) {
	if delta == "" {
		return
	}
	s.ensureReasoningAdded()
	s.reasoningBuilder.WriteString(delta)
	sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
		Type:         "response.reasoning_summary_text.delta",
		Delta:        delta,
		OutputIndex:  common.GetPointer(s.reasoningIndex),
		SummaryIndex: common.GetPointer(0),
		ItemID:       s.reasoningItemID,
	})
}

func (s *ResponsesStreamConverter) finalizeReasoning() {
	if !s.reasoningAdded || s.reasoningDone {
		return
	}
	s.reasoningDone = true
	reasoning := s.reasoningBuilder.String()
	sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
		Type:         "response.reasoning_summary_text.done",
		Text:         reasoning,
		OutputIndex:  common.GetPointer(s.reasoningIndex),
		SummaryIndex: common.GetPointer(0),
		ItemID:       s.reasoningItemID,
	})
	sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
		Type:         "response.reasoning_summary_part.done",
		OutputIndex:  common.GetPointer(s.reasoningIndex),
		SummaryIndex: common.GetPointer(0),
		ItemID:       s.reasoningItemID,
		Part: &dto.ResponsesReasoningSummaryPart{
			Type: "summary_text",
			Text: reasoning,
		},
	})
	reasoningItem := streamedReasoningOutput(s.reasoningItemID, reasoning)
	sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
		Type:        dto.ResponsesOutputTypeItemDone,
		Item:        &reasoningItem,
		OutputIndex: common.GetPointer(s.reasoningIndex),
	})
}

func (s *ResponsesStreamConverter) sendTextDelta(delta string) {
	if delta == "" {
		return
	}
	s.finalizeReasoning()
	s.ensureTextAdded()
	s.textBuilder.WriteString(delta)
	sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
		Type:         "response.output_text.delta",
		Delta:        delta,
		OutputIndex:  common.GetPointer(s.textIndex),
		ContentIndex: common.GetPointer(0),
		ItemID:       responseMessageID(s.responseID),
	})
}

func (s *ResponsesStreamConverter) drainCompleteInlineThink() {
	reasoning, answer, ok := service.SplitLeadingThinkBlock(s.inlineThinkBuffer.String())
	if !ok {
		return
	}
	s.inlineThinkMode = inlineThinkModeText
	s.inlineThinkBuffer.Reset()
	s.sendReasoningDelta(reasoning)
	s.finalizeReasoning()
	s.sendTextDelta(answer)
}

func (s *ResponsesStreamConverter) pushContentDelta(delta string) {
	switch s.inlineThinkMode {
	case inlineThinkModeText:
		s.sendTextDelta(delta)
	case inlineThinkModeDetecting:
		s.inlineThinkBuffer.WriteString(delta)
		switch leadingThinkPrefixDecision(s.inlineThinkBuffer.String()) {
		case thinkPrefixNeedMore:
		case thinkPrefixReasoning:
			s.inlineThinkMode = inlineThinkModeReasoning
			s.drainCompleteInlineThink()
		case thinkPrefixText:
			s.inlineThinkMode = inlineThinkModeText
			text := s.inlineThinkBuffer.String()
			s.inlineThinkBuffer.Reset()
			s.sendTextDelta(text)
		}
	case inlineThinkModeReasoning:
		s.inlineThinkBuffer.WriteString(delta)
		s.drainCompleteInlineThink()
	}
}

func (s *ResponsesStreamConverter) flushInlineThinkAtBoundary() {
	switch s.inlineThinkMode {
	case inlineThinkModeText:
	case inlineThinkModeDetecting:
		s.inlineThinkMode = inlineThinkModeText
		text := s.inlineThinkBuffer.String()
		s.inlineThinkBuffer.Reset()
		s.sendTextDelta(text)
	case inlineThinkModeReasoning:
		buffered := s.inlineThinkBuffer.String()
		s.inlineThinkBuffer.Reset()
		s.inlineThinkMode = inlineThinkModeText
		if reasoning, answer, ok := service.SplitLeadingThinkBlock(buffered); ok {
			s.sendReasoningDelta(reasoning)
			s.finalizeReasoning()
			s.sendTextDelta(answer)
			return
		}
		reasoning := stripLeadingThinkOpenTag(buffered)
		s.sendReasoningDelta(reasoning)
		s.finalizeReasoning()
	}
}

func (s *ResponsesStreamConverter) ensureToolAdded(index int, toolCall *streamedToolCall) {
	s.ensureStarted()
	if toolCall.added {
		return
	}
	toolCall.added = true
	toolCall.outputIndex = s.nextIndex
	s.nextIndex++
	if toolCall.id == "" {
		toolCall.id = fmt.Sprintf("call_%d", index)
	}
	if toolCall.name == "" {
		toolCall.name = "unknown_tool"
	}
	toolCall.itemID = toolCall.id
	sendResponsesStreamEvent(s.c, dto.ResponsesStreamResponse{
		Type: "response.output_item.added",
		Item: &dto.ResponsesOutput{
			Type:      "function_call",
			ID:        toolCall.itemID,
			Status:    "in_progress",
			CallId:    toolCall.id,
			Name:      toolCall.name,
			Namespace: toolCall.namespace,
			Arguments: chatStreamFunctionArgumentsRaw(""),
		},
		OutputIndex: common.GetPointer(toolCall.outputIndex),
	})
}

func ChatCompletionsToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	converter := NewResponsesStreamConverter(c, info)
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			logger.LogError(c, "failed to unmarshal chat completion stream response: "+err.Error())
			sr.Error(err)
			return
		}
		converter.HandleChatChunk(&chunk)
	})

	return converter.Finish(), nil
}

func sendResponsesStreamEvent(c *gin.Context, event dto.ResponsesStreamResponse) {
	jsonData, err := common.Marshal(event)
	if err != nil {
		logger.LogError(c, "failed to marshal responses stream event: "+err.Error())
		return
	}
	helper.ResponseChunkData(c, event, string(jsonData))
}

func buildStreamingResponsesResponse(responseID string, model string, createdAt int, output []dto.ResponsesOutput, usage *dto.Usage) *dto.OpenAIResponsesResponse {
	statusRaw, _ := common.Marshal("completed")
	if len(output) == 0 {
		statusRaw, _ = common.Marshal("in_progress")
	}
	return &dto.OpenAIResponsesResponse{
		ID:                responseID,
		Object:            "response",
		CreatedAt:         createdAt,
		Status:            statusRaw,
		Model:             model,
		Output:            output,
		ParallelToolCalls: true,
		Usage:             responseStreamUsage(usage),
	}
}

func responseStreamUsage(usage *dto.Usage) *dto.Usage {
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

func streamedResponsesOutput(responseID string, text string, textAdded bool, textIndex int, reasoning string, reasoningAdded bool, reasoningIndex int, reasoningItemID string, toolCalls map[int]*streamedToolCall) []dto.ResponsesOutput {
	type indexedOutput struct {
		index int
		item  dto.ResponsesOutput
	}
	indexed := make([]indexedOutput, 0, 2+len(toolCalls))
	if reasoningAdded {
		indexed = append(indexed, indexedOutput{
			index: reasoningIndex,
			item:  streamedReasoningOutput(reasoningItemID, reasoning),
		})
	}
	if textAdded && text != "" {
		indexed = append(indexed, indexedOutput{
			index: textIndex,
			item:  streamedTextOutput(responseID, text),
		})
	}
	for _, index := range sortedStreamedToolCallIndexes(toolCalls) {
		toolCall := toolCalls[index]
		if toolCall == nil {
			continue
		}
		indexed = append(indexed, indexedOutput{
			index: toolCall.outputIndex,
			item:  streamedToolCallOutput(responseID, index, toolCall),
		})
	}
	sort.SliceStable(indexed, func(i, j int) bool {
		return indexed[i].index < indexed[j].index
	})
	output := make([]dto.ResponsesOutput, 0, len(indexed))
	for _, item := range indexed {
		output = append(output, item.item)
	}
	return output
}

func streamedReasoningOutput(itemID string, reasoning string) dto.ResponsesOutput {
	return dto.ResponsesOutput{
		Type:             "reasoning",
		ID:               itemID,
		ReasoningContent: reasoning,
		Summary: []dto.ResponsesReasoningSummaryPart{
			{
				Type: "summary_text",
				Text: reasoning,
			},
		},
	}
}

func streamedTextOutput(responseID string, text string) dto.ResponsesOutput {
	return dto.ResponsesOutput{
		Type:   "message",
		ID:     responseMessageID(responseID),
		Status: "completed",
		Role:   "assistant",
		Content: []dto.ResponsesOutputContent{
			{
				Type:        "output_text",
				Text:        text,
				Annotations: []interface{}{},
			},
		},
	}
}

func streamedToolCallOutput(responseID string, index int, toolCall *streamedToolCall) dto.ResponsesOutput {
	callID := toolCall.id
	if callID == "" {
		callID = fmt.Sprintf("call_%s_%d", responseID, index)
	}
	return dto.ResponsesOutput{
		Type:      "function_call",
		ID:        callID,
		Status:    "completed",
		CallId:    callID,
		Name:      toolCall.name,
		Namespace: toolCall.namespace,
		Arguments: chatStreamFunctionArgumentsRaw(toolCall.arguments.String()),
	}
}

func sortedStreamedToolCallIndexes(toolCalls map[int]*streamedToolCall) []int {
	indexes := make([]int, 0, len(toolCalls))
	for index := range toolCalls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes
}

func GetResponsesToolNameMap(c *gin.Context) map[string]service.ResponsesToolName {
	if c == nil {
		return nil
	}
	value, exists := c.Get(ResponsesToolNameMapKey)
	if !exists {
		return nil
	}
	toolNameMap, _ := value.(map[string]service.ResponsesToolName)
	return toolNameMap
}

func chatStreamFunctionArgumentsRaw(arguments string) []byte {
	raw, _ := common.Marshal(arguments)
	return raw
}

func leadingThinkPrefixDecision(buffer string) thinkPrefixDecision {
	trimmed := strings.TrimLeftFunc(buffer, unicode.IsSpace)
	if trimmed == "" {
		return thinkPrefixNeedMore
	}
	if strings.HasPrefix(trimmed, "<think>") {
		return thinkPrefixReasoning
	}
	if strings.HasPrefix("<think>", trimmed) {
		return thinkPrefixNeedMore
	}
	return thinkPrefixText
}

func stripLeadingThinkOpenTag(text string) string {
	trimmed := strings.TrimLeftFunc(text, unicode.IsSpace)
	return strings.TrimSpace(strings.TrimPrefix(trimmed, "<think>"))
}

func responseReasoningID(responseID string) string {
	if responseID == "" {
		return "rs"
	}
	if strings.HasPrefix(responseID, "resp_") {
		return "rs_" + strings.TrimPrefix(responseID, "resp_")
	}
	if strings.HasPrefix(responseID, "chatcmpl_") {
		return "rs_" + strings.TrimPrefix(responseID, "chatcmpl_")
	}
	return "rs_" + responseID
}

func responseMessageID(responseID string) string {
	if responseID == "" {
		return "msg"
	}
	if strings.HasPrefix(responseID, "resp_") {
		return "msg_" + strings.TrimPrefix(responseID, "resp_")
	}
	if strings.HasPrefix(responseID, "chatcmpl_") {
		return "msg_" + strings.TrimPrefix(responseID, "chatcmpl_")
	}
	return "msg_" + responseID
}
