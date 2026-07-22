package perfmetrics

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeServiceStatusGranularityDefaultsToHour(t *testing.T) {
	assert.Equal(t, ServiceStatusGranularityHour, normalizeServiceStatusGranularity(""))
	assert.Equal(t, ServiceStatusGranularityHour, normalizeServiceStatusGranularity("invalid"))
	assert.Equal(t, ServiceStatusGranularityDay, normalizeServiceStatusGranularity("DAY"))
}

func TestServiceStatusMetricUsesPublicRequestDisplaySettings(t *testing.T) {
	accumulator := newServiceStatusAccumulator(1)
	accumulator.add(0, counters{
		requestCount:     10,
		successCount:     8,
		ttftSumMs:        600,
		ttftCount:        3,
		ttftMinMs:        100,
		ttftMaxMs:        300,
		ttftExtremaCount: 3,
	})

	metric := serviceStatusMetric(
		"model-a",
		accumulator,
		[]ServiceStatusPeriod{{BucketStart: 123}},
		common.PublicDisplaySettings{Multiplier: 2},
		"model",
	)

	assert.Equal(t, int64(20), metric.RequestCount)
	assert.Equal(t, int64(16), metric.SuccessCount)
	require.NotNil(t, metric.SuccessRate)
	assert.Equal(t, 80.0, *metric.SuccessRate)
	require.NotNil(t, metric.AvgTtftMs)
	assert.Equal(t, int64(200), *metric.AvgTtftMs)
}

func TestServiceStatusAverageTtft(t *testing.T) {
	tests := []struct {
		name     string
		value    counters
		expected *int64
	}{
		{name: "no samples", value: counters{}, expected: nil},
		{name: "one sample", value: counters{ttftSumMs: 100, ttftCount: 1, ttftMinMs: 100, ttftMaxMs: 100, ttftExtremaCount: 1}, expected: int64Pointer(100)},
		{name: "two samples", value: counters{ttftSumMs: 300, ttftCount: 2, ttftMinMs: 100, ttftMaxMs: 200, ttftExtremaCount: 2}, expected: int64Pointer(150)},
		{name: "trim highest and lowest", value: counters{ttftSumMs: 1000, ttftCount: 4, ttftMinMs: 100, ttftMaxMs: 500, ttftExtremaCount: 4}, expected: int64Pointer(200)},
		{name: "legacy extrema coverage falls back to average", value: counters{ttftSumMs: 1000, ttftCount: 4, ttftMinMs: 100, ttftMaxMs: 500, ttftExtremaCount: 2}, expected: int64Pointer(250)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := serviceStatusAverageTtft(tt.value)
			if tt.expected == nil {
				assert.Nil(t, actual)
				return
			}
			require.NotNil(t, actual)
			assert.Equal(t, *tt.expected, *actual)
		})
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}
