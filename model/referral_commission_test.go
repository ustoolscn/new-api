package model

import (
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferralInvitedUserTopUpCountJSONField(t *testing.T) {
	data, err := common.Marshal(ReferralInvitedUser{TopUpCount: 4})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	assert.Equal(t, float64(4), payload["top_up_count"])
	assert.NotContains(t, payload, "topup_count")
}

func TestReferralCommissionCreatedOnceAndClaimedToBalance(t *testing.T) {
	truncateTables(t)

	paymentSetting := operation_setting.GetPaymentSetting()
	originalRate := paymentSetting.ReferralCommissionRate
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ReferralCommissionRate = originalRate
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalVersion
	})
	paymentSetting.ReferralCommissionRate = 10
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	inviter := &User{
		Id:              701,
		Username:        "commission-inviter",
		Status:          common.UserStatusEnabled,
		AffCode:         "commission-inviter-code",
		AffCount:        1,
		AffQuota:        2000,
		AffHistoryQuota: 3000,
	}
	invitee := &User{Id: 702, Username: "commission-invitee", Status: common.UserStatusEnabled, AffCode: "commission-invitee-code", InviterId: inviter.Id}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(invitee).Error)

	topUp := &TopUp{
		UserId:          invitee.Id,
		Amount:          20,
		Money:           20,
		TradeNo:         "referral-commission-topup",
		PaymentMethod:   PaymentMethodWaffoPancake,
		PaymentProvider: PaymentProviderWaffoPancake,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
	require.NoError(t, RechargeWaffoPancake(topUp.TradeNo))
	require.NoError(t, RechargeWaffoPancake(topUp.TradeNo))

	expectedRechargeQuota := common.QuotaFromFloat(20 * common.QuotaPerUnit)
	expectedCommissionQuota := expectedRechargeQuota / 10
	var commissions []ReferralCommission
	require.NoError(t, DB.Find(&commissions).Error)
	require.Len(t, commissions, 1)
	assert.Equal(t, inviter.Id, commissions[0].InviterId)
	assert.Equal(t, invitee.Id, commissions[0].InviteeId)
	assert.Equal(t, expectedRechargeQuota, commissions[0].RechargeQuota)
	assert.Equal(t, expectedCommissionQuota, commissions[0].CommissionQuota)
	assert.Equal(t, ReferralCommissionStatusPending, commissions[0].Status)

	overview, err := GetReferralOverview(inviter.Id, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(expectedCommissionQuota), overview.PendingQuota)
	assert.Equal(t, int64(expectedCommissionQuota), overview.TotalQuota)
	assert.Equal(t, int64(1), overview.InviteCount)
	assert.Equal(t, 1, overview.RewardedInviteCount)
	assert.Equal(t, common.QuotaForInviter, overview.InviteRewardQuota)
	assert.Equal(t, 2000, overview.InviteRewardPendingQuota)
	assert.Equal(t, 3000, overview.InviteRewardTotalQuota)
	items, ok := overview.InvitedUsers.Items.([]ReferralInvitedUser)
	require.True(t, ok)
	require.Len(t, items, 1)
	assert.Equal(t, int64(1), items[0].TopUpCount)
	assert.Equal(t, int64(expectedRechargeQuota), items[0].RechargeQuotaTotal)
	assert.Equal(t, int64(expectedCommissionQuota), items[0].CommissionQuotaTotal)

	claimedQuota, err := ClaimReferralCommissions(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, expectedCommissionQuota, claimedQuota)
	assert.Equal(t, expectedCommissionQuota, getUserQuotaForPaymentGuardTest(t, inviter.Id))

	claimedAgain, err := ClaimReferralCommissions(inviter.Id)
	require.NoError(t, err)
	assert.Zero(t, claimedAgain)

	require.NoError(t, DB.First(&commissions[0], commissions[0].Id).Error)
	assert.Equal(t, ReferralCommissionStatusClaimed, commissions[0].Status)
	assert.NotZero(t, commissions[0].ClaimedAt)
}

