package model

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PerfMetric stores aggregated relay performance metrics for the model square.
type PerfMetric struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	ModelName      string `json:"model_name" gorm:"size:128;uniqueIndex:idx_perf_model_group_bucket,priority:1"`
	Group          string `json:"group" gorm:"column:group;size:64;uniqueIndex:idx_perf_model_group_bucket,priority:2"`
	BucketTs       int64  `json:"bucket_ts" gorm:"uniqueIndex:idx_perf_model_group_bucket,priority:3;index:idx_perf_bucket_ts"`
	RequestCount   int64  `json:"-" gorm:"default:0"`
	SuccessCount   int64  `json:"-" gorm:"default:0"`
	TotalLatencyMs int64  `json:"-" gorm:"default:0"`
	TtftSumMs      int64  `json:"-" gorm:"default:0"`
	TtftCount      int64  `json:"-" gorm:"default:0"`
	OutputTokens   int64  `json:"-" gorm:"default:0"`
	GenerationMs   int64  `json:"-" gorm:"default:0"`
}

func (PerfMetric) TableName() string {
	return "perf_metrics"
}

func UpsertPerfMetric(metric *PerfMetric) error {
	if metric == nil || metric.RequestCount == 0 {
		return nil
	}
	return upsertPerfMetricTx(DB, metric)
}

func UpsertPerfMetricWithTtftBins(metric *PerfMetric, bins []PerfMetricTtftBin) error {
	if metric == nil || metric.RequestCount == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := upsertPerfMetricTx(tx, metric); err != nil {
			return err
		}
		return upsertPerfMetricTtftBinsTx(tx, bins)
	})
}

func upsertPerfMetricTx(tx *gorm.DB, metric *PerfMetric) error {
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "model_name"},
			{Name: "group"},
			{Name: "bucket_ts"},
		},
		DoUpdates: clause.Set{
			{Column: clause.Column{Name: "request_count"}, Value: gorm.Expr("perf_metrics.request_count + ?", metric.RequestCount)},
			{Column: clause.Column{Name: "success_count"}, Value: gorm.Expr("perf_metrics.success_count + ?", metric.SuccessCount)},
			{Column: clause.Column{Name: "total_latency_ms"}, Value: gorm.Expr("perf_metrics.total_latency_ms + ?", metric.TotalLatencyMs)},
			{Column: clause.Column{Name: "ttft_sum_ms"}, Value: gorm.Expr("perf_metrics.ttft_sum_ms + ?", metric.TtftSumMs)},
			{Column: clause.Column{Name: "ttft_count"}, Value: gorm.Expr("perf_metrics.ttft_count + ?", metric.TtftCount)},
			{Column: clause.Column{Name: "output_tokens"}, Value: gorm.Expr("perf_metrics.output_tokens + ?", metric.OutputTokens)},
			{Column: clause.Column{Name: "generation_ms"}, Value: gorm.Expr("perf_metrics.generation_ms + ?", metric.GenerationMs)},
		},
	}).Create(metric).Error
}

func GetPerfMetrics(modelName string, group string, startTs int64, endTs int64) ([]PerfMetric, error) {
	var metrics []PerfMetric
	query := DB.Model(&PerfMetric{}).
		Where("model_name = ? AND bucket_ts >= ? AND bucket_ts <= ?", modelName, startTs, endTs)
	if group != "" {
		query = query.Where(commonGroupCol+" = ?", group)
	}
	err := query.Order("bucket_ts ASC").Find(&metrics).Error
	return metrics, err
}

type PerfMetricSummary struct {
	ModelName      string `json:"model_name"`
	RequestCount   int64  `json:"request_count"`
	SuccessCount   int64  `json:"success_count"`
	TotalLatencyMs int64  `json:"total_latency_ms"`
	OutputTokens   int64  `json:"output_tokens"`
	GenerationMs   int64  `json:"generation_ms"`
}

type PerfMetricSummaryBucket struct {
	ModelName      string `json:"model_name"`
	BucketTs       int64  `json:"bucket_ts"`
	RequestCount   int64  `json:"request_count"`
	SuccessCount   int64  `json:"success_count"`
	TotalLatencyMs int64  `json:"total_latency_ms"`
	OutputTokens   int64  `json:"output_tokens"`
	GenerationMs   int64  `json:"generation_ms"`
}

type PerfMetricStatusBucketRange struct {
	Start int64
	End   int64
}

type PerfMetricStatusBucketCounts struct {
	RequestCount int64
	SuccessCount int64
	TtftCount    int64
}

