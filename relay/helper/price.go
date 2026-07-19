package helper

import (
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func modelPriceNotConfiguredError(modelName string, userId int) error {
	if model.IsAdmin(userId) {
		return fmt.Errorf(
			"模型 %s 的价格未配置。请前往「系统设置 → 运营设置」开启自用模式，或在「系统设置 → 分组与模型定价设置」中为该模型配置价格；"+
				"Model %s price not configured. Go to System Settings → Operation Settings to enable self-use mode, or configure the model price in System Settings → Group & Model Pricing.",
			modelName, modelName,
		)
	}
	return fmt.Errorf(
		"模型 %s 的价格尚未由管理员配置，暂时无法使用，请联系站点管理员开启该模型；"+
			"Model %s has not been priced by the administrator yet. Please contact the site administrator to enable this model.",
		modelName, modelName,
	)
}

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

// defaultTieredPreConsumeMaxTokens is the fallback completion-token estimate
// used for tiered expression pre-consume when the client omits max_tokens, so
// the pre-consumed quota still reflects a plausible output cost in paid groups.
const defaultTieredPreConsumeMaxTokens = 8192

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.GroupRatioInfo {
	groupRatioInfo := types.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}

	// check auto group
	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		logger.LogDebug(ctx, "final group: %s", autoGroup)
		relayInfo.UsingGroup = autoGroup.(string)
	}

	// check user group special ratio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		// user group special ratio
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		// normal group ratio
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	}

	return groupRatioInfo
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	modelPrice, usePrice := ratio_setting.GetModelPrice(info.OriginModelName, false)

	groupRatioInfo := HandleGroupRatio(c, info)
	if billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeVideoSeconds {
		return types.PriceData{}, fmt.Errorf("model %s uses video per-second billing; use POST /v1/video/generations", info.OriginModelName)
	}

	// Check if this model uses tiered_expr billing
	if billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeTieredExpr {
		return modelPriceHelperTiered(c, info, promptTokens, meta, groupRatioInfo)
	}

	var preConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	var freeModel bool
	if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
		if !success {
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
		completionRatio = ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		cacheCreationRatio5m = cacheCreationRatio
		// 固定1h和5min缓存写入价格的比例
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(info.OriginModelName)
		audioRatio = ratio_setting.GetAudioRatio(info.OriginModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(info.OriginModelName)
		ratio := modelRatio * groupRatioInfo.GroupRatio
		quota, err := common.QuotaFromFloatStrict(float64(preConsumedTokens) * ratio)
		if err != nil {
			return types.PriceData{}, err
		}
		preConsumedQuota = quota
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
	}

	// check if free model pre-consume is disabled
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		// if model price or ratio is 0, do not pre-consume quota
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		} else if usePrice {
			if modelPrice == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		} else {
			if modelRatio == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:            freeModel,
		ModelPrice:           modelPrice,
		ModelRatio:           modelRatio,
		CompletionRatio:      completionRatio,
		GroupRatioInfo:       groupRatioInfo,
		UsePrice:             usePrice,
		CacheRatio:           cacheRatio,
		ImageRatio:           imageRatio,
		AudioRatio:           audioRatio,
		AudioCompletionRatio: audioCompletionRatio,
		CacheCreationRatio:   cacheCreationRatio,
		CacheCreation5mRatio: cacheCreationRatio5m,
		CacheCreation1hRatio: cacheCreationRatio1h,
		QuotaToPreConsume:    preConsumedQuota,
	}
	if usePrice {
		for name, ratio := range meta.BillingRatios {
			priceData.AddOtherRatio(name, ratio)
		}
		quotaToPreConsume := priceData.ApplyOtherRatiosToFloat(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		quota, err := common.QuotaFromFloatStrict(quotaToPreConsume)
		if err != nil {
			return types.PriceData{}, err
		}
		priceData.QuotaToPreConsume = quota
	}

	if common.DebugEnabled {
		logger.LogDebug(c, "model_price_helper result: %s", priceData.ToSetting())
	}
	info.PriceData = priceData
	return priceData, nil
}

