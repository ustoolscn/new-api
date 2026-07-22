package perfmetrics

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
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
	AvgTtftMs    *int64   `json:"avg_ttft_ms"`
}

type ServiceStatusMetric struct {
	Name         string               `json:"name"`
	RequestCount int64                `json:"request_count"`
	SuccessCount int64                `json:"success_count"`
	SuccessRate  *float64             `json:"success_rate"`
	AvgTtftMs    *int64               `json:"avg_ttft_ms"`
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

type serviceStatusAccumulator struct {
	total   counters
	buckets []counters
}

func QueryServiceStatus(granularity string, requestedEndTimestamp int64) (*ServiceStatusResult, error) {
	granularity = normalizeServiceStatusGranularity(granularity)
	now := time.Now()
	displaySettings := common.GetPublicDisplaySettings()
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
			value := counters{
				requestCount:     bucket.RequestCount,
				successCount:     bucket.SuccessCount,
				ttftSumMs:        bucket.TtftSumMs,
				ttftCount:        bucket.TtftCount,
				ttftMinMs:        bucket.TtftMinMs,
				ttftMaxMs:        bucket.TtftMaxMs,
				ttftExtremaCount: bucket.TtftExtremaCount,
			}
			overall.add(index, value)
			getServiceStatusAccumulator(modelAccumulators, row.ModelName, bucketCount).add(index, value)
			getServiceStatusAccumulator(groupAccumulators, row.Group, bucketCount).add(index, value)
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
		overall.add(index, counts)
		getServiceStatusAccumulator(modelAccumulators, bucketKey.model, bucketCount).add(index, counts)
		getServiceStatusAccumulator(groupAccumulators, bucketKey.group, bucketCount).add(index, counts)
		return true
	})

	models := serviceStatusMetrics(modelAccumulators, periods, displaySettings, "model")
	groups := serviceStatusMetrics(groupAccumulators, periods, displaySettings, "group")
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
		Overall:         serviceStatusMetric("", overall, periods, displaySettings, "overall"),
		Models:          models,
		Groups:          groups,
		ModelsTotal:     modelsTotal,
		GroupsTotal:     groupsTotal,
		ModelsTruncated: modelsTruncated,
		GroupsTruncated: groupsTruncated,
	}, nil
}

func normalizeServiceStatusGranularity(granularity string) string {
	if strings.EqualFold(strings.TrimSpace(granularity), ServiceStatusGranularityDay) {
		return ServiceStatusGranularityDay
	}
	return ServiceStatusGranularityHour
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
	return &serviceStatusAccumulator{buckets: make([]counters, bucketCount)}
}

func getServiceStatusAccumulator(accumulators map[string]*serviceStatusAccumulator, name string, bucketCount int) *serviceStatusAccumulator {
	if accumulator := accumulators[name]; accumulator != nil {
		return accumulator
	}
	accumulator := newServiceStatusAccumulator(bucketCount)
	accumulators[name] = accumulator
	return accumulator
}

func (accumulator *serviceStatusAccumulator) add(index int, value counters) {
	if accumulator == nil || index < 0 || index >= len(accumulator.buckets) || value.requestCount <= 0 {
		return
	}
	if value.successCount < 0 {
		value.successCount = 0
	}
	if value.successCount > value.requestCount {
		value.successCount = value.requestCount
	}
	mergeCounterValues(&accumulator.total, value)
	mergeCounterValues(&accumulator.buckets[index], value)
}

func serviceStatusBucketIndex(timestamp int64, ranges []model.PerfMetricStatusBucketRange) int {
	for index, bucket := range ranges {
		if timestamp >= bucket.Start && timestamp < bucket.End {
			return index
		}
	}
	return -1
}

func serviceStatusMetrics(accumulators map[string]*serviceStatusAccumulator, periods []ServiceStatusPeriod, settings common.PublicDisplaySettings, dimension string) []ServiceStatusMetric {
	metrics := make([]ServiceStatusMetric, 0, len(accumulators))
	for name, accumulator := range accumulators {
		if strings.TrimSpace(name) == "" || accumulator.total.requestCount <= 0 {
			continue
		}
		metrics = append(metrics, serviceStatusMetric(name, accumulator, periods, settings, dimension))
	}
	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].RequestCount == metrics[j].RequestCount {
			return metrics[i].Name < metrics[j].Name
		}
		return metrics[i].RequestCount > metrics[j].RequestCount
	})
	return metrics
}

func serviceStatusMetric(name string, accumulator *serviceStatusAccumulator, periods []ServiceStatusPeriod, settings common.PublicDisplaySettings, dimension string) ServiceStatusMetric {
	metric := ServiceStatusMetric{
		Name:        name,
		SuccessRate: serviceStatusSuccessRate(accumulator.total.requestCount, accumulator.total.successCount),
		AvgTtftMs:   serviceStatusAverageTtft(accumulator.total),
		Series:      make([]ServiceStatusPoint, len(accumulator.buckets)),
	}
	for index, bucket := range accumulator.buckets {
		bucketStart := int64(index)
		if index < len(periods) {
			bucketStart = periods[index].BucketStart
		}
		displayedRequestCount := common.PublicDisplayValue(
			bucket.requestCount,
			settings,
			fmt.Sprintf("service-status:%s:%s:%d", dimension, name, bucketStart),
		)
		displayedSuccessCount := serviceStatusDisplayedSuccessCount(bucket.requestCount, bucket.successCount, displayedRequestCount)
		metric.RequestCount = serviceStatusSaturatingAdd(metric.RequestCount, displayedRequestCount)
		metric.SuccessCount = serviceStatusSaturatingAdd(metric.SuccessCount, displayedSuccessCount)
		metric.Series[index] = ServiceStatusPoint{
			RequestCount: displayedRequestCount,
			SuccessCount: displayedSuccessCount,
			SuccessRate:  serviceStatusSuccessRate(bucket.requestCount, bucket.successCount),
			AvgTtftMs:    serviceStatusAverageTtft(bucket),
		}
	}
	return metric
}

func serviceStatusAverageTtft(value counters) *int64 {
	if value.ttftCount <= 0 || value.ttftSumMs < 0 {
		return nil
	}
	sum := value.ttftSumMs
	count := value.ttftCount
	if value.ttftExtremaCount == count && count >= 3 && value.ttftMinMs >= 0 && value.ttftMaxMs >= value.ttftMinMs && sum >= value.ttftMinMs && sum-value.ttftMinMs >= value.ttftMaxMs {
		sum -= value.ttftMinMs + value.ttftMaxMs
		count -= 2
	}
	average := sum / count
	return &average
}

func serviceStatusDisplayedSuccessCount(requestCount int64, successCount int64, displayedRequestCount int64) int64 {
	if requestCount <= 0 || successCount <= 0 || displayedRequestCount <= 0 {
		return 0
	}
	if successCount >= requestCount {
		return displayedRequestCount
	}
	displayed := math.Round(float64(displayedRequestCount) * float64(successCount) / float64(requestCount))
	if displayed <= 0 {
		return 0
	}
	if displayed >= float64(displayedRequestCount) {
		return displayedRequestCount
	}
	return int64(displayed)
}

func serviceStatusSaturatingAdd(total int64, value int64) int64 {
	if value <= 0 {
		return total
	}
	if total >= math.MaxInt64-value {
		return math.MaxInt64
	}
	return total + value
}

func serviceStatusSuccessRate(requestCount int64, successCount int64) *float64 {
	if requestCount <= 0 {
		return nil
	}
	rate := float64(successCount) / float64(requestCount) * 100
	rate = math.Round(rate*100) / 100
	return &rate
}
