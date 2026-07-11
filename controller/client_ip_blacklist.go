package controller

import (
	"net/http"
	"net/netip"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

type ClientIPSettingUpdateRequest struct {
	BlacklistEnabled bool     `json:"blacklist_enabled"`
	Blacklist        []string `json:"blacklist"`
	TrustedProxies   []string `json:"trusted_proxies"`
	ConfirmSelfBlock bool     `json:"confirm_self_block"`
}

func GetClientIPBlacklistSetting(c *gin.Context) {
	snapshot := system_setting.GetClientIPSnapshot()
	clientIP, err := common.ResolveClientIP(
		c.Request.RemoteAddr,
		c.GetHeader("X-Forwarded-For"),
		c.GetHeader("X-Real-IP"),
		snapshot.TrustedProxies,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	common.ApiSuccess(c, gin.H{
		"blacklist_enabled": snapshot.BlacklistEnabled,
		"blacklist":         clientIPPrefixStrings(snapshot.Blacklist),
		"trusted_proxies":   clientIPPrefixStrings(snapshot.TrustedProxies),
		"current_ip":        clientIP.String(),
	})
}

func UpdateClientIPBlacklistSetting(c *gin.Context) {
	var request ClientIPSettingUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid request",
		})
		return
	}

	candidate := system_setting.ClientIPSetting{
		BlacklistEnabled: request.BlacklistEnabled,
		Blacklist:        request.Blacklist,
		TrustedProxies:   request.TrustedProxies,
	}
	snapshot, err := system_setting.ValidateClientIPSetting(candidate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	activeSnapshot := system_setting.GetClientIPSnapshot()
	activeClientIP, err := common.ResolveClientIP(
		c.Request.RemoteAddr,
		c.GetHeader("X-Forwarded-For"),
		c.GetHeader("X-Real-IP"),
		activeSnapshot.TrustedProxies,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	candidateClientIP, err := common.ResolveClientIP(
		c.Request.RemoteAddr,
		c.GetHeader("X-Forwarded-For"),
		c.GetHeader("X-Real-IP"),
		snapshot.TrustedProxies,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	selfBlocked := false
	if snapshot.BlacklistEnabled {
		for _, prefix := range snapshot.Blacklist {
			if prefix.Contains(activeClientIP) || prefix.Contains(candidateClientIP) {
				selfBlocked = true
				break
			}
		}
	}
	if selfBlocked && !request.ConfirmSelfBlock {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "the blacklist contains your current client IP",
			"code":    "client_ip_self_block_confirmation_required",
		})
		return
	}

	normalizedBlacklist := clientIPPrefixStrings(snapshot.Blacklist)
	normalizedTrustedProxies := clientIPPrefixStrings(snapshot.TrustedProxies)
	blacklistJSON, err := common.Marshal(normalizedBlacklist)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	trustedProxiesJSON, err := common.Marshal(normalizedTrustedProxies)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if err := model.UpdateOptionsBulk(map[string]string{
		"client_ip_setting.blacklist_enabled": strconv.FormatBool(snapshot.BlacklistEnabled),
		"client_ip_setting.blacklist":         string(blacklistJSON),
		"client_ip_setting.trusted_proxies":   string(trustedProxiesJSON),
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "client_ip_blacklist.update", map[string]interface{}{
		"blacklist_enabled":    snapshot.BlacklistEnabled,
		"blacklist_count":      len(snapshot.Blacklist),
		"trusted_proxy_count":  len(snapshot.TrustedProxies),
		"self_block_confirmed": selfBlocked && request.ConfirmSelfBlock,
	})

	common.ApiSuccess(c, gin.H{
		"blacklist_enabled": snapshot.BlacklistEnabled,
		"blacklist":         normalizedBlacklist,
		"trusted_proxies":   normalizedTrustedProxies,
		"current_ip":        candidateClientIP.String(),
	})
}

func clientIPPrefixStrings(prefixes []netip.Prefix) []string {
	values := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		values = append(values, prefix.String())
	}
	return values
}
