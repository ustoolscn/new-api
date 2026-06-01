package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildErrorRequestSnapshotSkipsJSONParseWhenBodyIsTruncated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"deepseek-chat","input":"` + strings.Repeat("a", errorRequestSnapshotBodyLimit+1024) + `"}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	snapshot := buildErrorRequestSnapshot(c)

	require.Equal(t, true, snapshot["body_truncated"])
	require.NotEmpty(t, snapshot["body_preview"])
	require.NotContains(t, snapshot, "body_error")
	require.NotContains(t, snapshot, "body")
}
