package model

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	UserRegistrationStatsGranularityDay   = "day"
	UserRegistrationStatsGranularityMonth = "month"
	UserRegistrationStatsGranularityYear  = "year"

	maxUserRegistrationStatsDailyBuckets = 400
	maxUserRegistrationStatsMonthBuckets = 240
	maxUserRegistrationStatsYearBuckets  = 100
)

type UserRegistrationStatisticsQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	Granularity    string
}

type UserRegistrationStatisticsItem struct {
	BucketStart       int64  `json:"bucket_start"`
	BucketLabel       string `json:"bucket_label"`
	RegistrationCount int64  `json:"registration_count"`
}

type UserRegistrationStatisticsResult struct {
	StartTimestamp     int64                            `json:"start_timestamp"`
	EndTimestamp       int64                            `json:"end_timestamp"`
	Granularity        string                           `json:"granularity"`
	TotalRegistrations int64                            `json:"total_registrations"`
	Items              []UserRegistrationStatisticsItem `json:"items"`
}

func GetUserRegistrationStatistics(query UserRegistrationStatisticsQuery) (*UserRegistrationStatisticsResult, error) {
	query.Granularity = normalizeUserRegistrationStatsGranularity(query.Granularity)
	if query.EndTimestamp <= 0 {
		now := time.Now()
		query.EndTimestamp = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location()).Unix()
	}
	if query.StartTimestamp <= 0 {
		query.StartTimestamp = time.Unix(query.EndTimestamp, 0).AddDate(0, 0, -30).Unix()
	}
	if query.EndTimestamp <= query.StartTimestamp {
		return nil, errors.New("end_timestamp must be greater than start_timestamp")
	}

	start := time.Unix(query.StartTimestamp, 0)
	end := time.Unix(query.EndTimestamp, 0)
	var maxEnd time.Time
	switch query.Granularity {
	case UserRegistrationStatsGranularityMonth:
		maxEnd = start.AddDate(0, maxUserRegistrationStatsMonthBuckets, 0)
	case UserRegistrationStatsGranularityYear:
		maxEnd = start.AddDate(maxUserRegistrationStatsYearBuckets, 0, 0)
	default:
		maxEnd = start.AddDate(0, 0, maxUserRegistrationStatsDailyBuckets)
	}
	if end.After(maxEnd) {
		return nil, errors.New("registration statistics range is too large")
	}

	buckets := billingStatsBucketRanges(query.StartTimestamp, query.EndTimestamp, query.Granularity)
	items := make([]UserRegistrationStatisticsItem, 0, len(buckets))
	for _, bucket := range buckets {
		items = append(items, UserRegistrationStatisticsItem{
			BucketStart: bucket.Start,
			BucketLabel: bucket.Label,
		})
	}

	result := &UserRegistrationStatisticsResult{
		StartTimestamp: query.StartTimestamp,
		EndTimestamp:   query.EndTimestamp,
		Granularity:    query.Granularity,
		Items:          items,
	}
	if len(buckets) == 0 {
		return result, nil
	}

	selectParts := make([]string, 0, len(buckets))
	selectArgs := make([]any, 0, len(buckets)*2)
	for _, bucket := range buckets {
		selectParts = append(selectParts, "COALESCE(SUM(CASE WHEN created_at >= ? AND created_at < ? THEN 1 ELSE 0 END), 0)")
		selectArgs = append(selectArgs, bucket.Start, bucket.End)
	}
	rows, err := DB.Unscoped().Model(&User{}).
		Select(strings.Join(selectParts, ", "), selectArgs...).
		Where("created_at >= ? AND created_at < ?", query.StartTimestamp, query.EndTimestamp).
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return result, nil
	}

	values := make([]sql.NullInt64, len(buckets))
	scanArgs := make([]any, len(values))
	for index := range values {
		scanArgs[index] = &values[index]
	}
	if err := rows.Scan(scanArgs...); err != nil {
		return nil, err
	}
	for index := range values {
		result.Items[index].RegistrationCount = values[index].Int64
		result.TotalRegistrations += values[index].Int64
	}
	return result, nil
}

func normalizeUserRegistrationStatsGranularity(granularity string) string {
	switch strings.ToLower(strings.TrimSpace(granularity)) {
	case UserRegistrationStatsGranularityMonth:
		return UserRegistrationStatsGranularityMonth
	case UserRegistrationStatsGranularityYear:
		return UserRegistrationStatsGranularityYear
	default:
		return UserRegistrationStatsGranularityDay
	}
}
