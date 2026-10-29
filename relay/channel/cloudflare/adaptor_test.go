package cloudflare

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddingResponsePreservesVectorsAndUsage(t *testing.T) {
	body := `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.25,-0.5]}],"model":"embedding-model","usage":{"prompt_tokens":3,"total_tokens":3}}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeEmbeddings,
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelCloudflare},
	}
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
	usage, apiErr := (&Adaptor{}).DoResponse(c, response, info)
	require.Nil(t, apiErr)
	assert.JSONEq(t, body, recorder.Body.String())
	actual, ok := usage.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 3, actual.PromptTokens)
	assert.Equal(t, 3, actual.TotalTokens)
}
