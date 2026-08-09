package middleware

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

func setupOAuthAccessAuthTestDB(t *testing.T) *gorm.DB {
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.OAuthAccessToken{}))

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

func seedOAuthAccessAuthUser(t *testing.T, db *gorm.DB, status int) *model.User {
	t.Helper()
	user := &model.User{
		Username: fmt.Sprintf("oauth-access-%d", status),
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   status,
		Group:    "default",
	}
	require.NoError(t, db.Create(user).Error)
	return user
}

func createOAuthAccessAuthToken(t *testing.T, db *gorm.DB, userID int, value, clientID, scope string, expiresAt int64, revokedAt *int64) *model.OAuthAccessToken {
	t.Helper()
	token := &model.OAuthAccessToken{
		TokenHash: model.HashOAuthValue(value),
		UserId:    userID,
		ClientID:  clientID,
		Scope:     scope,
		CreatedAt: common.GetTimestamp() - 10,
		ExpiresAt: expiresAt,
		RevokedAt: revokedAt,
	}
	require.NoError(t, db.Create(token).Error)
	return token
}

func requestOAuthAccessAuth(t *testing.T, authorization string, requiredScope string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.GET("/protected", OAuthAccessAuth(requiredScope), func(c *gin.Context) {
		assert.NotZero(t, c.GetInt("id"))
		assert.NotZero(t, c.GetInt("oauth_access_token_id"))
		assert.Equal(t, model.OAuthPublicClientID, c.GetString("oauth_client_id"))
		assert.Equal(t, model.OAuthPublicScope, c.GetString("oauth_scopes"))
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestOAuthAccessAuthAcceptsValidTokenAndSetsContext(t *testing.T) {
	db := setupOAuthAccessAuthTestDB(t)
	user := seedOAuthAccessAuthUser(t, db, common.UserStatusEnabled)
	value := "oa-valid-test-token"
	createOAuthAccessAuthToken(t, db, user.Id, value, model.OAuthPublicClientID, model.OAuthPublicScope, common.GetTimestamp()+60, nil)

	recorder := requestOAuthAccessAuth(t, "Bearer "+value, model.OAuthScopeAPIKeysRead)
	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
}

func TestOAuthAccessAuthRejectsNonOAuthAndInvalidTokens(t *testing.T) {
	db := setupOAuthAccessAuthTestDB(t)
	user := seedOAuthAccessAuthUser(t, db, common.UserStatusEnabled)
	now := common.GetTimestamp()
	value := "oa-invalid-test-token"
	createOAuthAccessAuthToken(t, db, user.Id, value, model.OAuthPublicClientID, model.OAuthPublicScope, now+60, nil)

	for _, authorization := range []string{
		"",
		"Bearer sk-manual-token",
		value,
		"bearer " + value,
		"Bearer oa-missing-token",
	} {
		t.Run(fmt.Sprintf("authorization_%q", authorization), func(t *testing.T) {
			recorder := requestOAuthAccessAuth(t, authorization, model.OAuthScopeAPIKeysRead)
			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}

func TestOAuthAccessAuthRejectsExpiredRevokedWrongClientAndInsufficientScope(t *testing.T) {
	cases := []struct {
		name      string
		clientID  string
		scope     string
		expiresAt int64
		revoked   bool
		status    int
		wantCode  int
	}{
		{name: "expired", expiresAt: common.GetTimestamp() - 1, wantCode: http.StatusUnauthorized},
		{name: "revoked", expiresAt: common.GetTimestamp() + 60, revoked: true, wantCode: http.StatusUnauthorized},
		{name: "wrong client", clientID: "other-client", expiresAt: common.GetTimestamp() + 60, wantCode: http.StatusUnauthorized},
		{name: "insufficient scope", scope: model.OAuthScopeAPIKeysRead, expiresAt: common.GetTimestamp() + 60, wantCode: http.StatusUnauthorized},
		{name: "malformed scope", scope: "api", expiresAt: common.GetTimestamp() + 60, wantCode: http.StatusUnauthorized},
		{name: "disabled user", expiresAt: common.GetTimestamp() + 60, status: common.UserStatusDisabled, wantCode: http.StatusForbidden},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			db := setupOAuthAccessAuthTestDB(t)
			status := tt.status
			if status == 0 {
				status = common.UserStatusEnabled
			}
			user := seedOAuthAccessAuthUser(t, db, status)
			clientID := tt.clientID
			if clientID == "" {
				clientID = model.OAuthPublicClientID
			}
			scope := tt.scope
			if scope == "" {
				scope = model.OAuthPublicScope
			}
			var revokedAt *int64
			if tt.revoked {
				value := common.GetTimestamp() - 1
				revokedAt = &value
			}
			value := "oa-" + strings.ReplaceAll(tt.name, " ", "-")
			createOAuthAccessAuthToken(t, db, user.Id, value, clientID, scope, tt.expiresAt, revokedAt)
			recorder := requestOAuthAccessAuth(t, "Bearer "+value, model.OAuthScopeAPIKeysRead)
			assert.Equal(t, tt.wantCode, recorder.Code)
		})
	}
}

func TestOAuthAccessAuthRejectsDeletedUserAsUnauthorized(t *testing.T) {
	db := setupOAuthAccessAuthTestDB(t)
	user := seedOAuthAccessAuthUser(t, db, common.UserStatusEnabled)
	value := "oa-deleted-user-token"
	createOAuthAccessAuthToken(t, db, user.Id, value, model.OAuthPublicClientID, model.OAuthPublicScope, common.GetTimestamp()+60, nil)
	require.NoError(t, db.Delete(user).Error)

	recorder := requestOAuthAccessAuth(t, "Bearer "+value, model.OAuthScopeAPIKeysRead)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}