func TestReferralOverviewUsesSuccessfulTopUpsWithoutCommissionRows(t *testing.T) {
	truncateTables(t)

	paymentSetting := operation_setting.GetPaymentSetting()
	originalRate := paymentSetting.ReferralCommissionRate
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ReferralCommissionRate = originalRate
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalVersion
	})
	paymentSetting.ReferralCommissionRate = 0
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	inviter := &User{Id: 711, Username: "topup-stats-inviter", Status: common.UserStatusEnabled, AffCode: "topup-stats-inviter-code"}
	invitee := &User{Id: 712, Username: "topup-stats-invitee", Status: common.UserStatusEnabled, AffCode: "topup-stats-invitee-code", InviterId: inviter.Id}
	otherUser := &User{Id: 713, Username: "topup-stats-other", Status: common.UserStatusEnabled, AffCode: "topup-stats-other-code"}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(invitee).Error)
	require.NoError(t, DB.Create(otherUser).Error)

	topUps := []TopUp{
		{UserId: invitee.Id, Amount: 2, Money: 2, TradeNo: "referral-topup-waffo", PaymentMethod: PaymentMethodWaffo, PaymentProvider: PaymentProviderWaffo, Status: common.TopUpStatusSuccess},
		{UserId: invitee.Id, Amount: 3, Money: 3.5, TradeNo: "referral-topup-stripe", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess},
		{UserId: invitee.Id, Amount: 1234, Money: 10, TradeNo: "referral-topup-creem", PaymentMethod: PaymentMethodCreem, PaymentProvider: PaymentProviderCreem, Status: common.TopUpStatusSuccess},
		{UserId: invitee.Id, Amount: 1, Money: 1.25, TradeNo: "referral-topup-legacy-stripe", PaymentMethod: PaymentMethodStripe, Status: common.TopUpStatusSuccess},
		{UserId: invitee.Id, Amount: 9, Money: 9, TradeNo: "referral-topup-pending", PaymentMethod: PaymentMethodWaffoPancake, PaymentProvider: PaymentProviderWaffoPancake, Status: common.TopUpStatusPending},
		{UserId: invitee.Id, Amount: 0, Money: 20, TradeNo: "referral-topup-subscription", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess},
		{UserId: otherUser.Id, Amount: 99, Money: 99, TradeNo: "referral-topup-other-user", PaymentMethod: PaymentMethodWaffo, PaymentProvider: PaymentProviderWaffo, Status: common.TopUpStatusSuccess},
	}
	require.NoError(t, DB.Create(&topUps).Error)

	overview, err := GetReferralOverview(inviter.Id, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	items, ok := overview.InvitedUsers.Items.([]ReferralInvitedUser)
	require.True(t, ok)
	require.Len(t, items, 1)

	expectedRechargeQuota := int64(common.QuotaFromFloat(2*common.QuotaPerUnit)) +
		int64(common.QuotaFromFloat(3.5*common.QuotaPerUnit)) +
		1234 +
		int64(common.QuotaFromFloat(1.25*common.QuotaPerUnit))
	assert.Equal(t, int64(4), items[0].TopUpCount)
	assert.Equal(t, expectedRechargeQuota, items[0].RechargeQuotaTotal)
	assert.Zero(t, items[0].CommissionQuotaTotal)
	assert.Zero(t, items[0].LastCommissionAt)
	assert.Zero(t, overview.TotalQuota)
}

func TestGetReferralInviterSummariesAggregatesAndSearches(t *testing.T) {
	truncateTables(t)

	originalQuotaForInviter := common.QuotaForInviter
	t.Cleanup(func() {
		common.QuotaForInviter = originalQuotaForInviter
	})
	common.QuotaForInviter = 800

	firstInviter := &User{
		Id:              721,
		Username:        "summary-first",
		DisplayName:     "First Inviter",
		Status:          common.UserStatusEnabled,
		AffCode:         "summary-first-code",
		AffCount:        2,
		AffQuota:        500,
		AffHistoryQuota: 1600,
	}
	secondInviter := &User{
		Id:              722,
		Username:        "summary-second",
		DisplayName:     "Second Inviter",
		Status:          common.UserStatusEnabled,
		AffCode:         "summary-second-code",
		AffHistoryQuota: 400,
	}
	invitees := []User{
		{Id: 723, Username: "summary-invitee-one", Status: common.UserStatusEnabled, AffCode: "summary-invitee-one-code", InviterId: firstInviter.Id},
		{Id: 724, Username: "summary-invitee-two", Status: common.UserStatusEnabled, AffCode: "summary-invitee-two-code", InviterId: firstInviter.Id},
	}
	unrelated := &User{Id: 725, Username: "summary-unrelated", Status: common.UserStatusEnabled, AffCode: "summary-unrelated-code"}
	require.NoError(t, DB.Create(firstInviter).Error)
	require.NoError(t, DB.Create(secondInviter).Error)
	require.NoError(t, DB.Create(&invitees).Error)
	require.NoError(t, DB.Create(unrelated).Error)

	commissions := []ReferralCommission{
		{InviterId: firstInviter.Id, InviteeId: invitees[0].Id, TopUpId: 9101, CommissionQuota: 120, Status: ReferralCommissionStatusPending},
		{InviterId: firstInviter.Id, InviteeId: invitees[1].Id, TopUpId: 9102, CommissionQuota: 80, Status: ReferralCommissionStatusClaimed},
		{InviterId: secondInviter.Id, InviteeId: invitees[0].Id, TopUpId: 9103, CommissionQuota: 40, Status: ReferralCommissionStatusPending},
	}
	require.NoError(t, DB.Create(&commissions).Error)

	summaries, total, err := GetReferralInviterSummaries(&common.PageInfo{Page: 1, PageSize: 20}, "summary")
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, summaries, 2)

	byId := make(map[int]ReferralInviterSummary, len(summaries))
	for _, summary := range summaries {
		byId[summary.Id] = summary
	}
	first := byId[firstInviter.Id]
	assert.Equal(t, int64(2), first.InviteCount)
	assert.Equal(t, 2, first.RewardedInviteCount)
	assert.Equal(t, 800, first.InviteRewardQuota)
	assert.Equal(t, 500, first.InviteRewardPendingQuota)
	assert.Equal(t, 1600, first.InviteRewardTotalQuota)
	assert.Equal(t, int64(120), first.PendingQuota)
	assert.Equal(t, int64(80), first.ClaimedQuota)
	assert.Equal(t, int64(200), first.TotalQuota)

	second := byId[secondInviter.Id]
	assert.Zero(t, second.InviteCount)
	assert.Equal(t, 400, second.InviteRewardTotalQuota)
	assert.Equal(t, int64(40), second.PendingQuota)

	filtered, filteredTotal, err := GetReferralInviterSummaries(&common.PageInfo{Page: 1, PageSize: 20}, strconv.Itoa(firstInviter.Id))
	require.NoError(t, err)
	assert.Equal(t, int64(1), filteredTotal)
	require.Len(t, filtered, 1)
	assert.Equal(t, firstInviter.Id, filtered[0].Id)
}

