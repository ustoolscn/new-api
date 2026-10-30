package service

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func meteringResponse(t *testing.T, body, contentType string, rule dto.UsageAdjustment) (*http.Response, *relaycommon.RelayInfo) {
	t.Helper()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{UsageAdjustment: &rule}}}
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
	resp.Header.Set("Content-Type", contentType)
	require.NoError(t, PrepareUsageAdjustment(resp, info))
	t.Cleanup(func() { resp.Body.Close() })
	return resp, info
}

func TestUsageAdjustmentProtocolsAndConservation(t *testing.T) {
	rule := dto.UsageAdjustment{Enabled: true, CacheMin: -30, CacheMax: -30, OutputMin: 10, OutputMax: 10}
	for _, tt := range []struct {
		name, body, input, cache, output string
		wantInput                        int64
	}{
		{"claude", `{"usage":{"input_tokens":20000,"cache_read_input_tokens":130000,"cache_creation_input_tokens":50,"output_tokens":1000}}`, "usage.input_tokens", "usage.cache_read_input_tokens", "usage.output_tokens", 59000},
		{"chat", `{"usage":{"prompt_tokens":150050,"prompt_tokens_details":{"cached_tokens":130000,"cached_creation_tokens":50},"completion_tokens":1000,"total_tokens":151050}}`, "usage.prompt_tokens", "usage.prompt_tokens_details.cached_tokens", "usage.completion_tokens", 150050},
		{"responses", `{"response":{"usage":{"input_tokens":150050,"input_tokens_details":{"cached_tokens":130000},"output_tokens":1000,"total_tokens":151050}}}`, "response.usage.input_tokens", "response.usage.input_tokens_details.cached_tokens", "response.usage.output_tokens", 150050},
		{"gemini", `{"usageMetadata":{"promptTokenCount":150050,"cachedContentTokenCount":130000,"candidatesTokenCount":1000,"totalTokenCount":151050}}`, "usageMetadata.promptTokenCount", "usageMetadata.cachedContentTokenCount", "usageMetadata.candidatesTokenCount", 150050},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp, info := meteringResponse(t, tt.body, "application/json", rule)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.wantInput, gjson.GetBytes(body, tt.input).Int())
			assert.EqualValues(t, 91000, gjson.GetBytes(body, tt.cache).Int())
			assert.EqualValues(t, 1100, gjson.GetBytes(body, tt.output).Int())
			for _, record := range info.UsageAdjustment.Records {
				assert.Equal(t, record.Original.Input, record.Adjusted.Input)
				assert.Equal(t, record.Adjusted.Input, record.Adjusted.Uncached+record.Adjusted.CacheRead+record.Adjusted.CacheWrite)
			}
		})
	}
}

func TestUsageAdjustmentClaudeStreamUsesOneSampleAndPreservesEvents(t *testing.T) {
	stream := "event: message_start\r\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":20,\"cache_read_input_tokens\":130,\"output_tokens\":0}}}\r\n\r\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hello\"}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":100}}\n\n" +
		"data: [DONE]\n\n"
	resp, info := meteringResponse(t, stream, "text/event-stream", dto.UsageAdjustment{Enabled: true, CacheMin: -30, CacheMax: -10, OutputMin: 10, OutputMax: 30})
	cachePercent, outputPercent := info.UsageAdjustment.CachePercent, info.UsageAdjustment.OutputPercent
	result, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(result), `"text":"hello"`)
	assert.Contains(t, string(result), "event: message_delta")
	assert.Contains(t, string(result), "data: [DONE]")
	record := info.UsageAdjustment.Records["anthropic"]
	assert.EqualValues(t, common.QuotaRound(130*(1+cachePercent/100)), record.Adjusted.CacheRead)
	assert.EqualValues(t, common.QuotaRound(100*(1+outputPercent/100)), record.Adjusted.Output)
	assert.EqualValues(t, 150, record.Adjusted.Input)
	assert.Equal(t, cachePercent, info.UsageAdjustment.CachePercent)
	info.PriceData = types.PriceData{ModelRatio: 1, CompletionRatio: 2, CacheRatio: 0.1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	usage := &dto.Usage{PromptTokens: int(record.Adjusted.Uncached), CompletionTokens: int(record.Adjusted.Output), UsageSemantic: dto.BillingUsageSemanticAnthropic}
	usage.PromptTokensDetails.CachedTokens = int(record.Adjusted.CacheRead)
	AuditUsageAdjustmentCost(ctx, info, usage, 300)
	require.NotNil(t, info.UsageAdjustment.OriginalQuota)
	assert.Equal(t, 233, *info.UsageAdjustment.OriginalQuota)
	other := GenerateTextOtherInfo(ctx, info, 1, 1, 2, usage.PromptTokensDetails.CachedTokens, 0.1, 0, 0)
	encoded, err := common.Marshal(other)
	require.NoError(t, err)
	assert.EqualValues(t, 233, gjson.GetBytes(encoded, "admin_info.usage_adjustment.original_quota").Int())
	assert.False(t, gjson.GetBytes(encoded, "original_quota").Exists())
}