type PerfMetricStatusSeries struct {
	ModelName string
	Group     string
	Buckets   []PerfMetricStatusBucketCounts
}

func GetPerfMetricsSummaryAll(startTs int64, endTs int64, groups []string) ([]PerfMetricSummary, error) {
	var summaries []PerfMetricSummary
	query := DB.Model(&PerfMetric{}).
		Select("model_name, SUM(request_count) as request_count, SUM(success_count) as success_count, SUM(total_latency_ms) as total_latency_ms, SUM(output_tokens) as output_tokens, SUM(generation_ms) as generation_ms").
		Where("bucket_ts >= ? AND bucket_ts <= ?", startTs, endTs)
	if groups != nil {
		if len(groups) == 0 {
			return summaries, nil
		}
		query = query.Where(commonGroupCol+" IN ?", groups)
	}
	err := query.
		Group("model_name").
		Having("SUM(request_count) > 0").
		Find(&summaries).Error
	return summaries, err
}

func GetPerfMetricsSummaryBucketsAll(startTs int64, endTs int64, groups []string) ([]PerfMetricSummaryBucket, error) {
	var summaries []PerfMetricSummaryBucket
	query := DB.Model(&PerfMetric{}).
		Select("model_name, bucket_ts, SUM(request_count) as request_count, SUM(success_count) as success_count, SUM(total_latency_ms) as total_latency_ms, SUM(output_tokens) as output_tokens, SUM(generation_ms) as generation_ms").
		Where("bucket_ts >= ? AND bucket_ts <= ?", startTs, endTs)
	if groups != nil {
		if len(groups) == 0 {
			return summaries, nil
		}
		query = query.Where(commonGroupCol+" IN ?", groups)
	}
	err := query.
		Group("model_name, bucket_ts").
		Having("SUM(request_count) > 0").
		Order("bucket_ts ASC").
		Find(&summaries).Error
	return summaries, err
}

func GetPerfMetricStatusSeries(startTs int64, endTs int64, buckets []PerfMetricStatusBucketRange) ([]PerfMetricStatusSeries, error) {
	series := make([]PerfMetricStatusSeries, 0)
	if endTs <= startTs || len(buckets) == 0 {
		return series, nil
	}

	selectParts := []string{"model_name", commonGroupCol}
	for _, bucket := range buckets {
		condition := "bucket_ts >= " + strconv.FormatInt(bucket.Start, 10) + " AND bucket_ts < " + strconv.FormatInt(bucket.End, 10)
		selectParts = append(selectParts,
			"COALESCE(SUM(CASE WHEN "+condition+" THEN request_count ELSE 0 END), 0)",
			"COALESCE(SUM(CASE WHEN "+condition+" THEN success_count ELSE 0 END), 0)",
			"COALESCE(SUM(CASE WHEN "+condition+" THEN ttft_count ELSE 0 END), 0)",
		)
	}

	rows, err := DB.Model(&PerfMetric{}).
		Select(strings.Join(selectParts, ", ")).
		Where("bucket_ts >= ? AND bucket_ts < ?", startTs, endTs).
		Where("model_name <> '' AND " + commonGroupCol + " <> ''").
		Group("model_name, " + commonGroupCol).
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var modelName string
		var group string
		values := make([]sql.NullInt64, len(buckets)*3)
		scanArgs := make([]any, 0, len(values)+2)
		scanArgs = append(scanArgs, &modelName, &group)
		for index := range values {
			scanArgs = append(scanArgs, &values[index])
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, err
		}

		bucketCounts := make([]PerfMetricStatusBucketCounts, len(buckets))
		for index := range buckets {
			offset := index * 3
			bucketCounts[index] = PerfMetricStatusBucketCounts{
				RequestCount: values[offset].Int64,
				SuccessCount: values[offset+1].Int64,
				TtftCount:    values[offset+2].Int64,
			}
		}
		series = append(series, PerfMetricStatusSeries{
			ModelName: modelName,
			Group:     group,
			Buckets:   bucketCounts,
		})
	}
	return series, rows.Err()
}

func DeletePerfMetricsBefore(cutoffTs int64) error {
	if cutoffTs <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("bucket_ts < ?", cutoffTs).Delete(&PerfMetricTtftBin{}).Error; err != nil {
			return err
		}
		return tx.Where("bucket_ts < ?", cutoffTs).Delete(&PerfMetric{}).Error
	})
}

func PerfMetricStartTime(hours int) int64 {
	if hours <= 0 {
		hours = 24
	}
	return time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
}
