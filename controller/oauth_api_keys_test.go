package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type oauthAPIKeysTestResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Total int                   `json:"total"`
		Items []OAuthAPIKeyResponse `json:"items"`
	} `json:"data"`
}

func setupOAuthAPIKeysTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Token{}))

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled = oldRedisEnabled
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func callOAuthAPIKeysHandler(t *testing.T, userID int) (*httptest.ResponseRecorder, oauthAPIKeysTestResponse, map[string]any) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/oauth/api-keys", nil)
	c.Set("id", userID)
	GetOAuthAPIKeys(c)

	var response oauthAPIKeysTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	var raw map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &raw))
	return recorder, response, raw
}

func TestGetOAuthAPIKeysReturnsAllVisibleKeysInDescendingIDOrder(t *testing.T) {
	db := setupOAuthAPIKeysTestDB(t)
	allowIPs := "127.0.0.1\n::1"
	require.NoError(t, db.Create(&[]model.Token{
		{Id: 101, UserId: 7, Key: "older", Name: "older key", Status: common.TokenStatusEnabled, CreatedTime: 10},
		{Id: 102, UserId: 7, Key: "disabled", Name: "disabled key", Status: common.TokenStatusDisabled, CreatedTime: 20},
		{Id: 103, UserId: 7, Key: "expired", Name: "expired key", Status: common.TokenStatusExpired, CreatedTime: 30},
		{Id: 104, UserId: 7, Key: "exhausted", Name: "exhausted key", Status: common.TokenStatusExhausted, CreatedTime: 40},
		{Id: 999, UserId: 8, Key: "other-user", Name: "other user key", Status: common.TokenStatusEnabled},
	}).Error)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", 101).Updates(map[string]any{
		"allow_ips":            allowIPs,
		"remain_quota":         12,
		"used_quota":           34,
		"unlimited_quota":      true,
		"model_limits_enabled": true,
		"model_limits":         "gpt-4o,claude-3",
		"group":                "vip",
		"cross_group_retry":    true,
	}).Error)
	require.NoError(t, db.Delete(&model.Token{}, 102).Error)

	recorder, response, raw := callOAuthAPIKeysHandler(t, 7)
	require.True(t, response.Success)
	require.Empty(t, response.Message)
	require.Equal(t, 3, response.Data.Total)
	require.Len(t, response.Data.Items, 3)
	assert.Equal(t, []int{104, 103, 101}, []int{
		response.Data.Items[0].Id,
		response.Data.Items[1].Id,
		response.Data.Items[2].Id,
	})
	assert.Equal(t, common.TokenStatusExhausted, response.Data.Items[0].Status)
	assert.Equal(t, common.TokenStatusExpired, response.Data.Items[1].Status)
	assert.Equal(t, "sk-older", response.Data.Items[2].Key)
	assert.Equal(t, allowIPs, response.Data.Items[2].AllowIps)
	assert.Equal(t, 12, response.Data.Items[2].RemainQuota)
	assert.Equal(t, 34, response.Data.Items[2].UsedQuota)
	assert.True(t, response.Data.Items[2].UnlimitedQuota)
	assert.True(t, response.Data.Items[2].ModelLimitsEnabled)
	assert.Equal(t, "gpt-4o,claude-3", response.Data.Items[2].ModelLimits)
	assert.Equal(t, "vip", response.Data.Items[2].Group)
	assert.True(t, response.Data.Items[2].CrossGroupRetry)

	data, ok := raw["data"].(map[string]any)
	require.True(t, ok)
	items, ok := data["items"].([]any)
	require.True(t, ok)
	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, item, "user_id")
	assert.NotContains(t, item, "deleted_at")
	assert.NotContains(t, item, "source")
	assert.NotContains(t, item, "oauth_client_id")
	assert.NotContains(t, item, "oauth_scopes")

	assert.Equal(t, "no-store, no-cache, must-revalidate, private, max-age=0", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", recorder.Header().Get("Pragma"))
	assert.Equal(t, "0", recorder.Header().Get("Expires"))
	assert.Equal(t, "Authorization", recorder.Header().Get("Vary"))
}

func TestGetOAuthAPIKeysReturnsEmptyList(t *testing.T) {
	setupOAuthAPIKeysTestDB(t)

	_, response, _ := callOAuthAPIKeysHandler(t, 404)
	require.True(t, response.Success)
	assert.Equal(t, 0, response.Data.Total)
	assert.NotNil(t, response.Data.Items)
	assert.Empty(t, response.Data.Items)
}
