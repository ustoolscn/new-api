package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type PlaygroundImageHostConfig struct {
	UploadURL       string
	AuthHeader      string
	AuthValue       string
	FieldName       string
	ResponseURLPath string
	HTTPClient      *http.Client
}

type PlaygroundUploadedImage struct {
	URL              string  `json:"url"`
	ThumbnailURL     string  `json:"thumbnail_url,omitempty"`
	FirstFrameURL    string  `json:"first_frame_url,omitempty"`
	LastFrameURL     string  `json:"last_frame_url,omitempty"`
	Filename         string  `json:"filename,omitempty"`
	OriginalSize     int64   `json:"original_size,omitempty"`
	CompressedSize   int64   `json:"compressed_size,omitempty"`
	CompressionRatio float64 `json:"compression_ratio,omitempty"`
}

type playgroundImageHostResponse struct {
	Success bool   `json:"success"`
	URL     string `json:"url"`
	Data    struct {
		URL              string  `json:"url"`
		ThumbnailURL     string  `json:"thumbnail_url"`
		FirstFrameURL    string  `json:"first_frame_url"`
		LastFrameURL     string  `json:"last_frame_url"`
		Filename         string  `json:"filename"`
		OriginalSize     int64   `json:"original_size"`
		CompressedSize   int64   `json:"compressed_size"`
		CompressionRatio float64 `json:"compression_ratio"`
	} `json:"data"`
}

func DefaultPlaygroundImageHostConfig() PlaygroundImageHostConfig {
	return PlaygroundImageHostConfig{
		UploadURL:       envStringDefault("IMAGE_HOST_UPLOAD_URL", "https://2bad.lujilujilujilujiluji.com/"),
		AuthHeader:      envStringDefault("IMAGE_HOST_AUTH_HEADER", "Authorization"),
		AuthValue:       envStringDefault("IMAGE_HOST_AUTH_VALUE", "Bearer cooper"),
		FieldName:       envStringDefault("IMAGE_HOST_FIELD_NAME", "file"),
		ResponseURLPath: envStringDefault("IMAGE_HOST_RESPONSE_URL_PATH", "url"),
		HTTPClient:      &http.Client{Timeout: 120 * time.Second},
	}
}

func UploadPlaygroundImageToHost(ctx context.Context, cfg PlaygroundImageHostConfig, filename, contentType string, reader io.Reader) (PlaygroundUploadedImage, error) {
	if strings.TrimSpace(cfg.UploadURL) == "" {
		return PlaygroundUploadedImage{}, fmt.Errorf("缺少图床上传地址")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", formDataContentDisposition(defaultString(cfg.FieldName, "file"), filename))
	header.Set("Content-Type", defaultString(normalizeUploadContentType(filename, contentType), "application/octet-stream"))
	part, err := writer.CreatePart(header)
	if err != nil {
		return PlaygroundUploadedImage{}, err
	}
	if _, err := io.Copy(part, reader); err != nil {
		return PlaygroundUploadedImage{}, err
	}
	if err := writer.Close(); err != nil {
		return PlaygroundUploadedImage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.UploadURL, &body)
	if err != nil {
		return PlaygroundUploadedImage{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if cfg.AuthHeader != "" && cfg.AuthValue != "" {
		req.Header.Set(cfg.AuthHeader, cfg.AuthValue)
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return PlaygroundUploadedImage{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return PlaygroundUploadedImage{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PlaygroundUploadedImage{}, fmt.Errorf("图片上传失败：HTTP %d %s", resp.StatusCode, string(data))
	}
	return parsePlaygroundImageHostResponse(data, defaultString(cfg.ResponseURLPath, "url"))
}

func parsePlaygroundImageHostResponse(data []byte, urlPath string) (PlaygroundUploadedImage, error) {
	var parsed playgroundImageHostResponse
	if err := common.Unmarshal(data, &parsed); err != nil {
		return PlaygroundUploadedImage{}, err
	}

	url := valueAtJSONPath(data, urlPath)
	if url == "" {
		url = parsed.URL
	}
	if url == "" {
		url = parsed.Data.URL
	}
	if !parsed.Success && url == "" {
		return PlaygroundUploadedImage{}, fmt.Errorf("图片上传失败：%s", string(data))
	}
	if url == "" {
		return PlaygroundUploadedImage{}, fmt.Errorf("图片上传失败：图床未返回链接")
	}

	return PlaygroundUploadedImage{
		URL:              url,
		ThumbnailURL:     parsed.Data.ThumbnailURL,
		FirstFrameURL:    parsed.Data.FirstFrameURL,
		LastFrameURL:     parsed.Data.LastFrameURL,
		Filename:         parsed.Data.Filename,
		OriginalSize:     parsed.Data.OriginalSize,
		CompressedSize:   parsed.Data.CompressedSize,
		CompressionRatio: parsed.Data.CompressionRatio,
	}, nil
}

func valueAtJSONPath(data []byte, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	var value any
	if err := common.Unmarshal(data, &value); err != nil {
		return ""
	}
	current := value
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[part]
	}
	if text, ok := current.(string); ok {
		return text
	}
	return ""
}

func normalizeUploadContentType(filename, contentType string) string {
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	if contentType != "" && contentType != "application/octet-stream" {
		return contentType
	}
	if inferred := contentTypeFromFilename(filename); inferred != "" {
		return inferred
	}
	return contentType
}

func contentTypeFromFilename(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return ""
	}
}

func formDataContentDisposition(fieldName, filename string) string {
	return fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeMultipartQuotes(fieldName), escapeMultipartQuotes(filepath.Base(filename)))
}

func escapeMultipartQuotes(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, `\"`, "\r", " ", "\n", " ").Replace(value)
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func envStringDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
