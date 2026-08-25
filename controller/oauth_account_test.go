package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type oauthAccountEnvelope struct {
	Success bool                 `json:"success"`
	Message string               `json:"message"`
	Data    OAuthAccountResponse `json:"data"`
}

type oauthAccountQueryCounter struct {
	logger.Interface
	mu      sync.Mutex
	queries []string
}

func (counter *oauthAccountQueryCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT") {
		counter.mu.Lock()
		counter.queries = append(counter.queries, sql)
		counter.mu.Unlock()
	}
	counter.Interface.Trace(ctx, begin, fc, err)
}

func (counter *oauthAccountQueryCounter) selectQueries() []string {
	counter.mu.Lock()
	defer counter.mu.Unlock()
	return append([]string(nil), counter.queries...)
}

func setupOAuthAccountTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)

	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	oldQuotaPerUnit := common.QuotaPerUnit
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}))

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled = oldRedisEnabled
		common.QuotaPerUnit = oldQuotaPerUnit
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedOAuthAccountTestUser(t *testing.T, db *gorm.DB, id int, quota int, usedQuota int) {
	t.Helper()
	user := &model.User{
		Id:        id,
		Username:  fmt.Sprintf("oauth-account-%d", id),
		Password:  "password123",
		Status:    common.UserStatusEnabled,
		Role:      common.RoleCommonUser,
		Group:     "default",
		AffCode:   fmt.Sprintf("oauth-account-%d", id),
		Quota:     quota,
		UsedQuota: usedQuota,
	}
	require.NoError(t, db.Create(user).Error)
}

func requestOAuthAccount(t *testing.T, userID int) (httptest.ResponseRecorder, oauthAccountEnvelope) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/oauth/account", nil)
	ctx.Set("id", userID)
	common.SetContextKey(ctx, constant.ContextKeyUserName, fmt.Sprintf("oauth-account-%d", userID))
	GetOAuthAccount(ctx)

	var envelope oauthAccountEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
	return *recorder, envelope
}

func TestGetOAuthAccountReturnsBalanceAndActiveSubscriptionSnapshots(t *testing.T) {
	db := setupOAuthAccountTestDB(t)
	common.QuotaPerUnit = 123456.0
	seedOAuthAccountTestUser(t, db, 101, 7000, 321)

	now := common.GetTimestamp()
	plan := &model.SubscriptionPlan{
		Id:                      11,
		Title:                   "Disabled but purchased",
		Subtitle:                "snapshot metadata",
		DurationUnit:            model.SubscriptionDurationCustom,
		DurationValue:           0,
		CustomSeconds:           3600,
		Enabled:                 false,
		TotalAmount:             999999,
		QuotaResetPeriod:        model.SubscriptionResetCustom,
		QuotaResetCustomSeconds: 600,
	}
	require.NoError(t, db.Create(plan).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:            10,
		UserId:        101,
		PlanId:        plan.Id,
		AmountTotal:   100,
		AmountUsed:    125,
		StartTime:     now - 100,
		EndTime:       now + 100,
		Status:        "active",
		LastResetTime: now - 50,
		NextResetTime: now - 1,
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:          12,
		UserId:      101,
		PlanId:      plan.Id,
		AmountTotal: 0,
		AmountUsed:  999,
		StartTime:   now - 100,
		EndTime:     now + 200,
		Status:      "active",
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:          11,
		UserId:      101,
		PlanId:      999,
		AmountTotal: 200,
		AmountUsed:  20,
		StartTime:   now - 100,
		EndTime:     now + 200,
		Status:      "active",
	}).Error)
	// Expired and non-active rows must not leak into the account view.
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:          20,
		UserId:      101,
		PlanId:      plan.Id,
		AmountTotal: 500,
		EndTime:     now - 1,
		Status:      "active",
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:          21,
		UserId:      101,
		PlanId:      plan.Id,
		AmountTotal: 500,
		EndTime:     now + 500,
		Status:      "cancelled",
	}).Error)

	recorder, envelope := requestOAuthAccount(t, 101)
	require.True(t, envelope.Success, envelope.Message)
	assert.Equal(t, 101, envelope.Data.UserId)
	var raw map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &raw))
	rawData, ok := raw["data"].(map[string]any)
	require.True(t, ok)
	rawSubscriptions, ok := rawData["subscriptions"].([]any)
	require.True(t, ok)
	assert.Nil(t, rawSubscriptions[1].(map[string]any)["plan"])
	assert.Nil(t, rawSubscriptions[2].(map[string]any)["amount_remaining"])
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", recorder.Header().Get("Pragma"))
	assert.Equal(t, "Authorization", recorder.Header().Get("Vary"))
	assert.Equal(t, 7000, envelope.Data.Balance.Quota)
	assert.Equal(t, 321, envelope.Data.Balance.UsedQuota)
	assert.Equal(t, 123456.0, envelope.Data.Balance.QuotaPerUnit)
	assert.Equal(t, "oauth-account-101", envelope.Data.Username)

	require.Len(t, envelope.Data.Subscriptions, 3)
	assert.Equal(t, []int{10, 11, 12}, []int{
		envelope.Data.Subscriptions[0].Id,
		envelope.Data.Subscriptions[1].Id,
		envelope.Data.Subscriptions[2].Id,
	})
	finite := envelope.Data.Subscriptions[0]
	assert.Equal(t, int64(100), finite.AmountTotal, "must use the user subscription snapshot, not plan total")
	assert.Equal(t, int64(125), finite.AmountUsed)
	assert.NotNil(t, finite.AmountRemaining)
	assert.Equal(t, int64(0), *finite.AmountRemaining)
	assert.False(t, finite.Unlimited)
	assert.True(t, finite.ResetDue)
	assert.NotNil(t, finite.Plan, "disabled plans remain visible for purchased subscriptions")
	assert.Equal(t, plan.Title, finite.Plan.Title)
	assert.Equal(t, model.SubscriptionResetCustom, finite.Plan.QuotaResetPeriod)

	missing := envelope.Data.Subscriptions[1]
	assert.Nil(t, missing.Plan)
	assert.True(t, missing.PlanMissing)
	assert.Equal(t, int64(180), *missing.AmountRemaining)

	unlimited := envelope.Data.Subscriptions[2]
	assert.True(t, unlimited.Unlimited)
	assert.Nil(t, unlimited.AmountRemaining)

	var unchanged model.UserSubscription
	require.NoError(t, db.First(&unchanged, 10).Error)
	assert.Equal(t, int64(125), unchanged.AmountUsed, "GET must not trigger subscription reset/write")
}

