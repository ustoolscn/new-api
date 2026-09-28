package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOAuthTokenRoutes(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldMaster, oldPath := common.RedisEnabled, common.IsMasterNode, common.SQLitePath
	oldGlobal, oldCritical, oldSearch := common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable, common.SearchRateLimitEnable
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		common.RedisEnabled, common.IsMasterNode, common.SQLitePath = oldRedis, oldMaster, oldPath
		common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable, common.SearchRateLimitEnable = oldGlobal, oldCritical, oldSearch
	})
	common.RedisEnabled, common.IsMasterNode = false, false
	common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable, common.SearchRateLimitEnable = false, false, false
	common.SQLitePath = filepath.Join(t.TempDir(), "oauth-routes.db")
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitDB())
	db := model.DB
	model.LOG_DB = db
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.OAuthAccessToken{}))
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "oauth-owner", AffCode: "oauth-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"},
		{Id: 2, Username: "session-owner", AffCode: "session-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"},
	}).Error)
	for value, scope := range map[string]string{
		"oa-reader": model.OAuthPublicScope,
		"oa-writer": model.OAuthPublicScope + " " + model.OAuthScopeAPIKeysWrite,
	} {
		require.NoError(t, db.Create(&model.OAuthAccessToken{
			UserId: 1, ClientID: model.OAuthPublicClientID, Scope: scope,
			TokenHash: model.HashOAuthValue(value), ExpiresAt: common.GetTimestamp() + 600,
		}).Error)
	}
	require.NoError(t, db.Create(&[]model.Token{
		{Id: 11, UserId: 1, Name: "owned-key", Key: "owned-secret", Group: "default", RemainQuota: 100},
		{Id: 22, UserId: 2, Name: "other-key", Key: "other-secret", Group: "default", RemainQuota: 100},
	}).Error)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("oauth-route-test-secret"))))
	router.GET("/test-login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", 2)
		session.Set("username", "session-owner")
		session.Set("role", common.RoleCommonUser)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "default")
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	SetApiRouter(router)
	return router, db
}

func callOAuthTokenRoute(t *testing.T, router *gin.Engine, method, path, body, bearer string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+bearer)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	assert.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
	return recorder.Code, payload
}

func TestOAuthExistingAPIKeyRoutesEnforceScopes(t *testing.T) {
	router, db := setupOAuthTokenRoutes(t)
	for _, route := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/token/", ""},
		{http.MethodGet, "/api/token/search?keyword=owned", ""},
		{http.MethodGet, "/api/token/11", ""},
		{http.MethodPost, "/api/token/11/key", ""},
		{http.MethodPost, "/api/token/batch/keys", `{"ids":[11,22]}`},
		{http.MethodGet, "/api/user/self/groups", ""},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			status, payload := callOAuthTokenRoute(t, router, route.method, route.path, route.body, "oa-reader")
			require.Equal(t, http.StatusOK, status)
			assert.Equal(t, true, payload["success"], payload)
			encoded, err := common.Marshal(payload)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "other-secret")
			assert.NotContains(t, string(encoded), "other-key")
		})
	}
	for _, route := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/token/", `{"name":"blocked"}`},
		{http.MethodPut, "/api/token/", `{"id":11,"name":"blocked"}`},
		{http.MethodPut, "/api/token/?status_only=true", `{"id":11,"status":2}`},
		{http.MethodDelete, "/api/token/11", ""},
		{http.MethodPost, "/api/token/batch", `{"ids":[11]}`},
	} {
		t.Run("read-only denied "+route.method+" "+route.path, func(t *testing.T) {
			status, payload := callOAuthTokenRoute(t, router, route.method, route.path, route.body, "oa-reader")
			assert.Equal(t, http.StatusUnauthorized, status)
			assert.Equal(t, false, payload["success"])
		})
	}
	var tokens []model.Token
	require.NoError(t, db.Find(&tokens).Error)
	require.Len(t, tokens, 2)
	assert.Equal(t, "owned-key", tokens[0].Name)
	assert.Equal(t, common.TokenStatusEnabled, tokens[0].Status)
	for _, path := range []string{"/api/user/self", "/api/user/", "/api/option/"} {
		t.Run("unrelated route denied "+path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("Authorization", "Bearer oa-writer")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}

func TestOAuthExistingAPIKeyCRUDAndOwnership(t *testing.T) {
	router, db := setupOAuthTokenRoutes(t)
	status, payload := callOAuthTokenRoute(t, router, http.MethodPost, "/api/token/",
		`{"user_id":2,"name":"desktop","group":"default","remain_quota":123,"expired_time":-1}`, "oa-writer")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, payload["success"], payload)
	var token model.Token
	require.NoError(t, db.Where("name = ?", "desktop").First(&token).Error)
	assert.Equal(t, 1, token.UserId, "OAuth identity determines ownership, not the request body")
	id := strconv.Itoa(token.Id)
	_, payload = callOAuthTokenRoute(t, router, http.MethodPut, "/api/token/",
		fmt.Sprintf(`{"id":%d,"name":"edited","group":"vip","remain_quota":456,"expired_time":2000000000,"model_limits_enabled":true,"model_limits":"gpt-test","allow_ips":"127.0.0.1","cross_group_retry":true}`, token.Id), "oa-writer")
	require.Equal(t, true, payload["success"], payload)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, "edited", token.Name)
	assert.Equal(t, "vip", token.Group)
	assert.Equal(t, 456, token.RemainQuota)
	assert.Equal(t, int64(2000000000), token.ExpiredTime)
	assert.True(t, token.ModelLimitsEnabled)
	assert.Equal(t, "gpt-test", token.ModelLimits)
	require.NotNil(t, token.AllowIps)
	assert.Equal(t, "127.0.0.1", *token.AllowIps)
	assert.True(t, token.CrossGroupRetry)
	_, payload = callOAuthTokenRoute(t, router, http.MethodPut, "/api/token/?status_only=true", `{"id":`+id+`,"status":2}`, "oa-writer")
	require.Equal(t, true, payload["success"], payload)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
	assert.Equal(t, "edited", token.Name)
	assert.Equal(t, "vip", token.Group)
	_, payload = callOAuthTokenRoute(t, router, http.MethodDelete, "/api/token/"+id, "", "oa-writer")
	require.Equal(t, true, payload["success"], payload)
	assert.ErrorIs(t, db.First(&model.Token{}, token.Id).Error, gorm.ErrRecordNotFound)

	for _, route := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/token/22", ""},
		{http.MethodPost, "/api/token/22/key", ""},
		{http.MethodPut, "/api/token/", `{"id":22,"name":"hijacked"}`},
		{http.MethodPut, "/api/token/?status_only=true", `{"id":22,"status":2}`},
		{http.MethodDelete, "/api/token/22", ""},
	} {
		_, payload = callOAuthTokenRoute(t, router, route.method, route.path, route.body, "oa-writer")
		assert.Equal(t, false, payload["success"], route.path)
	}
	_, payload = callOAuthTokenRoute(t, router, http.MethodPost, "/api/token/batch", `{"ids":[11,22]}`, "oa-writer")
	require.Equal(t, true, payload["success"], payload)
	assert.Equal(t, float64(1), payload["data"])
	var other model.Token
	require.NoError(t, db.First(&other, 22).Error)
	assert.Equal(t, "other-key", other.Name)
	assert.Equal(t, common.TokenStatusEnabled, other.Status)
	assert.Equal(t, 100, other.RemainQuota)
}