func TestUsageAdjustmentCacheCapZeroOutputAndBilling(t *testing.T) {
	resp, info := meteringResponse(t, `{"usage":{"input_tokens":20,"cache_read_input_tokens":130,"output_tokens":10}}`, "application/json", dto.UsageAdjustment{Enabled: true, CacheMin: 100, CacheMax: 100, OutputMin: -100, OutputMax: -100})
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.EqualValues(t, 0, gjson.GetBytes(body, "usage.input_tokens").Int())
	assert.EqualValues(t, 150, gjson.GetBytes(body, "usage.cache_read_input_tokens").Int())
	assert.EqualValues(t, 0, gjson.GetBytes(body, "usage.output_tokens").Int())
	var response dto.ClaudeResponse
	require.NoError(t, common.Unmarshal(body, &response))
	usage := &dto.Usage{BillingUsage: dto.NewClaudeMessagesBillingUsage(response.Usage)}
	info.PriceData = types.PriceData{ModelRatio: 1, CompletionRatio: 2, CacheRatio: 0.1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	summary := calculateTextQuotaSummary(ctx, info, effectiveBillingUsage(usage))
	assert.Equal(t, 15, summary.Quota)
	assert.Equal(t, 0, summary.PromptTokens)
	assert.Equal(t, 150, summary.CacheTokens)
}

func TestUsageAdjustmentReturnedUsageMatchesSettlement(t *testing.T) {
	resp, info := meteringResponse(t, `{"usage":{"input_tokens":20000,"cache_read_input_tokens":130000,"output_tokens":1000}}`, "application/json", dto.UsageAdjustment{Enabled: true, CacheMin: -30, CacheMax: -30, OutputMin: 10, OutputMax: 10})
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var response dto.ClaudeResponse
	require.NoError(t, common.Unmarshal(body, &response))
	usage := &dto.Usage{BillingUsage: dto.NewClaudeMessagesBillingUsage(response.Usage)}
	info.PriceData = types.PriceData{ModelRatio: 1, CompletionRatio: 2, CacheRatio: 0.1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	summary := calculateTextQuotaSummary(ctx, info, effectiveBillingUsage(usage))
	assert.Equal(t, 70300, summary.Quota)
	assert.Equal(t, response.Usage.InputTokens, summary.PromptTokens)
	assert.Equal(t, response.Usage.OutputTokens, summary.CompletionTokens)
	params := BuildTieredTokenParams(effectiveBillingUsage(usage), true, map[string]bool{"cr": true})
	assert.EqualValues(t, 59000, params.P)
	assert.EqualValues(t, 91000, params.CR)
	assert.EqualValues(t, 150000, params.Len)
	assert.EqualValues(t, 1100, params.C)
}

func TestUsageAdjustmentDisabledAndUnknownFields(t *testing.T) {
	body := `{"id":9007199254740993,"usage":{"input_tokens":20,"cache_read_input_tokens":130,"output_tokens":10},"content":[{"text":"unchanged"}]}`
	resp, info := meteringResponse(t, body, "application/json", dto.UsageAdjustment{})
	result, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, body, string(result))
	assert.Nil(t, info.UsageAdjustment)
	resp, _ = meteringResponse(t, body, "application/json", dto.UsageAdjustment{Enabled: true, CacheMin: -30, CacheMax: -30})
	result, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "9007199254740993", gjson.GetBytes(result, "id").Raw)
	assert.Equal(t, "unchanged", gjson.GetBytes(result, "content.0.text").String())
}

