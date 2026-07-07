package helper

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
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
		preConsumedQuota = common.QuotaFromFloat(float64(preConsumedTokens) * ratio)
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
		preConsumedQuota = common.QuotaFromFloat(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
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
		quota = common.QuotaFromFloat(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelPrice == 0 {
				quota = 0
				freeModel = true
			}
		}
	} else {
		// 按量计费：以模型倍率的一半作为预扣额度
		quota = common.QuotaFromFloat(modelRatio / 2 * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
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

type videoSecondsBillingTrace struct {
	Resolution          string
	Duration            float64
	FPS                 float64
	BaseFPS             float64
	FPSMultiplier       float64
	PricePerSecond      float64
	BillableDuration    float64
	GeneratedVideoPrice float64
	InputContentCharged bool
	InputContentPrice   float64
	InputVideoDuration  float64
	TotalPrice          float64
}

func (t videoSecondsBillingTrace) toPriceDataTrace() *types.VideoSecondsTrace {
	return &types.VideoSecondsTrace{
		Resolution:          t.Resolution,
		Duration:            t.Duration,
		FPS:                 t.FPS,
		BaseFPS:             t.BaseFPS,
		FPSMultiplier:       t.FPSMultiplier,
		PricePerSecond:      t.PricePerSecond,
		BillableDuration:    t.BillableDuration,
		GeneratedVideoPrice: t.GeneratedVideoPrice,
		InputContentCharged: t.InputContentCharged,
		InputContentPrice:   t.InputContentPrice,
		InputVideoDuration:  t.InputVideoDuration,
		TotalPrice:          t.TotalPrice,
	}
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
	req = resolveInputVideoDurationForBilling(c, req)
	trace, err := calculateVideoSecondsBilling(req, cfg)
	if err != nil {
		return types.PriceData{}, err
	}
	quota := billingexpr.QuotaRound(trace.TotalPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
	priceData := types.PriceData{
		ModelPrice:        trace.TotalPrice,
		UsePrice:          true,
		Quota:             quota,
		GroupRatioInfo:    groupRatioInfo,
		VideoSecondsTrace: trace.toPriceDataTrace(),
	}
	return priceData, nil
}

func calculateVideoSecondsBilling(req relaycommon.TaskSubmitReq, cfg billing_setting.VideoPriceConfig) (videoSecondsBillingTrace, error) {
	resolution := resolveVideoResolution(req)
	if resolution == "" {
		return videoSecondsBillingTrace{}, fmt.Errorf("video resolution is required for video per-second billing")
	}
	pricePerSecond, ok := lookupVideoResolutionPrice(cfg.Prices, resolution)
	if !ok || pricePerSecond <= 0 {
		return videoSecondsBillingTrace{}, fmt.Errorf("video resolution %s price not configured", resolution)
	}
	duration := resolveVideoDuration(req)
	if duration <= 0 {
		return videoSecondsBillingTrace{}, fmt.Errorf("video duration is required for video per-second billing")
	}
	baseFPS := cfg.BaseFPS
	if baseFPS <= 0 {
		baseFPS = 24
	}
	fps := resolveVideoFPS(req)
	if fps <= 0 {
		fps = baseFPS
	}
	fpsMultiplier := fps / baseFPS
	inputContentCharged := hasInputContent(req) && cfg.InputContentPrice > 0
	inputContentPrice := 0.0
	if inputContentCharged {
		inputContentPrice = cfg.InputContentPrice
	}
	inputVideoDuration := 0.0
	if hasInputVideo(req) {
		inputVideoDuration = resolveInputVideoDuration(req)
		if inputVideoDuration <= 0 {
			return videoSecondsBillingTrace{}, fmt.Errorf("input video duration is required for video per-second billing")
		}
	}
	billableDuration := duration + inputVideoDuration
	generatedVideoPrice := pricePerSecond * billableDuration * fpsMultiplier
	totalPrice := generatedVideoPrice + inputContentPrice
	return videoSecondsBillingTrace{
		Resolution:          resolution,
		Duration:            duration,
		FPS:                 fps,
		BaseFPS:             baseFPS,
		FPSMultiplier:       fpsMultiplier,
		PricePerSecond:      pricePerSecond,
		BillableDuration:    billableDuration,
		GeneratedVideoPrice: generatedVideoPrice,
		InputContentCharged: inputContentCharged,
		InputContentPrice:   inputContentPrice,
		InputVideoDuration:  inputVideoDuration,
		TotalPrice:          totalPrice,
	}, nil
}

func lookupVideoResolutionPrice(prices map[string]float64, resolution string) (float64, bool) {
	normalized := normalizeVideoResolution(resolution)
	for key, price := range prices {
		if normalizeVideoResolution(key) == normalized {
			return price, true
		}
	}
	return 0, false
}

func resolveVideoDuration(req relaycommon.TaskSubmitReq) float64 {
	if req.Duration > 0 {
		return float64(req.Duration)
	}
	if sec, err := strconv.ParseFloat(strings.TrimSpace(req.Seconds), 64); err == nil && sec > 0 {
		return sec
	}
	return firstPositiveMetadataNumber(req.Metadata, "duration", "seconds", "duration_seconds", "durationSeconds")
}

func resolveInputVideoDuration(req relaycommon.TaskSubmitReq) float64 {
	if req.InputVideoDuration > 0 {
		return req.InputVideoDuration
	}
	if duration := firstPositiveMetadataNumber(
		req.Metadata,
		"input_video_duration",
		"inputVideoDuration",
		"input_video_seconds",
		"inputVideoSeconds",
		"inputVideoDurationSeconds",
	); duration > 0 {
		return duration
	}
	return firstPositiveMetadataContentNumber(req.Metadata, "duration", "seconds", "duration_seconds", "durationSeconds")
}

func resolveInputVideoDurationForBilling(c *gin.Context, req relaycommon.TaskSubmitReq) relaycommon.TaskSubmitReq {
	if resolveInputVideoDuration(req) > 0 || !hasInputVideoForBilling(c, req) {
		return req
	}
	if duration, ok, err := probeMultipartInputVideoDuration(c); ok {
		if err == nil && duration > 0 {
			req.InputVideoDuration = duration
			if strings.TrimSpace(req.InputVideo) == "" {
				req.InputVideo = "__multipart_input_video__"
			}
			return req
		}
		logger.LogWarn(c, fmt.Sprintf("failed to probe multipart input video duration: %v", err))
	}
	if duration, ok, err := probeURLInputVideoDuration(c, req); ok {
		if err == nil && duration > 0 {
			req.InputVideoDuration = duration
			return req
		}
		logger.LogWarn(c, fmt.Sprintf("failed to probe input video URL duration: %v", err))
	}
	return req
}

func hasInputVideoForBilling(c *gin.Context, req relaycommon.TaskSubmitReq) bool {
	if hasInputVideo(req) {
		return true
	}
	_, ok, _ := firstMultipartInputVideoFile(c)
	return ok
}

func probeMultipartInputVideoDuration(c *gin.Context) (float64, bool, error) {
	fileHeader, ok, err := firstMultipartInputVideoFile(c)
	if err != nil || !ok {
		return 0, ok, err
	}
	file, err := fileHeader.Open()
	if err != nil {
		return 0, true, err
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	duration, err := common.GetAudioDuration(c.Request.Context(), file, ext)
	return duration, true, err
}

func firstMultipartInputVideoFile(c *gin.Context) (*multipart.FileHeader, bool, error) {
	if c == nil || c.Request == nil {
		return nil, false, nil
	}
	contentType := c.GetHeader("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "multipart/") {
		return nil, false, nil
	}
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, false, err
	}
	for _, key := range []string{"input_video", "input_videos", "video"} {
		if files := form.File[key]; len(files) > 0 {
			return files[0], true, nil
		}
	}
	if files := form.File["input_reference"]; len(files) > 0 && normalizeInputVideoExt(filepath.Ext(files[0].Filename)) != "" {
		return files[0], true, nil
	}
	return nil, false, nil
}

func probeURLInputVideoDuration(c *gin.Context, req relaycommon.TaskSubmitReq) (float64, bool, error) {
	for _, rawURL := range inputVideoURLCandidates(req) {
		duration, ok, err := probeInputVideoURLDuration(c, rawURL)
		if !ok {
			continue
		}
		return duration, true, err
	}
	return 0, false, nil
}

func inputVideoURLCandidates(req relaycommon.TaskSubmitReq) []string {
	seen := map[string]bool{}
	candidates := make([]string, 0, 4)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		candidates = append(candidates, value)
	}
	add(req.InputVideo)
	for _, value := range req.InputVideos {
		add(value)
	}
	for _, value := range append([]string{req.InputReference}, req.Images...) {
		if mediaURLLooksVideo(value) {
			add(value)
		}
	}
	for _, input := range req.ImageInputs {
		if mediaURLLooksVideo(input.URL) || strings.Contains(strings.ToLower(input.Role), "video") {
			add(input.URL)
		}
	}
	addMetadataVideoURLs(req.Metadata, add)
	return candidates
}

func addMetadataVideoURLs(value interface{}, add func(string)) {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, item := range v {
			lowerKey := strings.ToLower(key)
			if strings.Contains(lowerKey, "video") || lowerKey == "url" || lowerKey == "input_reference" {
				addMetadataVideoURLs(item, add)
				continue
			}
			if lowerKey == "content" || lowerKey == "contents" {
				addMetadataVideoURLs(item, add)
			}
		}
	case []interface{}:
		for _, item := range v {
			addMetadataVideoURLs(item, add)
		}
	case []string:
		for _, item := range v {
			if mediaURLLooksVideo(item) {
				add(item)
			}
		}
	case string:
		if mediaURLLooksVideo(v) || strings.HasPrefix(strings.TrimSpace(v), "data:video/") {
			add(v)
		}
	}
}

func probeInputVideoURLDuration(c *gin.Context, rawURL string) (float64, bool, error) {
	rawURL = strings.TrimSpace(rawURL)
	if strings.HasPrefix(rawURL, "data:video/") {
		duration, err := probeDataURLVideoDuration(c, rawURL)
		return duration, true, err
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return 0, false, nil
	}
	resp, err := service.DoDownloadRequest(rawURL, "input_video_duration_probe")
	if err != nil {
		return 0, true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, true, fmt.Errorf("failed to download input video, status code: %d", resp.StatusCode)
	}
	maxFileSize := maxInputVideoProbeBytes()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFileSize+1))
	if err != nil {
		return 0, true, err
	}
	if int64(len(data)) > maxFileSize {
		return 0, true, fmt.Errorf("input video exceeds maximum probe size: %dMB", constant.MaxFileDownloadMB)
	}
	ext := inputVideoExtFromResponse(resp, parsed)
	if ext == "" {
		return 0, true, fmt.Errorf("unsupported input video format")
	}
	duration, err := common.GetAudioDuration(c.Request.Context(), bytes.NewReader(data), ext)
	return duration, true, err
}

