package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetUserRegistrationStatistics(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	result, err := model.GetUserRegistrationStatistics(model.UserRegistrationStatisticsQuery{
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		Granularity:    c.Query("granularity"),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
