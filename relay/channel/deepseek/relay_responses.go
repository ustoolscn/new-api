package deepseek

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

func chatCompletionsToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
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

	fillMissingChatUsage(c, info, &chatResponse)
	responsesResponse, usage, err := service.ChatCompletionsResponseToResponsesResponseWithToolMap(&chatResponse, getResponsesToolNameMap(c))
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

func fillMissingChatUsage(c *gin.Context, info *relaycommon.RelayInfo, chatResponse *dto.OpenAITextResponse) {
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

func chatCompletionsToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	var (
		responseID        string
		model             = info.UpstreamModelName
		createdAt         = int(time.Now().Unix())
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
		inlineThinkMode   = inlineThinkModeDetecting
		inlineThinkBuffer strings.Builder
		usage             = &dto.Usage{}
		toolCalls         = map[int]*streamedToolCall{}
		toolNameMap       = getResponsesToolNameMap(c)
	)
	ensureStarted := func() {
		if started {
			return
		}
		started = true
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:     "response.created",
			Response: buildStreamingResponsesResponse(responseID, model, createdAt, nil, nil),
		})
	}
	ensureTextAdded := func() {
		ensureStarted()
		if textAdded {
			return
		}
		textAdded = true
		textIndex = nextIndex
		nextIndex++
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type: "response.output_item.added",
			Item: &dto.ResponsesOutput{
				Type:   "message",
				ID:     responseMessageID(responseID),
				Status: "in_progress",
				Role:   "assistant",
			},
			OutputIndex: common.GetPointer(textIndex),
		})
	}
	ensureReasoningAdded := func() {
		ensureStarted()
		if reasoningAdded {
			return
		}
		reasoningAdded = true
		reasoningIndex = nextIndex
		nextIndex++
		reasoningItemID = responseReasoningID(responseID)
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type: "response.output_item.added",
			Item: &dto.ResponsesOutput{
				Type:             "reasoning",
				ID:               reasoningItemID,
				Status:           "in_progress",
				ReasoningContent: "",
			},
			OutputIndex: common.GetPointer(reasoningIndex),
		})
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:         "response.reasoning_summary_part.added",
			OutputIndex:  common.GetPointer(reasoningIndex),
			SummaryIndex: common.GetPointer(0),
			ItemID:       reasoningItemID,
			Part: &dto.ResponsesReasoningSummaryPart{
				Type: "summary_text",
				Text: "",
			},
		})
	}
	sendReasoningDelta := func(delta string) {
		if delta == "" {
			return
		}
		ensureReasoningAdded()
		reasoningBuilder.WriteString(delta)
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:         "response.reasoning_summary_text.delta",
			Delta:        delta,
			OutputIndex:  common.GetPointer(reasoningIndex),
			SummaryIndex: common.GetPointer(0),
			ItemID:       reasoningItemID,
		})
	}
	finalizeReasoning := func() {
		if !reasoningAdded || reasoningDone {
			return
		}
		reasoningDone = true
		reasoning := reasoningBuilder.String()
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:         "response.reasoning_summary_text.done",
			Text:         reasoning,
			OutputIndex:  common.GetPointer(reasoningIndex),
			SummaryIndex: common.GetPointer(0),
			ItemID:       reasoningItemID,
		})
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:         "response.reasoning_summary_part.done",
			OutputIndex:  common.GetPointer(reasoningIndex),
			SummaryIndex: common.GetPointer(0),
			ItemID:       reasoningItemID,
			Part: &dto.ResponsesReasoningSummaryPart{
				Type: "summary_text",
				Text: reasoning,
			},
		})
		reasoningItem := streamedReasoningOutput(reasoningItemID, reasoning)
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:        dto.ResponsesOutputTypeItemDone,
			Item:        &reasoningItem,
			OutputIndex: common.GetPointer(reasoningIndex),
		})
	}
	sendTextDelta := func(delta string) {
		if delta == "" {
			return
		}
		finalizeReasoning()
		ensureTextAdded()
		textBuilder.WriteString(delta)
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:         "response.output_text.delta",
			Delta:        delta,
			OutputIndex:  common.GetPointer(textIndex),
			ContentIndex: common.GetPointer(0),
			ItemID:       responseMessageID(responseID),
		})
	}
	drainCompleteInlineThink := func() {
		reasoning, answer, ok := service.SplitLeadingThinkBlock(inlineThinkBuffer.String())
		if !ok {
			return
		}
		inlineThinkMode = inlineThinkModeText
		inlineThinkBuffer.Reset()
		sendReasoningDelta(reasoning)
		finalizeReasoning()
		sendTextDelta(answer)
	}
	pushContentDelta := func(delta string) {
		switch inlineThinkMode {
		case inlineThinkModeText:
			sendTextDelta(delta)
		case inlineThinkModeDetecting:
			inlineThinkBuffer.WriteString(delta)
			switch leadingThinkPrefixDecision(inlineThinkBuffer.String()) {
			case thinkPrefixNeedMore:
			case thinkPrefixReasoning:
				inlineThinkMode = inlineThinkModeReasoning
				drainCompleteInlineThink()
			case thinkPrefixText:
				inlineThinkMode = inlineThinkModeText
				text := inlineThinkBuffer.String()
				inlineThinkBuffer.Reset()
				sendTextDelta(text)
			}
		case inlineThinkModeReasoning:
			inlineThinkBuffer.WriteString(delta)
			drainCompleteInlineThink()
		}
	}
	flushInlineThinkAtBoundary := func() {
		switch inlineThinkMode {
		case inlineThinkModeText:
		case inlineThinkModeDetecting:
			inlineThinkMode = inlineThinkModeText
			text := inlineThinkBuffer.String()
			inlineThinkBuffer.Reset()
			sendTextDelta(text)
		case inlineThinkModeReasoning:
			buffered := inlineThinkBuffer.String()
			inlineThinkBuffer.Reset()
			inlineThinkMode = inlineThinkModeText
			if reasoning, answer, ok := service.SplitLeadingThinkBlock(buffered); ok {
				sendReasoningDelta(reasoning)
				finalizeReasoning()
				sendTextDelta(answer)
				return
			}
			reasoning := stripLeadingThinkOpenTag(buffered)
			sendReasoningDelta(reasoning)
			finalizeReasoning()
		}
	}
	ensureToolAdded := func(index int, toolCall *streamedToolCall) {
		ensureStarted()
		if toolCall.added {
			return
		}
		toolCall.added = true
		toolCall.outputIndex = nextIndex
		nextIndex++
		if toolCall.id == "" {
			toolCall.id = fmt.Sprintf("call_%d", index)
		}
		if toolCall.name == "" {
			toolCall.name = "unknown_tool"
		}
		toolCall.itemID = toolCall.id
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
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

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			logger.LogError(c, "failed to unmarshal chat completion stream response: "+err.Error())
			sr.Error(err)
			return
		}

		if chunk.Id != "" {
			responseID = chunk.Id
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.Created != 0 {
			createdAt = int(chunk.Created)
		}
		ensureStarted()

		if chunk.Usage != nil && service.ValidUsage(chunk.Usage) {
			usage = chunk.Usage
			if usage.PromptTokensDetails.CachedTokens == 0 && usage.PromptCacheHitTokens != 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}

		for _, choice := range chunk.Choices {
			if reasoningDelta := choice.Delta.GetReasoningContent(); reasoningDelta != "" {
				sendReasoningDelta(reasoningDelta)
			}
			if delta := choice.Delta.GetContentString(); delta != "" {
				pushContentDelta(delta)
			}
			for _, toolCall := range choice.Delta.ToolCalls {
				flushInlineThinkAtBoundary()
				finalizeReasoning()
				index := 0
				if toolCall.Index != nil {
					index = *toolCall.Index
				}
				acc := toolCalls[index]
				if acc == nil {
					acc = &streamedToolCall{}
					toolCalls[index] = acc
				}
				if toolCall.ID != "" {
					acc.id = toolCall.ID
				}
				if toolCall.Function.Name != "" {
					if nameInfo, ok := toolNameMap[toolCall.Function.Name]; ok && nameInfo.Name != "" {
						acc.name = nameInfo.Name
						acc.namespace = nameInfo.Namespace
					} else {
						acc.name = toolCall.Function.Name
					}
				}
				if toolCall.ID != "" || toolCall.Function.Name != "" {
					ensureToolAdded(index, acc)
				}
				if toolCall.Function.Arguments != "" {
					ensureToolAdded(index, acc)
					acc.arguments.WriteString(toolCall.Function.Arguments)
					sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
						Type:        "response.function_call_arguments.delta",
						Delta:       toolCall.Function.Arguments,
						OutputIndex: common.GetPointer(acc.outputIndex),
						ItemID:      acc.itemID,
					})
				}
			}
		}
	})

	if responseID == "" {
		responseID = fmt.Sprintf("resp_%d", time.Now().UnixNano())
	}
	if model == "" {
		model = info.UpstreamModelName
	}
	if !started {
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:     "response.created",
			Response: buildStreamingResponsesResponse(responseID, model, createdAt, nil, nil),
		})
	}

	flushInlineThinkAtBoundary()
	text := textBuilder.String()
	reasoning := reasoningBuilder.String()
	completionText := text + reasoning
	if usage.CompletionTokens == 0 && completionText != "" {
		usage.CompletionTokens = service.CountTextToken(completionText, info.UpstreamModelName)
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	output := streamedResponsesOutput(responseID, text, textAdded, textIndex, reasoning, reasoningAdded, reasoningIndex, reasoningItemID, toolCalls)
	finalizeReasoning()
	if textAdded {
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:         "response.output_text.done",
			Text:         text,
			OutputIndex:  common.GetPointer(textIndex),
			ContentIndex: common.GetPointer(0),
			ItemID:       responseMessageID(responseID),
		})
		if text != "" {
			messageItem := streamedTextOutput(responseID, text)
			sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
				Type:        dto.ResponsesOutputTypeItemDone,
				Item:        &messageItem,
				OutputIndex: common.GetPointer(textIndex),
			})
		}
	}
	for _, index := range sortedStreamedToolCallIndexes(toolCalls) {
		toolCall := toolCalls[index]
		if toolCall == nil {
			continue
		}
		if !toolCall.added {
			ensureToolAdded(index, toolCall)
		}
		arguments := toolCall.arguments.String()
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:        "response.function_call_arguments.done",
			Arguments:   arguments,
			OutputIndex: common.GetPointer(toolCall.outputIndex),
			ItemID:      toolCall.itemID,
		})
		item := streamedToolCallOutput(responseID, index, toolCall)
		sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:        dto.ResponsesOutputTypeItemDone,
			Item:        &item,
			OutputIndex: common.GetPointer(toolCall.outputIndex),
		})
	}
	sendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
		Type:     "response.completed",
		Response: buildStreamingResponsesResponse(responseID, model, createdAt, output, usage),
	})
	helper.Done(c)

	return usage, nil
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

func getResponsesToolNameMap(c *gin.Context) map[string]service.ResponsesToolName {
	if c == nil {
		return nil
	}
	value, exists := c.Get(responsesToolNameMapKey)
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
