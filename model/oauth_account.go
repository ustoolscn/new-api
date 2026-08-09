package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// OAuthAccountSubscriptionData contains the active subscription snapshots and
// the current plan metadata needed by the lightweight OAuth account endpoint.
// Plans are loaded in one batch so a response with multiple subscriptions does
// not issue one query per subscription.
type OAuthAccountSubscriptionData struct {
	Subscriptions []UserSubscription
	Plans         map[int]SubscriptionPlan
}

// GetOAuthAccountSubscriptionData loads only active, non-expired subscriptions
// for a user and resolves their plans with one distinct-id IN query. It does
// not perform reset processing or mutate any records.
func GetOAuthAccountSubscriptionData(userID int, now int64) (*OAuthAccountSubscriptionData, error) {
	if userID <= 0 {
		return nil, errors.New("invalid userId")
	}
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	if now <= 0 {
		now = common.GetTimestamp()
	}

	var subscriptions []UserSubscription
	if err := DB.Where("user_id = ? AND status = ? AND end_time > ?", userID, "active", now).
		Select("id", "plan_id", "amount_total", "amount_used", "start_time", "end_time", "status", "last_reset_time", "next_reset_time").
		Order("end_time asc, id asc").
		Find(&subscriptions).Error; err != nil {
		return nil, err
	}

	result := &OAuthAccountSubscriptionData{
		Subscriptions: subscriptions,
		Plans:         make(map[int]SubscriptionPlan),
	}
	if len(subscriptions) == 0 {
		return result, nil
	}

	planIDs := make([]int, 0, len(subscriptions))
	seenPlanIDs := make(map[int]struct{}, len(subscriptions))
	for _, subscription := range subscriptions {
		if subscription.PlanId <= 0 {
			continue
		}
		if _, seen := seenPlanIDs[subscription.PlanId]; seen {
			continue
		}
		seenPlanIDs[subscription.PlanId] = struct{}{}
		planIDs = append(planIDs, subscription.PlanId)
	}
	if len(planIDs) == 0 {
		return result, nil
	}

	var plans []SubscriptionPlan
	if err := DB.Select("id", "title", "subtitle", "duration_unit", "duration_value", "custom_seconds", "quota_reset_period", "quota_reset_custom_seconds").
		Where("id IN ?", planIDs).Find(&plans).Error; err != nil {
		return nil, err
	}
	for _, plan := range plans {
		plan.NormalizeDefaults()
		result.Plans[plan.Id] = plan
	}
	return result, nil
}
