package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
)

type moderationImageURL struct {
	URL string `json:"url"`
}

type moderationInputPart struct {
	Type     string              `json:"type"`
	Text     string              `json:"text,omitempty"`
	ImageURL *moderationImageURL `json:"image_url,omitempty"`
}

type moderationRequest struct {
	Model string `json:"model"`
	Input any    `json:"input"`
}

type moderationCheckInput struct {
	Input      any
	InputTypes []string
}

type moderationResponse struct {
	ID      string                  `json:"id"`
	Model   string                  `json:"model"`
	Results []moderationResultEntry `json:"results"`
}

type moderationErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
		Param   string `json:"param"`
	} `json:"error"`
}

type moderationResultEntry struct {
	Flagged                   bool                `json:"flagged"`
	Categories                map[string]bool     `json:"categories"`
	CategoryScores            map[string]float64  `json:"category_scores"`
	CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types"`
}

type ModerationResult struct {
	Action                    string              `json:"action"`
	Flagged                   bool                `json:"flagged"`
	Model                     string              `json:"model,omitempty"`
	BlockedCategories         []string            `json:"blocked_categories,omitempty"`
	FlaggedCategories         []string            `json:"flagged_categories,omitempty"`
	CategoryScores            map[string]float64  `json:"category_scores,omitempty"`
	CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types,omitempty"`
	InputTypes                []string            `json:"input_types,omitempty"`
	Error                     string              `json:"error,omitempty"`
	Diagnostics               map[string]any      `json:"diagnostics,omitempty"`
}

