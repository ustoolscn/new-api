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

	// External subscription companion top-up (amount=0, money=20) is included by paid money.
	expectedRechargeQuota := int64(common.QuotaFromFloat(2*common.QuotaPerUnit)) +
		int64(common.QuotaFromFloat(3.5*common.QuotaPerUnit)) +
		1234 +
		int64(common.QuotaFromFloat(1.25*common.QuotaPerUnit)) +
		int64(common.QuotaFromFloat(20*common.QuotaPerUnit))
	assert.Equal(t, int64(5), items[0].TopUpCount)
	assert.Equal(t, expectedRechargeQuota, items[0].RechargeQuotaTotal)
	assert.Zero(t, items[0].CommissionQuotaTotal)
	assert.Zero(t, items[0].LastCommissionAt)
	assert.Zero(t, overview.TotalQuota)
}

func TestReferralOverviewIncludesExternalSubscriptionAndExcludesBalance(t *testing.T) {
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

	inviter := &User{Id: 731, Username: "sub-stats-inviter", Status: common.UserStatusEnabled, AffCode: "sub-stats-inviter-code"}
	invitee := &User{Id: 732, Username: "sub-stats-invitee", Status: common.UserStatusEnabled, AffCode: "sub-stats-invitee-code", InviterId: inviter.Id}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(invitee).Error)

	originalPrice := operation_setting.Price
	t.Cleanup(func() { operation_setting.Price = originalPrice })
	operation_setting.Price = 7

	topUps := []TopUp{
		{UserId: invitee.Id, Amount: 2, Money: 2, TradeNo: "sub-stats-recharge", PaymentMethod: PaymentMethodWaffo, PaymentProvider: PaymentProviderWaffo, Status: common.TopUpStatusSuccess},
		{UserId: invitee.Id, Amount: 0, Money: 15, TradeNo: "sub-stats-external", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess},
		// Epay subscription money is CNY: 140 CNY / Price 7 = 20 USD.
		{UserId: invitee.Id, Amount: 0, Money: 140, TradeNo: "sub-stats-epay-cny", PaymentMethod: "wxpay", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess},
		// Historical companion rows may omit provider and only keep epay method.
		{UserId: invitee.Id, Amount: 0, Money: 70, TradeNo: "sub-stats-epay-legacy", PaymentMethod: "alipay", Status: common.TopUpStatusSuccess},
		// Balance-paid subscription companion rows (if present) must never count.
		{UserId: invitee.Id, Amount: 0, Money: 99, TradeNo: "sub-stats-balance", PaymentMethod: PaymentMethodBalance, PaymentProvider: PaymentProviderBalance, Status: common.TopUpStatusSuccess},
		{UserId: invitee.Id, Amount: 0, Money: 0, TradeNo: "sub-stats-free", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess},
	}
	require.NoError(t, DB.Create(&topUps).Error)

	overview, err := GetReferralOverview(inviter.Id, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	items, ok := overview.InvitedUsers.Items.([]ReferralInvitedUser)
	require.True(t, ok)
	require.Len(t, items, 1)

	expected := int64(common.QuotaFromFloat(2*common.QuotaPerUnit)) +
		int64(common.QuotaFromFloat(15*common.QuotaPerUnit)) +
		int64(common.QuotaFromFloat(20*common.QuotaPerUnit)) +
		int64(common.QuotaFromFloat(10*common.QuotaPerUnit))
	assert.Equal(t, int64(4), items[0].TopUpCount)
	assert.Equal(t, expected, items[0].RechargeQuotaTotal)
}

