package zhipu

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIResponsesRequestUsesZhipuRequest(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://open.bigmodel.cn",
			UpstreamModelName: "chatglm_std",
		},
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "chatglm_std",
		Input: common.StringToByteSlice(`"hello"`),
	})

	require.NoError(t, err)
	zhipuReq, ok := converted.(*ZhipuRequest)
	require.True(t, ok)
	require.Len(t, zhipuReq.Prompt, 1)
	require.Equal(t, "user", zhipuReq.Prompt[0].Role)
	require.Equal(t, "hello", zhipuReq.Prompt[0].Content)

	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_std/invoke", url)
}
