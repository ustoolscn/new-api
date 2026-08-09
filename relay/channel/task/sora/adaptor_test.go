package sora

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskResultUnknownStatusContinuesPolling(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"id": "video_123",
		"object": "video",
		"status": "unknown",
		"progress": 0
	}`))

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.TaskStatusInProgress, result.Status)
	assert.Empty(t, result.Reason)
}

func TestParseTaskResultUnknownStatusPreservesExplicitError(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"id": "video_123",
		"object": "video",
		"status": "unknown",
		"error": {
			"code": "content_policy",
			"message": "video generation was blocked"
		}
	}`))

	require.NoError(t, err)
	require.NotNil(t, result)
	// Keep status empty so the service poller can inspect the original
	// OpenAI error envelope and retain its existing error handling.
	assert.Empty(t, result.Status)
}

func TestParseTaskResultMissingOrUnknownStatusRemainsEmpty(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing",
			body: `{
				"id": "video_123",
				"object": "video",
				"progress": 0
			}`,
		},
		{
			name: "empty",
			body: `{
				"id": "video_123",
				"object": "video",
				"status": "",
				"progress": 0
			}`,
		},
		{
			name: "unrecognized",
			body: `{
				"id": "video_123",
				"object": "video",
				"status": "processing_later",
				"progress": 0
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(tt.body))

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Empty(t, result.Status)
		})
	}
}
