package xai

import "github.com/QuantumNous/new-api/setting/ratio_setting"

var textModelList = []string{
	// language models
	"grok-4.5",
	"grok-4.5-latest",
	"grok-4.3",
	"grok-4.3-latest",
	"grok-4.20-0309-reasoning",
	"grok-4.20-0309-non-reasoning",
	"grok-4.20-multi-agent-0309",
	"grok-build-0.1",
	"grok-4-1-fast-reasoning",
	"grok-4-1-fast-non-reasoning",
	"grok-code-fast-1",
	"grok-4-fast-reasoning",
	"grok-4-fast-non-reasoning",
	"grok-4-0709",
	"grok-3-mini",
	"grok-3",
	"grok-2-vision-1212",
	// search variants
	"grok-4-1-fast-reasoning-search",
	"grok-4-1-fast-non-reasoning-search",
	"grok-4-fast-reasoning-search",
	"grok-4-fast-non-reasoning-search",
	"grok-4-0709-search",
	"grok-3-mini-search",
	"grok-3-search",
	// grok-3-mini reasoning effort variants
	"grok-3-mini-high", "grok-3-mini-low",
}

var mediaModelList = []string{
	// image generation models
	"grok-imagine-image-quality",
	"grok-imagine-image-pro",
	"grok-imagine-image",
	"grok-2-image-1212",
	// video generation model
	"grok-imagine-video",
	"grok-imagine-video-1.5",
}

var ModelList = append(ratio_setting.WithCompactModelVariants(textModelList), mediaModelList...)

var ChannelName = "xai"