func TestGetOAuthAccountReturnsEmptySubscriptionArray(t *testing.T) {
	db := setupOAuthAccountTestDB(t)
	seedOAuthAccountTestUser(t, db, 102, 1, 2)

	_, envelope := requestOAuthAccount(t, 102)
	require.True(t, envelope.Success, envelope.Message)
	assert.NotNil(t, envelope.Data.Subscriptions)
	assert.Empty(t, envelope.Data.Subscriptions)
}

func TestGetOAuthAccountLoadsPlansInOneBatch(t *testing.T) {
	db := setupOAuthAccountTestDB(t)
	seedOAuthAccountTestUser(t, db, 103, 10, 20)
	now := common.GetTimestamp()
	for i := 1; i <= 2; i++ {
		require.NoError(t, db.Create(&model.SubscriptionPlan{Id: i, Title: fmt.Sprintf("Plan %d", i)}).Error)
		require.NoError(t, db.Create(&model.UserSubscription{
			Id:          100 + i,
			UserId:      103,
			PlanId:      i,
			AmountTotal: 100,
			EndTime:     now + int64(i),
			Status:      "active",
		}).Error)
	}

	counter := &oauthAccountQueryCounter{Interface: logger.Default.LogMode(logger.Silent)}
	model.DB = db.Session(&gorm.Session{Logger: counter})
	_, envelope := requestOAuthAccount(t, 103)
	require.True(t, envelope.Success, envelope.Message)

	queries := counter.selectQueries()
	require.Len(t, queries, 4, "quota, used quota, active subscriptions, and one plan IN query")
	var subscriptionQueries, planQueries int
	for _, query := range queries {
		lower := strings.ToLower(query)
		if strings.Contains(lower, "user_subscriptions") {
			subscriptionQueries++
		}
		if strings.Contains(lower, "subscription_plans") {
			planQueries++
			assert.Contains(t, strings.ToUpper(query), " IN ")
		}
	}
	assert.Equal(t, 1, subscriptionQueries)
	assert.Equal(t, 1, planQueries)
}
