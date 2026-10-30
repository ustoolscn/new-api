package dto

import (
	"fmt"
	"math"
)

// UsageAdjustment is opt-in channel metering, in percentage points.
type UsageAdjustment struct {
	Enabled   bool    `json:"enabled"`
	CacheMin  float64 `json:"cache_min"`
	CacheMax  float64 `json:"cache_max"`
	OutputMin float64 `json:"output_min"`
	OutputMax float64 `json:"output_max"`
}

func (u *UsageAdjustment) Validate() error {
	if u == nil {
		return nil
	}
	for _, v := range []float64{u.CacheMin, u.CacheMax, u.OutputMin, u.OutputMax} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < -100 || v > 100 {
			return fmt.Errorf("usage adjustment percentages must be between -100 and 100")
		}
	}
	if u.CacheMin > u.CacheMax || u.OutputMin > u.OutputMax {
		return fmt.Errorf("usage adjustment minimum must not exceed maximum")
	}
	return nil
}

type MeteredTokens struct {
	Reasoning  int64 `json:"reasoning,omitempty"`
	ToolInput  int64 `json:"tool_input,omitempty"`
	Input      int64 `json:"input"` // Total input, including cache read and creation.
	Uncached   int64 `json:"uncached"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
	Output     int64 `json:"output"`
}

type UsageAdjustmentRecord struct {
	Original              MeteredTokens       `json:"original"`
	Adjusted              MeteredTokens       `json:"adjusted"`
	OriginalOutputDetails *OutputTokenDetails `json:"-"`
}

type UsageAdjustmentAudit struct {
	OriginalQuota *int                             `json:"original_quota,omitempty"`
	AdjustedQuota *int                             `json:"adjusted_quota,omitempty"`
	Rule          UsageAdjustment                  `json:"rule"`
	CachePercent  float64                          `json:"cache_percent"`
	OutputPercent float64                          `json:"output_percent"`
	Status        string                           `json:"status"`
	Records       map[string]UsageAdjustmentRecord `json:"records"`
}
