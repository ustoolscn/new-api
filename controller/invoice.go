package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type createInvoiceRequestRequest struct {
	InvoiceTitle string `json:"invoice_title"`
	TopUpIds     []int  `json:"top_up_ids"`
}

type reviewInvoiceRequestRequest struct {
	Action string `json:"action"`
	Remark string `json:"remark"`
}

type issueInvoiceRequestRequest struct {
	InvoiceURL string `json:"invoice_url"`
}

func GetUserOrders(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	result, err := model.GetUserOrders(c.GetInt("id"), c.Query("keyword"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func GetUserInvoiceRequests(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	result, err := model.GetUserInvoiceRequests(c.GetInt("id"), c.Query("status"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func CreateInvoiceRequest(c *gin.Context) {
	var request createInvoiceRequestRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "发票申请参数无效")
		return
	}
	invoiceRequest, err := model.CreateInvoiceRequest(c.GetInt("id"), request.InvoiceTitle, request.TopUpIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, invoiceRequest)
}

func AdminGetInvoiceRequests(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	result, err := model.GetAdminInvoiceRequests(c.Query("status"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func AdminReviewInvoiceRequest(c *gin.Context) {
	requestId, err := strconv.Atoi(c.Param("id"))
	if err != nil || requestId <= 0 {
		common.ApiErrorMsg(c, "发票申请编号无效")
		return
	}

	var request reviewInvoiceRequestRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "审核参数无效")
		return
	}
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action != "approve" && action != "reject" {
		common.ApiErrorMsg(c, "审核操作必须为 approve 或 reject")
		return
	}
	if err := model.ReviewInvoiceRequest(requestId, c.GetInt("id"), action == "approve", request.Remark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminSetInvoiceURL(c *gin.Context) {
	requestId, err := strconv.Atoi(c.Param("id"))
	if err != nil || requestId <= 0 {
		common.ApiErrorMsg(c, "发票申请编号无效")
		return
	}

	var request issueInvoiceRequestRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "发票下载地址参数无效")
		return
	}

	if err := model.SetInvoiceURL(requestId, c.GetInt("id"), request.InvoiceURL); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
