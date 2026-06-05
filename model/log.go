package model

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

func applyExplicitLogTextFilter(tx *gorm.DB, column string, value string) (*gorm.DB, error) {
	if value == "" {
		return tx, nil
	}
	if strings.Contains(value, "%") {
		pattern, err := sanitizeLikePattern(value)
		if err != nil {
			return nil, err
		}
		return tx.Where(column+" LIKE ? ESCAPE '!'", pattern), nil
	}
	return tx.Where(column+" = ?", value), nil
}

type Log struct {
	Id               int    `json:"id" gorm:"index:idx_created_at_id,priority:2;index:idx_user_id_id,priority:2"`
	UserId           int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:1;index:idx_created_at_type"`
	Type             int    `json:"type" gorm:"index:idx_created_at_type"`
	Content          string `json:"content"`
	Username         string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName        string `json:"token_name" gorm:"index;default:''"`
	ModelName        string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota            int    `json:"quota" gorm:"default:0"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`
	UseTime          int    `json:"use_time" gorm:"default:0"`
	IsStream         bool   `json:"is_stream"`
	ChannelId        int    `json:"channel" gorm:"index"`
	ChannelName      string `json:"channel_name" gorm:"->"`
	TokenId          int    `json:"token_id" gorm:"default:0;index"`
	Group            string `json:"group" gorm:"index"`
	Ip               string `json:"ip" gorm:"index;default:''"`
	RequestId        string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	// Keep the API field for compatibility, but do not read/write a DB column.
	// Some existing deployments do not have logs.upstream_request_id and should
	// not be forced to alter the logs table.
	UpstreamRequestId string `json:"upstream_request_id,omitempty" gorm:"-"`
	Other             string `json:"other"`
}

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
)

func formatUserLogs(logs []*Log, startIdx int) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// Remove admin-only debug fields.
			delete(otherMap, "admin_info")
			// delete(otherMap, "reject_reason")
			delete(otherMap, "stream_status")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
		logs[i].Id = startIdx + i + 1
	}
}

func GetLogByTokenId(tokenId int) (logs []*Log, err error) {
	err = LOG_DB.Model(&Log{}).Where("token_id = ?", tokenId).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs, 0)
	return logs, err
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// RecordLogWithAdminInfo 记录操作日志，并将管理员相关信息存入 Other.admin_info，
func RecordLogWithAdminInfo(userId int, logType int, content string, adminInfo map[string]interface{}) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	if len(adminInfo) > 0 {
		other := map[string]interface{}{
			"admin_info": adminInfo,
		}
		log.Other = common.MapToJsonStr(other)
	}
	if err := LOG_DB.Create(log).Error; err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

func RecordTopupLog(userId int, content string, callerIp string, paymentMethod string, callbackPaymentMethod string) {
	username, _ := GetUsernameById(userId, false)
	adminInfo := map[string]interface{}{
		"server_ip":               common.GetIp(),
		"node_name":               common.NodeName,
		"caller_ip":               callerIp,
		"payment_method":          paymentMethod,
		"callback_payment_method": callbackPaymentMethod,
		"version":                 common.Version,
	}
	other := map[string]interface{}{
		"admin_info": adminInfo,
	}
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Ip:        callerIp,
		Other:     common.MapToJsonStr(other),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record topup log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, common.LocalLogPreview(content)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	otherStr := common.MapToJsonStr(other)
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip:               c.ClientIP(),
		RequestId:        requestId,
		Other:            otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	otherStr := common.MapToJsonStr(params.Other)
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip:               c.ClientIP(),
		RequestId:        requestId,
		Other:            otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(userId, username, params.ModelName, params.Quota, common.GetTimestamp(), params.PromptTokens+params.CompletionTokens)
		})
	}
}

type RecordTaskBillingLogParams struct {
	UserId    int
	LogType   int
	Content   string
	ChannelId int
	ModelName string
	Quota     int
	TokenId   int
	Group     string
	Other     map[string]interface{}
}

