package xai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	//panic("implement me")
	return nil, errors.New("not available")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//not available
	return nil, errors.New("not available")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	xaiRequest := ImageRequest{
		Model:          request.Model,
		Prompt:         request.Prompt,
		N:              request.N,
		ResponseFormat: request.ResponseFormat,
		User:           request.User,
	}

	xaiRequest.AspectRatio, _ = imageExtraString(request.Extra, "aspect_ratio")
	xaiRequest.Resolution, _ = imageExtraString(request.Extra, "resolution")
	if raw, ok := request.Extra["storage_options"]; ok {
		xaiRequest.StorageOptions = raw
	}
	applyXAIImageSizeAliases(&xaiRequest, request.Size)

	if info.RelayMode == constant.RelayModeImagesEdits {
		if len(request.Files) > 0 {
			media := make([]ImageMedia, 0, len(request.Files))
			for _, file := range request.Files {
				if file == nil || file.Source == nil {
					continue
				}
				mimeType := "image/png"
				if base64Source, ok := file.Source.(*types.Base64Source); ok && strings.TrimSpace(base64Source.MimeType) != "" {
					mimeType = base64Source.MimeType
				}
				media = append(media, ImageMedia{URL: "data:" + mimeType + ";base64," + file.Source.GetRawData()})
			}
			if len(media) == 1 {
				xaiRequest.Image = &media[0]
			} else {
				xaiRequest.Images = media
			}
		} else {
			image, images, err := normalizeXAIImageInputs(request.Image, request.Images)
			if err != nil {
				return nil, err
			}
			xaiRequest.Image = image
			xaiRequest.Images = images
		}
		if xaiRequest.Image == nil && len(xaiRequest.Images) == 0 {
			return nil, errors.New("image is required")
		}
	}
	return xaiRequest, nil
}

func imageExtraString(extra map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := extra[key]
	if !ok || len(raw) == 0 {
		return "", false
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func applyXAIImageSizeAliases(request *ImageRequest, size string) {
	size = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(size, "×", "x")))
	if size == "" {
		return
	}
	if request.Resolution == "" && (size == "1k" || size == "2k") {
		request.Resolution = size
		return
	}
	if request.AspectRatio == "" && strings.Contains(size, ":") {
		request.AspectRatio = size
		return
	}
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return
	}
	width, widthErr := strconv.Atoi(parts[0])
	height, heightErr := strconv.Atoi(parts[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return
	}
	if request.Resolution == "" {
		if width >= 2048 || height >= 2048 {
			request.Resolution = "2k"
		} else {
			request.Resolution = "1k"
		}
	}
	if request.AspectRatio != "" {
		return
	}
	divisor := greatestCommonDivisor(width, height)
	ratio := fmt.Sprintf("%d:%d", width/divisor, height/divisor)
	if lo.Contains([]string{"1:1", "3:4", "4:3", "9:16", "16:9", "2:3", "3:2", "1:2", "2:1"}, ratio) {
		request.AspectRatio = ratio
	}
}

func greatestCommonDivisor(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a <= 0 {
		return 1
	}
	return a
}

func normalizeXAIImageInputs(imageRaw, imagesRaw json.RawMessage) (*ImageMedia, []ImageMedia, error) {
	if len(imageRaw) > 0 {
		image, err := parseXAIImageMedia(imageRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid image: %w", err)
		}
		if len(imagesRaw) > 0 {
			return nil, nil, errors.New("image and images are mutually exclusive")
		}
		return &image, nil, nil
	}
	if len(imagesRaw) == 0 {
		return nil, nil, nil
	}
	var rawItems []json.RawMessage
	if err := common.Unmarshal(imagesRaw, &rawItems); err != nil {
		return nil, nil, fmt.Errorf("invalid images: %w", err)
	}
	images := make([]ImageMedia, 0, len(rawItems))
	for _, raw := range rawItems {
		image, err := parseXAIImageMedia(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid images: %w", err)
		}
		images = append(images, image)
	}
	return nil, images, nil
}