func probeDataURLVideoDuration(c *gin.Context, rawURL string) (float64, error) {
	mediaType, data, ok := strings.Cut(rawURL, ",")
	if !ok || !strings.Contains(strings.ToLower(mediaType), ";base64") {
		return 0, fmt.Errorf("input video data URL must be base64 encoded")
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return 0, err
	}
	if int64(len(decoded)) > maxInputVideoProbeBytes() {
		return 0, fmt.Errorf("input video exceeds maximum probe size: %dMB", constant.MaxFileDownloadMB)
	}
	ext := inputVideoExtFromContentType(strings.TrimPrefix(strings.Split(mediaType, ";")[0], "data:"))
	if ext == "" {
		return 0, fmt.Errorf("unsupported input video data URL format")
	}
	return common.GetAudioDuration(c.Request.Context(), bytes.NewReader(decoded), ext)
}

func inputVideoExtFromResponse(resp *http.Response, parsed *url.URL) string {
	if ext := normalizeInputVideoExt(filepath.Ext(parsed.Path)); ext != "" {
		return ext
	}
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil {
		if filename := params["filename"]; filename != "" {
			if ext := normalizeInputVideoExt(filepath.Ext(filename)); ext != "" {
				return ext
			}
		}
	}
	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return inputVideoExtFromContentType(contentType)
}

func inputVideoExtFromContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "video/mp4", "video/x-m4v", "video/quicktime", "application/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	default:
		return ""
	}
}

func normalizeInputVideoExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4", ".m4v", ".mov":
		return ".mp4"
	case ".webm":
		return ".webm"
	default:
		return ""
	}
}

func maxInputVideoProbeBytes() int64 {
	limitMB := constant.MaxFileDownloadMB
	if limitMB <= 0 {
		limitMB = 64
	}
	return int64(limitMB) << 20
}

func resolveVideoFPS(req relaycommon.TaskSubmitReq) float64 {
	for _, v := range []int{req.FPS, req.FrameRate, req.FramesPerSecond, req.FramesPerSecondCamel} {
		if v > 0 {
			return float64(v)
		}
	}
	return firstPositiveMetadataNumber(req.Metadata, "fps", "frame_rate", "frameRate", "framespersecond", "framesPerSecond")
}

func resolveVideoResolution(req relaycommon.TaskSubmitReq) string {
	for _, key := range []string{"resolution", "quality", "size"} {
		if v, ok := req.Metadata[key].(string); ok && strings.TrimSpace(v) != "" {
			return normalizeVideoResolution(v)
		}
	}
	if strings.TrimSpace(req.Size) != "" {
		if res := resolutionFromSize(req.Size); res != "" {
			return res
		}
	}
	width := req.Width
	height := req.Height
	if width <= 0 {
		width = int(firstPositiveMetadataNumber(req.Metadata, "width"))
	}
	if height <= 0 {
		height = int(firstPositiveMetadataNumber(req.Metadata, "height"))
	}
	if width > 0 && height > 0 {
		shortSide := width
		if height < shortSide {
			shortSide = height
		}
		return normalizeVideoResolution(fmt.Sprintf("%dp", shortSide))
	}
	return ""
}

