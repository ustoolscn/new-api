package service

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service/openaicompat"
)

type ResponsesToolName = openaicompat.ResponsesToolName

func ChatCompletionsRequestToResponsesRequest(req *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error) {
	return openaicompat.ChatCompletionsRequestToResponsesRequest(req)
}

func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	return openaicompat.ResponsesRequestToChatCompletionsRequest(req)
}

func ResponsesRequestToChatCompletionsRequestWithToolMap(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, map[string]ResponsesToolName, error) {
	return openaicompat.ResponsesRequestToChatCompletionsRequestWithToolMap(req)
}

func ResponsesResponseToChatCompletionsResponse(resp *dto.OpenAIResponsesResponse, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	return openaicompat.ResponsesResponseToChatCompletionsResponse(resp, id)
}

func ChatCompletionsResponseToResponsesResponse(resp *dto.OpenAITextResponse) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	return openaicompat.ChatCompletionsResponseToResponsesResponse(resp)
}

func ChatCompletionsResponseToResponsesResponseWithToolMap(resp *dto.OpenAITextResponse, toolNameMap map[string]ResponsesToolName) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	return openaicompat.ChatCompletionsResponseToResponsesResponseWithToolMap(resp, toolNameMap)
}

func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	return openaicompat.ExtractOutputTextFromResponses(resp)
}
