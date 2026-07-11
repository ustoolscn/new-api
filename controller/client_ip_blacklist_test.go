package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type clientIPBlacklistTestResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Data    struct {
		BlacklistEnabled bool     `json:"blacklist_enabled"`
		Blacklist        []string `json:"blacklist"`
		TrustedProxies   []string `json:"trusted_proxies"`
		CurrentIP        string   `json:"current_ip"`
	} `json:"data"`
}

func TestGetClientIPBlacklistSettingReturnsNormalizedSettingAndCurrentIP(t *testing.T) {
	setupClientIPBlacklistControllerTest(t)
	require.NoError(t, model.UpdateOptionsBulk(map[string]string{
		"client_ip_setting.blacklist_enabled": "true",
		"client_ip_setting.blacklist":         `["203.0.113.7","2001:db8::/48"]`,
		"client_ip_setting.trusted_proxies":   `[]`,
	}))
	require.NoError(t, system_setting.UpdateAndSyncClientIPSetting())

	recorder := performClientIPSettingRequest(http.MethodGet, nil, "198.51.100.20:50000", GetClientIPBlacklistSetting)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response clientIPBlacklistTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, []string{"203.0.113.7/32", "2001:db8::/48"}, response.Data.Blacklist)
	assert.Equal(t, "198.51.100.20", response.Data.CurrentIP)
}

func TestUpdateClientIPBlacklistSettingRejectsInvalidRuleWithoutPersistence(t *testing.T) {
	db := setupClientIPBlacklistControllerTest(t)

	recorder := performClientIPSettingRequest(http.MethodPut, map[string]any{
		"blacklist_enabled":  true,
		"blacklist":          []string{"invalid"},
		"trusted_proxies":    []string{},
		"confirm_self_block": false,
	}, "198.51.100.20:50000", UpdateClientIPBlacklistSetting)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var count int64
	require.NoError(t, db.Model(&model.Option{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateClientIPBlacklistSettingRequiresSelfBlockConfirmation(t *testing.T) {
	setupClientIPBlacklistControllerTest(t)

	recorder := performClientIPSettingRequest(http.MethodPut, map[string]any{
		"blacklist_enabled":  true,
		"blacklist":          []string{"203.0.113.0/24"},
		"trusted_proxies":    []string{},
		"confirm_self_block": false,
	}, "203.0.113.7:50000", UpdateClientIPBlacklistSetting)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	var response clientIPBlacklistTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "client_ip_self_block_confirmation_required", response.Code)
}

func TestUpdateClientIPBlacklistSettingPersistsConfirmedSetting(t *testing.T) {
	db := setupClientIPBlacklistControllerTest(t)

	recorder := performClientIPSettingRequest(http.MethodPut, map[string]any{
		"blacklist_enabled":  true,
		"blacklist":          []string{"203.0.113.7", "203.0.113.7/32"},
		"trusted_proxies":    []string{"10.0.0.0/8"},
		"confirm_self_block": true,
	}, "203.0.113.7:50000", UpdateClientIPBlacklistSetting)

	assert.Equal(t, http.StatusOK, recorder.Code)
	setting := system_setting.GetClientIPSetting()
	assert.True(t, setting.BlacklistEnabled)
	assert.Equal(t, []string{"203.0.113.7/32"}, setting.Blacklist)
	assert.Equal(t, []string{"10.0.0.0/8"}, setting.TrustedProxies)

	var options []model.Option
	require.NoError(t, db.Order("key").Find(&options).Error)
	assert.Len(t, options, 3)
	assert.True(t, system_setting.GetClientIPSnapshot().BlacklistEnabled)
}

func TestUpdateOptionRejectsClientIPSettingKeys(t *testing.T) {
	setupClientIPBlacklistControllerTest(t)

	recorder := performClientIPSettingRequest(http.MethodPut, map[string]any{
		"key":   "client_ip_setting.blacklist",
		"value": `[]`,
	}, "198.51.100.20:50000", UpdateOption)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "dedicated endpoint")
}

func setupClientIPBlacklistControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	originalSetting := system_setting.GetClientIPSetting()
	originalSnapshot := system_setting.GetClientIPSnapshot()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.User{}, &model.Log{}))

	cfg := config.GlobalConfig.Get("client_ip_setting")
	require.NotNil(t, cfg)
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		"blacklist_enabled": "false",
		"blacklist":         `[]`,
		"trusted_proxies":   `[]`,
	}))
	require.NoError(t, system_setting.UpdateAndSyncClientIPSetting())
	model.InitOptionMap(false)

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
			"blacklist_enabled": fmt.Sprintf("%t", originalSetting.BlacklistEnabled),
			"blacklist":         common.GetJsonString(originalSetting.Blacklist),
			"trusted_proxies":   common.GetJsonString(originalSetting.TrustedProxies),
		}))
		require.NoError(t, system_setting.UpdateAndSyncClientIPSetting())
		assert.Equal(t, originalSnapshot, system_setting.GetClientIPSnapshot())
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func performClientIPSettingRequest(method string, body any, remoteAddr string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	var bodyReader *strings.Reader
	if body == nil {
		bodyReader = strings.NewReader("")
	} else {
		bodyReader = strings.NewReader(common.GetJsonString(body))
	}
	req := httptest.NewRequest(method, "/api/option/client-ip-blacklist", bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	handler(c)
	return recorder
}