// ModelPriceHelperPerCall 按次/按量计费的 PriceHelper (MJ、Task)
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) (types.PriceData, error) {
	groupRatioInfo := HandleGroupRatio(c, info)
	if billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeVideoSeconds {
		return modelPriceHelperVideoSeconds(c, info, groupRatioInfo)
	}

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	usePrice := success
	var modelRatio float64

	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]
		if ok {
			modelPrice = defaultPrice
			usePrice = true
		} else {
			var ratioSuccess bool
			var matchName string
			modelRatio, ratioSuccess, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !ratioSuccess && !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
	}

	var quota int
	freeModel := false

	if usePrice {
		var err error
		quota, err = common.QuotaFromFloatStrict(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if err != nil {
			return types.PriceData{}, err
		}
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelPrice == 0 {
				quota = 0
				freeModel = true
			}
		}
	} else {
		// 按量计费：以模型倍率的一半作为预扣额度
		var err error
		quota, err = common.QuotaFromFloatStrict(modelRatio / 2 * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if err != nil {
			return types.PriceData{}, err
		}
		modelPrice = -1
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelRatio == 0 {
				quota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:      freeModel,
		ModelPrice:     modelPrice,
		ModelRatio:     modelRatio,
		UsePrice:       usePrice,
		Quota:          quota,
		GroupRatioInfo: groupRatioInfo,
	}
	return priceData, nil
}

func modelPriceHelperVideoSeconds(c *gin.Context, info *relaycommon.RelayInfo, groupRatioInfo types.GroupRatioInfo) (types.PriceData, error) {
	cfg, ok := billing_setting.GetVideoPriceConfig(info.OriginModelName)
	if !ok || len(cfg.Prices) == 0 {
		return types.PriceData{}, fmt.Errorf("model %s video per-second price not configured", info.OriginModelName)
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return types.PriceData{}, err
	}
	markMultipartTaskMedia(c, &req)
	trace, err := calculateVideoSecondsBilling(req, cfg)
	if err != nil {
		return types.PriceData{}, err
	}
	if groupRatioInfo.GroupRatio < 0 || math.IsNaN(groupRatioInfo.GroupRatio) || math.IsInf(groupRatioInfo.GroupRatio, 0) {
		return types.PriceData{}, fmt.Errorf("group ratio is invalid for video per-second billing")
	}
	quota, clamp := common.QuotaRoundChecked(trace.TotalPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
	if clamp != nil {
		info.QuotaClamp = clamp
		return types.PriceData{}, clamp
	}
	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume && (groupRatioInfo.GroupRatio == 0 || trace.TotalPrice == 0) {
		freeModel = true
		quota = 0
	}
	return types.PriceData{
		FreeModel:         freeModel,
		ModelPrice:        trace.TotalPrice,
		UsePrice:          true,
		Quota:             quota,
		GroupRatioInfo:    groupRatioInfo,
		VideoSecondsTrace: trace,
	}, nil
}

func markMultipartTaskMedia(c *gin.Context, req *relaycommon.TaskSubmitReq) {
	if c == nil || c.Request == nil || req == nil || !strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "multipart/") {
		return
	}
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return
	}
	for field, files := range form.File {
		if len(files) == 0 {
			continue
		}
		normalizedField := strings.ToLower(field)
		if strings.Contains(normalizedField, "video") {
			req.InputVideo = "__multipart_video__"
		}
		if normalizedField == "image" || normalizedField == "images" || normalizedField == "input_reference" {
			req.Image = "__multipart_image__"
		}
	}
}

