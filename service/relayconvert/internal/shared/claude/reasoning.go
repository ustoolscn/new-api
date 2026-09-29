package claude

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
)

// ApplyClaude55Reasoning translates OpenAI reasoning for models that reject
// sampling parameters and manual thinking budgets. Unknown models are untouched.
func ApplyClaude55Reasoning(request *dto.ClaudeRequest, effort string) (bool, error) {
	baseModel, suffixEffort, _ := reasoning.TrimEffortSuffix(request.Model)
	thinkingSuffix := strings.HasSuffix(baseModel, "-thinking")
	baseModel = strings.TrimSuffix(baseModel, "-thinking")
	if baseModel != "claude-opus-5-5" && baseModel != "claude-sonnet-5-5" {
		return false, nil
	}
	if effort == "" {
		effort = suffixEffort
	}
	if thinkingSuffix && model_setting.GetClaudeSettings().ThinkingAdapterEnabled && effort == "" {
		effort = "high"
	}
	if effort != "" {
		switch effort {
		case "low", "medium", "high", "xhigh", "max":
		default:
			return true, fmt.Errorf("unsupported reasoning effort %q for %s; use low, medium, high, xhigh, or max", effort, baseModel)
		}
		outputConfig, err := common.Marshal(map[string]string{"effort": effort})
		if err != nil {
			return true, err
		}
		request.OutputConfig = outputConfig
	}
	request.Temperature = nil
	request.TopP = nil
	request.TopK = nil
	request.Thinking = &dto.Thinking{Type: "adaptive", Display: "summarized"}
	if !thinkingSuffix || (model_setting.GetClaudeSettings().ThinkingAdapterEnabled && !model_setting.ShouldPreserveThinkingSuffix(request.Model)) {
		request.Model = baseModel
	}
	return true, nil
}
