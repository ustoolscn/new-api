package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// OAuthAPIKeyResponse is the public representation of a user's API key. It
// deliberately does not embed model.Token so internal OAuth and ownership
// fields cannot leak into the response.
type OAuthAPIKeyResponse struct {
	Id                 int    `json:"id"`
	Name               string `json:"name"`
	Key                string `json:"key"`
	Status             int    `json:"status"`
	CreatedTime        int64  `json:"created_time"`
	AccessedTime       int64  `json:"accessed_time"`
	ExpiredTime        int64  `json:"expired_time"`
	RemainQuota        int    `json:"remain_quota"`
	UsedQuota          int    `json:"used_quota"`
	UnlimitedQuota     bool   `json:"unlimited_quota"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
	ModelLimits        string `json:"model_limits"`
	AllowIps           string `json:"allow_ips"`
	Group              string `json:"group"`
	CrossGroupRetry    bool   `json:"cross_group_retry"`
}

type OAuthAPIKeyListResponse struct {
	Total int                   `json:"total"`
	Items []OAuthAPIKeyResponse `json:"items"`
}

func toOAuthAPIKeyResponse(token *model.Token) OAuthAPIKeyResponse {
	allowIPs := ""
	if token.AllowIps != nil {
		allowIPs = *token.AllowIps
	}

	key := token.Key
	if !strings.HasPrefix(key, "sk-") {
		key = "sk-" + key
	}

	return OAuthAPIKeyResponse{
		Id:                 token.Id,
		Name:               token.Name,
		Key:                key,
		Status:             token.Status,
		CreatedTime:        token.CreatedTime,
		AccessedTime:       token.AccessedTime,
		ExpiredTime:        token.ExpiredTime,
		RemainQuota:        token.RemainQuota,
		UsedQuota:          token.UsedQuota,
		UnlimitedQuota:     token.UnlimitedQuota,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
		AllowIps:           allowIPs,
		Group:              token.Group,
		CrossGroupRetry:    token.CrossGroupRetry,
	}
}

// GetOAuthAPIKeys returns the complete API-key inventory for the user
// authenticated by OAuthAccessAuth. The handler repeats the sensitive
// response cache policy here so it remains true even when called directly in
// tests or by a differently composed route.
func GetOAuthAPIKeys(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Vary", "Authorization")

	tokens, err := model.GetAllOAuthAPIKeys(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	items := make([]OAuthAPIKeyResponse, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, toOAuthAPIKeyResponse(token))
	}

	common.ApiSuccess(c, OAuthAPIKeyListResponse{
		Total: len(items),
		Items: items,
	})
}