func TestUsageAdjustmentRejectsInvalidCountsAndSaturatesOutput(t *testing.T) {
	for _, value := range []string{"-1", "1.5", "2147483648", `"10"`} {
		resp, _ := meteringResponse(t, `{"usage":{"input_tokens":`+value+`,"output_tokens":1}}`, "application/json", dto.UsageAdjustment{Enabled: true})
		_, err := io.ReadAll(resp.Body)
		require.Error(t, err)
	}
	resp, info := meteringResponse(t, `{"usage":{"input_tokens":1,"output_tokens":2147483647}}`, "application/json", dto.UsageAdjustment{Enabled: true, OutputMin: 100, OutputMax: 100})
	result, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.EqualValues(t, math.MaxInt32, gjson.GetBytes(result, "usage.output_tokens").Int())
	require.NotNil(t, info.QuotaClamp)
}

func TestUsageAdjustmentGeminiReasoningAndToolInput(t *testing.T) {
	resp, info := meteringResponse(t, `{"usageMetadata":{"promptTokenCount":100,"toolUsePromptTokenCount":20,"cachedContentTokenCount":80,"candidatesTokenCount":30,"thoughtsTokenCount":10,"totalTokenCount":160}}`, "application/json", dto.UsageAdjustment{Enabled: true, CacheMin: -50, CacheMax: -50, OutputMin: 100, OutputMax: 100})
	result, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.EqualValues(t, 100, gjson.GetBytes(result, "usageMetadata.promptTokenCount").Int())
	assert.EqualValues(t, 20, gjson.GetBytes(result, "usageMetadata.thoughtsTokenCount").Int())
	assert.EqualValues(t, 60, gjson.GetBytes(result, "usageMetadata.candidatesTokenCount").Int())
	assert.EqualValues(t, 200, gjson.GetBytes(result, "usageMetadata.totalTokenCount").Int())
	assert.EqualValues(t, 120, info.UsageAdjustment.Records["gemini"].Adjusted.Input)
}

func TestUsageAdjustmentPreservesSemanticAliasesAndNestedBilling(t *testing.T) {
	resp, info := meteringResponse(t, `{"usage":{"usage_semantic":"anthropic","prompt_tokens":20,"input_tokens":170,"prompt_tokens_details":{"cached_tokens":130,"cached_creation_tokens":20},"completion_tokens":10,"output_tokens":10,"total_tokens":30,"billing_usage":{"claude_usage":{"input_tokens":20,"cache_read_input_tokens":130,"cache_creation_input_tokens":20,"output_tokens":10}}}}`, "application/json", dto.UsageAdjustment{Enabled: true, CacheMin: -50, CacheMax: -50, OutputMin: -100, OutputMax: -100})
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.EqualValues(t, 85, gjson.GetBytes(body, "usage.prompt_tokens").Int())
	assert.EqualValues(t, 170, gjson.GetBytes(body, "usage.input_tokens").Int())
	assert.EqualValues(t, 0, gjson.GetBytes(body, "usage.output_tokens").Int())
	assert.EqualValues(t, 0, gjson.GetBytes(body, "usage.completion_tokens").Int())
	assert.EqualValues(t, 85, gjson.GetBytes(body, "usage.billing_usage.claude_usage.input_tokens").Int())
	assert.EqualValues(t, 65, gjson.GetBytes(body, "usage.billing_usage.claude_usage.cache_read_input_tokens").Int())
	assert.Equal(t, info.UsageAdjustment.Records["anthropic"], info.UsageAdjustment.Records["openai_chat"])
}

