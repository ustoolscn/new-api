package advancedcustom

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildRequestBodyRendersTypedValuesAndOmitsMissingFields(t *testing.T) {
	seed := 0
	config := newVideoTaskConfig()
	config.Submit.Body = map[string]any{
		"model":    "{model}",
		"prompt":   "{prompt}",
		"images":   "{images}",
		"seed":     "{seed}",
		"optional": "{negative_prompt}",
	}
	adaptor := &TaskAdaptor{config: config, apiKey: "secret"}
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt: "keep {literal} braces",
		Images: []string{"https://example.com/1.png", "https://example.com/2.png"},
		Seed:   &seed,
	})

	body, err := adaptor.BuildRequestBody(context, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-imagine-video"},
	})
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)

	assert.Equal(t, "grok-imagine-video", gjson.GetBytes(data, "model").String())
	assert.Equal(t, "keep {literal} braces", gjson.GetBytes(data, "prompt").String())
	assert.Equal(t, int64(2), gjson.GetBytes(data, "images.#").Int())
	assert.True(t, gjson.GetBytes(data, "seed").Exists())
	assert.Equal(t, int64(0), gjson.GetBytes(data, "seed").Int())
	assert.False(t, gjson.GetBytes(data, "optional").Exists())
}

func TestFetchTaskAppliesQueryTemplateAndAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/videos/req-123", r.URL.Path)
		assert.Equal(t, "secret", r.URL.Query().Get("key"))
		assert.Equal(t, "trace-req-123", r.Header.Get("X-Trace"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"done","video":{"url":"https://cdn.example/video.mp4"}}`))
	}))
	defer server.Close()

	config := newVideoTaskConfig()
	config.Query.Path = "/v1/videos/{task_id}"
	config.Query.Auth = &dto.AdvancedCustomRouteAuth{Type: dto.AdvancedCustomAuthTypeQuery, Name: "key", Value: "{api_key}"}
	config.Query.Headers = map[string]string{"X-Trace": "trace-{task_id}"}
	adaptor := &TaskAdaptor{config: config}

	resp, err := adaptor.FetchTask(server.URL, "secret", map[string]any{"task_id": "req-123"}, "")
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestDoResponseAndParseTaskResultUseConfiguredPaths(t *testing.T) {
	config := newVideoTaskConfig()
	adaptor := &TaskAdaptor{config: config}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	response := &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(strings.NewReader(`{"request_id":"req-123"}`)),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-imagine-video",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task-public",
		},
	}

	taskID, taskData, taskErr := adaptor.DoResponse(context, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "req-123", taskID)
	assert.JSONEq(t, `{"request_id":"req-123"}`, string(taskData))
	assert.Equal(t, "task-public", gjson.Get(recorder.Body.String(), "id").String())
	assert.Equal(t, dto.VideoStatusQueued, gjson.Get(recorder.Body.String(), "status").String())

	result, err := adaptor.ParseTaskResult([]byte(`{
		"status":"done",
		"progress":100,
		"video":{"url":"https://cdn.example/video.mp4"}
	}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
	assert.Equal(t, "100%", result.Progress)
	assert.Equal(t, "https://cdn.example/video.mp4", result.Url)
}

func newVideoTaskConfig() *dto.AdvancedCustomVideoTaskConfig {
	return &dto.AdvancedCustomVideoTaskConfig{
		Submit: dto.AdvancedCustomVideoTaskEndpoint{
			Path: "/v1/videos/generations",
			Body: map[string]any{"request": "{request}"},
		},
		Query: dto.AdvancedCustomVideoTaskEndpoint{
			Path: "/v1/videos/{task_id}",
		},
		Response: dto.AdvancedCustomVideoTaskResponseMapping{
			TaskIDPath:    "request_id",
			StatusPath:    "status",
			ProgressPath:  "progress",
			ResultURLPath: "video.url",
			ErrorPath:     "error.message",
			StatusMap: map[string]string{
				"done":   "SUCCESS",
				"failed": "FAILURE",
			},
		},
	}
}
