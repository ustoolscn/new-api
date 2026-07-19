package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserRegistrationStatistics(t *testing.T) {
	truncateTables(t)

	location := time.Local
	users := []User{
		{Username: "registration-stats-1", AffCode: "registration-stats-code-1", Status: common.UserStatusEnabled, CreatedAt: time.Date(2026, 7, 1, 8, 0, 0, 0, location).Unix()},
		{Username: "registration-stats-2", AffCode: "registration-stats-code-2", Status: common.UserStatusEnabled, CreatedAt: time.Date(2026, 7, 1, 20, 0, 0, 0, location).Unix()},
		{Username: "registration-stats-3", AffCode: "registration-stats-code-3", Status: common.UserStatusEnabled, CreatedAt: time.Date(2026, 7, 3, 12, 0, 0, 0, location).Unix()},
		{Username: "registration-stats-4", AffCode: "registration-stats-code-4", Status: common.UserStatusEnabled, CreatedAt: time.Date(2026, 7, 5, 0, 0, 0, 0, location).Unix()},
		{Username: "registration-stats-5", AffCode: "registration-stats-code-5", Status: common.UserStatusEnabled, CreatedAt: time.Date(2026, 8, 10, 12, 0, 0, 0, location).Unix()},
	}
	for index := range users {
		require.NoError(t, DB.Create(&users[index]).Error)
	}
	require.NoError(t, DB.Delete(&users[1]).Error)

	dailyStart := time.Date(2026, 7, 1, 0, 0, 0, 0, location)
	dailyEnd := time.Date(2026, 7, 5, 0, 0, 0, 0, location)
	daily, err := GetUserRegistrationStatistics(UserRegistrationStatisticsQuery{
		StartTimestamp: dailyStart.Unix(),
		EndTimestamp:   dailyEnd.Unix(),
		Granularity:    UserRegistrationStatsGranularityDay,
	})
	require.NoError(t, err)
	require.Len(t, daily.Items, 4)
	assert.Equal(t, int64(3), daily.TotalRegistrations)
	assert.Equal(t, []int64{2, 0, 1, 0}, []int64{
		daily.Items[0].RegistrationCount,
		daily.Items[1].RegistrationCount,
		daily.Items[2].RegistrationCount,
		daily.Items[3].RegistrationCount,
	})
	assert.Equal(t, "2026-07-01", daily.Items[0].BucketLabel)

	monthly, err := GetUserRegistrationStatistics(UserRegistrationStatisticsQuery{
		StartTimestamp: dailyStart.Unix(),
		EndTimestamp:   time.Date(2026, 9, 1, 0, 0, 0, 0, location).Unix(),
		Granularity:    UserRegistrationStatsGranularityMonth,
	})
	require.NoError(t, err)
	require.Len(t, monthly.Items, 2)
	assert.Equal(t, int64(5), monthly.TotalRegistrations)
	assert.Equal(t, int64(4), monthly.Items[0].RegistrationCount)
	assert.Equal(t, int64(1), monthly.Items[1].RegistrationCount)

	_, err = GetUserRegistrationStatistics(UserRegistrationStatisticsQuery{
		StartTimestamp: dailyEnd.Unix(),
		EndTimestamp:   dailyStart.Unix(),
	})
	require.ErrorContains(t, err, "end_timestamp")

	_, err = GetUserRegistrationStatistics(UserRegistrationStatisticsQuery{
		StartTimestamp: dailyStart.Unix(),
		EndTimestamp:   dailyStart.AddDate(0, 0, maxUserRegistrationStatsDailyBuckets+1).Unix(),
		Granularity:    UserRegistrationStatsGranularityDay,
	})
	require.ErrorContains(t, err, "range is too large")
}