func TestUsageAdjustmentPreservesCacheCreationDetails(t *testing.T) {
	resp, info := meteringResponse(t, `{"usage":{"input_tokens":20,"cache_read_input_tokens":130,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":30},"output_tokens":10}}`, "application/json", dto.UsageAdjustment{Enabled: true, CacheMin: 100, CacheMax: 100})
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.EqualValues(t, 150, gjson.GetBytes(body, "usage.cache_read_input_tokens").Int())
	assert.EqualValues(t, 0, gjson.GetBytes(body, "usage.input_tokens").Int())
	assert.EqualValues(t, 10, gjson.GetBytes(body, "usage.cache_creation.ephemeral_5m_input_tokens").Int())
	assert.EqualValues(t, 30, gjson.GetBytes(body, "usage.cache_creation.ephemeral_1h_input_tokens").Int())
	assert.EqualValues(t, 190, info.UsageAdjustment.Records["anthropic"].Adjusted.Input)
	assert.EqualValues(t, 40, info.UsageAdjustment.Records["anthropic"].Adjusted.CacheWrite)
}

func TestUsageAdjustmentCostAuditUsesOriginalCountsAndSamePricing(t *testing.T) {
	for _, mode := range []string{"ratios", "tiered", "per_call", "free", "invalid_expr"} {
		t.Run(mode, func(t *testing.T) {
			resp, info := meteringResponse(t, `{"usage":{"input_tokens":20000,"cache_read_input_tokens":130000,"output_tokens":1000}}`, "application/json", dto.UsageAdjustment{Enabled: true, CacheMin: -30, CacheMax: -30, OutputMin: -100, OutputMax: -100})
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			var response dto.ClaudeResponse
			require.NoError(t, common.Unmarshal(body, &response))
			usage := effectiveBillingUsage(&dto.Usage{BillingUsage: dto.NewClaudeMessagesBillingUsage(response.Usage)})
			info.PriceData = types.PriceData{ModelRatio: 1, CompletionRatio: 2, CacheRatio: 0.1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
			want := 35000
			switch mode {
			case "tiered", "invalid_expr":
				expr := "p * 2 + cr * 0.2 + c * 4"
				if mode == "invalid_expr" {
					expr = "invalid("
				}
				info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), GroupRatio: 1, QuotaPerUnit: 500000}
			case "per_call":
				info.PriceData.UsePrice = true
				info.PriceData.ModelPrice = 0.01
				want = common.QuotaFromFloat(0.01 * common.QuotaPerUnit)
			case "free":
				info.PriceData.GroupRatioInfo.GroupRatio = 0
				want = 0
			}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			before := *usage
			AuditUsageAdjustmentCost(ctx, info, usage, 68100)
			require.NotNil(t, info.UsageAdjustment.AdjustedQuota)
			assert.Equal(t, 68100, *info.UsageAdjustment.AdjustedQuota)
			if mode == "invalid_expr" {
				assert.Nil(t, info.UsageAdjustment.OriginalQuota)
			} else {
				require.NotNil(t, info.UsageAdjustment.OriginalQuota)
				assert.Equal(t, want, *info.UsageAdjustment.OriginalQuota)
			}
			assert.Equal(t, before, *usage)
			assert.Nil(t, info.QuotaClamp)
		})
	}
}

func TestUsageAdjustmentCostRestoresOutputDetailsWithoutDividingByZero(t *testing.T) {
	resp, info := meteringResponse(t, `{"usage":{"prompt_tokens":100,"completion_tokens":100,"completion_tokens_details":{"audio_tokens":40,"image_tokens":20},"total_tokens":200}}`, "application/json", dto.UsageAdjustment{Enabled: true, OutputMin: -100, OutputMax: -100})
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var response struct {
		Usage dto.Usage `json:"usage"`
	}
	require.NoError(t, common.Unmarshal(body, &response))
	info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: "p * 2 + c * 4 + ao * 8 + img_o * 10", GroupRatio: 1, QuotaPerUnit: 500000}
	info.TieredBillingSnapshot.ExprHash = billingexpr.ExprHashString(info.TieredBillingSnapshot.ExprString)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	AuditUsageAdjustmentCost(ctx, info, &response.Usage, 100)
	require.NotNil(t, info.UsageAdjustment.OriginalQuota)
	assert.Equal(t, 440, *info.UsageAdjustment.OriginalQuota)
	assert.Zero(t, response.Usage.CompletionTokenDetails.AudioTokens)
	encoded, err := common.Marshal(info.UsageAdjustment)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "OriginalOutputDetails")
}
