package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}

func TestModelPriceHelperTieredPreConsumeMaxTokensFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"tiered-fallback-model":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"tiered-fallback-model":"tier(\"base\", p * 3 + c * 15)"}`,
		"group_ratio_setting.group_ratio": `{"default":1,"free":0}`,
	}))

	const promptTokens = 1000

	cases := []struct {
		name      string
		group     string
		maxTokens int
		expected  int
	}{
		{
			// max_tokens omitted in a paid group -> fall back to 8192 completion tokens.
			// p*3 + c*15 = 1000*3 + 8192*15 = 125880 -> /1e6 * 500000 = 62940
			name:      "non-free group falls back to 8192 completion tokens",
			group:     "default",
			maxTokens: 0,
			expected:  62940,
		},
		{
			// explicit max_tokens is used verbatim, no fallback.
			// 1000*3 + 100*15 = 4500 -> /1e6 * 500000 = 2250
			name:      "explicit max_tokens is used verbatim",
			group:     "default",
			maxTokens: 100,
			expected:  2250,
		},
		{
			// free group (ratio 0) stays zero; fallback is gated on non-zero group ratio.
			name:      "free group stays zero without fallback",
			group:     "free",
			maxTokens: 0,
			expected:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req.Header.Set("Content-Type", "application/json")
			ctx.Request = req
			ctx.Set("group", tc.group)

			info := &relaycommon.RelayInfo{
				OriginModelName: "tiered-fallback-model",
				UserGroup:       tc.group,
				UsingGroup:      tc.group,
				RequestHeaders:  map[string]string{"Content-Type": "application/json"},
				BillingRequestInput: &billingexpr.RequestInput{
					Headers: map[string]string{"Content-Type": "application/json"},
					Body:    []byte(`{}`),
				},
			}

			priceData, err := ModelPriceHelper(ctx, info, promptTokens, &types.TokenCountMeta{MaxTokens: tc.maxTokens})
			require.NoError(t, err)
			require.Equal(t, tc.expected, priceData.QuotaToPreConsume)
		})
	}
}

func TestCalculateVideoSecondsBilling(t *testing.T) {
	trace, err := calculateVideoSecondsBilling(relaycommon.TaskSubmitReq{
		Duration: 5,
		Width:    1280,
		Height:   720,
		FPS:      30,
	}, billing_setting.VideoPriceConfig{
		BaseFPS: 24,
		Prices: map[string]float64{
			"720p":  1,
			"1080p": 2,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "720p", trace.Resolution)
	require.Equal(t, 5.0, trace.Duration)
	require.Equal(t, 30.0/24.0, trace.FPSMultiplier)
	require.Equal(t, 6.25, trace.GeneratedVideoPrice)
	require.Equal(t, 6.25, trace.TotalPrice)
}

func TestCalculateVideoSecondsBillingIncludesInputContentAndVideo(t *testing.T) {
	trace, err := calculateVideoSecondsBilling(relaycommon.TaskSubmitReq{
		Duration:           5,
		Width:              1280,
		Height:             720,
		InputVideo:         "https://example.com/ref.mp4",
		InputVideoDuration: 3,
	}, billing_setting.VideoPriceConfig{
		BaseFPS:           24,
		InputContentPrice: 0.5,
		Prices: map[string]float64{
			"720p": 1,
		},
	})
	require.NoError(t, err)
	require.True(t, trace.InputContentCharged)
	require.Equal(t, 0.5, trace.InputContentPrice)
	require.Equal(t, 3.0, trace.InputVideoDuration)
	require.Equal(t, 8.0, trace.BillableDuration)
	require.Equal(t, 8.0, trace.GeneratedVideoPrice)
	require.Equal(t, 8.5, trace.TotalPrice)
}

func TestCalculateVideoSecondsBillingRequiresInputVideoDuration(t *testing.T) {
	_, err := calculateVideoSecondsBilling(relaycommon.TaskSubmitReq{
		Duration:   5,
		Width:      1280,
		Height:     720,
		InputVideo: "https://example.com/ref.mp4",
	}, billing_setting.VideoPriceConfig{
		Prices: map[string]float64{
			"720p": 1,
		},
	})
	require.ErrorContains(t, err, "input video duration is required")
}

func TestModelPriceHelperVideoSecondsDoesNotExposeBillableRatios(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"video-test-model":"video_seconds"}`,
		"billing_setting.video_price":  `{"video-test-model":{"base_fps":24,"prices":{"720p":1}}}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    "video-test-model",
		Prompt:   "astronaut walking on the moon",
		Duration: 5,
		Width:    1280,
		Height:   720,
	})

	priceData, err := modelPriceHelperVideoSeconds(ctx, &relaycommon.RelayInfo{
		OriginModelName: "video-test-model",
	}, types.GroupRatioInfo{GroupRatio: 1})
	require.NoError(t, err)
	require.Equal(t, billingexpr.QuotaRound(5*common.QuotaPerUnit), priceData.Quota)
	require.Equal(t, 5.0, priceData.ModelPrice)
	require.Empty(t, priceData.OtherRatios())
}

func TestHasModelBillingConfigAcceptsVideoSeconds(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"doubao-seedance-2.0":"video_seconds"}`,
		"billing_setting.video_price":  `{"doubao-seedance-2.0":{"base_fps":24,"prices":{"720p":1}}}`,
	}))

	require.True(t, HasModelBillingConfig("doubao-seedance-2.0"))
	require.True(t, HasVideoSecondsBillingConfig("doubao-seedance-2.0"))
	require.False(t, HasVideoSecondsBillingConfig("missing-video-model"))
}