func NewModerationErrorResult(err error) *ModerationResult {
	result := &ModerationResult{
		Action: "error",
	}
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func NewModerationErrorResultWithDiagnostics(err error, diagnostics map[string]any) *ModerationResult {
	result := NewModerationErrorResult(err)
	result.Diagnostics = diagnostics
	return result
}

func ModerationFailureModeClosed() bool {
	return setting.NormalizeModerationFailureMode(setting.ModerationFailureMode) == "closed"
}

func ModerateRelayRequest(ctx context.Context, request dto.Request, meta *types.TokenCountMeta) (*ModerationResult, error) {
	if !setting.ModerationEnabled {
		return nil, nil
	}
	if strings.TrimSpace(setting.ModerationAPIKey) == "" {
		return nil, fmt.Errorf("moderation api key is not configured")
	}
	if meta == nil && request != nil {
		meta = request.GetTokenCountMeta()
	}
	checks := buildModerationInputs(meta)
	if len(checks) == 0 {
		return nil, nil
	}

	model := strings.TrimSpace(setting.ModerationModel)
	if model == "" {
		model = "omni-moderation-latest"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(setting.ModerationBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	timeout := time.Duration(setting.ModerationTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var combined *ModerationResult
	for _, check := range checks {
		payload, err := common.Marshal(moderationRequest{
			Model: model,
			Input: check.Input,
		})
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/moderations", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+setting.ModerationAPIKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			message := readModerationErrorBody(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("moderation endpoint returned status %d: %s", resp.StatusCode, message)
		}

		var parsed moderationResponse
		err = common.DecodeJson(resp.Body, &parsed)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		combined = mergeModerationResults(combined, normalizeModerationResult(parsed, check.InputTypes))
	}
	return combined, nil
}

func readModerationErrorBody(body io.Reader) string {
	if body == nil {
		return "empty response body"
	}
	data, err := io.ReadAll(io.LimitReader(body, 4096))
	if err != nil {
		return "failed to read response body: " + err.Error()
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "empty response body"
	}

	var parsed moderationErrorResponse
	if err := common.Unmarshal(data, &parsed); err == nil && parsed.Error.Message != "" {
		parts := []string{parsed.Error.Message}
		if parsed.Error.Type != "" {
			parts = append(parts, "type="+parsed.Error.Type)
		}
		if parsed.Error.Param != "" {
			parts = append(parts, "param="+parsed.Error.Param)
		}
		if parsed.Error.Code != nil {
			parts = append(parts, "code="+common.Interface2String(parsed.Error.Code))
		}
		return strings.Join(parts, ", ")
	}
	return text
}

func buildModerationInputs(meta *types.TokenCountMeta) []moderationCheckInput {
	if meta == nil {
		return nil
	}
	checks := make([]moderationCheckInput, 0, 1+len(meta.Files))
	if strings.TrimSpace(meta.CombineText) != "" {
		checks = append(checks, moderationCheckInput{
			Input:      meta.CombineText,
			InputTypes: []string{"text"},
		})
	}
	for _, file := range meta.Files {
		if file == nil || file.FileType != types.FileTypeImage || file.Source == nil {
			continue
		}
		url := moderationImageSourceURL(file.Source)
		if url == "" {
			continue
		}
		checks = append(checks, moderationCheckInput{
			Input: []moderationInputPart{
				{
					Type:     "image_url",
					ImageURL: &moderationImageURL{URL: url},
				},
			},
			InputTypes: []string{"image"},
		})
	}
	return checks
}

func ModerationDiagnostics(meta *types.TokenCountMeta) map[string]any {
	diagnostics := map[string]any{
		"combine_text_len":         0,
		"files_total":              0,
		"image_count":              0,
		"audio_count":              0,
		"file_count":               0,
		"video_count":              0,
		"moderation_image_inputs":  0,
		"moderation_request_count": 0,
		"tools_count":              0,
		"messages_count":           0,
		"max_tokens":               0,
	}
	if meta == nil {
		return diagnostics
	}
	diagnostics["combine_text_len"] = len(meta.CombineText)
	diagnostics["tools_count"] = meta.ToolsCount
	diagnostics["messages_count"] = meta.MessagesCount
	diagnostics["max_tokens"] = meta.MaxTokens
	diagnostics["moderation_request_count"] = len(buildModerationInputs(meta))
	for _, file := range meta.Files {
		if file == nil {
			continue
		}
		diagnostics["files_total"] = diagnostics["files_total"].(int) + 1
		switch file.FileType {
		case types.FileTypeImage:
			diagnostics["image_count"] = diagnostics["image_count"].(int) + 1
			if file.Source != nil && moderationImageSourceURL(file.Source) != "" {
				diagnostics["moderation_image_inputs"] = diagnostics["moderation_image_inputs"].(int) + 1
			}
		case types.FileTypeAudio:
			diagnostics["audio_count"] = diagnostics["audio_count"].(int) + 1
		case types.FileTypeFile:
			diagnostics["file_count"] = diagnostics["file_count"].(int) + 1
		case types.FileTypeVideo:
			diagnostics["video_count"] = diagnostics["video_count"].(int) + 1
		}
	}
	return diagnostics
}

func FormatModerationDiagnostics(diagnostics map[string]any) string {
	if len(diagnostics) == 0 {
		return "{}"
	}
	data, err := common.Marshal(diagnostics)
	if err != nil {
		return fmt.Sprintf("%v", diagnostics)
	}
	return string(data)
}

func moderationImageSourceURL(source types.FileSource) string {
	raw := strings.TrimSpace(source.GetRawData())
	if raw == "" {
		return ""
	}
	if source.IsURL() || strings.HasPrefix(raw, "data:image/") {
		return raw
	}
	if base64Source, ok := source.(*types.Base64Source); ok && base64Source.MimeType != "" {
		return fmt.Sprintf("data:%s;base64,%s", base64Source.MimeType, raw)
	}
	return ""
}

func normalizeModerationResult(response moderationResponse, inputTypes []string) *ModerationResult {
	result := &ModerationResult{
		Action:     "pass",
		Model:      response.Model,
		InputTypes: inputTypes,
	}
	if len(response.Results) == 0 {
		return result
	}
	entry := response.Results[0]
	result.Flagged = entry.Flagged
	result.CategoryScores = entry.CategoryScores
	result.CategoryAppliedInputTypes = entry.CategoryAppliedInputTypes

	blockSet := make(map[string]struct{}, len(setting.ModerationBlockCategories))
	for _, category := range setting.ModerationBlockCategories {
		blockSet[strings.TrimSpace(category)] = struct{}{}
	}
	for category, flagged := range entry.Categories {
		if !flagged {
			continue
		}
		result.FlaggedCategories = append(result.FlaggedCategories, category)
		if _, ok := blockSet[category]; ok {
			result.BlockedCategories = append(result.BlockedCategories, category)
		}
	}
	if len(result.BlockedCategories) > 0 {
		result.Action = "block"
	} else if result.Flagged {
		result.Action = "warn"
	}
	return result
}

func mergeModerationResults(current *ModerationResult, next *ModerationResult) *ModerationResult {
	if next == nil {
		return current
	}
	if current == nil {
		next.Action = moderationAction(next)
		return next
	}
	if current.Model == "" {
		current.Model = next.Model
	}
	current.Flagged = current.Flagged || next.Flagged
	for _, inputType := range next.InputTypes {
		current.InputTypes = appendUnique(current.InputTypes, inputType)
	}
	for _, category := range next.FlaggedCategories {
		current.FlaggedCategories = appendUnique(current.FlaggedCategories, category)
	}
	for _, category := range next.BlockedCategories {
		current.BlockedCategories = appendUnique(current.BlockedCategories, category)
	}
	if len(next.CategoryScores) > 0 && current.CategoryScores == nil {
		current.CategoryScores = make(map[string]float64, len(next.CategoryScores))
	}
	for category, score := range next.CategoryScores {
		if existing, ok := current.CategoryScores[category]; !ok || score > existing {
			current.CategoryScores[category] = score
		}
	}
	if len(next.CategoryAppliedInputTypes) > 0 && current.CategoryAppliedInputTypes == nil {
		current.CategoryAppliedInputTypes = make(map[string][]string, len(next.CategoryAppliedInputTypes))
	}
	for category, inputTypes := range next.CategoryAppliedInputTypes {
		for _, inputType := range inputTypes {
			current.CategoryAppliedInputTypes[category] = appendUnique(current.CategoryAppliedInputTypes[category], inputType)
		}
	}
	current.Action = moderationAction(current)
	return current
}

func moderationAction(result *ModerationResult) string {
	if result == nil {
		return "pass"
	}
	if len(result.BlockedCategories) > 0 {
		return "block"
	}
	if result.Flagged {
		return "warn"
	}
	return "pass"
}

func appendUnique(items []string, item string) []string {
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}
