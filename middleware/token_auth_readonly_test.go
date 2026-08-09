package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenAuthReadOnlyExpiryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))

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

func TestTokenAuthReadOnlyRejectsExpiredOAuthTokenButAllowsLegacyExpiredToken(t *testing.T) {
	db := setupTokenAuthReadOnlyExpiryTestDB(t)
	user := &model.User{
		Username: "readonly-expiry-user",
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(user).Error)

	expiredAt := common.GetTimestamp() - 1
	oauthToken := &model.Token{
		UserId:      user.Id,
		Key:         "oauthExpiredReadonlyTokenKey",
		Status:      common.TokenStatusEnabled,
		ExpiredTime: expiredAt,
		Source:      model.OAuthTokenSource,
	}
	legacyToken := &model.Token{
		UserId:      user.Id,
		Key:         "legacyExpiredReadonlyTokenKey",
		Status:      common.TokenStatusEnabled,
		ExpiredTime: expiredAt,
		Source:      "",
	}
	require.NoError(t, db.Create(oauthToken).Error)
	require.NoError(t, db.Create(legacyToken).Error)

	router := gin.New()
	router.GET("/protected", TokenAuthReadOnly(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	tests := []struct {
		name       string
		key        string
		statusCode int
	}{
		{
			name:       "expired oauth token is rejected",
			key:        oauthToken.Key,
			statusCode: http.StatusUnauthorized,
		},
		{
			name:       "expired legacy token remains readable",
			key:        legacyToken.Key,
			statusCode: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			request.Header.Set("Authorization", "Bearer sk-"+tt.key)
			router.ServeHTTP(recorder, request)

			assert.Equal(t, tt.statusCode, recorder.Code)
		})
	}
}
