package common

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerificationCodeUsesRedisWhenEnabled(t *testing.T) {
	server := miniredis.RunT(t)
	previousRDB := RDB
	previousRedisEnabled := RedisEnabled
	previousValidMinutes := VerificationValidMinutes
	t.Cleanup(func() {
		RDB = previousRDB
		RedisEnabled = previousRedisEnabled
		VerificationValidMinutes = previousValidMinutes
		resetVerificationMapForTest()
	})

	RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	RedisEnabled = true
	VerificationValidMinutes = 10
	resetVerificationMapForTest()

	RegisterVerificationCodeWithKey("User@Example.com", "ABC123", EmailVerificationPurpose)
	resetVerificationMapForTest()

	assert.True(t, VerifyCodeWithKey("user@example.com", "ABC123", EmailVerificationPurpose))
	ttl := server.TTL("verification:v:user@example.com")
	require.Greater(t, ttl, 9*time.Minute)
	require.LessOrEqual(t, ttl, 10*time.Minute)
}

func resetVerificationMapForTest() {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap = make(map[string]verificationValue)
}