func hasInputContent(req relaycommon.TaskSubmitReq) bool {
	if strings.TrimSpace(req.Image) != "" || strings.TrimSpace(req.InputReference) != "" || strings.TrimSpace(req.InputVideo) != "" {
		return true
	}
	if len(req.Images) > 0 || len(req.ImageInputs) > 0 || len(req.InputVideos) > 0 {
		return true
	}
	return metadataHasMedia(req.Metadata)
}

func hasInputVideo(req relaycommon.TaskSubmitReq) bool {
	if strings.TrimSpace(req.InputVideo) != "" || len(req.InputVideos) > 0 {
		return true
	}
	for _, value := range append([]string{req.InputReference}, req.Images...) {
		if mediaURLLooksVideo(value) {
			return true
		}
	}
	for _, input := range req.ImageInputs {
		if mediaURLLooksVideo(input.URL) || strings.Contains(strings.ToLower(input.Role), "video") {
			return true
		}
	}
	return metadataHasVideo(req.Metadata)
}

func metadataHasMedia(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	for _, key := range []string{"image", "images", "input_reference", "input_video", "input_videos", "video", "video_url"} {
		if value, ok := metadata[key]; ok && metadataValuePresent(value) {
			return true
		}
	}
	return metadataContentHasMedia(metadata)
}

