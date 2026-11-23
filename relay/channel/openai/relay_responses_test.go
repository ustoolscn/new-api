package openai

import (
	"io"
	"testing"
	"testing/iotest"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOaiResponsesStreamUsageSettlement(t *testing.T) {
	service.InitTokenEncoders()
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	const created = `data: {"type":"response.created","response":{"id":"resp_test"}}` + "\n\n"
	const toolDelta = `data: {"type":"response.function_call_arguments.delta","delta":"lookup value"}` + "\n\n"
	tests := []struct {
		name        string
		body        string
		interrupted bool
		adjusted    bool
		prompt      int
		completion  int
		cached      int
		estimated   bool
	}{
		{name: "interrupted after first event", body: created, interrupted: true, prompt: 42, estimated: true},
		{name: "interrupted after tool delta", body: created + toolDelta, interrupted: true, prompt: 42, completion: -1, estimated: true},
		{name: "empty stream", interrupted: true},
		{name: "explicit upstream failure without usage", body: created + `data: {"type":"response.failed","response":{"status":"failed"}}` + "\n\n"},
		{
			name:   "incomplete terminal with usage",
			body:   created + `data: {"type":"response.incomplete","response":{"status":"incomplete","usage":{"input_tokens":60,"output_tokens":17,"input_tokens_details":{"cached_tokens":22}}}}` + "\n\n",
			prompt: 60, completion: 17, cached: 22,
		},
		{
			name:   "failed terminal with usage",
			body:   created + `data: {"type":"response.failed","response":{"status":"failed","usage":{"input_tokens":51,"output_tokens":0}}}` + "\n\n",
			prompt: 51,
		},
		{
			name:   "completed usage takes precedence over local count",
			body:   created + toolDelta + `data: {"type":"response.completed","response":{"usage":{"input_tokens":19,"output_tokens":8}}}` + "\n\n",
			prompt: 19, completion: 8,
		},
		{
			name:     "adjusted output remains zero",
			body:     created + toolDelta + `data: {"type":"response.completed","response":{"usage":{"input_tokens":19,"output_tokens":0}}}` + "\n\n",
			adjusted: true, prompt: 19,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _, resp, info := newResponsesChatTestContext(t, test.body, true)
			ctx.Request.URL.Path = "/v1/responses"
			info.SetEstimatePromptTokens(42)
			if test.adjusted {
				info.UsageAdjustment = &dto.UsageAdjustmentAudit{Status: "applied"}
			}
			if test.interrupted {
				resp.Body = io.NopCloser(io.MultiReader(resp.Body, iotest.ErrReader(io.ErrUnexpectedEOF)))
			}

			usage, apiErr := OaiResponsesStreamHandler(ctx, info, resp)

			require.Nil(t, apiErr)
			require.Equal(t, test.prompt, usage.PromptTokens)
			if test.completion < 0 {
				require.Positive(t, usage.CompletionTokens)
			} else {
				require.Equal(t, test.completion, usage.CompletionTokens)
			}
			require.Equal(t, test.cached, usage.PromptTokensDetails.CachedTokens)
			require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
			require.Equal(t, test.estimated, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens))
			if test.interrupted {
				require.Equal(t, relaycommon.StreamEndReasonScannerErr, info.StreamStatus.EndReason)
			}
		})
	}
}
