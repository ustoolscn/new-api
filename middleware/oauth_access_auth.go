package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// OAuthAccessAuth authenticates only dedicated oa- bearer tokens issued by
// the Hi Codex PKCE flow. It intentionally does not fall back to TokenAuth or
// UserAuth, so these credentials can never reach relay quota/token handling.
func OAuthAccessAuth(requiredScopes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")

		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			oauthAccessUnauthorized(c)
			return
		}
		value := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		if !strings.HasPrefix(value, "oa-") || len(value) <= len("oa-") || strings.ContainsAny(value, " \t\r\n") {
			oauthAccessUnauthorized(c)
			return
		}

		token, err := model.GetOAuthAccessTokenByValue(value)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				oauthAccessUnauthorized(c)
			} else {
				common.SysLog("OAuthAccessAuth lookup error: " + err.Error())
				oauthAccessDatabaseError(c)
			}
			return
		}
		now := common.GetTimestamp()
		if token.ClientID != model.OAuthPublicClientID ||
			token.RevokedAt != nil || token.ExpiresAt <= now ||
			model.ValidateOAuthAccessTokenScope(token, requiredScopes...) != nil {
			oauthAccessUnauthorized(c)
			return
		}

		userCache, err := model.GetUserCache(token.UserId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				oauthAccessUnauthorized(c)
				return
			}
			common.SysLog("OAuthAccessAuth user lookup error: " + err.Error())
			oauthAccessDatabaseError(c)
			return
		}
		if userCache.Status != common.UserStatusEnabled {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgAuthUserBanned),
			})
			c.Abort()
			return
		}

		normalizedScope, _ := model.NormalizeOAuthScope(token.Scope)
		c.Set("id", token.UserId)
		common.SetContextKey(c, constant.ContextKeyUserName, userCache.Username)
		c.Set("oauth_access_token_id", token.Id)
		c.Set("oauth_client_id", token.ClientID)
		c.Set("oauth_scopes", normalizedScope)
		c.Next()
	}
}

func oauthAccessUnauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"message": common.TranslateMessage(c, i18n.MsgTokenInvalid),
	})
	c.Abort()
}

func oauthAccessDatabaseError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"message": common.TranslateMessage(c, i18n.MsgDatabaseError),
	})
	c.Abort()
}
