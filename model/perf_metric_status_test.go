package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPerfMetricStatusSeriesAggregatesByModelGroupAndBucket(t *testing.T) {
	truncateTables(t)

	start := time.Date(2026, 7, 20, 8, 0, 0, 0, time.Local)
	metrics := []PerfMetric{
		{ModelName: "model-a", Group: "default", BucketTs: start.Unix(), RequestCount: 10, SuccessCount: 9, TtftSumMs: 600, TtftCount: 3},
		{ModelName: "model-a", Group: "default", BucketTs: start.Add(30 * time.Minute).Unix(), RequestCount: 2, SuccessCount: 2, TtftSumMs: 90, TtftCount: 2},
		{ModelName: "model-a", Group: "premium", BucketTs: start.Add(time.Hour).Unix(), RequestCount: 5, SuccessCount: 5},
		{ModelName: "model-b", Group: "default", BucketTs: start.Add(time.Hour).Unix(), RequestCount: 4, SuccessCount: 1},
		{ModelName: "outside", Group: "default", BucketTs: start.Add(2 * time.Hour).Unix(), RequestCount: 20, SuccessCount: 20},
	}
	for index := range metrics {
		require.NoError(t, DB.Create(&metrics[index]).Error)
	}

	series, err := GetPerfMetricStatusSeries(start.Unix(), start.Add(2*time.Hour).Unix(), []PerfMetricStatusBucketRange{
		{Start: start.Unix(), End: start.Add(time.Hour).Unix()},
		{Start: start.Add(time.Hour).Unix(), End: start.Add(2 * time.Hour).Unix()},
	})
	require.NoError(t, err)
	require.Len(t, series, 3)

	byDimension := make(map[string]PerfMetricStatusSeries, len(series))
	for _, item := range series {
		byDimension[item.ModelName+"/"+item.Group] = item
	}

	defaultModel := byDimension["model-a/default"]
	require.Len(t, defaultModel.Buckets, 2)
	assert.Equal(t, int64(12), defaultModel.Buckets[0].RequestCount)
	assert.Equal(t, int64(11), defaultModel.Buckets[0].SuccessCount)
	assert.Equal(t, int64(5), defaultModel.Buckets[0].TtftCount)
	assert.Zero(t, defaultModel.Buckets[1].RequestCount)

	premiumModel := byDimension["model-a/premium"]
	assert.Equal(t, int64(5), premiumModel.Buckets[1].RequestCount)
	assert.Equal(t, int64(5), premiumModel.Buckets[1].SuccessCount)

	failedModel := byDimension["model-b/default"]
	assert.Equal(t, int64(4), failedModel.Buckets[1].RequestCount)
	assert.Equal(t, int64(1), failedModel.Buckets[1].SuccessCount)
}

func TestUpsertPerfMetricWithTtftBinsMergesHistogramCounts(t *testing.T) {
	truncateTables(t)

	bucketTs := time.Now().Unix()
	metric := &PerfMetric{
		ModelName:    "model-a",
		Group:        "default",
		BucketTs:     bucketTs,
		RequestCount: 2,
		SuccessCount: 2,
	}
	require.NoError(t, UpsertPerfMetricWithTtftBins(metric, []PerfMetricTtftBin{
		{ModelName: "model-a", Group: "default", BucketTs: bucketTs, BinIndex: 10, SampleCount: 1},
		{ModelName: "model-a", Group: "default", BucketTs: bucketTs, BinIndex: 11, SampleCount: 1},
	}))

	metric.Id = 0
	metric.RequestCount = 3
	metric.SuccessCount = 2
	require.NoError(t, UpsertPerfMetricWithTtftBins(metric, []PerfMetricTtftBin{
		{ModelName: "model-a", Group: "default", BucketTs: bucketTs, BinIndex: 10, SampleCount: 2},
		{ModelName: "model-a", Group: "default", BucketTs: bucketTs, BinIndex: 12, SampleCount: 1},
	}))

	var storedMetric PerfMetric
	require.NoError(t, DB.Where("model_name = ? AND "+commonGroupCol+" = ? AND bucket_ts = ?", "model-a", "default", bucketTs).First(&storedMetric).Error)
	assert.Equal(t, int64(5), storedMetric.RequestCount)
	assert.Equal(t, int64(4), storedMetric.SuccessCount)

	var storedBins []PerfMetricTtftBin
	require.NoError(t, DB.Where("model_name = ? AND "+commonGroupCol+" = ? AND bucket_ts = ?", "model-a", "default", bucketTs).Order("bin_index ASC").Find(&storedBins).Error)
	require.Len(t, storedBins, 3)
	assert.Equal(t, 10, storedBins[0].BinIndex)
	assert.Equal(t, int64(3), storedBins[0].SampleCount)
	assert.Equal(t, 11, storedBins[1].BinIndex)
	assert.Equal(t, int64(1), storedBins[1].SampleCount)
	assert.Equal(t, 12, storedBins[2].BinIndex)
	assert.Equal(t, int64(1), storedBins[2].SampleCount)
}

