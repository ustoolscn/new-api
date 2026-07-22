package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
