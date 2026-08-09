package router

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/default-dist
var defaultFrontendTestFS embed.FS

func TestSetWebRouterAlwaysServesDefaultFrontend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	setWebRouterWithBuildPath(engine, ThemeAssets{
		DefaultBuildFS:   defaultFrontendTestFS,
		DefaultIndexPage: []byte("default-index"),
	}, "testdata/default-dist")
	defaultFS := common.EmbedFolder(defaultFrontendTestFS, "testdata/default-dist")
	assert.True(t, defaultFS.Exists("/", "/asset.txt"))

	request := httptest.NewRequest(http.MethodGet, "/console/topup", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "default-index", recorder.Body.String())

	request = httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "default-asset\n", recorder.Body.String())
}
