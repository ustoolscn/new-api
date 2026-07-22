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

func TestServiceStatusPointUsesMedianTtftResponseField(t *testing.T) {
	payload, err := common.Marshal(ServiceStatusPoint{
		RequestCount: 1,
		SuccessCount: 1,
		MedianTtftMs: int64Pointer(100),
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"request_count":1,"success_count":1,"success_rate":null,"median_ttft_ms":100}`, string(payload))
}

func TestServiceStatusMetricUsesPublicRequestDisplaySettings(t *testing.T) {
	accumulator := newServiceStatusAccumulator(1)
	value := counters{requestCount: 10, successCount: 8, ttftCount: 3}
	addTtftHistogramSample(&value.ttftHistogram, 100)
	addTtftHistogramSample(&value.ttftHistogram, 200)
	addTtftHistogramSample(&value.ttftHistogram, 300)
	accumulator.add(0, value)

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
	require.NotNil(t, metric.MedianTtftMs)
	assert.Equal(t, int64(200), *metric.MedianTtftMs)
}

func TestServiceStatusMedianTtft(t *testing.T) {
	tests := []struct {
		name     string
		value    counters
		expected *int64
	}{
		{name: "no samples", value: counters{}, expected: nil},
		{name: "one sample", value: countersWithTtftSamples(100), expected: int64Pointer(100)},
		{name: "odd samples", value: countersWithTtftSamples(10, 100, 500), expected: int64Pointer(100)},
		{name: "even samples", value: countersWithTtftSamples(100, 200), expected: int64Pointer(150)},
		{name: "legacy aggregate without histogram", value: counters{ttftSumMs: 300, ttftCount: 2}, expected: nil},
		{name: "partial histogram coverage", value: counters{ttftCount: 3, ttftHistogram: histogramWithSamples(100, 200)}, expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := serviceStatusMedianTtft(tt.value)
			if tt.expected == nil {
				assert.Nil(t, actual)
				return
			}
			require.NotNil(t, actual)
			assert.Equal(t, *tt.expected, *actual)
		})
	}
}

func countersWithTtftSamples(values ...int64) counters {
	return counters{
		ttftCount:     int64(len(values)),
		ttftHistogram: histogramWithSamples(values...),
	}
}

func histogramWithSamples(values ...int64) [ttftHistogramBinCount]int64 {
	histogram := [ttftHistogramBinCount]int64{}
	for _, value := range values {
		addTtftHistogramSample(&histogram, value)
	}
	return histogram
}

func int64Pointer(value int64) *int64 {
	return &value
}
