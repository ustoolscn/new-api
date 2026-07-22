package perfmetrics

import (
	"sync"
	"sync/atomic"
)

type Store interface {
	Record(sample Sample)
	Query(params QueryParams) (QueryResult, error)
}

type Sample struct {
	Model        string
	Group        string
	LatencyMs    int64
	TtftMs       int64
	HasTtft      bool
	Success      bool
	OutputTokens int64
	GenerationMs int64
}

type QueryParams struct {
	Model string
	Group string
	Hours int
}

type BucketPoint struct {
	Ts           int64   `json:"ts"`
	AvgTtftMs    int64   `json:"avg_ttft_ms"`
	AvgLatencyMs int64   `json:"avg_latency_ms"`
	SuccessRate  float64 `json:"success_rate"`
	AvgTps       float64 `json:"avg_tps"`
}

type GroupResult struct {
	Group        string        `json:"group"`
	AvgTtftMs    int64         `json:"avg_ttft_ms"`
	AvgLatencyMs int64         `json:"avg_latency_ms"`
	SuccessRate  float64       `json:"success_rate"`
	AvgTps       float64       `json:"avg_tps"`
	Series       []BucketPoint `json:"series"`
}

type QueryResult struct {
	ModelName    string        `json:"model_name"`
	SeriesSchema string        `json:"series_schema"`
	Groups       []GroupResult `json:"groups"`
}

type ModelSummary struct {
	ModelName          string    `json:"model_name"`
	AvgLatencyMs       int64     `json:"avg_latency_ms"`
	SuccessRate        float64   `json:"success_rate"`
	AvgTps             float64   `json:"avg_tps"`
	RecentSuccessRates []float64 `json:"recent_success_rates,omitempty"`
	RequestCount       int64     `json:"-"`
}

type SummaryAllResult struct {
	Models []ModelSummary `json:"models"`
}

type bucketKey struct {
	model    string
	group    string
	bucketTs int64
}

type counters struct {
	requestCount     int64
	successCount     int64
	totalLatencyMs   int64
	ttftSumMs        int64
	ttftCount        int64
	ttftMinMs        int64
	ttftMaxMs        int64
	ttftExtremaCount int64
	outputTokens     int64
	generationMs     int64
}

type atomicBucket struct {
	requestCount     atomic.Int64
	successCount     atomic.Int64
	totalLatencyMs   atomic.Int64
	ttftMu           sync.Mutex
	ttftSumMs        int64
	ttftCount        int64
	ttftMinMs        int64
	ttftMaxMs        int64
	ttftExtremaCount int64
	outputTokens     atomic.Int64
	generationMs     atomic.Int64
}

func (b *atomicBucket) add(sample Sample) {
	b.requestCount.Add(1)
	if sample.Success {
		b.successCount.Add(1)
	}
	if sample.LatencyMs > 0 {
		b.totalLatencyMs.Add(sample.LatencyMs)
	}
	if sample.HasTtft && sample.TtftMs >= 0 {
		b.ttftMu.Lock()
		b.ttftSumMs += sample.TtftMs
		if b.ttftCount == 0 || sample.TtftMs < b.ttftMinMs {
			b.ttftMinMs = sample.TtftMs
		}
		if b.ttftCount == 0 || sample.TtftMs > b.ttftMaxMs {
			b.ttftMaxMs = sample.TtftMs
		}
		b.ttftCount++
		b.ttftExtremaCount++
		b.ttftMu.Unlock()
	}
	if sample.OutputTokens > 0 && sample.GenerationMs > 0 {
		b.outputTokens.Add(sample.OutputTokens)
		b.generationMs.Add(sample.GenerationMs)
	}
}

func (b *atomicBucket) snapshot() counters {
	b.ttftMu.Lock()
	ttftSumMs := b.ttftSumMs
	ttftCount := b.ttftCount
	ttftMinMs := b.ttftMinMs
	ttftMaxMs := b.ttftMaxMs
	ttftExtremaCount := b.ttftExtremaCount
	b.ttftMu.Unlock()
	return counters{
		requestCount:     b.requestCount.Load(),
		successCount:     b.successCount.Load(),
		totalLatencyMs:   b.totalLatencyMs.Load(),
		ttftSumMs:        ttftSumMs,
		ttftCount:        ttftCount,
		ttftMinMs:        ttftMinMs,
		ttftMaxMs:        ttftMaxMs,
		ttftExtremaCount: ttftExtremaCount,
		outputTokens:     b.outputTokens.Load(),
		generationMs:     b.generationMs.Load(),
	}
}

