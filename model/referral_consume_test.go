package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferralModelConsumptionScopeTotalsAndPagination(t *testing.T) {
	truncateTables(t)
	users := []User{
		{Id: 801, Username: "model-inviter", AffCode: "model-inviter"},
		{Id: 802, Username: "first-invitee", AffCode: "first-invitee", InviterId: 801},
		{Id: 803, Username: "second-invitee", AffCode: "second-invitee", InviterId: 801},
		{Id: 804, Username: "foreign-invitee", AffCode: "foreign-invitee", InviterId: 900},
		{Id: 805, Username: "empty-invitee", AffCode: "empty-invitee", InviterId: 801},
	}
	require.NoError(t, DB.Create(&users).Error)
	require.NoError(t, DB.Create(&[]QuotaData{
		{UserID: 802, ModelName: "model-a", CreatedAt: 1000, Quota: 1500000000, Count: 2, TokenUsed: 100, TokenID: 1, ChannelID: 1},
		{UserID: 802, ModelName: "model-a", CreatedAt: 1100, Quota: 1500000000, Count: 3, TokenUsed: 200, TokenID: 2, ChannelID: 2, UseGroup: "vip"},
		{UserID: 803, ModelName: "model-a", CreatedAt: 1200, Quota: 50, Count: 1, TokenUsed: 30},
		{UserID: 802, ModelName: "model-b", CreatedAt: 1300, Quota: 70, Count: 4, TokenUsed: 40},
		{UserID: 803, ModelName: "model-c", CreatedAt: 1400, Quota: 70, Count: 2, TokenUsed: 50},
		{UserID: 803, ModelName: "", CreatedAt: 1500, Quota: 0, Count: 1, TokenUsed: 10},
		{UserID: 804, ModelName: "private-model", CreatedAt: 1100, Quota: 999, Count: 10},
		{UserID: 801, ModelName: "inviter-own-model", CreatedAt: 1100, Quota: 888},
		{UserID: 802, ModelName: "too-early", CreatedAt: 999, Quota: 777},
		{UserID: 802, ModelName: "end-exclusive", CreatedAt: 2000, Quota: 666},
	}).Error)

	first, err := GetReferralModelConsumeReport(801, 0, 1000, 2000, &common.PageInfo{Page: 1, PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3000000190), first.ConsumeQuota)
	assert.Equal(t, int64(13), first.RequestCount)
	assert.Equal(t, int64(430), first.TokenUsed)
	assert.Equal(t, 4, first.Models.Total)
	assert.Equal(t, []ReferralModelConsumption{
		{ModelName: "model-a", ConsumeQuota: 3000000050, RequestCount: 6, TokenUsed: 330},
		{ModelName: "model-b", ConsumeQuota: 70, RequestCount: 4, TokenUsed: 40},
	}, first.Models.Items)

	second, err := GetReferralModelConsumeReport(801, 0, 1000, 2000, &common.PageInfo{Page: 2, PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, first.ConsumeQuota, second.ConsumeQuota)
	assert.Equal(t, []ReferralModelConsumption{
		{ModelName: "model-c", ConsumeQuota: 70, RequestCount: 2, TokenUsed: 50},
		{ModelName: "", ConsumeQuota: 0, RequestCount: 1, TokenUsed: 10},
	}, second.Models.Items)

	single, err := GetReferralModelConsumeReport(801, 802, 1000, 2000, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(3000000070), single.ConsumeQuota)
	assert.Equal(t, int64(9), single.RequestCount)
	assert.Equal(t, 2, single.Models.Total)

	activity, err := GetInviteeConsumeReport(801, 1000, 2000, &common.PageInfo{Page: 1, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, activity.RangeConsumeTotal, first.ConsumeQuota, "model totals include invitees on every user page")

	for _, inviteeID := range []int{801, 804, 999} {
		_, err := GetReferralModelConsumeReport(801, inviteeID, 1000, 2000, &common.PageInfo{Page: 1, PageSize: 10})
		assert.EqualError(t, err, "invitee not found")
	}
	empty, err := GetReferralModelConsumeReport(801, 805, 1000, 2000, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Zero(t, empty.ConsumeQuota)
	assert.Zero(t, empty.Models.Total)
	assert.Equal(t, []ReferralModelConsumption{}, empty.Models.Items)

	require.NoError(t, DB.Delete(&users[2]).Error)
	historical, err := GetReferralModelConsumeReport(801, 0, 1000, 2000, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	activity, err = GetInviteeConsumeReport(801, 1000, 2000, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, first.ConsumeQuota, historical.ConsumeQuota)
	assert.Equal(t, activity.RangeConsumeTotal, historical.ConsumeQuota)
}

func TestReferralModelConsumptionRejectsInvalidRange(t *testing.T) {
	for _, tt := range []struct {
		name      string
		inviterID int
		inviteeID int
		start     int64
		end       int64
	}{
		{"missing inviter", 0, 0, 1000, 2000},
		{"negative invitee", 801, -1, 1000, 2000},
		{"reversed range", 801, 0, 2000, 1000},
		{"empty range", 801, 0, 1000, 1000},
		{"over one year", 801, 0, 1000, 1000 + 367*24*60*60},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetReferralModelConsumeReport(tt.inviterID, tt.inviteeID, tt.start, tt.end, &common.PageInfo{Page: 1, PageSize: 10})
			require.Error(t, err)
		})
	}
}
