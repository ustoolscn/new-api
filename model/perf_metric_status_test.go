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
		{ModelName: "model-a", Group: "default", BucketTs: start.Unix(), RequestCount: 10, SuccessCount: 9},
		{ModelName: "model-a", Group: "default", BucketTs: start.Add(30 * time.Minute).Unix(), RequestCount: 2, SuccessCount: 2},
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
	assert.Zero(t, defaultModel.Buckets[1].RequestCount)

	premiumModel := byDimension["model-a/premium"]
	assert.Equal(t, int64(5), premiumModel.Buckets[1].RequestCount)
	assert.Equal(t, int64(5), premiumModel.Buckets[1].SuccessCount)

	failedModel := byDimension["model-b/default"]
	assert.Equal(t, int64(4), failedModel.Buckets[1].RequestCount)
	assert.Equal(t, int64(1), failedModel.Buckets[1].SuccessCount)
}