func metadataHasVideo(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	for _, key := range []string{"input_video", "input_videos", "video", "video_url", "inputVideo", "inputVideos"} {
		if value, ok := metadata[key]; ok && metadataValuePresent(value) {
			return true
		}
	}
	return metadataContentHasVideo(metadata)
}

func metadataValuePresent(value interface{}) bool {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case []interface{}:
		return len(v) > 0
	case []string:
		return len(v) > 0
	case map[string]interface{}:
		return len(v) > 0
	default:
		return value != nil
	}
}

func metadataContentHasVideo(metadata map[string]interface{}) bool {
	return metadataContentHasMediaType(metadata, "video")
}

func metadataContentHasMedia(metadata map[string]interface{}) bool {
	return metadataContentHasMediaType(metadata, "")
}

func metadataContentHasMediaType(metadata map[string]interface{}, mediaType string) bool {
	contentRaw, ok := metadata["content"]
	if !ok {
		return false
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		for _, key := range []string{"image_url", "video_url", "audio_url", "file", "file_url", "image", "video", "audio"} {
			if mediaType != "" && !strings.Contains(key, mediaType) {
				continue
			}
			if _, ok := itemMap[key]; ok {
				return true
			}
		}
		if value, ok := itemMap["type"].(string); ok {
			lower := strings.ToLower(value)
			if mediaType == "" {
				if strings.Contains(lower, "image") || strings.Contains(lower, "video") || strings.Contains(lower, "audio") || strings.Contains(lower, "file") {
					return true
				}
			} else if strings.Contains(lower, mediaType) {
				return true
			}
		}
	}
	return false
}

func firstPositiveMetadataContentNumber(metadata map[string]interface{}, keys ...string) float64 {
	if metadata == nil {
		return 0
	}
	contentRaw, ok := metadata["content"]
	if !ok {
		return 0
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return 0
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		isVideo := false
		if value, ok := itemMap["type"].(string); ok && strings.Contains(strings.ToLower(value), "video") {
			isVideo = true
		}
		if _, ok := itemMap["video_url"]; ok {
			isVideo = true
		}
		if !isVideo {
			continue
		}
		if n := firstPositiveMetadataNumber(itemMap, keys...); n > 0 {
			return n
		}
	}
	return 0
}

func mediaURLLooksVideo(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "data:video/") {
		return true
	}
	for _, ext := range []string{".mp4", ".mov", ".webm", ".mkv", ".avi", ".m4v"} {
		if strings.Contains(value, ext) {
			return true
		}
	}
	return false
}

func resolutionFromSize(size string) string {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(size)), func(r rune) bool {
		return r == 'x' || r == '*' || r == '×'
	})
	if len(parts) != 2 {
		return normalizeVideoResolution(size)
	}
	width, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return normalizeVideoResolution(size)
	}
	shortSide := width
	if height < shortSide {
		shortSide = height
	}
	return normalizeVideoResolution(fmt.Sprintf("%dp", shortSide))
}

func normalizeVideoResolution(resolution string) string {
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	resolution = strings.ReplaceAll(resolution, " ", "")
	if strings.HasSuffix(resolution, "p") {
		return resolution
	}
	if v, err := strconv.Atoi(resolution); err == nil && v > 0 {
		return fmt.Sprintf("%dp", v)
	}
	return resolution
}

func firstPositiveMetadataNumber(metadata map[string]interface{}, keys ...string) float64 {
	if metadata == nil {
		return 0
	}
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case int:
			if v > 0 {
				return float64(v)
			}
		case int64:
			if v > 0 {
				return float64(v)
			}
		case float64:
			if v > 0 {
				return v
			}
		case string:
			if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
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
	preConsumedQuota := billingexpr.QuotaRound(quotaBeforeGroup * groupRatioInfo.GroupRatio)

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
