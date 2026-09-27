package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// GetReferralModelConsumeReport serves the self route and the AdminAuth-protected
// /referrals/admin/:id/consume/models route. Query parameters cannot override the
// authenticated inviter on the self route.
func GetReferralModelConsumeReport(c *gin.Context) {
	inviterID := c.GetInt("id")
	if rawID := c.Param("id"); rawID != "" {
		id, err := strconv.Atoi(rawID)
		if err != nil || id <= 0 {
			common.ApiError(c, errors.New("invalid inviter id"))
			return
		}
		inviterID = id
	}
	inviteeID := 0
	if rawID, present := c.GetQuery("invitee_id"); present {
		id, err := strconv.Atoi(strings.TrimSpace(rawID))
		if err != nil || id <= 0 {
			common.ApiError(c, errors.New("invalid invitee id"))
			return
		}
		inviteeID = id
	}
	var timestamps [2]int64
	for index, key := range []string{"start_timestamp", "end_timestamp"} {
		value, err := strconv.ParseInt(strings.TrimSpace(c.Query(key)), 10, 64)
		if err != nil || value <= 0 {
			common.ApiError(c, errors.New("invalid "+key))
			return
		}
		timestamps[index] = value
	}
	report, err := model.GetReferralModelConsumeReport(inviterID, inviteeID, timestamps[0], timestamps[1], common.GetPageQuery(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, report)
}
