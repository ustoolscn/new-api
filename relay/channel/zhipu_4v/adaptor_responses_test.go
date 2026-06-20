package zhipu_4v

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIResponsesRequestUsesChatCompletionsCompat(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://open.bigmodel.cn",
			UpstreamModelName: "glm-4.5",
		},
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "glm-4.5",
		Input: common.StringToByteSlice(`"hello"`),
	})

	require.NoError(t, err)
	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "glm-4.5", chatReq.Model)
	require.Len(t, chatReq.Messages, 1)
	require.Equal(t, "hello", chatReq.Messages[0].Content)

	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://open.bigmodel.cn/api/paas/v4/chat/completions", url)
}
