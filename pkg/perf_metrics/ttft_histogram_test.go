package perfmetrics

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTtftHistogramBinIndexUsesBoundedIntervals(t *testing.T) {
	assert.Equal(t, 0, ttftHistogramBinIndex(0))
	assert.Equal(t, 1, ttftHistogramBinIndex(10))
	assert.Equal(t, 2, ttftHistogramBinIndex(11))
	assert.Equal(t, ttftHistogramBinCount-1, ttftHistogramBinIndex(1_200_001))
}

func TestAddTtftHistogramSampleRejectsNegativeAndSaturates(t *testing.T) {
	histogram := [ttftHistogramBinCount]int64{}
	addTtftHistogramSample(&histogram, -1)
	assert.Zero(t, histogram[0])

	histogram[1] = math.MaxInt64
	addTtftHistogramSample(&histogram, 10)
	assert.Equal(t, int64(math.MaxInt64), histogram[1])
}

func TestTtftHistogramMedianUsesBucketValues(t *testing.T) {
	histogram := histogramWithSamples(101, 500, 900)
	median := ttftHistogramMedian(histogram)
	require.NotNil(t, median)
	assert.Equal(t, int64(500), *median)
}
