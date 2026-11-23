package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
)

func TestLogRequestSnapshotCanBeFetchedAfterOmittingItFromList(t *testing.T) {
	body := strings.Repeat("request body ", 10000)
	log := Log{Type: LogTypeError, Other: common.MapToJsonStr(map[string]any{
		"stream_status": "error",
		"admin_info": map[string]any{
			"request_snapshot": map[string]any{"body": body},
			"status_code":      502,
		},
	})}
	require.NoError(t, LOG_DB.Create(&log).Error)
	t.Cleanup(func() { LOG_DB.Delete(&log) })

	log.OmitRequestSnapshot()
	require.True(t, log.HasRequestSnapshot)
	require.JSONEq(t, `{"stream_status":"error","admin_info":{"status_code":502}}`, log.Other)

	snapshot, err := GetLogRequestSnapshot(log.Id, "", 0, 0, 0)
	require.NoError(t, err)
	require.JSONEq(t, common.MapToJsonStr(map[string]any{"body": body}), string(snapshot))
}

// TestFormatUserLogsStripsQuotaSaturation verifies the admin-only quota
// saturation marker (nested under other.admin_info) is removed for non-admin
// log views, since formatUserLogs strips the whole admin_info object.
func TestFormatUserLogsStripsQuotaSaturation(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"model_price": 0.004,
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromDecimal",
				"kind":    "overflow",
				"clamped": common.MaxQuota,
			},
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := parsed["admin_info"]
	require.False(t, hasAdminInfo, "admin_info (and nested quota_saturation) must be stripped for non-admin views")
	// Non-admin billing fields remain visible.
	require.Contains(t, parsed, "model_price")
}
