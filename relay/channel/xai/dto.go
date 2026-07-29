package xai

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/dto"
)

// ChatCompletionResponse represents the response from XAI chat completion API
type ChatCompletionResponse struct {
	Id                string                         `json:"id"`
	Object            string                         `json:"object"`
	Created           int64                          `json:"created"`
	Model             string                         `json:"model"`
	Choices           []dto.OpenAITextResponseChoice `json:"choices"`
	Usage             *dto.Usage                     `json:"usage"`
	SystemFingerprint string                         `json:"system_fingerprint"`
}

type ImageRequest struct {
	Model          string          `json:"model"`
	Prompt         string          `json:"prompt" binding:"required"`
	N              *uint           `json:"n,omitempty"`
	AspectRatio    string          `json:"aspect_ratio,omitempty"`
	Resolution     string          `json:"resolution,omitempty"`
	ResponseFormat string          `json:"response_format,omitempty"`
	Image          *ImageMedia     `json:"image,omitempty"`
	Images         []ImageMedia    `json:"images,omitempty"`
	StorageOptions json.RawMessage `json:"storage_options,omitempty"`
	User           json.RawMessage `json:"user,omitempty"`
}

type ImageMedia struct {
	URL    string `json:"url,omitempty"`
	FileID string `json:"file_id,omitempty"`
}
