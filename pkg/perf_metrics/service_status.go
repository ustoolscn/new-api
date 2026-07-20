package perfmetrics

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

const (
	ServiceStatusGranularityHour = "hour"
	ServiceStatusGranularityDay  = "day"

	serviceStatusHourBucketCount = 24
	serviceStatusDayBucketCount  = 90
	serviceStatusMaxModels       = 200
	serviceStatusMaxGroups       = 100
)

type ServiceStatusPeriod struct {
	BucketStart int64  `json:"bucket_start"`
	BucketLabel string `json:"bucket_label"`
}

type ServiceStatusPoint struct {
	RequestCount int64    `json:"request_count"`
	SuccessCount int64    `json:"success_count"`
	SuccessRate  *float64 `json:"success_rate"`
}

type ServiceStatusMetric struct {
	Name         string               `json:"name"`
	RequestCount int64                `json:"request_count"`
	SuccessCount int64                `json:"success_count"`
	SuccessRate  *float64             `json:"success_rate"`
	Series       []ServiceStatusPoint `json:"series"`
}

type ServiceStatusResult struct {
	GeneratedAt     int64                 `json:"generated_at"`
	StartTimestamp  int64                 `json:"start_timestamp"`
	EndTimestamp    int64                 `json:"end_timestamp"`
	Granularity     string                `json:"granularity"`
	IsCurrentPeriod bool                  `json:"is_current_period"`
	Periods         []ServiceStatusPeriod `json:"periods"`
	Overall         ServiceStatusMetric   `json:"overall"`
	Models          []ServiceStatusMetric `json:"models"`
	Groups          []ServiceStatusMetric `json:"groups"`
	ModelsTotal     int                   `json:"models_total"`
	GroupsTotal     int                   `json:"groups_total"`
	ModelsTruncated bool                  `json:"models_truncated"`
	GroupsTruncated bool                  `json:"groups_truncated"`
}

type serviceStatusBucketCounts struct {
	requestCount int64
	successCount int64
}

type serviceStatusAccumulator struct {
	requestCount int64
	successCount int64
	buckets      []serviceStatusBucketCounts
}

func QueryServiceStatus(granularity string, requestedEndTimestamp int64) (*ServiceStatusResult, error) {
	granularity = normalizeServiceStatusGranularity(granularity)
	now := time.Now()
	currentEnd := serviceStatusPeriodEnd(now, granularity)
	end := currentEnd
	if requestedEndTimestamp > 0 {
		end = serviceStatusPeriodEnd(time.Unix(requestedEndTimestamp, 0), granularity)
		if end.After(currentEnd) {
			end = currentEnd
		}
	}

	bucketCount := serviceStatusDayBucketCount
	start := end.AddDate(0, 0, -bucketCount)
	if granularity == ServiceStatusGranularityHour {
		bucketCount = serviceStatusHourBucketCount
		start = end.Add(-time.Duration(bucketCount) * time.Hour)
	}

	periods, bucketRanges := serviceStatusPeriods(start, bucketCount, granularity)
	rows, err := model.GetPerfMetricStatusSeries(start.Unix(), end.Unix(), bucketRanges)
	if err != nil {
		return nil, err
	}

	overall := newServiceStatusAccumulator(bucketCount)
	modelAccumulators := make(map[string]*serviceStatusAccumulator)
	groupAccumulators := make(map[string]*serviceStatusAccumulator)
	for _, row := range rows {
		for index, bucket := range row.Buckets {
			overall.add(index, bucket.RequestCount, bucket.SuccessCount)
			getServiceStatusAccumulator(modelAccumulators, row.ModelName, bucketCount).add(index, bucket.RequestCount, bucket.SuccessCount)
			getServiceStatusAccumulator(groupAccumulators, row.Group, bucketCount).add(index, bucket.RequestCount, bucket.SuccessCount)
		}
	}

	hotBuckets.Range(func(key, value any) bool {
		bucketKey := key.(bucketKey)
		index := serviceStatusBucketIndex(bucketKey.bucketTs, bucketRanges)
		if index < 0 {
			return true
		}
		counts := value.(*atomicBucket).snapshot()
		if counts.requestCount <= 0 {
			return true
		}
		overall.add(index, counts.requestCount, counts.successCount)
		getServiceStatusAccumulator(modelAccumulators, bucketKey.model, bucketCount).add(index, counts.requestCount, counts.successCount)
		getServiceStatusAccumulator(groupAccumulators, bucketKey.group, bucketCount).add(index, counts.requestCount, counts.successCount)
		return true
	})

	models := serviceStatusMetrics(modelAccumulators)
	groups := serviceStatusMetrics(groupAccumulators)
	modelsTotal := len(models)
	groupsTotal := len(groups)
	modelsTruncated := modelsTotal > serviceStatusMaxModels
	groupsTruncated := groupsTotal > serviceStatusMaxGroups
	if modelsTruncated {
		models = models[:serviceStatusMaxModels]
	}
	if groupsTruncated {
		groups = groups[:serviceStatusMaxGroups]
	}

	return &ServiceStatusResult{
		GeneratedAt:     now.Unix(),
		StartTimestamp:  start.Unix(),
		EndTimestamp:    end.Unix(),
		Granularity:     granularity,
		IsCurrentPeriod: end.Equal(currentEnd),
		Periods:         periods,
		Overall:         serviceStatusMetric("", overall),
		Models:          models,
		Groups:          groups,
		ModelsTotal:     modelsTotal,
		GroupsTotal:     groupsTotal,
		ModelsTruncated: modelsTruncated,
		GroupsTruncated: groupsTruncated,
	}, nil
}

