package common

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPublicDisplayValueAppliesMultiplierAndStableJitter(t *testing.T) {
	settings := PublicDisplaySettings{Multiplier: 12, Jitter: 0}
	assert.Equal(t, int64(1200), PublicDisplayValue(100, settings, "model-a"))

	settings = PublicDisplaySettings{Multiplier: 1, Jitter: 0.25}
	first := PublicDisplayValue(1000, settings, "stable")
	second := PublicDisplayValue(1000, settings, "stable")
	assert.Equal(t, first, second)
	assert.GreaterOrEqual(t, first, int64(1000))
	assert.LessOrEqual(t, first, int64(1250))
}

func TestPublicDisplayValueSaturatesOverflow(t *testing.T) {
	settings := PublicDisplaySettings{Multiplier: math.MaxFloat64}
	assert.Equal(t, int64(math.MaxInt64), PublicDisplayValue(math.MaxInt64, settings, "overflow"))
}
