package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/gin-gonic/gin"
)

func GetServiceStatus(c *gin.Context) {
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	result, err := perfmetrics.QueryServiceStatus(c.Query("granularity"), endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=60, stale-while-revalidate=60")
	common.ApiSuccess(c, result)
}
