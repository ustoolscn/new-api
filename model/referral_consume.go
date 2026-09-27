package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type ReferralModelConsumption struct {
	ModelName    string `json:"model_name"`
	ConsumeQuota int64  `json:"consume_quota"`
	RequestCount int64  `json:"request_count"`
	TokenUsed    int64  `json:"token_used"`
}

type ReferralModelConsumeReport struct {
	StartTimestamp int64            `json:"start_timestamp"`
	EndTimestamp   int64            `json:"end_timestamp"`
	ConsumeQuota   int64            `json:"consume_quota"`
	RequestCount   int64            `json:"request_count"`
	TokenUsed      int64            `json:"token_used"`
	Models         *common.PageInfo `json:"models"`
}

func normalizeReferralConsumeRange(startTimestamp, endTimestamp int64) (int64, int64, error) {
	now := time.Now()
	if endTimestamp <= 0 {
		endTimestamp = now.Unix()
	}
	if startTimestamp <= 0 {
		startTimestamp = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	}
	if endTimestamp <= startTimestamp {
		return 0, 0, errors.New("end_timestamp must be greater than start_timestamp")
	}
	if endTimestamp-startTimestamp > maxReferralInviteeConsumeRangeSeconds {
		return 0, 0, errors.New("invitee consumption range is too large")
	}
	return startTimestamp, endTimestamp, nil
}

// GetReferralModelConsumeReport uses the same persisted quota_data and half-open
// interval as the invitee activity report. An invitee ID of zero selects all
// invitees; an explicit ID must belong to this inviter, even for administrators.
func GetReferralModelConsumeReport(inviterID, inviteeID int, startTimestamp, endTimestamp int64, pageInfo *common.PageInfo) (*ReferralModelConsumeReport, error) {
	if inviterID <= 0 || inviteeID < 0 || pageInfo == nil {
		return nil, errors.New("invalid referral consume query")
	}
	startTimestamp, endTimestamp, err := normalizeReferralConsumeRange(startTimestamp, endTimestamp)
	if err != nil {
		return nil, err
	}
	if pageInfo.Page < 1 {
		pageInfo.Page = 1
	}
	if pageInfo.PageSize < 1 {
		pageInfo.PageSize = common.ItemsPerPage
	} else if pageInfo.PageSize > 100 {
		pageInfo.PageSize = 100
	}
	if inviteeID > 0 {
		var count int64
		// Historical consumption remains included after an invitee is deleted,
		// matching the raw users join in GetInviteeConsumeReport.
		if err := DB.Unscoped().Model(&User{}).Where("id = ? AND inviter_id = ?", inviteeID, inviterID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, errors.New("invitee not found")
		}
	}
	query := DB.Table("quota_data").
		Joins("JOIN users ON users.id = quota_data.user_id").
		Where("users.inviter_id = ? AND quota_data.created_at >= ? AND quota_data.created_at < ?", inviterID, startTimestamp, endTimestamp)
	if inviteeID > 0 {
		query = query.Where("quota_data.user_id = ?", inviteeID)
	}
	// Clone the builder for each aggregate so pagination and grouping do not
	// leak into the totals query. Aggregate counters use 64 bits, not quota caps.
	query = query.Session(&gorm.Session{})
	var totals ReferralModelConsumption
	if err := query.Select("COALESCE(SUM(quota_data.quota), 0) AS consume_quota, COALESCE(SUM(quota_data.count), 0) AS request_count, COALESCE(SUM(quota_data.token_used), 0) AS token_used").Scan(&totals).Error; err != nil {
		return nil, err
	}
	var total int64
	if err := query.Distinct("quota_data.model_name").Count(&total).Error; err != nil {
		return nil, err
	}
	items := make([]ReferralModelConsumption, 0)
	if err := query.Select("quota_data.model_name, COALESCE(SUM(quota_data.quota), 0) AS consume_quota, COALESCE(SUM(quota_data.count), 0) AS request_count, COALESCE(SUM(quota_data.token_used), 0) AS token_used").
		Group("quota_data.model_name").Order("consume_quota DESC, quota_data.model_name ASC").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Scan(&items).Error; err != nil {
		return nil, err
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	return &ReferralModelConsumeReport{
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ConsumeQuota:   totals.ConsumeQuota,
		RequestCount:   totals.RequestCount,
		TokenUsed:      totals.TokenUsed,
		Models:         pageInfo,
	}, nil
}