func TestReferralOverviewInviteeConsumeStatsUseQuotaDataAndUsedQuota(t *testing.T) {
	truncateTables(t)

	now := time.Now()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	previousMonthStart := currentMonthStart.AddDate(0, -1, 0)

	inviter := &User{Id: 801, Username: "consume-inviter", Status: common.UserStatusEnabled, AffCode: "consume-inviter-code"}
	inviteeA := &User{Id: 802, Username: "consume-invitee-a", Status: common.UserStatusEnabled, AffCode: "consume-invitee-a-code", InviterId: inviter.Id, UsedQuota: 300}
	inviteeB := &User{Id: 803, Username: "consume-invitee-b", Status: common.UserStatusEnabled, AffCode: "consume-invitee-b-code", InviterId: inviter.Id, UsedQuota: 500}
	unrelated := &User{Id: 804, Username: "consume-unrelated", Status: common.UserStatusEnabled, AffCode: "consume-unrelated-code", UsedQuota: 900}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(inviteeA).Error)
	require.NoError(t, DB.Create(inviteeB).Error)
	require.NoError(t, DB.Create(unrelated).Error)

	require.NoError(t, DB.Table("quota_data").Create([]QuotaData{
		{UserID: inviteeA.Id, Username: inviteeA.Username, ModelName: "gpt-test", CreatedAt: previousMonthStart.Unix() + 3600, Quota: 120},
		{UserID: inviteeB.Id, Username: inviteeB.Username, ModelName: "gpt-test", CreatedAt: previousMonthStart.Unix() + 7200, Quota: 80},
		{UserID: inviteeA.Id, Username: inviteeA.Username, ModelName: "gpt-test", CreatedAt: currentMonthStart.Unix() + 3600, Quota: 50},
		{UserID: unrelated.Id, Username: unrelated.Username, ModelName: "gpt-test", CreatedAt: currentMonthStart.Unix() + 3600, Quota: 999},
	}).Error)

	overview, err := GetReferralOverview(inviter.Id, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(800), overview.InviteeConsumeTotal)
	require.Len(t, overview.InviteeConsumeMonths, referralInviteeConsumeMonthCount)
	assert.Equal(t, previousMonthStart.Format("2006-01"), overview.InviteeConsumeMonths[len(overview.InviteeConsumeMonths)-2].MonthLabel)
	assert.Equal(t, int64(200), overview.InviteeConsumeMonths[len(overview.InviteeConsumeMonths)-2].ConsumeQuota)
	assert.Equal(t, currentMonthStart.Format("2006-01"), overview.InviteeConsumeMonths[len(overview.InviteeConsumeMonths)-1].MonthLabel)
	assert.Equal(t, int64(50), overview.InviteeConsumeMonths[len(overview.InviteeConsumeMonths)-1].ConsumeQuota)

	items, ok := overview.InvitedUsers.Items.([]ReferralInvitedUser)
	require.True(t, ok)
	require.Len(t, items, 2)
	byID := map[int]ReferralInvitedUser{}
	for _, item := range items {
		byID[item.Id] = item
	}
	assert.Equal(t, int64(300), byID[inviteeA.Id].UsedQuota)
	assert.Equal(t, int64(500), byID[inviteeB.Id].UsedQuota)
}

func TestReferralInviterSummariesIncludeInviteeConsumeTotal(t *testing.T) {
	truncateTables(t)

	inviter := &User{Id: 821, Username: "admin-consume-inviter", Status: common.UserStatusEnabled, AffCode: "admin-consume-inviter", AffCount: 1}
	inviteeA := &User{Id: 822, Username: "admin-consume-a", Status: common.UserStatusEnabled, AffCode: "admin-consume-a", InviterId: inviter.Id, UsedQuota: 150}
	inviteeB := &User{Id: 823, Username: "admin-consume-b", Status: common.UserStatusEnabled, AffCode: "admin-consume-b", InviterId: inviter.Id, UsedQuota: 250}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(inviteeA).Error)
	require.NoError(t, DB.Create(inviteeB).Error)

	summaries, total, err := GetReferralInviterSummaries(&common.PageInfo{Page: 1, PageSize: 20}, inviter.Username)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, summaries, 1)
	assert.Equal(t, int64(400), summaries[0].InviteeConsumeTotal)
}
