package middleware

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func ClientIPBlacklist() gin.HandlerFunc {
	return ClientIPBlacklistWithSnapshot(system_setting.GetClientIPSnapshot)
}

func ClientIPBlacklistWithSnapshot(getSnapshot func() system_setting.ClientIPSnapshot) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := getSnapshot()
		if !snapshot.BlacklistEnabled || len(snapshot.Blacklist) == 0 || c.Request.URL.Path == "/api/status" {
			c.Next()
			return
		}

		clientIP, err := common.ResolveClientIP(
			c.Request.RemoteAddr,
			c.GetHeader("X-Forwarded-For"),
			c.GetHeader("X-Real-IP"),
			snapshot.TrustedProxies,
		)
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("client IP blacklist rejected request path=%q error=%q", c.Request.URL.Path, err.Error()))
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "client IP is blocked",
				"code":    "client_ip_blocked",
			})
			return
		}

		for _, prefix := range snapshot.Blacklist {
			if !prefix.Contains(clientIP) {
				continue
			}
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("client IP blacklist blocked request client_ip=%s path=%q", clientIP.String(), c.Request.URL.Path))
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "client IP is blocked",
				"code":    "client_ip_blocked",
			})
			return
		}

		c.Next()
	}
}