func RecordTaskBillingLog(params RecordTaskBillingLogParams) {
	if params.LogType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(params.UserId, false)
	tokenName := ""
	if params.TokenId > 0 {
		if token, err := GetTokenById(params.TokenId); err == nil {
			tokenName = token.Name
		}
	}
	log := &Log{
		UserId:    params.UserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      params.LogType,
		Content:   params.Content,
		TokenName: tokenName,
		ModelName: params.ModelName,
		Quota:     params.Quota,
		ChannelId: params.ChannelId,
		TokenId:   params.TokenId,
		Group:     params.Group,
		Other:     common.MapToJsonStr(params.Other),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record task billing log: " + err.Error())
	}
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("logs.type = ?", logType)
	}

	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", username); err != nil {
		return nil, 0, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.created_at desc, logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if common.MemoryCacheEnabled {
			// Cache get channel
			for _, channelId := range channelIds.Items() {
				if cacheChannel, err := CacheGetChannel(channelId); err == nil {
					channels = append(channels, struct {
						Id   int    `gorm:"column:id"`
						Name string `gorm:"column:name"`
					}{
						Id:   channelId,
						Name: cacheChannel.Name,
					})
				}
			}
		} else {
			// Bulk query channels from DB
			if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
				return logs, total, err
			}
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

const logSearchCountLimit = 10000

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("logs.user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("logs.user_id = ? and logs.type = ?", userId, logType)
	}

	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}

	formatUserLogs(logs, startIdx)
	return logs, total, err
}

type Stat struct {
	Quota            int              `json:"quota"`
	Rpm              int              `json:"rpm"`
	Tpm              int              `json:"tpm"`
	InputTokens      int              `json:"input_tokens"`
	PromptTokens     int              `json:"prompt_tokens"`
	CacheTokens      int              `json:"cache_tokens"`
	CompletionTokens int              `json:"completion_tokens"`
	ModelStats       []ModelTokenStat `json:"model_stats" gorm:"-"`
}