func calculateVideoSecondsBilling(req relaycommon.TaskSubmitReq, cfg billing_setting.VideoPriceConfig) (*types.VideoSecondsTrace, error) {
	resolution := relaycommon.NormalizeVideoResolution(req.Size)
	if resolution == "" && req.Width != nil && req.Height != nil && *req.Width > 0 && *req.Height > 0 {
		resolution = relaycommon.NormalizeVideoResolution(fmt.Sprintf("%dx%d", *req.Width, *req.Height))
	}
	if resolution == "" {
		return nil, fmt.Errorf("size is required for video per-second billing")
	}
	outputPricePerSecond, ok := lookupVideoResolutionPrice(cfg.Prices, resolution)
	if !ok {
		return nil, fmt.Errorf("video resolution %s price not configured", resolution)
	}
	if outputPricePerSecond < 0 || math.IsNaN(outputPricePerSecond) || math.IsInf(outputPricePerSecond, 0) {
		return nil, fmt.Errorf("video resolution %s price is invalid", resolution)
	}

	outputSeconds := req.OutputSeconds()
	if outputSeconds <= 0 || outputSeconds > relaycommon.MaxTaskDurationSeconds || math.IsNaN(outputSeconds) || math.IsInf(outputSeconds, 0) {
		return nil, fmt.Errorf("seconds must be between 1 and %d for video per-second billing", relaycommon.MaxTaskDurationSeconds)
	}
	baseFPS := cfg.BaseFPS
	if baseFPS == 0 {
		baseFPS = 24
	}
	if baseFPS <= 0 || baseFPS > relaycommon.MaxTaskFPS || math.IsNaN(baseFPS) || math.IsInf(baseFPS, 0) {
		return nil, fmt.Errorf("base_fps must be between 1 and %d", relaycommon.MaxTaskFPS)
	}
	fps := baseFPS
	if req.FPS != nil {
		fps = float64(*req.FPS)
	}
	if fps <= 0 || fps > relaycommon.MaxTaskFPS {
		return nil, fmt.Errorf("fps must be between 1 and %d", relaycommon.MaxTaskFPS)
	}
	fpsMultiplier := fps / baseFPS
	outputPrice := outputSeconds * outputPricePerSecond * fpsMultiplier

	inputContentPrice := cfg.InputContentPrice
	if inputContentPrice < 0 || math.IsNaN(inputContentPrice) || math.IsInf(inputContentPrice, 0) {
		return nil, fmt.Errorf("input_content_price is invalid")
	}
	inputContentCharged := req.HasAnyInputContent() && inputContentPrice > 0
	if !inputContentCharged {
		inputContentPrice = 0
	}

	inputVideoPricePerSecond := cfg.InputVideoPricePerSecond
	if inputVideoPricePerSecond < 0 || math.IsNaN(inputVideoPricePerSecond) || math.IsInf(inputVideoPricePerSecond, 0) {
		return nil, fmt.Errorf("input_video_price_per_second is invalid")
	}
	inputVideoSeconds := 0.0
	inputVideoPrice := 0.0
	if req.HasAnyInputVideo() && inputVideoPricePerSecond > 0 {
		if req.InputVideoSeconds == nil {
			return nil, fmt.Errorf("input_video_seconds is required when input video per-second pricing is configured")
		}
		inputVideoSeconds = *req.InputVideoSeconds
		if inputVideoSeconds <= 0 || inputVideoSeconds > relaycommon.MaxTaskDurationSeconds || math.IsNaN(inputVideoSeconds) || math.IsInf(inputVideoSeconds, 0) {
			return nil, fmt.Errorf("input_video_seconds must be between 1 and %d", relaycommon.MaxTaskDurationSeconds)
		}
		inputVideoPrice = inputVideoSeconds * inputVideoPricePerSecond
	}

	totalPrice := outputPrice + inputVideoPrice + inputContentPrice
	if totalPrice < 0 || math.IsNaN(totalPrice) || math.IsInf(totalPrice, 0) {
		return nil, fmt.Errorf("calculated video price is invalid")
	}
	return &types.VideoSecondsTrace{
		Resolution:               resolution,
		OutputSeconds:            outputSeconds,
		FPS:                      fps,
		BaseFPS:                  baseFPS,
		FPSMultiplier:            fpsMultiplier,
		OutputPricePerSecond:     outputPricePerSecond,
		OutputPrice:              outputPrice,
		InputContentCharged:      inputContentCharged,
		InputContentPrice:        inputContentPrice,
		InputVideoSeconds:        inputVideoSeconds,
		InputVideoPricePerSecond: inputVideoPricePerSecond,
		InputVideoPrice:          inputVideoPrice,
		TotalPrice:               totalPrice,
	}, nil
}