func TestCompleteEpaySubscriptionOrderCommissionsUSDNotCNY(t *testing.T) {
	truncateTables(t)

	paymentSetting := operation_setting.GetPaymentSetting()
	originalRate := paymentSetting.ReferralCommissionRate
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalVersion := paymentSetting.ComplianceTermsVersion
	originalPrice := operation_setting.Price
	t.Cleanup(func() {
		paymentSetting.ReferralCommissionRate = originalRate
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalVersion
		operation_setting.Price = originalPrice
	})
	paymentSetting.ReferralCommissionRate = 10
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	operation_setting.Price = 7

	inviter := &User{Id: 761, Username: "epay-sub-inviter", Status: common.UserStatusEnabled, AffCode: "epay-sub-inviter-code"}
	invitee := &User{Id: 762, Username: "epay-sub-invitee", Status: common.UserStatusEnabled, AffCode: "epay-sub-invitee-code", InviterId: inviter.Id}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(invitee).Error)

	plan := &SubscriptionPlan{
		Id:            7601,
		Title:         "Epay Plan",
		PriceAmount:   20,
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)

	order := &SubscriptionOrder{
		UserId:          invitee.Id,
		PlanId:          plan.Id,
		Money:           140, // CNY paid via epay
		TradeNo:         "epay-sub-commission-order",
		PaymentMethod:   "wxpay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
		CreateTime:      common.GetTimestamp(),
	}
	require.NoError(t, order.Insert())
	require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, "{\"ok\":true}", PaymentProviderEpay, "wxpay"))

	expectedRechargeQuota := common.QuotaFromFloat(20 * common.QuotaPerUnit) // 140 CNY / 7 = 20 USD
	expectedCommissionQuota := expectedRechargeQuota / 10

	var commissions []ReferralCommission
	require.NoError(t, DB.Find(&commissions).Error)
	require.Len(t, commissions, 1)
	assert.Equal(t, expectedRechargeQuota, commissions[0].RechargeQuota)
	assert.Equal(t, expectedCommissionQuota, commissions[0].CommissionQuota)

	overview, err := GetReferralOverview(inviter.Id, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	items, ok := overview.InvitedUsers.Items.([]ReferralInvitedUser)
	require.True(t, ok)
	require.Len(t, items, 1)
	assert.Equal(t, int64(1), items[0].TopUpCount)
	assert.Equal(t, int64(expectedRechargeQuota), items[0].RechargeQuotaTotal)
}

func TestCompleteSubscriptionOrderCreatesReferralCommission(t *testing.T) {
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

	inviter := &User{Id: 741, Username: "sub-commission-inviter", Status: common.UserStatusEnabled, AffCode: "sub-commission-inviter-code"}
	invitee := &User{Id: 742, Username: "sub-commission-invitee", Status: common.UserStatusEnabled, AffCode: "sub-commission-invitee-code", InviterId: inviter.Id, Quota: 0}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(invitee).Error)

	plan := &SubscriptionPlan{
		Id:            7401,
		Title:         "Commission Plan",
		PriceAmount:   20,
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)

	order := &SubscriptionOrder{
		UserId:          invitee.Id,
		PlanId:          plan.Id,
		Money:           20,
		TradeNo:         "sub-commission-order",
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		Status:          common.TopUpStatusPending,
		CreateTime:      common.GetTimestamp(),
	}
	require.NoError(t, order.Insert())

	require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, "{\"ok\":true}", PaymentProviderStripe, ""))
	// Idempotent second completion should not create another commission.
	require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, "{\"ok\":true}", PaymentProviderStripe, ""))

	expectedRechargeQuota := common.QuotaFromFloat(20 * common.QuotaPerUnit)
	expectedCommissionQuota := expectedRechargeQuota / 10

	var commissions []ReferralCommission
	require.NoError(t, DB.Find(&commissions).Error)
	require.Len(t, commissions, 1)
	assert.Equal(t, inviter.Id, commissions[0].InviterId)
	assert.Equal(t, invitee.Id, commissions[0].InviteeId)
	assert.Equal(t, expectedRechargeQuota, commissions[0].RechargeQuota)
	assert.Equal(t, expectedCommissionQuota, commissions[0].CommissionQuota)

	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo).First(&topUp).Error)
	assert.Equal(t, int64(0), topUp.Amount)
	assert.Equal(t, 20.0, topUp.Money)
	assert.Equal(t, PaymentProviderStripe, topUp.PaymentProvider)
}

