package model

import (
	"errors"
	"math"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	InvoiceRequestStatusPending  = "pending"
	InvoiceRequestStatusApproved = "approved"
	InvoiceRequestStatusRejected = "rejected"
	InvoiceRequestStatusIssued   = "issued"

	MaxInvoiceOrderCount = 100
	MaxInvoiceURLLength  = 2048

	UserOrderTypeRecharge     = "recharge"
	UserOrderTypeSubscription = "subscription"
)

type InvoiceRequest struct {
	Id           int    `json:"id"`
	UserId       int    `json:"user_id" gorm:"index;index:idx_invoice_request_user_status,priority:1"`
	InvoiceTitle string `json:"invoice_title" gorm:"type:varchar(200)"`
	AmountCents  int64  `json:"-"`
	Status       string `json:"status" gorm:"type:varchar(20);index;index:idx_invoice_request_user_status,priority:2"`
	InvoiceURL   string `json:"invoice_url" gorm:"type:text"`
	ReviewRemark string `json:"review_remark" gorm:"type:varchar(500)"`
	ReviewedBy   int    `json:"reviewed_by"`
	ReviewedAt   int64  `json:"reviewed_at"`
	IssuedAt     int64  `json:"issued_at"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type InvoiceRequestItem struct {
	Id               int    `json:"id"`
	InvoiceRequestId int    `json:"invoice_request_id" gorm:"uniqueIndex:idx_invoice_request_order;index"`
	TopUpId          int    `json:"top_up_id" gorm:"uniqueIndex:idx_invoice_request_order;index"`
	TradeNo          string `json:"trade_no" gorm:"type:varchar(255)"`
	PaymentMethod    string `json:"payment_method" gorm:"type:varchar(50)"`
	AmountCents      int64  `json:"-"`
	CompleteTime     int64  `json:"complete_time"`
}

type UserOrder struct {
	Id               int     `json:"id"`
	InvoiceSourceId  int     `json:"invoice_source_id"`
	OrderType        string  `json:"order_type"`
	PlanId           int     `json:"plan_id"`
	ProductName      string  `json:"product_name"`
	Amount           int64   `json:"amount"`
	Money            float64 `json:"money"`
	TradeNo          string  `json:"trade_no"`
	PaymentMethod    string  `json:"payment_method"`
	PaymentProvider  string  `json:"payment_provider"`
	CreateTime       int64   `json:"create_time"`
	CompleteTime     int64   `json:"complete_time"`
	Status           string  `json:"status"`
	InvoiceRequestId int     `json:"invoice_request_id"`
	InvoiceStatus    string  `json:"invoice_status"`
	InvoiceEligible  bool    `json:"invoice_eligible"`
}

type InvoiceOrder struct {
	TopUpId       int     `json:"top_up_id"`
	TradeNo       string  `json:"trade_no"`
	PaymentMethod string  `json:"payment_method"`
	Amount        float64 `json:"amount"`
	CompleteTime  int64   `json:"complete_time"`
}

type InvoiceRequestView struct {
	Id                int            `json:"id"`
	UserId            int            `json:"user_id"`
	Username          string         `json:"username"`
	DisplayName       string         `json:"display_name"`
	InvoiceTitle      string         `json:"invoice_title"`
	Amount            float64        `json:"amount"`
	Status            string         `json:"status"`
	ReviewRemark      string         `json:"review_remark"`
	ReviewedBy        int            `json:"reviewed_by"`
	ReviewedAt        int64          `json:"reviewed_at"`
	IssuedAt          int64          `json:"issued_at"`
	CreatedAt         int64          `json:"created_at"`
	UpdatedAt         int64          `json:"updated_at"`
	OrderCount        int            `json:"order_count"`
	Orders            []InvoiceOrder `json:"orders"`
	InvoiceURL        string         `json:"invoice_url"`
	DownloadAvailable bool           `json:"download_available"`
}

func normalizeInvoicePageInfo(pageInfo *common.PageInfo) error {
	if pageInfo == nil {
		return errors.New("无效的分页参数")
	}
	if pageInfo.Page < 1 {
		pageInfo.Page = 1
	}
	if pageInfo.PageSize < 1 {
		pageInfo.PageSize = common.ItemsPerPage
	} else if pageInfo.PageSize > 100 {
		pageInfo.PageSize = 100
	}
	return nil
}

func invoiceMoneyToCents(value float64) (int64, error) {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("订单支付金额无效")
	}
	scaled := math.Round(value * 100)
	if scaled <= 0 || scaled >= float64(math.MaxInt64) {
		return 0, errors.New("订单支付金额超出允许范围")
	}
	return int64(scaled), nil
}

func invoiceCentsToMoney(value int64) float64 {
	return float64(value) / 100
}

func isInvoiceRequestStatus(status string) bool {
	switch status {
	case InvoiceRequestStatusPending,
		InvoiceRequestStatusApproved,
		InvoiceRequestStatusRejected,
		InvoiceRequestStatusIssued:
		return true
	default:
		return false
	}
}

func GetUserOrders(userId int, keyword string, pageInfo *common.PageInfo) (*common.PageInfo, error) {
	if userId <= 0 {
		return nil, errors.New("无效的用户")
	}
	if err := normalizeInvoicePageInfo(pageInfo); err != nil {
		return nil, err
	}

	topUpQuery := DB.Model(&TopUp{}).
		Where("user_id = ? AND amount > 0 AND status = ?", userId, common.TopUpStatusSuccess)
	subscriptionQuery := DB.Model(&SubscriptionOrder{}).
		Where("user_id = ? AND status = ? AND (payment_method IS NULL OR payment_method <> ?) AND (payment_provider IS NULL OR payment_provider <> ?)",
			userId, common.TopUpStatusSuccess, PaymentMethodBalance, PaymentProviderBalance)

	topUpKeywordCondition := ""
	subscriptionKeywordCondition := ""
	keywordPattern := ""
	queryArgs := make([]any, 0, 10)
	queryArgs = append(queryArgs, userId, common.TopUpStatusSuccess)
	if strings.TrimSpace(keyword) != "" {
		pattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, err
		}
		keywordPattern = pattern
		topUpQuery = topUpQuery.Where("trade_no LIKE ? ESCAPE '!'", pattern)
		subscriptionQuery = subscriptionQuery.Where("trade_no LIKE ? ESCAPE '!'", pattern)
		topUpKeywordCondition = " AND top_ups.trade_no LIKE ? ESCAPE '!'"
		subscriptionKeywordCondition = " AND subscription_orders.trade_no LIKE ? ESCAPE '!'"
		queryArgs = append(queryArgs, pattern)
	}

	var topUpTotal int64
	if err := topUpQuery.Count(&topUpTotal).Error; err != nil {
		return nil, err
	}
	var subscriptionTotal int64
	if err := subscriptionQuery.Count(&subscriptionTotal).Error; err != nil {
		return nil, err
	}

	start := pageInfo.GetStartIdx()
	if start < 0 {
		return nil, errors.New("无效的分页参数")
	}
	orders := make([]UserOrder, 0, pageInfo.GetPageSize())
	queryArgs = append(queryArgs,
		userId,
		common.TopUpStatusSuccess,
		PaymentMethodBalance,
		PaymentProviderBalance,
	)
	if keywordPattern != "" {
		queryArgs = append(queryArgs, keywordPattern)
	}
	queryArgs = append(queryArgs, pageInfo.GetPageSize(), start)
	orderQuery := `
		SELECT id, id AS invoice_source_id, '` + UserOrderTypeRecharge + `' AS order_type,
			0 AS plan_id, '' AS product_name,
			amount, money, trade_no, payment_method, payment_provider, create_time, complete_time,
			status, invoice_request_id
		FROM top_ups
		WHERE user_id = ? AND amount > 0 AND status = ?` + topUpKeywordCondition + `
		UNION ALL
		SELECT subscription_orders.id, COALESCE(top_ups.id, 0) AS invoice_source_id,
			'` + UserOrderTypeSubscription + `' AS order_type,
			subscription_orders.plan_id, COALESCE(subscription_plans.title, '') AS product_name,
			0 AS amount, subscription_orders.money, subscription_orders.trade_no,
			subscription_orders.payment_method, subscription_orders.payment_provider,
			subscription_orders.create_time, subscription_orders.complete_time,
			subscription_orders.status, COALESCE(top_ups.invoice_request_id, 0) AS invoice_request_id
		FROM subscription_orders
		LEFT JOIN subscription_plans ON subscription_plans.id = subscription_orders.plan_id
		LEFT JOIN top_ups ON top_ups.trade_no = subscription_orders.trade_no
			AND top_ups.user_id = subscription_orders.user_id AND top_ups.amount = 0
		WHERE subscription_orders.user_id = ? AND subscription_orders.status = ?
			AND (subscription_orders.payment_method IS NULL OR subscription_orders.payment_method <> ?)
			AND (subscription_orders.payment_provider IS NULL OR subscription_orders.payment_provider <> ?)` + subscriptionKeywordCondition + `
		ORDER BY create_time DESC, order_type ASC, id DESC
		LIMIT ? OFFSET ?`
	if err := DB.Raw(orderQuery, queryArgs...).Scan(&orders).Error; err != nil {
		return nil, err
	}

	requestIds := make([]int, 0, len(orders))
	for _, order := range orders {
		if order.InvoiceRequestId > 0 {
			requestIds = append(requestIds, order.InvoiceRequestId)
		}
	}

	requestStatuses := make(map[int]string, len(requestIds))
	if len(requestIds) > 0 {
		var requests []InvoiceRequest
		if err := DB.Select("id", "status").Where("id IN ?", requestIds).Find(&requests).Error; err != nil {
			return nil, err
		}
		for _, request := range requests {
			requestStatuses[request.Id] = request.Status
		}
	}

	for index := range orders {
		if orders[index].InvoiceRequestId > 0 {
			orders[index].InvoiceStatus = requestStatuses[orders[index].InvoiceRequestId]
		}
		_, amountErr := invoiceMoneyToCents(orders[index].Money)
		orders[index].InvoiceEligible = orders[index].InvoiceSourceId > 0 &&
			amountErr == nil && orders[index].InvoiceRequestId <= 0
	}

	pageInfo.SetTotal(int(topUpTotal + subscriptionTotal))
	pageInfo.SetItems(orders)
	return pageInfo, nil
}

func CreateInvoiceRequest(userId int, invoiceTitle string, topUpIds []int) (*InvoiceRequestView, error) {
	invoiceTitle = strings.TrimSpace(invoiceTitle)
	if userId <= 0 {
		return nil, errors.New("无效的用户")
	}
	if invoiceTitle == "" || utf8.RuneCountInString(invoiceTitle) > 200 {
		return nil, errors.New("发票抬头长度必须在 1 到 200 个字符之间")
	}
	if len(topUpIds) == 0 || len(topUpIds) > MaxInvoiceOrderCount {
		return nil, errors.New("请选择 1 到 100 个订单")
	}

	uniqueIds := make(map[int]struct{}, len(topUpIds))
	orderedIds := make([]int, 0, len(topUpIds))
	for _, topUpId := range topUpIds {
		if topUpId <= 0 {
			return nil, errors.New("订单参数无效")
		}
		if _, exists := uniqueIds[topUpId]; exists {
			return nil, errors.New("订单不能重复选择")
		}
		uniqueIds[topUpId] = struct{}{}
		orderedIds = append(orderedIds, topUpId)
	}
	sort.Ints(orderedIds)

	request := &InvoiceRequest{}
	items := make([]InvoiceRequestItem, 0, len(orderedIds))
	err := DB.Transaction(func(tx *gorm.DB) error {
		var topUps []TopUp
		if err := lockForUpdate(tx).
			Where("id IN ? AND user_id = ?", orderedIds, userId).
			Order("id ASC").
			Find(&topUps).Error; err != nil {
			return err
		}
		if len(topUps) != len(orderedIds) {
			return errors.New("部分订单不存在或不属于当前用户")
		}

		subscriptionTradeNos := make([]string, 0, len(topUps))
		for _, topUp := range topUps {
			if topUp.Amount == 0 {
				subscriptionTradeNos = append(subscriptionTradeNos, topUp.TradeNo)
			}
		}
		externalSubscriptions := make(map[string]struct{}, len(subscriptionTradeNos))
		if len(subscriptionTradeNos) > 0 {
			var subscriptionOrders []SubscriptionOrder
			if err := tx.Select("trade_no").
				Where("user_id = ? AND trade_no IN ? AND status = ?", userId, subscriptionTradeNos, common.TopUpStatusSuccess).
				Where("payment_method IS NULL OR payment_method <> ?", PaymentMethodBalance).
				Where("payment_provider IS NULL OR payment_provider <> ?", PaymentProviderBalance).
				Find(&subscriptionOrders).Error; err != nil {
				return err
			}
			for _, subscriptionOrder := range subscriptionOrders {
				externalSubscriptions[subscriptionOrder.TradeNo] = struct{}{}
			}
		}

		var totalCents int64
		for _, topUp := range topUps {
			if topUp.InvoiceRequestId > 0 {
				return errors.New("所选订单中包含已申请开票的订单")
			}
			if topUp.Status != common.TopUpStatusSuccess {
				return errors.New("只有支付成功的订单可以申请开票")
			}
			if topUp.Amount < 0 {
				return errors.New("订单类型无效")
			}
			if topUp.Amount == 0 {
				if _, exists := externalSubscriptions[topUp.TradeNo]; !exists {
					return errors.New("只有充值订单或外部支付的订阅订单可以申请开票")
				}
			}
			amountCents, err := invoiceMoneyToCents(topUp.Money)
			if err != nil {
				return err
			}
			if totalCents > math.MaxInt64-amountCents {
				return errors.New("发票金额超出允许范围")
			}
			totalCents += amountCents
			items = append(items, InvoiceRequestItem{
				TopUpId:       topUp.Id,
				TradeNo:       topUp.TradeNo,
				PaymentMethod: topUp.PaymentMethod,
				AmountCents:   amountCents,
				CompleteTime:  topUp.CompleteTime,
			})
		}

		request = &InvoiceRequest{
			UserId:       userId,
			InvoiceTitle: invoiceTitle,
			AmountCents:  totalCents,
			Status:       InvoiceRequestStatusPending,
		}
		if err := tx.Create(request).Error; err != nil {
			return err
		}
		result := tx.Model(&TopUp{}).
			Where("id IN ? AND user_id = ? AND (invoice_request_id = 0 OR invoice_request_id IS NULL)", orderedIds, userId).
			Update("invoice_request_id", request.Id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(orderedIds)) {
			return errors.New("所选订单的开票状态已发生变化，请刷新后重试")
		}
		for index := range items {
			items[index].InvoiceRequestId = request.Id
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		return nil, err
	}

	return buildInvoiceRequestView(request, items, nil), nil
}

func GetUserInvoiceRequests(userId int, status string, pageInfo *common.PageInfo) (*common.PageInfo, error) {
	if userId <= 0 {
		return nil, errors.New("无效的用户")
	}
	return getInvoiceRequests(userId, status, pageInfo)
}

func GetAdminInvoiceRequests(status string, pageInfo *common.PageInfo) (*common.PageInfo, error) {
	return getInvoiceRequests(0, status, pageInfo)
}

func getInvoiceRequests(userId int, status string, pageInfo *common.PageInfo) (*common.PageInfo, error) {
	if err := normalizeInvoicePageInfo(pageInfo); err != nil {
		return nil, err
	}
	if status != "" && !isInvoiceRequestStatus(status) {
		return nil, errors.New("无效的发票申请状态")
	}

	query := DB.Model(&InvoiceRequest{})
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var requests []InvoiceRequest
	if err := query.Order("id DESC").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&requests).Error; err != nil {
		return nil, err
	}

	requestIds := make([]int, 0, len(requests))
	userIds := make([]int, 0, len(requests))
	for _, request := range requests {
		requestIds = append(requestIds, request.Id)
		userIds = append(userIds, request.UserId)
	}

	itemsByRequest := make(map[int][]InvoiceRequestItem, len(requestIds))
	usersById := make(map[int]User, len(userIds))
	if len(requestIds) > 0 {
		var items []InvoiceRequestItem
		if err := DB.Where("invoice_request_id IN ?", requestIds).
			Order("id ASC").
			Find(&items).Error; err != nil {
			return nil, err
		}
		for _, item := range items {
			itemsByRequest[item.InvoiceRequestId] = append(itemsByRequest[item.InvoiceRequestId], item)
		}

		var users []User
		if err := DB.Select("id", "username", "display_name").
			Where("id IN ?", userIds).
			Find(&users).Error; err != nil {
			return nil, err
		}
		for _, user := range users {
			usersById[user.Id] = user
		}
	}

	views := make([]InvoiceRequestView, 0, len(requests))
	for index := range requests {
		user := usersById[requests[index].UserId]
		views = append(views, *buildInvoiceRequestView(&requests[index], itemsByRequest[requests[index].Id], &user))
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(views)
	return pageInfo, nil
}

func buildInvoiceRequestView(request *InvoiceRequest, items []InvoiceRequestItem, user *User) *InvoiceRequestView {
	view := &InvoiceRequestView{
		Id:           request.Id,
		UserId:       request.UserId,
		InvoiceTitle: request.InvoiceTitle,
		Amount:       invoiceCentsToMoney(request.AmountCents),
		Status:       request.Status,
		ReviewRemark: request.ReviewRemark,
		ReviewedBy:   request.ReviewedBy,
		ReviewedAt:   request.ReviewedAt,
		IssuedAt:     request.IssuedAt,
		CreatedAt:    request.CreatedAt,
		UpdatedAt:    request.UpdatedAt,
		OrderCount:   len(items),
		Orders:       make([]InvoiceOrder, 0, len(items)),
		InvoiceURL:   request.InvoiceURL,
	}
	if user != nil {
		view.Username = user.Username
		view.DisplayName = user.DisplayName
	}
	for _, item := range items {
		view.Orders = append(view.Orders, InvoiceOrder{
			TopUpId:       item.TopUpId,
			TradeNo:       item.TradeNo,
			PaymentMethod: item.PaymentMethod,
			Amount:        invoiceCentsToMoney(item.AmountCents),
			CompleteTime:  item.CompleteTime,
		})
	}
	view.DownloadAvailable = request.Status == InvoiceRequestStatusIssued && request.InvoiceURL != ""
	return view
}

func ReviewInvoiceRequest(requestId int, adminId int, approve bool, remark string) error {
	if requestId <= 0 || adminId <= 0 {
		return errors.New("无效的审核参数")
	}
	remark = strings.TrimSpace(remark)
	if utf8.RuneCountInString(remark) > 500 {
		return errors.New("审核备注不能超过 500 个字符")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var request InvoiceRequest
		if err := lockForUpdate(tx).Where("id = ?", requestId).First(&request).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("发票申请不存在")
			}
			return err
		}
		if request.Status != InvoiceRequestStatusPending {
			return errors.New("该发票申请已完成审核")
		}

		status := InvoiceRequestStatusRejected
		if approve {
			status = InvoiceRequestStatusApproved
		}
		if err := tx.Model(&InvoiceRequest{}).Where("id = ?", requestId).Updates(map[string]interface{}{
			"status":        status,
			"review_remark": remark,
			"reviewed_by":   adminId,
			"reviewed_at":   common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
		if approve {
			return nil
		}
		return tx.Model(&TopUp{}).
			Where("invoice_request_id = ?", requestId).
			Update("invoice_request_id", 0).Error
	})
}

func SetInvoiceURL(requestId int, adminId int, invoiceURL string) error {
	if requestId <= 0 || adminId <= 0 {
		return errors.New("无效的发票链接参数")
	}
	invoiceURL = strings.TrimSpace(invoiceURL)
	if invoiceURL == "" || len(invoiceURL) > MaxInvoiceURLLength {
		return errors.New("发票下载地址长度必须在 1 到 2048 个字符之间")
	}
	parsedURL, err := url.Parse(invoiceURL)
	if err != nil || parsedURL.Host == "" || parsedURL.User != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return errors.New("发票下载地址必须是有效的 HTTP 或 HTTPS URL")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var request InvoiceRequest
		if err := lockForUpdate(tx).Where("id = ?", requestId).First(&request).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("发票申请不存在")
			}
			return err
		}
		if request.Status != InvoiceRequestStatusApproved && request.Status != InvoiceRequestStatusIssued {
			return errors.New("发票申请审核通过后才能设置下载地址")
		}

		now := common.GetTimestamp()
		return tx.Model(&InvoiceRequest{}).Where("id = ?", requestId).Updates(map[string]interface{}{
			"status":      InvoiceRequestStatusIssued,
			"invoice_url": invoiceURL,
			"issued_at":   now,
		}).Error
	})
}
