package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUsageAdjustmentAuditIsAdminOnly(t *testing.T) {
	encoded, err := common.Marshal(map[string]any{"cache_tokens": 91, "admin_info": map[string]any{"usage_adjustment": map[string]any{"cache_percent": -30, "original": 130, "adjusted": 91}}})
	require.NoError(t, err)
	logs := []*Log{{PromptTokens: 59, CompletionTokens: 11, Other: string(encoded)}}
	formatUserLogs(logs, 0)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
	assert.NotContains(t, other, "admin_info")
	assert.EqualValues(t, 91, other["cache_tokens"])
	assert.Equal(t, 59, logs[0].PromptTokens)
}

func TestChannelUsageAdjustmentValidation(t *testing.T) {
	valid := `{"usage_adjustment":{"enabled":true,"cache_min":-30,"cache_max":-10,"output_min":0,"output_max":10}}`
	require.NoError(t, (&Channel{Setting: &valid}).ValidateSettings())
	invalid := `{"usage_adjustment":{"enabled":true,"cache_min":30,"cache_max":-10}}`
	require.Error(t, (&Channel{Setting: &invalid}).ValidateSettings())
}
