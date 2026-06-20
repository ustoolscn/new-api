package controller

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func Playground(c *gin.Context) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	useAccessToken := c.GetBool("use_access_token")
	if useAccessToken {
		newAPIError = types.NewError(errors.New("暂不支持使用 access token"), types.ErrorCodeAccessDenied, types.ErrOptionWithSkipRetry())
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		return
	}

	userId := c.GetInt("id")

	// Write user context to ensure acceptUnsetRatio is available
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		return
	}
	userCache.WriteContext(c)

	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("playground-%s", relayInfo.UsingGroup),
		Group:  relayInfo.UsingGroup,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	Relay(c, types.RelayFormatOpenAI)
}

func UploadPlaygroundImage(c *gin.Context) {
	if c.GetBool("use_access_token") {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "暂不支持使用 access token",
		})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 512<<20)
	if err := c.Request.ParseMultipartForm(512 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "图片上传请求无效",
		})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "缺少图片文件",
		})
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if !isPlaygroundImageFile(header.Filename, contentType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "只支持图片文件",
		})
		return
	}

	image, err := service.UploadPlaygroundImageToHost(
		c.Request.Context(),
		service.DefaultPlaygroundImageHostConfig(),
		header.Filename,
		contentType,
		file,
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"url":     image.URL,
		"data":    image,
	})
}

func isPlaygroundImageFile(filename, contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if strings.HasPrefix(contentType, "image/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	default:
		return false
	}
}