func TestOAuthTokenRoutesPreserveSessionsAndDoNotFallBack(t *testing.T) {
	router, db := setupOAuthTokenRoutes(t)
	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/test-login", nil))
	require.Equal(t, http.StatusNoContent, login.Code)
	cookies := login.Result().Cookies()
	require.NotEmpty(t, cookies)
	for _, tc := range []struct {
		name, method, path, auth, body string
		wantStatus                     int
		wantSuccess                    bool
	}{
		{name: "session read", method: http.MethodGet, path: "/api/token/22", wantStatus: 200, wantSuccess: true},
		{name: "session groups", method: http.MethodGet, path: "/api/user/self/groups", wantStatus: 200, wantSuccess: true},
		{name: "session write", method: http.MethodPost, path: "/api/token/", body: `{"name":"session-created"}`, wantStatus: 200, wantSuccess: true},
		{name: "explicit OAuth uses OAuth owner", method: http.MethodGet, path: "/api/token/11", auth: "Bearer oa-reader", wantStatus: 200, wantSuccess: true},
		{name: "OAuth cannot use cookie owner", method: http.MethodGet, path: "/api/token/22", auth: "Bearer oa-reader", wantStatus: 200},
		{name: "read scope cannot use cookie write", method: http.MethodPost, path: "/api/token/", auth: "Bearer oa-reader", body: `{"name":"blocked"}`, wantStatus: 401},
		{name: "invalid OAuth cannot use cookie", method: http.MethodPost, path: "/api/token/", auth: "Bearer oa-invalid", body: `{"name":"blocked"}`, wantStatus: 401},
		{name: "malformed OAuth cannot use cookie", method: http.MethodPost, path: "/api/token/", auth: "oa-writer", body: `{"name":"blocked"}`, wantStatus: 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("New-Api-User", "2")
			if tc.auth != "" {
				request.Header.Set("Authorization", tc.auth)
			}
			for _, c := range cookies {
				request.AddCookie(c)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			var payload struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Equal(t, tc.wantSuccess, payload.Success)
		})
	}
	var created model.Token
	require.NoError(t, db.Where("name = ?", "session-created").First(&created).Error)
	assert.Equal(t, 2, created.UserId)
	var count int64
	require.NoError(t, db.Model(&model.Token{}).Where("name = ?", "blocked").Count(&count).Error)
	assert.Zero(t, count)
}
