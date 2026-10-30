package service

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const maxMeteringFrameBytes = 64 << 20

// AuditUsageAdjustmentCost only evaluates prices. It never settles a balance,
// updates counters, or replaces the actual request's quota-clamp marker.
func AuditUsageAdjustmentCost(ctx *gin.Context, info *relaycommon.RelayInfo, billed *dto.Usage, chargedQuota int) {
	if !HasAdjustedUsage(info) || billed == nil {
		return
	}
	audit := info.UsageAdjustment
	audit.OriginalQuota = nil
	audit.AdjustedQuota = &chargedQuota
	protocol := map[string]string{
		dto.BillingUsageSourceClaudeMessages: "anthropic",
		dto.BillingUsageSourceOAIChat:        "openai_chat",
		dto.BillingUsageSourceOAIResponses:   "openai_responses",
		dto.BillingUsageSourceGeminiChat:     "gemini",
	}[billed.UsageSource]
	record, ok := audit.Records[protocol]
	if !ok && len(audit.Records) == 1 {
		for _, candidate := range audit.Records {
			record, ok = candidate, true
		}
	}
	if !ok || int64(billed.CompletionTokens) != record.Adjusted.Output || int64(billed.PromptTokensDetails.CachedTokens) != record.Adjusted.CacheRead {
		return // Do not fabricate a price when adapter billing cannot be matched.
	}
	original := *billed
	original.BillingUsage = nil // Already normalized, just like the actual summary.
	exclusive := usageSemanticFromUsage(info, billed) == dto.BillingUsageSemanticAnthropic
	if info.ChannelMeta != nil && info.ChannelType == constant.ChannelTypeOpenRouter {
		exclusive = false
	}
	expectedPrompt, originalPrompt := record.Adjusted.Input, record.Original.Input
	if exclusive {
		expectedPrompt, originalPrompt = record.Adjusted.Uncached, record.Original.Uncached
	}
	if int64(billed.PromptTokens) != expectedPrompt {
		return
	}
	original.PromptTokens = int(originalPrompt)
	original.InputTokens = int(record.Original.Input)
	original.CompletionTokens = int(record.Original.Output)
	original.OutputTokens = original.CompletionTokens
	original.TotalTokens = original.PromptTokens + original.CompletionTokens
	original.PromptTokensDetails.CachedTokens = int(record.Original.CacheRead)
	if record.OriginalOutputDetails != nil {
		original.CompletionTokenDetails = *record.OriginalOutputDetails
	}
	shadow := *info
	shadow.QuotaClamp = nil
	summary := calculateTextQuotaSummary(ctx, &shadow, &original)
	if snap := shadow.TieredBillingSnapshot; snap != nil && snap.BillingMode == "tiered_expr" {
		used := billingexpr.UsedVars(snap.ExprString)
		applied, quota, result := TryTieredSettle(&shadow, BuildTieredTokenParams(&original, summary.IsClaudeUsageSemantic, used))
		if !applied || result == nil {
			return // An expression failure is not an original-cost calculation.
		}
		summary.Quota = composeTieredTextQuota(&shadow, summary, quota, result)
	}
	audit.OriginalQuota = &summary.Quota
}

func HasAdjustedUsage(info *relaycommon.RelayInfo) bool {
	return info != nil && info.UsageAdjustment != nil && info.UsageAdjustment.Status == "applied"
}

