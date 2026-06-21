package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHiCodexUpdateProxiesDynamicUpstreamJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/update.json", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"version":"2099.01.02.030405","url":"https://files.example.test/Hi-Codex.exe","sha256":"abc"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(upstream.Close)

	originalURL := hiCodexUpdateURL
	t.Cleanup(func() {
		hiCodexUpdateURL = originalURL
	})
	hiCodexUpdateURL = upstream.URL + "/update.json"

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/hi-codex/update.json", nil)

	GetHiCodexUpdate(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	assert.JSONEq(t, `{"version":"2099.01.02.030405","url":"https://files.example.test/Hi-Codex.exe","sha256":"abc"}`, recorder.Body.String())
}