func (b *atomicBucket) drain() counters {
	b.ttftMu.Lock()
	ttftSumMs := b.ttftSumMs
	ttftCount := b.ttftCount
	ttftMinMs := b.ttftMinMs
	ttftMaxMs := b.ttftMaxMs
	ttftExtremaCount := b.ttftExtremaCount
	b.ttftSumMs = 0
	b.ttftCount = 0
	b.ttftMinMs = 0
	b.ttftMaxMs = 0
	b.ttftExtremaCount = 0
	b.ttftMu.Unlock()
	return counters{
		requestCount:     b.requestCount.Swap(0),
		successCount:     b.successCount.Swap(0),
		totalLatencyMs:   b.totalLatencyMs.Swap(0),
		ttftSumMs:        ttftSumMs,
		ttftCount:        ttftCount,
		ttftMinMs:        ttftMinMs,
		ttftMaxMs:        ttftMaxMs,
		ttftExtremaCount: ttftExtremaCount,
		outputTokens:     b.outputTokens.Swap(0),
		generationMs:     b.generationMs.Swap(0),
	}
}

func (b *atomicBucket) addCounters(c counters) {
	if c.requestCount != 0 {
		b.requestCount.Add(c.requestCount)
	}
	if c.successCount != 0 {
		b.successCount.Add(c.successCount)
	}
	if c.totalLatencyMs != 0 {
		b.totalLatencyMs.Add(c.totalLatencyMs)
	}
	if c.ttftCount > 0 {
		b.ttftMu.Lock()
		current := counters{
			ttftSumMs:        b.ttftSumMs,
			ttftCount:        b.ttftCount,
			ttftMinMs:        b.ttftMinMs,
			ttftMaxMs:        b.ttftMaxMs,
			ttftExtremaCount: b.ttftExtremaCount,
		}
		mergeCounterValues(&current, counters{
			ttftSumMs:        c.ttftSumMs,
			ttftCount:        c.ttftCount,
			ttftMinMs:        c.ttftMinMs,
			ttftMaxMs:        c.ttftMaxMs,
			ttftExtremaCount: c.ttftExtremaCount,
		})
		b.ttftSumMs = current.ttftSumMs
		b.ttftCount = current.ttftCount
		b.ttftMinMs = current.ttftMinMs
		b.ttftMaxMs = current.ttftMaxMs
		b.ttftExtremaCount = current.ttftExtremaCount
		b.ttftMu.Unlock()
	}
	if c.outputTokens != 0 {
		b.outputTokens.Add(c.outputTokens)
	}
	if c.generationMs != 0 {
		b.generationMs.Add(c.generationMs)
	}
}

func mergeCounterValues(current *counters, value counters) {
	if current == nil {
		return
	}
	current.requestCount += value.requestCount
	current.successCount += value.successCount
	current.totalLatencyMs += value.totalLatencyMs
	current.ttftSumMs += value.ttftSumMs
	current.ttftCount += value.ttftCount
	current.outputTokens += value.outputTokens
	current.generationMs += value.generationMs

	extremaCount := value.ttftExtremaCount
	if extremaCount <= 0 || value.ttftCount <= 0 || value.ttftMinMs < 0 || value.ttftMaxMs < value.ttftMinMs {
		return
	}
	if extremaCount > value.ttftCount {
		extremaCount = value.ttftCount
	}
	if current.ttftExtremaCount == 0 {
		current.ttftMinMs = value.ttftMinMs
		current.ttftMaxMs = value.ttftMaxMs
	} else {
		if value.ttftMinMs < current.ttftMinMs {
			current.ttftMinMs = value.ttftMinMs
		}
		if value.ttftMaxMs > current.ttftMaxMs {
			current.ttftMaxMs = value.ttftMaxMs
		}
	}
	current.ttftExtremaCount += extremaCount
}