func normalizeServiceStatusGranularity(granularity string) string {
	if strings.EqualFold(strings.TrimSpace(granularity), ServiceStatusGranularityHour) {
		return ServiceStatusGranularityHour
	}
	return ServiceStatusGranularityDay
}

func serviceStatusPeriodEnd(value time.Time, granularity string) time.Time {
	if granularity == ServiceStatusGranularityHour {
		start := time.Date(value.Year(), value.Month(), value.Day(), value.Hour(), 0, 0, 0, value.Location())
		if value.After(start) {
			return start.Add(time.Hour)
		}
		return start
	}
	start := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
	if value.After(start) {
		return start.AddDate(0, 0, 1)
	}
	return start
}

func serviceStatusPeriods(start time.Time, count int, granularity string) ([]ServiceStatusPeriod, []model.PerfMetricStatusBucketRange) {
	periods := make([]ServiceStatusPeriod, 0, count)
	ranges := make([]model.PerfMetricStatusBucketRange, 0, count)
	current := start
	for index := 0; index < count; index++ {
		next := current.AddDate(0, 0, 1)
		label := current.Format("2006-01-02")
		if granularity == ServiceStatusGranularityHour {
			next = current.Add(time.Hour)
			label = current.Format("2006-01-02 15:00")
		}
		periods = append(periods, ServiceStatusPeriod{
			BucketStart: current.Unix(),
			BucketLabel: label,
		})
		ranges = append(ranges, model.PerfMetricStatusBucketRange{
			Start: current.Unix(),
			End:   next.Unix(),
		})
		current = next
	}
	return periods, ranges
}

func newServiceStatusAccumulator(bucketCount int) *serviceStatusAccumulator {
	return &serviceStatusAccumulator{buckets: make([]serviceStatusBucketCounts, bucketCount)}
}

func getServiceStatusAccumulator(accumulators map[string]*serviceStatusAccumulator, name string, bucketCount int) *serviceStatusAccumulator {
	if accumulator := accumulators[name]; accumulator != nil {
		return accumulator
	}
	accumulator := newServiceStatusAccumulator(bucketCount)
	accumulators[name] = accumulator
	return accumulator
}

func (accumulator *serviceStatusAccumulator) add(index int, requestCount int64, successCount int64) {
	if accumulator == nil || index < 0 || index >= len(accumulator.buckets) || requestCount <= 0 {
		return
	}
	if successCount < 0 {
		successCount = 0
	}
	if successCount > requestCount {
		successCount = requestCount
	}
	accumulator.requestCount += requestCount
	accumulator.successCount += successCount
	accumulator.buckets[index].requestCount += requestCount
	accumulator.buckets[index].successCount += successCount
}

func serviceStatusBucketIndex(timestamp int64, ranges []model.PerfMetricStatusBucketRange) int {
	for index, bucket := range ranges {
		if timestamp >= bucket.Start && timestamp < bucket.End {
			return index
		}
	}
	return -1
}

func serviceStatusMetrics(accumulators map[string]*serviceStatusAccumulator) []ServiceStatusMetric {
	metrics := make([]ServiceStatusMetric, 0, len(accumulators))
	for name, accumulator := range accumulators {
		if strings.TrimSpace(name) == "" || accumulator.requestCount <= 0 {
			continue
		}
		metrics = append(metrics, serviceStatusMetric(name, accumulator))
	}
	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].RequestCount == metrics[j].RequestCount {
			return metrics[i].Name < metrics[j].Name
		}
		return metrics[i].RequestCount > metrics[j].RequestCount
	})
	return metrics
}

func serviceStatusMetric(name string, accumulator *serviceStatusAccumulator) ServiceStatusMetric {
	metric := ServiceStatusMetric{
		Name:         name,
		RequestCount: accumulator.requestCount,
		SuccessCount: accumulator.successCount,
		SuccessRate:  serviceStatusSuccessRate(accumulator.requestCount, accumulator.successCount),
		Series:       make([]ServiceStatusPoint, len(accumulator.buckets)),
	}
	for index, bucket := range accumulator.buckets {
		metric.Series[index] = ServiceStatusPoint{
			RequestCount: bucket.requestCount,
			SuccessCount: bucket.successCount,
			SuccessRate:  serviceStatusSuccessRate(bucket.requestCount, bucket.successCount),
		}
	}
	return metric
}

func serviceStatusSuccessRate(requestCount int64, successCount int64) *float64 {
	if requestCount <= 0 {
		return nil
	}
	rate := float64(successCount) / float64(requestCount) * 100
	rate = math.Round(rate*100) / 100
	return &rate
}
