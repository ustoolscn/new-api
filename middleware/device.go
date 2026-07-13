package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func DeviceBanCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		fingerprint := strings.TrimSpace(c.GetHeader(model.DeviceFingerprintHeader))
		if fingerprint == "" {
			c.Next()
			return
		}
		banned, err := model.IsDeviceFingerprintBanned(fingerprint)
		if err != nil {
			common.SysLog("device ban check failed: " + err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgDatabaseError)})
			c.Abort()
			return
		}
		if banned {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgAuthDeviceBanned)})
			c.Abort()
			return
		}
		c.Next()
	}
}
