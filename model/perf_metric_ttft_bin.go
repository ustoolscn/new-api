package model

import (
	"database/sql"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PerfMetricTtftBin struct {
	Id          int    `json:"id" gorm:"primaryKey"`
	ModelName   string `json:"model_name" gorm:"size:128;uniqueIndex:idx_perf_ttft_model_group_bucket_bin,priority:1;index:idx_perf_ttft_model_bucket,priority:1"`
	Group       string `json:"group" gorm:"column:group;size:64;uniqueIndex:idx_perf_ttft_model_group_bucket_bin,priority:2;index:idx_perf_ttft_group_bucket,priority:1"`
	BucketTs    int64  `json:"bucket_ts" gorm:"uniqueIndex:idx_perf_ttft_model_group_bucket_bin,priority:3;index:idx_perf_ttft_bucket_ts;index:idx_perf_ttft_model_bucket,priority:2;index:idx_perf_ttft_group_bucket,priority:2"`
	BinIndex    int    `json:"bin_index" gorm:"uniqueIndex:idx_perf_ttft_model_group_bucket_bin,priority:4"`
	SampleCount int64  `json:"sample_count" gorm:"default:0"`
}

func (PerfMetricTtftBin) TableName() string {
	return "perf_metric_ttft_bins"
}

type PerfMetricTtftHistogramSeries struct {
	Name     string
	BinIndex int
	Buckets  []int64
}

func upsertPerfMetricTtftBinsTx(tx *gorm.DB, bins []PerfMetricTtftBin) error {
	for index := range bins {
		bin := &bins[index]
		if bin.ModelName == "" || bin.Group == "" || bin.BucketTs <= 0 || bin.BinIndex < 0 || bin.SampleCount <= 0 {
			continue
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "model_name"},
				{Name: "group"},
				{Name: "bucket_ts"},
				{Name: "bin_index"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"sample_count": gorm.Expr("perf_metric_ttft_bins.sample_count + ?", bin.SampleCount),
			}),
		}).Create(bin).Error; err != nil {
			return err
		}
	}
	return nil
}

func GetPerfMetricTtftHistogramOverallSeries(startTs int64, endTs int64, buckets []PerfMetricStatusBucketRange) ([]PerfMetricTtftHistogramSeries, error) {
	return getPerfMetricTtftHistogramStatusSeries(startTs, endTs, buckets, "", nil)
}

func GetPerfMetricTtftHistogramModelSeries(startTs int64, endTs int64, buckets []PerfMetricStatusBucketRange, modelNames []string) ([]PerfMetricTtftHistogramSeries, error) {
	return getPerfMetricTtftHistogramStatusSeries(startTs, endTs, buckets, "model_name", modelNames)
}

func GetPerfMetricTtftHistogramGroupSeries(startTs int64, endTs int64, buckets []PerfMetricStatusBucketRange, groups []string) ([]PerfMetricTtftHistogramSeries, error) {
	return getPerfMetricTtftHistogramStatusSeries(startTs, endTs, buckets, commonGroupCol, groups)
}

func getPerfMetricTtftHistogramStatusSeries(startTs int64, endTs int64, buckets []PerfMetricStatusBucketRange, dimensionColumn string, names []string) ([]PerfMetricTtftHistogramSeries, error) {
	series := make([]PerfMetricTtftHistogramSeries, 0)
	if endTs <= startTs || len(buckets) == 0 || (dimensionColumn != "" && len(names) == 0) {
		return series, nil
	}

	selectParts := make([]string, 0, len(buckets)+2)
	if dimensionColumn != "" {
		selectParts = append(selectParts, dimensionColumn)
	}
	selectParts = append(selectParts, "bin_index")
	for _, bucket := range buckets {
		condition := "bucket_ts >= " + strconv.FormatInt(bucket.Start, 10) + " AND bucket_ts < " + strconv.FormatInt(bucket.End, 10)
		selectParts = append(selectParts, "COALESCE(SUM(CASE WHEN "+condition+" THEN sample_count ELSE 0 END), 0)")
	}

	query := DB.Model(&PerfMetricTtftBin{}).
		Select(strings.Join(selectParts, ", ")).
		Where("bucket_ts >= ? AND bucket_ts < ?", startTs, endTs)
	groupColumns := "bin_index"
	if dimensionColumn != "" {
		query = query.Where(dimensionColumn+" IN ?", names)
		groupColumns = dimensionColumn + ", bin_index"
	}
	rows, err := query.Group(groupColumns).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		item := PerfMetricTtftHistogramSeries{Buckets: make([]int64, len(buckets))}
		values := make([]sql.NullInt64, len(buckets))
		scanArgs := make([]any, 0, len(buckets)+2)
		if dimensionColumn != "" {
			scanArgs = append(scanArgs, &item.Name)
		}
		scanArgs = append(scanArgs, &item.BinIndex)
		for index := range values {
			scanArgs = append(scanArgs, &values[index])
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, err
		}
		for index := range values {
			item.Buckets[index] = values[index].Int64
		}
		series = append(series, item)
	}
	return series, rows.Err()
}
