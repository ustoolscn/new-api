package claude

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	kit "github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func init() {
	kit.SetMediaResolver(kit.MediaResolver{
		GetBase64Data: func(ctx context.Context, source kittypes.FileSource, reason ...string) (string, string, error) {
			c, _ := ctx.(*gin.Context)
			var hostSource types.FileSource
			switch source := source.(type) {
			case *kittypes.URLSource:
				hostSource = &types.URLSource{URL: source.URL}
			case *kittypes.Base64Source:
				hostSource = &types.Base64Source{Base64Data: source.Base64Data, MimeType: source.MimeType}
			default:
				return "", "", fmt.Errorf("unsupported Claude media source %T", source)
			}
			return service.GetBase64Data(c, hostSource, reason...)
		},
		DecodeBase64FileData: service.DecodeBase64FileData,
	})
}

// convertOfficialClaudeRequest bridges the host DTOs to the unchanged main-branch
// relaykit module. Authentication, transport and accounting remain in the host.
func convertOfficialClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request any) (any, error) {
	encoded, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	var target any
	var model *string
	switch request.(type) {
	case *dto.GeneralOpenAIRequest:
		r := &kitdto.GeneralOpenAIRequest{}
		target, model = r, &r.Model
	case *dto.OpenAIResponsesRequest:
		r := &kitdto.OpenAIResponsesRequest{}
		target, model = r, &r.Model
	case *dto.ClaudeRequest:
		r := &kitdto.ClaudeRequest{}
		target, model = r, &r.Model
	default:
		return nil, fmt.Errorf("unsupported Claude conversion request %T", request)
	}
	if err := common.Unmarshal(encoded, target); err != nil {
		return nil, err
	}
	settings := model_setting.GetClaudeSettings()
	meta := &convmeta.Values{Options: &convmeta.Options{
		Claude: convmeta.ClaudeOptions{
			ThinkingAdapterEnabled:                settings.ThinkingAdapterEnabled,
			ThinkingAdapterBudgetTokensPercentage: settings.ThinkingAdapterBudgetTokensPercentage,
			DefaultMaxTokens:                      settings.GetDefaultMaxTokens,
		},
		PreserveThinkingSuffix: model_setting.ShouldPreserveThinkingSuffix,
	}}
	if info != nil {
		meta.OriginModelName = info.OriginModelName
	}
	if !model_setting.ShouldPreserveThinkingSuffix(*model) && !model_setting.ShouldPreserveThinkingSuffix(meta.OriginModelName) {
		_, originIntent, originFound, err := reasoning.ParseClaudeModelSuffix(meta.OriginModelName, settings.ThinkingAdapterEnabled)
		if err != nil {
			return nil, err
		}
		if originFound {
			meta.ReasoningConversion = reasoning.StateFromIntent(originIntent)
		}
		base, intent, found, err := reasoning.ParseClaudeModelSuffix(*model, settings.ThinkingAdapterEnabled)
		if err != nil {
			return nil, err
		}
		if found {
			*model = base
			meta.ReasoningConversion = reasoning.StateFromIntent(intent)
		}
	}
	meta.UpstreamModelName = *model
	var ctx context.Context = context.Background()
	if c != nil {
		ctx = c
	}
	if native, ok := target.(*kitdto.ClaudeRequest); ok {
		if native.MaxTokens != nil && *native.MaxTokens == 0 {
			native.MaxTokens = nil
		}
		if err := kit.ApplyClaudeThinkingModel(native, meta); err != nil {
			return nil, err
		}
		if native.MaxTokens == nil {
			native.MaxTokens = common.GetPointer(uint(settings.GetDefaultMaxTokens(native.Model)))
		}
		if info != nil {
			info.ReasoningEffort = meta.ReasoningEffort
			if info.ChannelMeta != nil {
				info.UpstreamModelName = native.Model
			}
		}
		return native, nil
	}
	result, err := kit.ConvertRequest(ctx, meta, kittypes.RelayFormatClaude, target)
	if err != nil {
		return nil, err
	}
	for _, diagnostic := range result.Diagnostics {
		logger.LogWarn(ctx, "Claude conversion: "+diagnostic.Code+": "+diagnostic.Message)
	}
	if info != nil {
		info.ReasoningEffort = meta.ReasoningEffort
		info.AppendRequestConversion(types.RelayFormatClaude)
		if info.ChannelMeta != nil {
			info.UpstreamModelName = *model
		}
	}
	return result.Value, nil
}
