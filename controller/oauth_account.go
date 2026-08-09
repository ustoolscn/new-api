package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type OAuthAccountBalance struct {
	Quota        int     `json:"quota"`
	UsedQuota    int     `json:"used_quota"`
	QuotaPerUnit float64 `json:"quota_per_unit"`
}

type OAuthAccountPlan struct {
	Id                      int    `json:"id"`
	Title                   string `json:"title"`
	Subtitle                string `json:"subtitle"`
	DurationUnit            string `json:"duration_unit"`
	DurationValue           int    `json:"duration_value"`
	CustomSeconds           int64  `json:"custom_seconds"`
	QuotaResetPeriod        string `json:"quota_reset_period"`
	QuotaResetCustomSeconds int64  `json:"quota_reset_custom_seconds"`
}

type OAuthAccountSubscription struct {
	Id              int               `json:"id"`
	PlanId          int               `json:"plan_id"`
	Status          string            `json:"status"`
	StartTime       int64             `json:"start_time"`
	EndTime         int64             `json:"end_time"`
	AmountTotal     int64             `json:"amount_total"`
	AmountUsed      int64             `json:"amount_used"`
	AmountRemaining *int64            `json:"amount_remaining"`
	Unlimited       bool              `json:"unlimited"`
	LastResetTime   int64             `json:"last_reset_time"`
	NextResetTime   int64             `json:"next_reset_time"`
	ResetDue        bool              `json:"reset_due"`
	Plan            *OAuthAccountPlan `json:"plan"`
	PlanMissing     bool              `json:"plan_missing,omitempty"`
}

type OAuthAccountResponse struct {
	Balance       OAuthAccountBalance        `json:"balance"`
	Subscriptions []OAuthAccountSubscription `json:"subscriptions"`
}

// GetOAuthAccount returns the small, read-only account view intended for
// OAuth clients. It deliberately avoids the dashboard self/history queries and
// does not trigger subscription reset processing.
func GetOAuthAccount(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Vary", "Authorization")

	userID := c.GetInt("id")
	if userID <= 0 {
		userID = c.GetInt("user_id")
	}
	if userID <= 0 {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}

	quota, err := model.GetUserQuota(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usedQuota, err := model.GetUserUsedQuota(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	now := common.GetTimestamp()
	subscriptionData, err := model.GetOAuthAccountSubscriptionData(userID, now)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	subscriptions := make([]OAuthAccountSubscription, 0, len(subscriptionData.Subscriptions))
	for _, subscription := range subscriptionData.Subscriptions {
		item := OAuthAccountSubscription{
			Id:            subscription.Id,
			PlanId:        subscription.PlanId,
			Status:        subscription.Status,
			StartTime:     subscription.StartTime,
			EndTime:       subscription.EndTime,
			AmountTotal:   subscription.AmountTotal,
			AmountUsed:    subscription.AmountUsed,
			Unlimited:     subscription.AmountTotal == 0,
			LastResetTime: subscription.LastResetTime,
			NextResetTime: subscription.NextResetTime,
			ResetDue:      subscription.NextResetTime > 0 && subscription.NextResetTime <= now,
		}
		if subscription.AmountTotal != 0 {
			remaining := int64(0)
			if subscription.AmountTotal > 0 && subscription.AmountUsed < subscription.AmountTotal {
				if subscription.AmountUsed >= 0 {
					remaining = subscription.AmountTotal - subscription.AmountUsed
				} else {
					remaining = subscription.AmountTotal
				}
			}
			item.AmountRemaining = &remaining
		}

		if plan, ok := subscriptionData.Plans[subscription.PlanId]; ok {
			item.Plan = &OAuthAccountPlan{
				Id:                      plan.Id,
				Title:                   plan.Title,
				Subtitle:                plan.Subtitle,
				DurationUnit:            plan.DurationUnit,
				DurationValue:           plan.DurationValue,
				CustomSeconds:           plan.CustomSeconds,
				QuotaResetPeriod:        plan.QuotaResetPeriod,
				QuotaResetCustomSeconds: plan.QuotaResetCustomSeconds,
			}
		} else {
			item.PlanMissing = true
		}
		subscriptions = append(subscriptions, item)
	}

	common.ApiSuccess(c, OAuthAccountResponse{
		Balance: OAuthAccountBalance{
			Quota:        quota,
			UsedQuota:    usedQuota,
			QuotaPerUnit: common.QuotaPerUnit,
		},
		Subscriptions: subscriptions,
	})
}
