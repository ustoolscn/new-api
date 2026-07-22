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
		{ModelName: "model-a", Group: "default", BucketTs: start.Unix(), RequestCount: 10, SuccessCount: 9, TtftSumMs: 600, TtftCount: 3, TtftMinMs: 100, TtftMaxMs: 300, TtftExtremaCount: 3},
		{ModelName: "model-a", Group: "default", BucketTs: start.Add(30 * time.Minute).Unix(), RequestCount: 2, SuccessCount: 2, TtftSumMs: 90, TtftCount: 2, TtftMinMs: 40, TtftMaxMs: 50, TtftExtremaCount: 2},
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
	assert.Equal(t, int64(690), defaultModel.Buckets[0].TtftSumMs)
	assert.Equal(t, int64(5), defaultModel.Buckets[0].TtftCount)
	assert.Equal(t, int64(40), defaultModel.Buckets[0].TtftMinMs)
	assert.Equal(t, int64(300), defaultModel.Buckets[0].TtftMaxMs)
	assert.Equal(t, int64(5), defaultModel.Buckets[0].TtftExtremaCount)
	assert.Zero(t, defaultModel.Buckets[1].RequestCount)

	premiumModel := byDimension["model-a/premium"]
	assert.Equal(t, int64(5), premiumModel.Buckets[1].RequestCount)
	assert.Equal(t, int64(5), premiumModel.Buckets[1].SuccessCount)

	failedModel := byDimension["model-b/default"]
	assert.Equal(t, int64(4), failedModel.Buckets[1].RequestCount)
	assert.Equal(t, int64(1), failedModel.Buckets[1].SuccessCount)
}

func TestUpsertPerfMetricMergesTtftExtrema(t *testing.T) {
	truncateTables(t)

	metric := &PerfMetric{
		ModelName:        "model-a",
		Group:            "default",
		BucketTs:         time.Now().Unix(),
		RequestCount:     2,
		SuccessCount:     2,
		TtftSumMs:        500,
		TtftCount:        2,
		TtftMinMs:        200,
		TtftMaxMs:        300,
		TtftExtremaCount: 2,
	}
	require.NoError(t, UpsertPerfMetric(metric))

	metric.Id = 0
	metric.RequestCount = 3
	metric.SuccessCount = 2
	metric.TtftSumMs = 1000
	metric.TtftCount = 3
	metric.TtftMinMs = 100
	metric.TtftMaxMs = 600
	metric.TtftExtremaCount = 3
	require.NoError(t, UpsertPerfMetric(metric))

	var stored PerfMetric
	require.NoError(t, DB.Where("model_name = ? AND "+commonGroupCol+" = ? AND bucket_ts = ?", metric.ModelName, metric.Group, metric.BucketTs).First(&stored).Error)
	assert.Equal(t, int64(5), stored.RequestCount)
	assert.Equal(t, int64(1500), stored.TtftSumMs)
	assert.Equal(t, int64(5), stored.TtftCount)
	assert.Equal(t, int64(100), stored.TtftMinMs)
	assert.Equal(t, int64(600), stored.TtftMaxMs)
	assert.Equal(t, int64(5), stored.TtftExtremaCount)
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
}
