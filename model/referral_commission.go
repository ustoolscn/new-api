package model

import (
	"errors"
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	ReferralCommissionStatusPending = "pending"
	ReferralCommissionStatusClaimed = "claimed"
)

type ReferralCommission struct {
	Id              int     `json:"id"`
	InviterId       int     `json:"inviter_id" gorm:"index;index:idx_referral_commission_inviter_status,priority:1"`
	InviteeId       int     `json:"invitee_id" gorm:"index"`
	TopUpId         int     `json:"topup_id" gorm:"uniqueIndex"`
	RechargeQuota   int     `json:"recharge_quota"`
	CommissionQuota int     `json:"commission_quota"`
	Rate            float64 `json:"rate"`
	Status          string  `json:"status" gorm:"type:varchar(20);index:idx_referral_commission_inviter_status,priority:2"`
	CreatedAt       int64   `json:"created_at" gorm:"autoCreateTime"`
	ClaimedAt       int64   `json:"claimed_at"`
}

type ReferralInvitedUser struct {
	Id                   int    `json:"id"`
	Username             string `json:"username"`
	DisplayName          string `json:"display_name"`
	CreatedAt            int64  `json:"created_at"`
	TopUpCount           int64  `json:"topup_count"`
	RechargeQuotaTotal   int64  `json:"recharge_quota_total"`
	CommissionQuotaTotal int64  `json:"commission_quota_total"`
	LastCommissionAt     int64  `json:"last_commission_at"`
}

type ReferralOverview struct {
	CommissionRate           float64          `json:"commission_rate"`
	ClaimEnabled             bool             `json:"claim_enabled"`
	PendingQuota             int64            `json:"pending_quota"`
	ClaimedQuota             int64            `json:"claimed_quota"`
	TotalQuota               int64            `json:"total_quota"`
	InviteCount              int64            `json:"invite_count"`
	RewardedInviteCount      int              `json:"rewarded_invite_count"`
	InviteRewardQuota        int              `json:"invite_reward_quota"`
	InviteRewardPendingQuota int              `json:"invite_reward_pending_quota"`
	InviteRewardTotalQuota   int              `json:"invite_reward_total_quota"`
	InvitedUsers             *common.PageInfo `json:"invited_users"`
}

func createReferralCommissionTx(tx *gorm.DB, topUp *TopUp, rechargeQuota int) error {
	if tx == nil || topUp == nil || topUp.Id == 0 || topUp.UserId == 0 || rechargeQuota <= 0 {
		return nil
	}
	if !operation_setting.IsPaymentComplianceConfirmed() {
		return nil
	}
	rate := operation_setting.GetPaymentSetting().ReferralCommissionRate
	if rate <= 0 {
		return nil
	}
	if rate > 100 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return fmt.Errorf("invalid referral commission rate")
	}

	var invitee User
	if err := tx.Select("id", "inviter_id").Where("id = ?", topUp.UserId).First(&invitee).Error; err != nil {
		return err
	}
	if invitee.InviterId <= 0 || invitee.InviterId == invitee.Id {
		return nil
	}

	commissionDecimal := decimal.NewFromInt(int64(rechargeQuota)).
		Mul(decimal.NewFromFloat(rate)).
		Div(decimal.NewFromInt(100))
	commissionQuota, clamp := common.QuotaFromDecimalChecked(commissionDecimal)
	if clamp != nil {
		return clamp
	}
	if commissionQuota <= 0 {
		return nil
	}

	commission := &ReferralCommission{
		InviterId:       invitee.InviterId,
		InviteeId:       invitee.Id,
		TopUpId:         topUp.Id,
		RechargeQuota:   rechargeQuota,
		CommissionQuota: commissionQuota,
		Rate:            rate,
		Status:          ReferralCommissionStatusPending,
		CreatedAt:       common.GetTimestamp(),
	}
	return tx.Create(commission).Error
}