type ModelTokenStat struct {
	ModelName        string `json:"model_name"`
	Quota            int    `json:"quota"`
	InputTokens      int    `json:"input_tokens"`
	PromptTokens     int    `json:"prompt_tokens"`
	CacheTokens      int    `json:"cache_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	cacheReadTokens  int    `gorm:"-"`
}

type logOtherTokenUsage struct {
	CacheTokens           int `json:"cache_tokens"`
	CacheCreationTokens   int `json:"cache_creation_tokens"`
	CacheCreationTokens5m int `json:"cache_creation_tokens_5m"`
	CacheCreationTokens1h int `json:"cache_creation_tokens_1h"`
}

type cacheTokenStats struct {
	Read  int
	Total int
}

func cacheTokenStatsFromLogOther(other string) cacheTokenStats {
	if other == "" {
		return cacheTokenStats{}
	}
	var usage logOtherTokenUsage
	if err := common.UnmarshalJsonStr(other, &usage); err != nil {
		return cacheTokenStats{}
	}
	cacheWriteTokens := usage.CacheCreationTokens
	if usage.CacheCreationTokens5m > 0 || usage.CacheCreationTokens1h > 0 {
		cacheWriteTokens = usage.CacheCreationTokens5m + usage.CacheCreationTokens1h
	}
	return cacheTokenStats{
		Read:  usage.CacheTokens,
		Total: usage.CacheTokens + cacheWriteTokens,
	}
}

func cacheTokensFromLogOther(other string) int {
	return cacheTokenStatsFromLogOther(other).Total
}

func visibleInputTokens(promptTokens int, cacheReadTokens int) int {
	inputTokens := promptTokens - cacheReadTokens
	if inputTokens < 0 {
		return 0
	}
	return inputTokens
}

func applyLogStatFilters(tx *gorm.DB, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, requestId string) (*gorm.DB, error) {
	var err error
	if tx, err = applyExplicitLogTextFilter(tx, "username", username); err != nil {
		return nil, err
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("request_id = ?", requestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if tx, err = applyExplicitLogTextFilter(tx, "model_name", modelName); err != nil {
		return nil, err
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
	}
	return tx.Where("type = ?", LogTypeConsume), nil
}

func applyCacheTokenCandidateFilter(tx *gorm.DB) *gorm.DB {
	return tx.Where(
		"(other LIKE ? OR other LIKE ? OR other LIKE ? OR other LIKE ?)",
		`%"cache_tokens"%`,
		`%"cache_creation_tokens"%`,
		`%"cache_creation_tokens_5m"%`,
		`%"cache_creation_tokens_1h"%`,
	)
}

func sumCacheTokenStatsFromLogQuery(tx *gorm.DB, modelStats map[string]*ModelTokenStat) (cacheTokenStats, error) {
	var stats cacheTokenStats
	var logs []Log
	err := applyCacheTokenCandidateFilter(tx).
		Select("id", "model_name", "other").
		FindInBatches(&logs, 1000, func(_ *gorm.DB, _ int) error {
			for _, log := range logs {
				logCacheStats := cacheTokenStatsFromLogOther(log.Other)
				stats.Read += logCacheStats.Read
				stats.Total += logCacheStats.Total
				if modelStats == nil {
					continue
				}
				modelStat, ok := modelStats[log.ModelName]
				if !ok {
					modelStat = &ModelTokenStat{ModelName: log.ModelName}
					modelStats[log.ModelName] = modelStat
				}
				modelStat.CacheTokens += logCacheStats.Total
				modelStat.cacheReadTokens += logCacheStats.Read
			}
			return nil
		}).Error
	return stats, err
}

func collectModelTokenStatsFromLogQuery(tx *gorm.DB) (map[string]*ModelTokenStat, error) {
	var rows []ModelTokenStat
	if err := tx.
		Select("model_name, COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens, COALESCE(SUM(completion_tokens), 0) AS completion_tokens").
		Group("model_name").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	modelStats := make(map[string]*ModelTokenStat, len(rows))
	for i := range rows {
		rows[i].InputTokens = visibleInputTokens(rows[i].PromptTokens, 0)
		modelStats[rows[i].ModelName] = &rows[i]
	}
	return modelStats, nil
}

func modelTokenStatsMapToSortedSlice(modelStats map[string]*ModelTokenStat) []ModelTokenStat {
	items := make([]ModelTokenStat, 0, len(modelStats))
	for _, item := range modelStats {
		item.InputTokens = visibleInputTokens(item.PromptTokens, item.cacheReadTokens)
		item.cacheReadTokens = 0
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		leftTotal := items[i].Quota
		rightTotal := items[j].Quota
		if leftTotal == rightTotal {
			return items[i].ModelName < items[j].ModelName
		}
		return leftTotal > rightTotal
	})
	return items
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, requestId string) (stat Stat, err error) {
	tx, err := applyLogStatFilters(
		LOG_DB.Table("logs").Select("COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens, COALESCE(SUM(completion_tokens), 0) AS completion_tokens"),
		startTimestamp,
		endTimestamp,
		modelName,
		username,
		tokenName,
		channel,
		group,
		requestId,
	)
	if err != nil {
		return stat, err
	}

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery, err := applyLogStatFilters(
		LOG_DB.Table("logs").Select("COUNT(*) AS rpm, COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(completion_tokens), 0) AS tpm"),
		startTimestamp,
		endTimestamp,
		modelName,
		username,
		tokenName,
		channel,
		group,
		requestId,
	)
	if err != nil {
		return stat, err
	}

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 执行查询
	if err := tx.Scan(&stat).Error; err != nil {
		common.SysError("failed to query log stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	stat.InputTokens = visibleInputTokens(stat.PromptTokens, 0)
	if err := rpmTpmQuery.Scan(&stat).Error; err != nil {
		common.SysError("failed to query rpm/tpm stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}

	modelStatsQuery, err := applyLogStatFilters(
		LOG_DB.Table("logs"),
		startTimestamp,
		endTimestamp,
		modelName,
		username,
		tokenName,
		channel,
		group,
		requestId,
	)
	if err != nil {
		return stat, err
	}
	modelStats, err := collectModelTokenStatsFromLogQuery(modelStatsQuery)
	if err != nil {
		common.SysError("failed to query log model token stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}

	cacheQuery, err := applyLogStatFilters(
		LOG_DB.Model(&Log{}),
		startTimestamp,
		endTimestamp,
		modelName,
		username,
		tokenName,
		channel,
		group,
		requestId,
	)
	if err != nil {
		return stat, err
	}
	cacheStats, err := sumCacheTokenStatsFromLogQuery(cacheQuery, modelStats)
	if err != nil {
		common.SysError("failed to query log cache token stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	stat.CacheTokens = cacheStats.Total
	stat.InputTokens = visibleInputTokens(stat.PromptTokens, cacheStats.Read)
	stat.ModelStats = modelTokenStatsMapToSortedSlice(modelStats)

	return stat, nil
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}