func lookupVideoResolutionPrice(prices map[string]float64, resolution string) (float64, bool) {
	normalized := relaycommon.NormalizeVideoResolution(resolution)
	for key, price := range prices {
		if relaycommon.NormalizeVideoResolution(key) == normalized {
			return price, true
		}
	}
	return 0, false
}

func HasModelBillingConfig(modelName string) bool {
	if _, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		return true
	}
	if _, ok, _ := ratio_setting.GetModelRatio(modelName); ok {
		return true
	}
	switch billing_setting.GetBillingMode(modelName) {
	case billing_setting.BillingModeTieredExpr:
		expr, ok := billing_setting.GetBillingExpr(modelName)
		return ok && strings.TrimSpace(expr) != ""
	case billing_setting.BillingModeVideoSeconds:
		return HasVideoSecondsBillingConfig(modelName)
	default:
		return false
	}
}

func HasVideoSecondsBillingConfig(modelName string) bool {
	if billing_setting.GetBillingMode(modelName) != billing_setting.BillingModeVideoSeconds {
		return false
	}
	cfg, ok := billing_setting.GetVideoPriceConfig(modelName)
	return ok && len(cfg.Prices) > 0
}

func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta, groupRatioInfo types.GroupRatioInfo) (types.PriceData, error) {
	exprStr, ok := billing_setting.GetBillingExpr(info.OriginModelName)
	if !ok {
		return types.PriceData{}, fmt.Errorf("model %s is configured as tiered_expr but has no billing expression", info.OriginModelName)
	}

	estimatedCompletionTokens := meta.MaxTokens
	if estimatedCompletionTokens == 0 && groupRatioInfo.GroupRatio != 0 {
		estimatedCompletionTokens = defaultTieredPreConsumeMaxTokens
	}

	requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return types.PriceData{}, err
	}

	rawCost, trace, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{
		P:   float64(promptTokens),
		C:   float64(estimatedCompletionTokens),
		Len: float64(promptTokens),
	}, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered expr run failed: %w", info.OriginModelName, err)
	}

	// Expression coefficients are $/1M tokens prices; convert to quota the same way per-call billing does.
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	preConsumedQuota, err := billingexpr.QuotaRoundStrict(quotaBeforeGroup * groupRatioInfo.GroupRatio)
	if err != nil {
		return types.PriceData{}, err
	}

	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		}
	}

	exprHash := billingexpr.ExprHashString(exprStr)
	snapshot := &billingexpr.BillingSnapshot{
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 info.OriginModelName,
		ExprString:                exprStr,
		ExprHash:                  exprHash,
		GroupRatio:                groupRatioInfo.GroupRatio,
		EstimatedPromptTokens:     promptTokens,
		EstimatedCompletionTokens: estimatedCompletionTokens,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprStr),
	}
	info.TieredBillingSnapshot = snapshot
	info.BillingRequestInput = &requestInput

	priceData := types.PriceData{
		FreeModel:         freeModel,
		GroupRatioInfo:    groupRatioInfo,
		QuotaToPreConsume: preConsumedQuota,
	}

	logger.LogDebug(c, "model_price_helper_tiered result: model=%s preConsume=%d quotaBeforeGroup=%.2f groupRatio=%.2f tier=%s", info.OriginModelName, preConsumedQuota, quotaBeforeGroup, groupRatioInfo.GroupRatio, trace.MatchedTier)

	info.PriceData = priceData
	return priceData, nil
}
