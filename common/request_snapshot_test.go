package common

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestSnapshotOmitsOversizedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"api_key":"sensitive","input":"` + strings.Repeat("a", requestSnapshotBodyLimit) + `"}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	t.Cleanup(func() { CleanupBodyStorage(c) })
	snapshot := BuildRequestSnapshot(c)

	require.Equal(t, true, snapshot["body_truncated"])
	require.Equal(t, int64(len(body)), snapshot["content_length"])
	require.NotContains(t, snapshot, "body_preview")
	require.NotContains(t, snapshot, "body_error")
	require.NotContains(t, snapshot, "body")
}

func TestBuildRequestSnapshotKeepsLongJSONAndRedactsCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	longInput := strings.Repeat("上下文", 10000) + "tail"
	body := `{"input":[{"text":"` + longInput + `","api_key":"body-secret"}]}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses?api_key=query-secret", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer header-secret")
	t.Cleanup(func() { CleanupBodyStorage(c) })

	snapshot := BuildRequestSnapshot(c)

	require.NotContains(t, snapshot, "body_truncated")
	require.Equal(t, map[string]any{
		"input": []any{map[string]any{"text": longInput, "api_key": "***"}},
	}, snapshot["body"])
	require.Equal(t, "api_key=%2A%2A%2A", snapshot["query"])
	require.Equal(t, []string{"***"}, snapshot["headers"].(map[string][]string)["Authorization"])
}
