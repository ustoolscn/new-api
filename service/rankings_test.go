package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRankingDisplayValueMultiplier(t *testing.T) {
	settings := common.PublicDisplaySettings{Multiplier: 12, Jitter: 0}

	value := common.PublicDisplayValue(100, settings, "model-a")

	assert.Equal(t, int64(1200), value)
}

func TestApplyRankingDisplayToTotalsSortsByDisplayedValue(t *testing.T) {
	settings := common.PublicDisplaySettings{Multiplier: 1, Jitter: 1}
	totals := []model.RankingQuotaTotal{
		{ModelName: "model-a", TotalTokens: 100},
		{ModelName: "model-b", TotalTokens: 100},
	}

	rows := applyRankingDisplayToTotals(totals, settings, "test")

	require.Len(t, rows, len(totals))
	for _, row := range rows {
		assert.GreaterOrEqual(t, row.TotalTokens, int64(100))
		assert.LessOrEqual(t, row.TotalTokens, int64(200))
	}
	assert.GreaterOrEqual(t, rows[0].TotalTokens, rows[1].TotalTokens)
}
