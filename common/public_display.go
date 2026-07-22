package common

import (
	"hash/fnv"
	"math"
	"strconv"
)

const (
	publicDisplayMultiplierOption = "RankingsDisplayMultiplier"
	publicDisplayJitterOption     = "RankingsDisplayJitterRatio"
)

type PublicDisplaySettings struct {
	Multiplier float64
	Jitter     float64
}

func GetPublicDisplaySettings() PublicDisplaySettings {
	OptionMapRWMutex.RLock()
	multiplierValue := OptionMap[publicDisplayMultiplierOption]
	jitterValue := OptionMap[publicDisplayJitterOption]
	OptionMapRWMutex.RUnlock()

	multiplier, err := strconv.ParseFloat(multiplierValue, 64)
	if err != nil || multiplier < 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		multiplier = 1
	}
	jitter, err := strconv.ParseFloat(jitterValue, 64)
	if err != nil || jitter < 0 || math.IsNaN(jitter) || math.IsInf(jitter, 0) {
		jitter = 0
	}
	return PublicDisplaySettings{
		Multiplier: multiplier,
		Jitter:     jitter,
	}
}

func PublicDisplayEnabled(settings PublicDisplaySettings) bool {
	return settings.Multiplier != 1 || settings.Jitter != 0
}

func PublicDisplayValue(value int64, settings PublicDisplaySettings, salt string) int64 {
	if value <= 0 {
		return 0
	}
	scaled := float64(value) * settings.Multiplier
	if settings.Jitter > 0 {
		scaled += scaled * settings.Jitter * publicDisplayStableRandom01(salt)
	}
	if scaled <= 0 || math.IsNaN(scaled) {
		return 0
	}
	if math.IsInf(scaled, 1) || scaled >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(math.Round(scaled))
}

func publicDisplayStableRandom01(salt string) float64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(salt))
	return float64(hasher.Sum64()%1_000_000) / 1_000_000
}
