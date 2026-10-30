package dto

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

func TestUsageAdjustmentValidation(t *testing.T) {
	require.NoError(t, (*UsageAdjustment)(nil).Validate())
	require.NoError(t, (&UsageAdjustment{Enabled: true, CacheMin: -100, CacheMax: 100, OutputMin: -100, OutputMax: 100}).Validate())
	for _, rule := range []UsageAdjustment{
		{CacheMin: -101}, {CacheMax: 101}, {OutputMin: 20, OutputMax: 10}, {CacheMin: 1, CacheMax: 0}, {OutputMax: math.Inf(1)}, {CacheMin: math.NaN()},
	} {
		require.Error(t, rule.Validate())
	}
}
