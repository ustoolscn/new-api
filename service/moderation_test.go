package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModerateRelayRequestSplitsMultipleImagesAndAggregatesResults(t *testing.T) {
	previousEnabled := setting.ModerationEnabled
	previousAPIKey := setting.ModerationAPIKey
	previousBaseURL := setting.ModerationBaseURL
	previousModel := setting.ModerationModel
	previousTimeout := setting.ModerationTimeoutSeconds
	previousBlockCategories := setting.ModerationBlockCategories
	t.Cleanup(func() {
		setting.ModerationEnabled = previousEnabled
		setting.ModerationAPIKey = previousAPIKey
		setting.ModerationBaseURL = previousBaseURL
		setting.ModerationModel = previousModel
		setting.ModerationTimeoutSeconds = previousTimeout
		setting.ModerationBlockCategories = previousBlockCategories
	})

	var requestBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/moderations", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		body := string(data)
		requestBodies = append(requestBodies, body)

		if strings.Count(body, `"type":"image_url"`) > 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"message":"Number of images (2) exceeds maximum of 1","type":"invalid_request_error","param":"input","code":"too_many_images"}}`)
			return
		}

		if strings.Contains(body, "image-2.png") {
			_, _ = fmt.Fprint(w, `{"id":"modr_2","model":"omni-moderation-latest","results":[{"flagged":true,"categories":{"violence":true},"category_scores":{"violence":0.91},"category_applied_input_types":{"violence":["image"]}}]}`)
			return
		}

		_, _ = fmt.Fprint(w, `{"id":"modr_1","model":"omni-moderation-latest","results":[{"flagged":false,"categories":{"violence":false},"category_scores":{"violence":0.01},"category_applied_input_types":{}}]}`)
	}))
	t.Cleanup(server.Close)

	setting.ModerationEnabled = true
	setting.ModerationAPIKey = "test-key"
	setting.ModerationBaseURL = server.URL
	setting.ModerationModel = "omni-moderation-latest"
	setting.ModerationTimeoutSeconds = 5
	setting.ModerationBlockCategories = []string{"violence"}

	result, err := ModerateRelayRequest(context.Background(), nil, &types.TokenCountMeta{
		CombineText: "check this prompt",
		Files: []*types.FileMeta{
			types.NewImageFileMeta(types.NewURLFileSource("https://example.com/image-1.png"), "auto"),
			types.NewImageFileMeta(types.NewURLFileSource("https://example.com/image-2.png"), "auto"),
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "block", result.Action)
	assert.True(t, result.Flagged)
	assert.ElementsMatch(t, []string{"violence"}, result.FlaggedCategories)
	assert.ElementsMatch(t, []string{"violence"}, result.BlockedCategories)
	assert.ElementsMatch(t, []string{"text", "image"}, result.InputTypes)
	assert.Equal(t, 0.91, result.CategoryScores["violence"])
	assert.Equal(t, []string{"image"}, result.CategoryAppliedInputTypes["violence"])
	require.Len(t, requestBodies, 3)
	for _, body := range requestBodies {
		assert.LessOrEqual(t, strings.Count(body, `"type":"image_url"`), 1)
	}
}

func TestModerateRelayRequestReturnsErrorWhenAnySplitRequestFails(t *testing.T) {
	previousEnabled := setting.ModerationEnabled
	previousAPIKey := setting.ModerationAPIKey
	previousBaseURL := setting.ModerationBaseURL
	previousModel := setting.ModerationModel
	previousTimeout := setting.ModerationTimeoutSeconds
	t.Cleanup(func() {
		setting.ModerationEnabled = previousEnabled
		setting.ModerationAPIKey = previousAPIKey
		setting.ModerationBaseURL = previousBaseURL
		setting.ModerationModel = previousModel
		setting.ModerationTimeoutSeconds = previousTimeout
	})

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		if strings.Contains(string(data), "image-2.png") {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = fmt.Fprint(w, `{"error":{"message":"upstream moderation failed","type":"server_error","code":"server_error"}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"id":"modr_1","model":"omni-moderation-latest","results":[{"flagged":false,"categories":{},"category_scores":{},"category_applied_input_types":{}}]}`)
	}))
	t.Cleanup(server.Close)

	setting.ModerationEnabled = true
	setting.ModerationAPIKey = "test-key"
	setting.ModerationBaseURL = server.URL
	setting.ModerationModel = "omni-moderation-latest"
	setting.ModerationTimeoutSeconds = 5

	result, err := ModerateRelayRequest(context.Background(), nil, &types.TokenCountMeta{
		Files: []*types.FileMeta{
			types.NewImageFileMeta(types.NewURLFileSource("https://example.com/image-1.png"), "auto"),
			types.NewImageFileMeta(types.NewURLFileSource("https://example.com/image-2.png"), "auto"),
			types.NewImageFileMeta(types.NewURLFileSource("https://example.com/image-3.png"), "auto"),
		},
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "moderation endpoint returned status 502")
	assert.Equal(t, 2, requestCount)
}

func TestModerationDiagnosticsSummarizesMetaWithoutRawContent(t *testing.T) {
	meta := &types.TokenCountMeta{
		CombineText:   "private prompt text",
		ToolsCount:    2,
		MessagesCount: 3,
		MaxTokens:     1024,
		Files: []*types.FileMeta{
			types.NewImageFileMeta(types.NewURLFileSource("https://example.com/private-image.png"), "auto"),
			types.NewFileMeta(types.FileTypeAudio, types.NewBase64FileSource("private-audio-data", "audio/wav")),
			types.NewFileMeta(types.FileTypeFile, types.NewBase64FileSource("private-file-data", "text/plain")),
			types.NewFileMeta(types.FileTypeVideo, types.NewURLFileSource("https://example.com/private-video.mp4")),
		},
	}

	diagnostics := ModerationDiagnostics(meta)
	formatted := FormatModerationDiagnostics(diagnostics)

	assert.Equal(t, len(meta.CombineText), diagnostics["combine_text_len"])
	assert.Equal(t, 4, diagnostics["files_total"])
	assert.Equal(t, 1, diagnostics["image_count"])
	assert.Equal(t, 1, diagnostics["audio_count"])
	assert.Equal(t, 1, diagnostics["file_count"])
	assert.Equal(t, 1, diagnostics["video_count"])
	assert.Equal(t, 1, diagnostics["moderation_image_inputs"])
	assert.Equal(t, 2, diagnostics["moderation_request_count"])
	assert.Equal(t, 2, diagnostics["tools_count"])
	assert.Equal(t, 3, diagnostics["messages_count"])
	assert.Equal(t, 1024, diagnostics["max_tokens"])
	assert.NotContains(t, formatted, meta.CombineText)
	assert.NotContains(t, formatted, "private-image")
	assert.NotContains(t, formatted, "private-audio")
	assert.NotContains(t, formatted, "private-file")
	assert.NotContains(t, formatted, "private-video")
}