func GetReferralOverview(inviterId int, pageInfo *common.PageInfo) (*ReferralOverview, error) {
	if inviterId <= 0 || pageInfo == nil {
		return nil, errors.New("invalid referral query")
	}
	if pageInfo.Page < 1 {
		pageInfo.Page = 1
	}
	if pageInfo.PageSize < 1 {
		pageInfo.PageSize = common.ItemsPerPage
	} else if pageInfo.PageSize > 100 {
		pageInfo.PageSize = 100
	}

	var inviteCount int64
	if err := DB.Model(&User{}).Where("inviter_id = ?", inviterId).Count(&inviteCount).Error; err != nil {
		return nil, err
	}
	var inviter User
	if err := DB.Select("aff_count", "aff_quota", "aff_history").Where("id = ?", inviterId).First(&inviter).Error; err != nil {
		return nil, err
	}

	users := make([]ReferralInvitedUser, 0)
	if err := DB.Model(&User{}).
		Select("id", "username", "display_name", "created_at").
		Where("inviter_id = ?", inviterId).
		Order("id DESC").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Scan(&users).Error; err != nil {
		return nil, err
	}

	if len(users) > 0 {
		userIds := make([]int, 0, len(users))
		userIndex := make(map[int]int, len(users))
		for index := range users {
			userIds = append(userIds, users[index].Id)
			userIndex[users[index].Id] = index
		}

		type referralTopUpAggregate struct {
			UserId          int
			PaymentProvider string
			PaymentMethod   string
			TopUpCount      int64
			AmountTotal     int64
			MoneyTotal      float64
		}
		var topUpAggregates []referralTopUpAggregate
		if err := DB.Model(&TopUp{}).
			Select("user_id, payment_provider, payment_method, COUNT(*) AS top_up_count, COALESCE(SUM(amount), 0) AS amount_total, COALESCE(SUM(money), 0) AS money_total").
			Where("user_id IN ? AND status = ? AND amount > 0", userIds, common.TopUpStatusSuccess).
			Group("user_id, payment_provider, payment_method").
			Scan(&topUpAggregates).Error; err != nil {
			return nil, err
		}

		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		rechargeQuotaTotals := make(map[int]decimal.Decimal, len(users))
		maxInt64 := int64(^uint64(0) >> 1)
		for _, aggregate := range topUpAggregates {
			index, ok := userIndex[aggregate.UserId]
			if !ok {
				continue
			}
			if aggregate.TopUpCount < 0 || users[index].TopUpCount > maxInt64-aggregate.TopUpCount {
				return nil, errors.New("referral top-up count exceeds limit")
			}

			paymentProvider := aggregate.PaymentProvider
			if paymentProvider == "" {
				paymentProvider = aggregate.PaymentMethod
			}
			creditedQuota := decimal.NewFromInt(aggregate.AmountTotal).Mul(quotaPerUnit)
			switch paymentProvider {
			case PaymentProviderStripe:
				creditedQuota = decimal.NewFromFloat(aggregate.MoneyTotal).Mul(quotaPerUnit)
			case PaymentProviderCreem:
				creditedQuota = decimal.NewFromInt(aggregate.AmountTotal)
			}
			if creditedQuota.IsNegative() {
				return nil, errors.New("invalid referral top-up quota")
			}

			users[index].TopUpCount += aggregate.TopUpCount
			rechargeQuotaTotals[aggregate.UserId] = rechargeQuotaTotals[aggregate.UserId].Add(creditedQuota)
		}
		for userId, total := range rechargeQuotaTotals {
			rounded := total.Round(0).BigInt()
			if !rounded.IsInt64() || rounded.Sign() < 0 {
				return nil, errors.New("referral top-up quota exceeds limit")
			}
			users[userIndex[userId]].RechargeQuotaTotal = rounded.Int64()
		}

		type referralCommissionAggregate struct {
			InviteeId            int
			CommissionQuotaTotal int64
			LastCommissionAt     int64
		}
		var aggregates []referralCommissionAggregate
		if err := DB.Model(&ReferralCommission{}).
			Select("invitee_id, COALESCE(SUM(commission_quota), 0) AS commission_quota_total, COALESCE(MAX(created_at), 0) AS last_commission_at").
			Where("inviter_id = ? AND invitee_id IN ?", inviterId, userIds).
			Group("invitee_id").
			Scan(&aggregates).Error; err != nil {
			return nil, err
		}
		for _, aggregate := range aggregates {
			index, ok := userIndex[aggregate.InviteeId]
			if !ok {
				continue
			}
			users[index].CommissionQuotaTotal = aggregate.CommissionQuotaTotal
			users[index].LastCommissionAt = aggregate.LastCommissionAt
		}
	}

	type referralTotals struct {
		PendingQuota int64
		ClaimedQuota int64
		TotalQuota   int64
	}
	var totals referralTotals
	if err := DB.Model(&ReferralCommission{}).
		Select("COALESCE(SUM(CASE WHEN status = ? THEN commission_quota ELSE 0 END), 0) AS pending_quota, COALESCE(SUM(CASE WHEN status = ? THEN commission_quota ELSE 0 END), 0) AS claimed_quota, COALESCE(SUM(commission_quota), 0) AS total_quota", ReferralCommissionStatusPending, ReferralCommissionStatusClaimed).
		Where("inviter_id = ?", inviterId).
		Scan(&totals).Error; err != nil {
		return nil, err
	}

	pageInfo.SetTotal(int(inviteCount))
	pageInfo.SetItems(users)
	return &ReferralOverview{
		CommissionRate:           operation_setting.GetPaymentSetting().ReferralCommissionRate,
		ClaimEnabled:             operation_setting.IsPaymentComplianceConfirmed(),
		PendingQuota:             totals.PendingQuota,
		ClaimedQuota:             totals.ClaimedQuota,
		TotalQuota:               totals.TotalQuota,
		InviteCount:              inviteCount,
		RewardedInviteCount:      inviter.AffCount,
		InviteRewardQuota:        common.QuotaForInviter,
		InviteRewardPendingQuota: inviter.AffQuota,
		InviteRewardTotalQuota:   inviter.AffHistoryQuota,
		InvitedUsers:             pageInfo,
	}, nil
}