func TestPurchaseSubscriptionWithBalanceDoesNotCreateReferralCommission(t *testing.T) {
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

	inviter := &User{Id: 751, Username: "sub-balance-inviter", Status: common.UserStatusEnabled, AffCode: "sub-balance-inviter-code"}
	invitee := &User{Id: 752, Username: "sub-balance-invitee", Status: common.UserStatusEnabled, AffCode: "sub-balance-invitee-code", InviterId: inviter.Id, Quota: common.QuotaFromFloat(100 * common.QuotaPerUnit)}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(invitee).Error)

	allowBalance := true
	plan := &SubscriptionPlan{
		Id:              7501,
		Title:           "Balance Plan",
		PriceAmount:     10,
		DurationUnit:    SubscriptionDurationMonth,
		DurationValue:   1,
		Enabled:         true,
		TotalAmount:     500,
		AllowBalancePay: &allowBalance,
	}
	require.NoError(t, DB.Create(plan).Error)
	require.NoError(t, PurchaseSubscriptionWithBalance(invitee.Id, plan.Id))

	var commissions []ReferralCommission
	require.NoError(t, DB.Find(&commissions).Error)
	assert.Empty(t, commissions)

	var topUpCount int64
	require.NoError(t, DB.Model(&TopUp{}).Count(&topUpCount).Error)
	assert.Zero(t, topUpCount)

	overview, err := GetReferralOverview(inviter.Id, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	items, ok := overview.InvitedUsers.Items.([]ReferralInvitedUser)
	require.True(t, ok)
	require.Len(t, items, 1)
	assert.Zero(t, items[0].TopUpCount)
	assert.Zero(t, items[0].RechargeQuotaTotal)
	assert.Zero(t, items[0].CommissionQuotaTotal)
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

func TestInviteeConsumeReportUsesRangeForTopUpCommissionAndConsume(t *testing.T) {
	truncateTables(t)

	now := time.Now()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	previousMonthStart := currentMonthStart.AddDate(0, -1, 0)
	rangeEnd := currentMonthStart.AddDate(0, 1, 0)

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

	require.NoError(t, DB.Create(&[]TopUp{
		{UserId: inviteeA.Id, Amount: 2, Money: 2, TradeNo: "range-topup-a-prev", PaymentMethod: PaymentMethodWaffo, PaymentProvider: PaymentProviderWaffo, Status: common.TopUpStatusSuccess, CompleteTime: previousMonthStart.Unix() + 100},
		{UserId: inviteeB.Id, Amount: 3, Money: 3, TradeNo: "range-topup-b-curr", PaymentMethod: PaymentMethodWaffo, PaymentProvider: PaymentProviderWaffo, Status: common.TopUpStatusSuccess, CompleteTime: currentMonthStart.Unix() + 100},
		{UserId: unrelated.Id, Amount: 9, Money: 9, TradeNo: "range-topup-other", PaymentMethod: PaymentMethodWaffo, PaymentProvider: PaymentProviderWaffo, Status: common.TopUpStatusSuccess, CompleteTime: currentMonthStart.Unix() + 100},
	}).Error)
	require.NoError(t, DB.Create(&[]ReferralCommission{
		{InviterId: inviter.Id, InviteeId: inviteeA.Id, TopUpId: 91001, RechargeQuota: 1000, CommissionQuota: 100, Status: ReferralCommissionStatusPending, CreatedAt: previousMonthStart.Unix() + 200},
		{InviterId: inviter.Id, InviteeId: inviteeB.Id, TopUpId: 91002, RechargeQuota: 2000, CommissionQuota: 200, Status: ReferralCommissionStatusPending, CreatedAt: currentMonthStart.Unix() + 200},
	}).Error)

	previousReport, err := GetInviteeConsumeReport(inviter.Id, previousMonthStart.Unix(), currentMonthStart.Unix(), &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(200), previousReport.RangeConsumeTotal)
	assert.Equal(t, int64(1), previousReport.TopUpCountTotal)
	assert.Equal(t, int64(common.QuotaFromFloat(2*common.QuotaPerUnit)), previousReport.RechargeQuotaTotal)
	assert.Equal(t, int64(100), previousReport.CommissionQuotaTotal)
	assert.Equal(t, int64(800), previousReport.LifetimeConsumeTotal)

	previousItems, ok := previousReport.Users.Items.([]ReferralInviteeConsumeUser)
	require.True(t, ok)
	previousByID := map[int]ReferralInviteeConsumeUser{}
	for _, item := range previousItems {
		previousByID[item.Id] = item
	}
	assert.Equal(t, int64(1), previousByID[inviteeA.Id].TopUpCount)
	assert.Equal(t, int64(common.QuotaFromFloat(2*common.QuotaPerUnit)), previousByID[inviteeA.Id].RechargeQuotaTotal)
	assert.Equal(t, int64(100), previousByID[inviteeA.Id].CommissionQuotaTotal)
	assert.Equal(t, int64(120), previousByID[inviteeA.Id].RangeConsumeQuota)
	assert.Zero(t, previousByID[inviteeB.Id].TopUpCount)
	assert.Zero(t, previousByID[inviteeB.Id].CommissionQuotaTotal)
	assert.Equal(t, int64(80), previousByID[inviteeB.Id].RangeConsumeQuota)

	currentReport, err := GetInviteeConsumeReport(inviter.Id, currentMonthStart.Unix(), rangeEnd.Unix(), &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(50), currentReport.RangeConsumeTotal)
	assert.Equal(t, int64(1), currentReport.TopUpCountTotal)
	assert.Equal(t, int64(200), currentReport.CommissionQuotaTotal)
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

func TestAdminAssignReferralInviteeAndUserListReferralStats(t *testing.T) {
	truncateTables(t)

	inviter := &User{Id: 901, Username: "assign-inviter", Status: common.UserStatusEnabled, AffCode: "assign-inviter", AffHistoryQuota: 1000}
	invitee := &User{Id: 902, Username: "assign-invitee", Status: common.UserStatusEnabled, AffCode: "assign-invitee"}
	taken := &User{Id: 903, Username: "assign-taken", Status: common.UserStatusEnabled, AffCode: "assign-taken", InviterId: 999}
	require.NoError(t, DB.Create(inviter).Error)
	require.NoError(t, DB.Create(invitee).Error)
	require.NoError(t, DB.Create(taken).Error)
	require.NoError(t, DB.Create(&ReferralCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, TopUpId: 92001, CommissionQuota: 250, Status: ReferralCommissionStatusPending, CreatedAt: 1,
	}).Error)

	require.NoError(t, AdminAssignReferralInvitee(inviter.Id, invitee.Id))
	var got User
	require.NoError(t, DB.Select("id", "inviter_id").Where("id = ?", invitee.Id).First(&got).Error)
	assert.Equal(t, inviter.Id, got.InviterId)

	err := AdminAssignReferralInvitee(inviter.Id, invitee.Id)
	require.Error(t, err)
	err = AdminAssignReferralInvitee(inviter.Id, taken.Id)
	require.Error(t, err)
	err = AdminAssignReferralInvitee(inviter.Id, inviter.Id)
	require.Error(t, err)

	resolved, err := ResolveUserIdByIdentifier("assign-invitee")
	require.NoError(t, err)
	assert.Equal(t, invitee.Id, resolved)

	users := []*User{inviter}
	require.NoError(t, fillUsersReferralStats(users))
	assert.Equal(t, int64(1), users[0].InviteCount)
	assert.Equal(t, int64(250), users[0].ReferralCommissionTotal)
	assert.Equal(t, int64(1250), users[0].ReferralRevenueTotal)
}
