package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceRequestLifecycleAndOrderEligibility(t *testing.T) {
	truncateTables(t)

	user := &User{Id: 801, Username: "invoice-user", AffCode: "invoice-user-code", Status: common.UserStatusEnabled}
	otherUser := &User{Id: 802, Username: "invoice-other", AffCode: "invoice-other-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Create(otherUser).Error)

	topUps := []TopUp{
		{UserId: user.Id, Amount: 10, Money: 10.25, TradeNo: "invoice-order-1", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, CreateTime: 1, CompleteTime: 2, Status: common.TopUpStatusSuccess},
		{UserId: user.Id, Amount: 20, Money: 20, TradeNo: "invoice-order-2", PaymentMethod: PaymentMethodWaffo, PaymentProvider: PaymentProviderWaffo, CreateTime: 3, CompleteTime: 4, Status: common.TopUpStatusSuccess},
		{UserId: user.Id, Amount: 30, Money: 30, TradeNo: "invoice-order-pending", PaymentMethod: PaymentMethodWaffoPancake, PaymentProvider: PaymentProviderWaffoPancake, CreateTime: 5, Status: common.TopUpStatusPending},
		{UserId: otherUser.Id, Amount: 40, Money: 40, TradeNo: "invoice-order-other", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, CreateTime: 6, CompleteTime: 7, Status: common.TopUpStatusSuccess},
		{UserId: user.Id, Amount: 0, Money: 15, TradeNo: "invoice-subscription-order", PaymentMethod: PaymentMethodStripe, CreateTime: 8, CompleteTime: 9, Status: common.TopUpStatusSuccess},
		{UserId: user.Id, Amount: 0, Money: 12, TradeNo: "invoice-orphan-zero-amount", PaymentMethod: PaymentMethodStripe, CreateTime: 14, CompleteTime: 15, Status: common.TopUpStatusSuccess},
	}
	for index := range topUps {
		require.NoError(t, DB.Create(&topUps[index]).Error)
	}
	plan := &SubscriptionPlan{
		Title:         "Professional Plan",
		PriceAmount:   15,
		Currency:      "USD",
		DurationUnit:  "month",
		DurationValue: 1,
		Enabled:       true,
	}
	require.NoError(t, DB.Create(plan).Error)
	subscriptionOrders := []SubscriptionOrder{
		{UserId: user.Id, PlanId: plan.Id, Money: 15, TradeNo: topUps[4].TradeNo, PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess, CreateTime: 8, CompleteTime: 9},
		{UserId: user.Id, PlanId: plan.Id, Money: 15, TradeNo: "invoice-subscription-balance", PaymentMethod: PaymentMethodBalance, PaymentProvider: PaymentProviderBalance, Status: common.TopUpStatusSuccess, CreateTime: 10, CompleteTime: 10},
		{UserId: user.Id, PlanId: plan.Id, Money: 15, TradeNo: "invoice-subscription-pending", PaymentMethod: PaymentMethodCreem, PaymentProvider: PaymentProviderCreem, Status: common.TopUpStatusPending, CreateTime: 11},
		{UserId: otherUser.Id, PlanId: plan.Id, Money: 15, TradeNo: "invoice-subscription-other", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess, CreateTime: 12, CompleteTime: 13},
	}
	for index := range subscriptionOrders {
		require.NoError(t, DB.Create(&subscriptionOrders[index]).Error)
	}

	ordersPage, err := GetUserOrders(user.Id, "", &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	orders, ok := ordersPage.Items.([]UserOrder)
	require.True(t, ok)
	require.Len(t, orders, 3)
	ordersByTradeNo := make(map[string]UserOrder, len(orders))
	for _, order := range orders {
		ordersByTradeNo[order.TradeNo] = order
	}
	assert.True(t, ordersByTradeNo[topUps[0].TradeNo].InvoiceEligible)
	assert.Equal(t, topUps[0].Id, ordersByTradeNo[topUps[0].TradeNo].InvoiceSourceId)
	assert.True(t, ordersByTradeNo[topUps[1].TradeNo].InvoiceEligible)
	subscriptionOrder := ordersByTradeNo[topUps[4].TradeNo]
	assert.Equal(t, UserOrderTypeSubscription, subscriptionOrder.OrderType)
	assert.Equal(t, plan.Id, subscriptionOrder.PlanId)
	assert.Equal(t, plan.Title, subscriptionOrder.ProductName)
	assert.Equal(t, topUps[4].Id, subscriptionOrder.InvoiceSourceId)
	assert.True(t, subscriptionOrder.InvoiceEligible)
	_, exists := ordersByTradeNo[topUps[2].TradeNo]
	assert.False(t, exists)
	_, exists = ordersByTradeNo[subscriptionOrders[1].TradeNo]
	assert.False(t, exists)
	_, exists = ordersByTradeNo[subscriptionOrders[2].TradeNo]
	assert.False(t, exists)

	searchPage, err := GetUserOrders(user.Id, topUps[4].TradeNo, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	searchOrders, ok := searchPage.Items.([]UserOrder)
	require.True(t, ok)
	require.Len(t, searchOrders, 1)
	assert.Equal(t, UserOrderTypeSubscription, searchOrders[0].OrderType)

	request, err := CreateInvoiceRequest(user.Id, "Example Company", []int{topUps[0].Id, topUps[4].Id})
	require.NoError(t, err)
	assert.Equal(t, 25.25, request.Amount)
	assert.Equal(t, 2, request.OrderCount)
	assert.Equal(t, InvoiceRequestStatusPending, request.Status)
	assert.Equal(t, request.Id, GetTopUpById(topUps[0].Id).InvoiceRequestId)
	assert.Equal(t, request.Id, GetTopUpById(topUps[4].Id).InvoiceRequestId)
	assert.ElementsMatch(t, []int{topUps[0].Id, topUps[4].Id}, []int{request.Orders[0].TopUpId, request.Orders[1].TopUpId})

	_, err = CreateInvoiceRequest(user.Id, "Duplicate Request", []int{topUps[0].Id})
	require.ErrorContains(t, err, "已申请开票")
	_, err = CreateInvoiceRequest(user.Id, "Wrong Owner", []int{topUps[3].Id})
	require.ErrorContains(t, err, "不属于当前用户")
	_, err = CreateInvoiceRequest(user.Id, "Pending Order", []int{topUps[2].Id})
	require.ErrorContains(t, err, "只有支付成功")
	_, err = CreateInvoiceRequest(user.Id, "Invalid Zero Amount Order", []int{topUps[5].Id})
	require.ErrorContains(t, err, "外部支付的订阅订单")

	require.NoError(t, ReviewInvoiceRequest(request.Id, 900, false, "抬头信息需要调整"))
	assert.Zero(t, GetTopUpById(topUps[0].Id).InvoiceRequestId)
	assert.Zero(t, GetTopUpById(topUps[4].Id).InvoiceRequestId)
	retryRequest, err := CreateInvoiceRequest(user.Id, "Example Company Limited", []int{topUps[4].Id})
	require.NoError(t, err)
	assert.Equal(t, 15.0, retryRequest.Amount)
	require.NoError(t, ReviewInvoiceRequest(retryRequest.Id, 900, true, "审核通过"))

	invoiceURL := "https://example.com/invoices/example-invoice.pdf?signature=test"
	require.NoError(t, SetInvoiceURL(retryRequest.Id, 900, invoiceURL))

	issuedPage, err := GetUserInvoiceRequests(user.Id, InvoiceRequestStatusIssued, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	issuedRequests, ok := issuedPage.Items.([]InvoiceRequestView)
	require.True(t, ok)
	require.Len(t, issuedRequests, 1)
	assert.True(t, issuedRequests[0].DownloadAvailable)
	assert.Equal(t, invoiceURL, issuedRequests[0].InvoiceURL)
	assert.Equal(t, InvoiceRequestStatusIssued, issuedRequests[0].Status)
}

func TestSetInvoiceURLValidation(t *testing.T) {
	truncateTables(t)

	user := &User{Id: 811, Username: "invoice-url-user", AffCode: "invoice-url-user-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	topUp := &TopUp{
		UserId:          user.Id,
		Amount:          10,
		Money:           10,
		TradeNo:         "invoice-url-order",
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		CreateTime:      1,
		CompleteTime:    2,
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(topUp).Error)

	request, err := CreateInvoiceRequest(user.Id, "URL Validation", []int{topUp.Id})
	require.NoError(t, err)
	require.NoError(t, ReviewInvoiceRequest(request.Id, 900, true, ""))

	err = SetInvoiceURL(request.Id, 900, "file:///tmp/invoice.pdf")
	require.ErrorContains(t, err, "HTTP 或 HTTPS")
	err = SetInvoiceURL(request.Id, 900, "https://user:password@example.com/invoice.pdf")
	require.ErrorContains(t, err, "HTTP 或 HTTPS")

	var stored InvoiceRequest
	require.NoError(t, DB.First(&stored, request.Id).Error)
	assert.Equal(t, InvoiceRequestStatusApproved, stored.Status)
	assert.Empty(t, stored.InvoiceURL)
}
