package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestInitTaskStoresXAIKeyForPolling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		UserId:        1,
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeXai,
			ChannelId:   2,
			ApiKey:      "xai-selected-key",
		},
	}

	task := InitTask(constant.TaskPlatform("48"), info)

	assert.Equal(t, "xai-selected-key", task.PrivateData.Key)
}
