package claude

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestUsageAdjustmentClaudeAdaptorPreservesAdjustedZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{true: "stream", false: "json"}[stream], func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, IsStream: stream, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-opus-5-5", ChannelSetting: dto.ChannelSettings{UsageAdjustment: &dto.UsageAdjustment{Enabled: true, CacheMin: -30, CacheMax: -30, OutputMin: -100, OutputMax: -100}}}}
			body := `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":20,"cache_read_input_tokens":130,"output_tokens":10}}`
			contentType := "application/json"
			if stream {
				contentType = "text/event-stream"
				body = "data: " + `{"type":"message_start","message":{"id":"msg_1","model":"claude-opus-5-5","usage":{"input_tokens":20,"cache_read_input_tokens":130,"output_tokens":1}}}` + "\n\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}` + "\n\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}` + "\n\ndata: " + `{"type":"message_stop"}` + "\n\n"
			}
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
			require.NoError(t, service.PrepareUsageAdjustment(resp, info))
			handler := ClaudeHandler
			if stream {
				handler = ClaudeStreamHandler
			}
			usage, err := handler(c, resp, info)
			require.Nil(t, err)
			require.NotNil(t, usage)
			assert.Equal(t, 59, usage.PromptTokens)
			assert.Equal(t, 91, usage.PromptTokensDetails.CachedTokens)
			assert.Zero(t, usage.CompletionTokens)
			assert.Contains(t, recorder.Body.String(), "hello")
			if stream {
				assert.Contains(t, recorder.Body.String(), `"output_tokens":0`)
			} else {
				assert.EqualValues(t, 59, gjson.GetBytes(recorder.Body.Bytes(), "usage.input_tokens").Int())
			}
			require.NotNil(t, usage.BillingUsage)
			assert.Equal(t, 59, usage.BillingUsage.ClaudeUsage.InputTokens)
			assert.Zero(t, usage.BillingUsage.ClaudeUsage.OutputTokens)
		})
	}
}