func TestGetPerfMetricTtftHistogramSeriesAggregatesDimensionsAndBuckets(t *testing.T) {
	truncateTables(t)

	start := time.Date(2026, 7, 20, 8, 0, 0, 0, time.Local)
	bins := []PerfMetricTtftBin{
		{ModelName: "model-a", Group: "default", BucketTs: start.Unix(), BinIndex: 10, SampleCount: 2},
		{ModelName: "model-a", Group: "premium", BucketTs: start.Add(30 * time.Minute).Unix(), BinIndex: 10, SampleCount: 1},
		{ModelName: "model-b", Group: "default", BucketTs: start.Add(time.Hour).Unix(), BinIndex: 20, SampleCount: 3},
		{ModelName: "outside", Group: "default", BucketTs: start.Add(2 * time.Hour).Unix(), BinIndex: 30, SampleCount: 4},
	}
	require.NoError(t, DB.Create(&bins).Error)
	buckets := []PerfMetricStatusBucketRange{
		{Start: start.Unix(), End: start.Add(time.Hour).Unix()},
		{Start: start.Add(time.Hour).Unix(), End: start.Add(2 * time.Hour).Unix()},
	}

	overall, err := GetPerfMetricTtftHistogramOverallSeries(start.Unix(), start.Add(2*time.Hour).Unix(), buckets)
	require.NoError(t, err)
	overallByBin := make(map[int][]int64, len(overall))
	for _, item := range overall {
		overallByBin[item.BinIndex] = item.Buckets
	}
	assert.Equal(t, []int64{3, 0}, overallByBin[10])
	assert.Equal(t, []int64{0, 3}, overallByBin[20])

	models, err := GetPerfMetricTtftHistogramModelSeries(start.Unix(), start.Add(2*time.Hour).Unix(), buckets, []string{"model-a"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "model-a", models[0].Name)
	assert.Equal(t, 10, models[0].BinIndex)
	assert.Equal(t, []int64{3, 0}, models[0].Buckets)

	groups, err := GetPerfMetricTtftHistogramGroupSeries(start.Unix(), start.Add(2*time.Hour).Unix(), buckets, []string{"default"})
	require.NoError(t, err)
	groupsByBin := make(map[int][]int64, len(groups))
	for _, item := range groups {
		assert.Equal(t, "default", item.Name)
		groupsByBin[item.BinIndex] = item.Buckets
	}
	assert.Equal(t, []int64{2, 0}, groupsByBin[10])
	assert.Equal(t, []int64{0, 3}, groupsByBin[20])
}

func TestUpsertPerfMetricMergesTtftCounters(t *testing.T) {
	truncateTables(t)

	metric := &PerfMetric{
		ModelName:    "model-a",
		Group:        "default",
		BucketTs:     time.Now().Unix(),
		RequestCount: 2,
		SuccessCount: 2,
		TtftSumMs:    500,
		TtftCount:    2,
	}
	require.NoError(t, UpsertPerfMetric(metric))

	metric.Id = 0
	metric.RequestCount = 3
	metric.SuccessCount = 2
	metric.TtftSumMs = 1000
	metric.TtftCount = 3
	require.NoError(t, UpsertPerfMetric(metric))

	var stored PerfMetric
	require.NoError(t, DB.Where("model_name = ? AND "+commonGroupCol+" = ? AND bucket_ts = ?", metric.ModelName, metric.Group, metric.BucketTs).First(&stored).Error)
	assert.Equal(t, int64(5), stored.RequestCount)
	assert.Equal(t, int64(1500), stored.TtftSumMs)
	assert.Equal(t, int64(5), stored.TtftCount)
}

func TestGetPerfMetricStatusSeriesSupportsDailyBucketCount(t *testing.T) {
	truncateTables(t)

	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)
	require.NoError(t, DB.Create(&PerfMetric{
		ModelName:    "model-a",
		Group:        "default",
		BucketTs:     start.Unix(),
		RequestCount: 1,
		SuccessCount: 1,
	}).Error)
	require.NoError(t, DB.Create(&PerfMetricTtftBin{
		ModelName:   "model-a",
		Group:       "default",
		BucketTs:    start.Unix(),
		BinIndex:    10,
		SampleCount: 1,
	}).Error)

	buckets := make([]PerfMetricStatusBucketRange, 0, 90)
	for index := 0; index < 90; index++ {
		bucketStart := start.AddDate(0, 0, index)
		buckets = append(buckets, PerfMetricStatusBucketRange{
			Start: bucketStart.Unix(),
			End:   bucketStart.AddDate(0, 0, 1).Unix(),
		})
	}

	series, err := GetPerfMetricStatusSeries(start.Unix(), start.AddDate(0, 0, 90).Unix(), buckets)
	require.NoError(t, err)
	require.Len(t, series, 1)
	require.Len(t, series[0].Buckets, 90)
	assert.Equal(t, int64(1), series[0].Buckets[0].RequestCount)
	assert.Zero(t, series[0].Buckets[89].RequestCount)

	histograms, err := GetPerfMetricTtftHistogramOverallSeries(start.Unix(), start.AddDate(0, 0, 90).Unix(), buckets)
	require.NoError(t, err)
	require.Len(t, histograms, 1)
	require.Len(t, histograms[0].Buckets, 90)
	assert.Equal(t, int64(1), histograms[0].Buckets[0])
	assert.Zero(t, histograms[0].Buckets[89])
}

func TestDeletePerfMetricsBeforeDeletesHistogramRows(t *testing.T) {
	truncateTables(t)

	cutoff := time.Now().Unix()
	metrics := []PerfMetric{
		{ModelName: "old", Group: "default", BucketTs: cutoff - 1, RequestCount: 1},
		{ModelName: "new", Group: "default", BucketTs: cutoff, RequestCount: 1},
	}
	bins := []PerfMetricTtftBin{
		{ModelName: "old", Group: "default", BucketTs: cutoff - 1, BinIndex: 10, SampleCount: 1},
		{ModelName: "new", Group: "default", BucketTs: cutoff, BinIndex: 10, SampleCount: 1},
	}
	require.NoError(t, DB.Create(&metrics).Error)
	require.NoError(t, DB.Create(&bins).Error)
	require.NoError(t, DeletePerfMetricsBefore(cutoff))

	var metricCount int64
	require.NoError(t, DB.Model(&PerfMetric{}).Count(&metricCount).Error)
	assert.Equal(t, int64(1), metricCount)
	var binCount int64
	require.NoError(t, DB.Model(&PerfMetricTtftBin{}).Count(&binCount).Error)
	assert.Equal(t, int64(1), binCount)
}
