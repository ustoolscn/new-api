package perfmetrics

import "math"

const ttftHistogramBinCount = 38

var ttftHistogramUpperBoundsMs = [ttftHistogramBinCount]int64{
	0, 10, 20, 30, 40, 50, 75, 100, 125, 150,
	200, 250, 300, 400, 500, 750, 1_000, 1_250, 1_500, 2_000,
	2_500, 3_000, 4_000, 5_000, 7_500, 10_000, 15_000, 20_000, 30_000, 45_000,
	60_000, 90_000, 120_000, 180_000, 300_000, 600_000, 900_000, 1_200_000,
}

func ttftHistogramBinIndex(value int64) int {
	for index, upperBound := range ttftHistogramUpperBoundsMs {
		if value <= upperBound {
			return index
		}
	}
	return ttftHistogramBinCount - 1
}

func addTtftHistogramSample(histogram *[ttftHistogramBinCount]int64, value int64) {
	if histogram == nil || value < 0 {
		return
	}
	index := ttftHistogramBinIndex(value)
	if histogram[index] < math.MaxInt64 {
		histogram[index]++
	}
}

func ttftHistogramMedian(histogram [ttftHistogramBinCount]int64) *int64 {
	total := ttftHistogramSampleCount(histogram)
	if total <= 0 {
		return nil
	}

	lower := ttftHistogramValueAtRank(histogram, (total-1)/2)
	upper := ttftHistogramValueAtRank(histogram, total/2)
	median := lower + (upper-lower)/2
	return &median
}

func ttftHistogramSampleCount(histogram [ttftHistogramBinCount]int64) int64 {
	total := int64(0)
	for _, count := range histogram {
		if count <= 0 {
			continue
		}
		if total >= math.MaxInt64-count {
			total = math.MaxInt64
			break
		}
		total += count
	}
	return total
}

func ttftHistogramValueAtRank(histogram [ttftHistogramBinCount]int64, rank int64) int64 {
	cumulative := int64(0)
	for index, count := range histogram {
		if count <= 0 {
			continue
		}
		if cumulative >= math.MaxInt64-count || cumulative+count > rank {
			return ttftHistogramUpperBoundsMs[index]
		}
		cumulative += count
	}
	return ttftHistogramUpperBoundsMs[ttftHistogramBinCount-1]
}
