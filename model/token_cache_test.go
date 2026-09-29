package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Wait for the asynchronous cache refill before restoring global test state.
type tokenCacheRefillHook struct {
	done chan struct{}
}

func (h tokenCacheRefillHook) BeforeProcess(ctx context.Context, _ redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (h tokenCacheRefillHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (h tokenCacheRefillHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (h tokenCacheRefillHook) AfterProcessPipeline(_ context.Context, cmds []redis.Cmder) error {
	for _, cmd := range cmds {
		if cmd.Name() == "hset" {
			h.done <- struct{}{}
		}
	}
	return nil
}

func TestValidateUserTokenRecoversIncompleteCache(t *testing.T) {
	server := miniredis.RunT(t)
	oldDB, oldRedis, oldEnabled := DB, common.RDB, common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Token{}))
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	refilled := make(chan struct{}, 1)
	client.AddHook(tokenCacheRefillHook{done: refilled})
	DB, common.RDB, common.RedisEnabled = db, client, true
	t.Cleanup(func() {
		DB, common.RDB, common.RedisEnabled = oldDB, oldRedis, oldEnabled
		assert.NoError(t, client.Close())
		assert.NoError(t, sqlDB.Close())
	})

	tests := []struct {
		name      string
		status    int
		expires   int64
		quota     int
		unlimited bool
		valid     bool
	}{
		{name: "active", status: common.TokenStatusEnabled, expires: -1, quota: 100, valid: true},
		{name: "unlimited", status: common.TokenStatusEnabled, expires: -1, quota: -100, unlimited: true, valid: true},
		{name: "disabled", status: common.TokenStatusDisabled, expires: -1, quota: 100},
		{name: "expired", status: common.TokenStatusEnabled, expires: common.GetTimestamp() - 60, quota: 100},
		{name: "exhausted", status: common.TokenStatusEnabled, expires: -1, quota: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := Token{UserId: 123, Key: tt.name, Status: tt.status, ExpiredTime: tt.expires,
				RemainQuota: tt.quota, UnlimitedQuota: tt.unlimited}
			require.NoError(t, DB.Create(&token).Error)
			cacheKey := "token:" + common.GenerateHMAC(token.Key)
			server.HSet(cacheKey, "RemainQuota", "-10")
			server.SetTTL(cacheKey, time.Minute)

			got, authErr := ValidateUserToken(token.Key)
			// The fallback must refill the complete hash even for rejected keys.
			select {
			case <-refilled:
			case <-time.After(5 * time.Second):
				t.Fatal("incomplete cache was not refreshed from the database")
			}
			if tt.valid {
				require.NoError(t, authErr)
			} else {
				require.ErrorIs(t, authErr, ErrTokenInvalid)
			}
			require.NotNil(t, got)
			assert.Equal(t, token.Id, got.Id)
			assert.Equal(t, token.Status, got.Status)
			assert.Equal(t, token.UnlimitedQuota, got.UnlimitedQuota)
			cached, err := cacheGetTokenByKey(token.Key)
			require.NoError(t, err)
			assert.Equal(t, token.Id, cached.Id)
			assert.Equal(t, token.RemainQuota, cached.RemainQuota)
		})
	}
}