func ClaimReferralCommissions(inviterId int) (int, error) {
	if inviterId <= 0 {
		return 0, errors.New("invalid inviter")
	}

	claimedQuota := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var inviter User
		if err := lockForUpdate(tx).Select("id", "quota").Where("id = ?", inviterId).First(&inviter).Error; err != nil {
			return err
		}

		var commissions []ReferralCommission
		if err := lockForUpdate(tx).
			Where("inviter_id = ? AND status = ?", inviterId, ReferralCommissionStatusPending).
			Order("id ASC").
			Find(&commissions).Error; err != nil {
			return err
		}
		if len(commissions) == 0 {
			return nil
		}

		var total int64
		commissionIds := make([]int, 0, len(commissions))
		for _, commission := range commissions {
			if commission.CommissionQuota <= 0 || total > int64(common.MaxQuota)-int64(commission.CommissionQuota) {
				return errors.New("referral commission exceeds quota limit")
			}
			total += int64(commission.CommissionQuota)
			commissionIds = append(commissionIds, commission.Id)
		}
		if total <= 0 || int64(inviter.Quota) > int64(common.MaxQuota)-total {
			return errors.New("claim would exceed account balance limit")
		}

		claimedQuota = int(total)
		if err := tx.Model(&User{}).Where("id = ?", inviterId).
			Update("quota", gorm.Expr("quota + ?", claimedQuota)).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		result := tx.Model(&ReferralCommission{}).
			Where("id IN ? AND inviter_id = ? AND status = ?", commissionIds, inviterId, ReferralCommissionStatusPending).
			Updates(map[string]interface{}{
				"status":     ReferralCommissionStatusClaimed,
				"claimed_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(commissionIds)) {
			return errors.New("referral commission state changed, please retry")
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if claimedQuota > 0 {
		_ = invalidateUserCache(inviterId)
		RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("领取邀请充值返佣 %s", logger.LogQuota(claimedQuota)))
	}
	return claimedQuota, nil
}