func parseXAIImageMedia(raw json.RawMessage) (ImageMedia, error) {
	var url string
	if err := common.Unmarshal(raw, &url); err == nil {
		url = strings.TrimSpace(url)
		if url == "" {
			return ImageMedia{}, errors.New("image URL is empty")
		}
		return ImageMedia{URL: url}, nil
	}

	var value struct {
		URL      string          `json:"url,omitempty"`
		FileID   string          `json:"file_id,omitempty"`
		ImageURL json.RawMessage `json:"image_url,omitempty"`
	}
	if err := common.Unmarshal(raw, &value); err != nil {
		return ImageMedia{}, err
	}
	value.URL = strings.TrimSpace(value.URL)
	value.FileID = strings.TrimSpace(value.FileID)
	if value.URL == "" && len(value.ImageURL) > 0 {
		if err := common.Unmarshal(value.ImageURL, &value.URL); err != nil {
			var nested struct {
				URL string `json:"url"`
			}
			if nestedErr := common.Unmarshal(value.ImageURL, &nested); nestedErr == nil {
				value.URL = strings.TrimSpace(nested.URL)
			}
		}
	}
	if (value.URL == "") == (value.FileID == "") {
		return ImageMedia{}, errors.New("exactly one of url or file_id is required")
	}
	return ImageMedia{URL: value.URL, FileID: value.FileID}, nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, info.RequestURLPath, info.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	req.Set("Content-Type", "application/json")
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if strings.HasSuffix(info.UpstreamModelName, "-search") {
		info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-search")
		request.Model = info.UpstreamModelName
		toMap := request.ToMap()
		toMap["search_parameters"] = map[string]any{
			"mode": "on",
		}
		return toMap, nil
	}
	if strings.HasPrefix(request.Model, "grok-3-mini") {
		if lo.FromPtrOr(request.MaxCompletionTokens, uint(0)) == 0 && lo.FromPtrOr(request.MaxTokens, uint(0)) != 0 {
			request.MaxCompletionTokens = request.MaxTokens
			request.MaxTokens = nil
		}
		if strings.HasSuffix(request.Model, "-high") {
			request.ReasoningEffort = "high"
			request.Model = strings.TrimSuffix(request.Model, "-high")
		} else if strings.HasSuffix(request.Model, "-low") {
			request.ReasoningEffort = "low"
			request.Model = strings.TrimSuffix(request.Model, "-low")
		}
		info.ReasoningEffort = request.ReasoningEffort
		info.UpstreamModelName = request.Model
	}
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//not available
	return nil, errors.New("not available")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	if request.Model == "" && info != nil {
		request.Model = info.UpstreamModelName
	}
	if strings.HasSuffix(request.Model, "-search") {
		request.Model = strings.TrimSuffix(request.Model, "-search")
		if info != nil {
			info.UpstreamModelName = request.Model
		}
		if len(request.Tools) == 0 {
			tools, err := common.Marshal([]map[string]string{
				{"type": "web_search"},
				{"type": "x_search"},
			})
			if err != nil {
				return nil, err
			}
			request.Tools = tools
		}
	}
	if strings.HasPrefix(request.Model, "grok-3-mini") {
		effort := ""
		if strings.HasSuffix(request.Model, "-high") {
			effort = "high"
			request.Model = strings.TrimSuffix(request.Model, "-high")
		} else if strings.HasSuffix(request.Model, "-low") {
			effort = "low"
			request.Model = strings.TrimSuffix(request.Model, "-low")
		}
		if effort != "" {
			if request.Reasoning == nil {
				request.Reasoning = &dto.Reasoning{}
			}
			request.Reasoning.Effort = effort
			if info != nil {
				info.ReasoningEffort = effort
				info.UpstreamModelName = request.Model
			}
		}
	}
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	switch info.RelayMode {
	case constant.RelayModeImagesGenerations, constant.RelayModeImagesEdits:
		usage, err = openai.OpenaiImageHandler(c, info, resp)
	case constant.RelayModeResponses:
		if info.IsStream {
			usage, err = openai.OaiResponsesStreamHandler(c, info, resp)
		} else {
			usage, err = openai.OaiResponsesHandler(c, info, resp)
		}
	case constant.RelayModeResponsesCompact:
		usage, err = openai.OaiResponsesCompactionHandler(c, resp)
	default:
		if info.IsStream {
			usage, err = xAIStreamHandler(c, info, resp)
		} else {
			usage, err = xAIHandler(c, info, resp)
		}
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