// PrepareUsageAdjustment changes only recognized upstream usage fields before
// adaptors consume them. The same bytes feed downstream usage and settlement.
// No goroutine, timers, or buffering of the entire stream are introduced.
func PrepareUsageAdjustment(resp *http.Response, info *relaycommon.RelayInfo) error {
	if info == nil || info.ChannelMeta == nil {
		return nil
	}
	info.UsageAdjustment = nil
	rule := info.ChannelSetting.UsageAdjustment
	if rule == nil || !rule.Enabled {
		return nil
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	audit := &dto.UsageAdjustmentAudit{
		Rule:          *rule,
		CachePercent:  rule.CacheMin + rand.Float64()*(rule.CacheMax-rule.CacheMin),
		OutputPercent: rule.OutputMin + rand.Float64()*(rule.OutputMax-rule.OutputMin),
		Status:        "no_upstream_usage",
		Records:       make(map[string]dto.UsageAdjustmentRecord),
	}
	info.UsageAdjustment = audit
	if resp == nil || resp.Body == nil {
		return nil
	}
	contentType := resp.Header.Get("Content-Type")
	stream := strings.HasPrefix(contentType, "text/event-stream")
	if (!stream && !strings.Contains(contentType, "json")) || resp.Header.Get("Content-Encoding") != "" {
		audit.Status = "unsupported_response_format"
		return fmt.Errorf("usage adjustment requires an uncompressed JSON or SSE upstream response")
	}
	resp.Body = &meteringReader{source: resp.Body, reader: bufio.NewReader(resp.Body), stream: stream, audit: audit, info: info}
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	resp.Header.Del("ETag")
	resp.Header.Del("Content-MD5")
	return nil
}

type meteringReader struct {
	source   io.ReadCloser
	reader   *bufio.Reader
	stream   bool
	finished bool
	pending  []byte
	audit    *dto.UsageAdjustmentAudit
	info     *relaycommon.RelayInfo
}

func (r *meteringReader) Close() error { return r.source.Close() }

func (r *meteringReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(r.pending) == 0 {
		if r.finished {
			return 0, io.EOF
		}
		var err error
		if r.stream {
			r.pending, err = r.readEvent()
		} else {
			var body []byte
			body, err = io.ReadAll(io.LimitReader(r.reader, maxMeteringFrameBytes+1))
			if err == nil && len(body) > maxMeteringFrameBytes {
				err = fmt.Errorf("usage adjustment response exceeds size limit")
			}
			if err == nil {
				r.pending, err = r.adjustJSON(body)
			}
			r.finished = true
		}
		if err != nil {
			return 0, err
		}
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *meteringReader) readEvent() ([]byte, error) {
	var frame []byte
	for {
		line, err := r.reader.ReadSlice('\n')
		if len(frame)+len(line) > maxMeteringFrameBytes {
			return nil, fmt.Errorf("usage adjustment event exceeds size limit")
		}
		frame = append(frame, line...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		if err == io.EOF {
			r.finished = true
			break
		}
		if bytes.Equal(line, []byte("\n")) || bytes.Equal(line, []byte("\r\n")) {
			break
		}
	}
	lines := bytes.Split(frame, []byte("\n"))
	var data []byte
	for _, line := range lines {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if bytes.HasPrefix(line, []byte("data:")) {
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, bytes.TrimPrefix(line[5:], []byte(" "))...)
		}
	}
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return frame, nil
	}
	adjusted, err := r.adjustJSON(data)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(data, adjusted) {
		return frame, nil
	}
	var out []byte
	written := false
	for i, line := range lines {
		if bytes.HasPrefix(line, []byte("data:")) {
			if written {
				continue
			}
			out = append(out, []byte("data: ")...)
			out = append(out, adjusted...)
			written = true
		} else {
			out = append(out, line...)
		}
		if i < len(lines)-1 {
			out = append(out, '\n')
		}
	}
	return out, nil
}

func (r *meteringReader) adjustJSON(body []byte) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("invalid JSON in upstream usage adjustment response")
	}
	root := gjson.ParseBytes(body)
	if root.IsArray() {
		for i, item := range root.Array() {
			if item.IsArray() {
				return nil, fmt.Errorf("nested response arrays are not supported for usage adjustment")
			}
			path := fmt.Sprint(i)
			adjusted, err := r.adjustJSON([]byte(gjson.GetBytes(body, path).Raw))
			if err != nil {
				return nil, err
			}
			body, err = sjson.SetRawBytes(body, path, adjusted)
			if err != nil {
				return nil, err
			}
		}
		return body, nil
	}
	for _, path := range []string{"usage", "message.usage", "response.usage", "usageMetadata"} {
		u := gjson.GetBytes(body, path)
		if !u.IsObject() {
			continue
		}
		adjusted, err := r.adjustUsage([]byte(u.Raw), path == "usageMetadata", 0)
		if err != nil {
			return nil, err
		}
		body, err = sjson.SetRawBytes(body, path, adjusted)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

// adjustUsage retains unrelated fields byte-for-byte, including tool arguments
// and unknown extension fields; numeric counts are validated before arithmetic.
func (r *meteringReader) adjustUsage(body []byte, gemini bool, depth int) ([]byte, error) {
	if depth > 4 {
		return nil, fmt.Errorf("upstream billing usage nesting exceeds limit")
	}
	protocol := "anthropic"
	input, cache, write, output, total := "input_tokens", "cache_read_input_tokens", "cache_creation_input_tokens", "output_tokens", ""
	inclusive := false
	if gemini {
		protocol = "gemini"
		input = "promptTokenCount"
		cache = "cachedContentTokenCount"
		write = ""
		output = "candidatesTokenCount"
		total = "totalTokenCount"
		inclusive = true
	} else if gjson.GetBytes(body, "prompt_tokens").Exists() || gjson.GetBytes(body, "completion_tokens").Exists() {
		protocol = "openai_chat"
		input = "prompt_tokens"
		cache = "prompt_tokens_details.cached_tokens"
		write = "prompt_tokens_details.cached_creation_tokens"
		output = "completion_tokens"
		total = "total_tokens"
		inclusive = true
	} else if gjson.GetBytes(body, "input_tokens_details").Exists() || gjson.GetBytes(body, "total_tokens").Exists() {
		protocol = "openai_responses"
		cache = "input_tokens_details.cached_tokens"
		write = ""
		total = "total_tokens"
		inclusive = true
	}
	cacheAliases := []string{}
	if protocol == "openai_chat" {
		if gjson.GetBytes(body, "usage_semantic").String() == "anthropic" {
			inclusive = false
		}
		cacheAliases = []string{"prompt_tokens_details.cached_tokens", "input_tokens_details.cached_tokens", "cached_tokens", "prompt_cache_hit_tokens"}
		if gjson.GetBytes(body, cache).Int() == 0 {
			for _, alias := range cacheAliases {
				if gjson.GetBytes(body, alias).Int() > 0 {
					cache = alias
					break
				}
			}
		}
	}
	if !gjson.GetBytes(body, input).Exists() && !gjson.GetBytes(body, output).Exists() && !gjson.GetBytes(body, cache).Exists() {
		return body, nil
	}
	previous := r.audit.Records[protocol].Original
	counts := map[string]int64{input: previous.Input, cache: previous.CacheRead, output: previous.Output}
	if !inclusive {
		counts[input] = previous.Uncached
	}
	if write != "" {
		counts[write] = previous.CacheWrite
	}
	writeAlias := ""
	if protocol == "openai_chat" {
		writeAlias = "prompt_tokens_details.cache_write_tokens"
	}
	if protocol == "openai_responses" {
		writeAlias = "input_tokens_details.cache_write_tokens"
	}
	if writeAlias != "" {
		counts[writeAlias] = 0
	}
	creationDetails := []string{"cache_creation.ephemeral_5m_input_tokens", "cache_creation.ephemeral_1h_input_tokens", "claude_cache_creation_5_m_tokens", "claude_cache_creation_1_h_tokens"}
	if protocol == "anthropic" || protocol == "openai_chat" {
		for _, path := range creationDetails {
			counts[path] = 0
		}
	}
	if gemini {
		counts[input] = previous.Input - previous.ToolInput
		counts[output] = previous.Output - previous.Reasoning
		counts["thoughtsTokenCount"] = previous.Reasoning
		counts["toolUsePromptTokenCount"] = previous.ToolInput
	}
	for path := range counts {
		value := gjson.GetBytes(body, path)
		if !value.Exists() {
			continue
		}
		v := value.Float()
		if value.Type != gjson.Number || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > common.MaxQuota || math.Trunc(v) != v {
			return nil, fmt.Errorf("invalid upstream token count: %s", path)
		}
		counts[path] = value.Int()
	}
	raw := dto.MeteredTokens{Input: counts[input], CacheRead: counts[cache], CacheWrite: counts[write], Output: counts[output]}
	if writeAlias != "" {
		raw.CacheWrite = max(raw.CacheWrite, counts[writeAlias])
	}
	if raw.CacheWrite == 0 {
		raw.CacheWrite = max(counts[creationDetails[0]], counts[creationDetails[2]]) + max(counts[creationDetails[1]], counts[creationDetails[3]])
	}
	if gemini {
		raw.Reasoning = counts["thoughtsTokenCount"]
		raw.ToolInput = counts["toolUsePromptTokenCount"]
		raw.Input += raw.ToolInput
		raw.Output += raw.Reasoning
	}
	if inclusive {
		raw.Uncached = raw.Input - raw.CacheRead - raw.CacheWrite
	} else {
		raw.Uncached = raw.Input
		raw.Input += raw.CacheRead + raw.CacheWrite
	}
	if raw.Uncached < 0 || raw.Input > common.MaxQuota || raw.Output > common.MaxQuota {
		return nil, fmt.Errorf("inconsistent upstream input token counts")
	}
	adjusted := raw
	cacheCount, clamp := common.QuotaRoundChecked(float64(raw.CacheRead) * (1 + r.audit.CachePercent/100))
	if clamp != nil && r.info != nil {
		r.info.QuotaClamp = clamp
	}
	adjusted.CacheRead = min(int64(cacheCount), raw.CacheRead+raw.Uncached)
	adjusted.Uncached = raw.Input - raw.CacheWrite - adjusted.CacheRead
	outputCount, clamp := common.QuotaRoundChecked(float64(raw.Output) * (1 + r.audit.OutputPercent/100))
	if clamp != nil && r.info != nil {
		r.info.QuotaClamp = clamp
	}
	adjusted.Output = int64(outputCount)
	if gemini {
		adjusted.Reasoning = min(adjusted.Output, int64(common.QuotaRound(float64(raw.Reasoning)*(1+r.audit.OutputPercent/100))))
	}
	updates := map[string]int64{}
	originalOutputDetails := r.audit.Records[protocol].OriginalOutputDetails
	for _, path := range []string{"completion_tokens_details", "output_tokens_details"} {
		if detail := gjson.GetBytes(body, path); detail.IsObject() {
			var parsed dto.OutputTokenDetails
			if err := common.Unmarshal([]byte(detail.Raw), &parsed); err != nil {
				return nil, err
			}
			originalOutputDetails = &parsed
		}
	}
	if gjson.GetBytes(body, input).Exists() || (!inclusive && gjson.GetBytes(body, cache).Exists()) {
		updates[input] = adjusted.Input
		if gemini {
			updates[input] -= raw.ToolInput
		}
		if !inclusive {
			updates[input] = adjusted.Uncached
		}
	}
	if gjson.GetBytes(body, cache).Exists() {
		updates[cache] = adjusted.CacheRead
	}
	for _, alias := range cacheAliases {
		if gjson.GetBytes(body, alias).Exists() {
			updates[alias] = adjusted.CacheRead
		}
	}
	if protocol == "openai_chat" && gjson.GetBytes(body, "prompt_cache_miss_tokens").Exists() {
		updates["prompt_cache_miss_tokens"] = adjusted.Input - adjusted.CacheRead
	}
	if gjson.GetBytes(body, output).Exists() {
		updates[output] = adjusted.Output
		if gemini {
			updates[output] -= adjusted.Reasoning
		}
	}
	if protocol == "openai_chat" {
		if gjson.GetBytes(body, "input_tokens").Exists() {
			updates["input_tokens"] = adjusted.Input
		}
		if gjson.GetBytes(body, "output_tokens").Exists() {
			updates["output_tokens"] = adjusted.Output
		}
	}
	if gemini && gjson.GetBytes(body, "thoughtsTokenCount").Exists() {
		updates["thoughtsTokenCount"] = adjusted.Reasoning
	}
	if total != "" && gjson.GetBytes(body, total).Exists() {
		updates[total] = adjusted.Input + adjusted.Output
	}
	for _, detail := range []string{"completion_tokens_details.reasoning_tokens", "output_tokens_details.reasoning_tokens", "completion_tokens_details.text_tokens", "completion_tokens_details.audio_tokens", "completion_tokens_details.image_tokens"} {
		v := gjson.GetBytes(body, detail)
		if v.Exists() {
			if v.Type != gjson.Number || v.Float() < 0 || math.Trunc(v.Float()) != v.Float() || v.Float() > float64(raw.Output) {
				return nil, fmt.Errorf("invalid output detail token count")
			}
			updates[detail] = min(adjusted.Output, int64(common.QuotaRound(v.Float()*(1+r.audit.OutputPercent/100))))
		}
	}
	if gemini {
		for i, detail := range gjson.GetBytes(body, "candidatesTokensDetails").Array() {
			v := detail.Get("tokenCount")
			if v.Type != gjson.Number || v.Float() < 0 || math.Trunc(v.Float()) != v.Float() || v.Float() > float64(raw.Output-raw.Reasoning) {
				return nil, fmt.Errorf("invalid Gemini output detail token count")
			}
			updates[fmt.Sprintf("candidatesTokensDetails.%d.tokenCount", i)] = min(adjusted.Output-adjusted.Reasoning, int64(common.QuotaRound(v.Float()*(1+r.audit.OutputPercent/100))))
		}
	}
	for path, value := range updates {
		var err error
		body, err = sjson.SetBytes(body, path, value)
		if err != nil {
			return nil, err
		}
	}
	// Vendor billing_usage is another metering representation, not opaque metadata.
	for _, nested := range []string{"billing_usage.openai_usage", "billing_usage.claude_usage", "billing_usage.gemini_usage_metadata"} {
		u := gjson.GetBytes(body, nested)
		if !u.IsObject() {
			continue
		}
		adjustedBody, err := r.adjustUsage([]byte(u.Raw), strings.HasSuffix(nested, "gemini_usage_metadata"), depth+1)
		if err != nil {
			return nil, err
		}
		body, err = sjson.SetRawBytes(body, nested, adjustedBody)
		if err != nil {
			return nil, err
		}
	}
	r.audit.Records[protocol] = dto.UsageAdjustmentRecord{Original: raw, Adjusted: adjusted, OriginalOutputDetails: originalOutputDetails}
	r.audit.Status = "applied"
	return body, nil
}
