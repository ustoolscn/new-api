package common

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRedisHashTest(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	previous := RDB
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	RDB = client
	t.Cleanup(func() {
		RDB = previous
		assert.NoError(t, client.Close())
	})
	return server
}

func TestRedisHGetObjRejectsIncompleteAuthenticationData(t *testing.T) {
	server := setupRedisHashTest(t)
	type credential struct {
		Id             int
		Status         int
		RemainQuota    int
		UnlimitedQuota bool
		DeletedAt      gorm.DeletedAt
	}
	want := credential{Id: 7, Status: 1, RemainQuota: -10, UnlimitedQuota: true}
	require.NoError(t, RedisHSetObj("token:test", &want, time.Minute))
	var got credential
	require.NoError(t, RedisHGetObj("token:test", &got))
	assert.Equal(t, want, got)

	// A missing bool must not turn an unlimited key into an exhausted key.
	server.HDel("token:test", "UnlimitedQuota")
	got = credential{}
	require.Error(t, RedisHGetObj("token:test", &got))

	// An interrupted cache update must not deserialize as a disabled key.
	server.Del("token:test")
	server.HSet("token:test", "RemainQuota", "-10")
	got = credential{}
	require.Error(t, RedisHGetObj("token:test", &got))
}

// Invalidate the hash immediately before its mutation reaches Redis. This
// reproduces expiry/deletion after the old standalone TTL check without sleeps.
type invalidateHashBeforeWrite struct {
	server *miniredis.Miniredis
	key    string
}

func (h invalidateHashBeforeWrite) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() == "eval" || cmd.Name() == "evalsha" {
		h.server.Del(h.key)
	}
	return ctx, nil
}

func (h invalidateHashBeforeWrite) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (h invalidateHashBeforeWrite) BeforeProcessPipeline(ctx context.Context, cmds []redis.Cmder) (context.Context, error) {
	for _, cmd := range cmds {
		if cmd.Name() == "hincrby" || cmd.Name() == "hset" {
			h.server.Del(h.key)
			break
		}
	}
	return ctx, nil
}

func (h invalidateHashBeforeWrite) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestRedisHashUpdatesDoNotRecreateInvalidatedCache(t *testing.T) {
	for _, operation := range []string{"increment", "set"} {
		t.Run(operation, func(t *testing.T) {
			server := setupRedisHashTest(t)
			server.HSet("token:test", "RemainQuota", "100", "Status", "1")
			server.SetTTL("token:test", time.Minute)
			RDB.AddHook(invalidateHashBeforeWrite{server: server, key: "token:test"})
			if operation == "increment" {
				require.NoError(t, RedisHIncrBy("token:test", "RemainQuota", -10))
			} else {
				require.NoError(t, RedisHSetField("token:test", "Status", "2"))
			}
			assert.False(t, server.Exists("token:test"), "a cache delta must not create an incomplete credential")
		})
	}
}

func TestRedisHashUpdatesPreserveValuesAndExpiry(t *testing.T) {
	server := setupRedisHashTest(t)
	server.HSet("token:test", "RemainQuota", "100", "Status", "1")
	server.SetTTL("token:test", 1500*time.Millisecond)
	require.NoError(t, RedisHIncrBy("token:test", "RemainQuota", -10))
	require.NoError(t, RedisHSetField("token:test", "Status", "2"))
	assert.Equal(t, "90", server.HGet("token:test", "RemainQuota"))
	assert.Equal(t, "2", server.HGet("token:test", "Status"))
	assert.Equal(t, 1500*time.Millisecond, server.TTL("token:test"))

	server.FastForward(1500 * time.Millisecond)
	require.NoError(t, RedisHIncrBy("token:test", "RemainQuota", 10))
	require.NoError(t, RedisHSetField("token:test", "Status", "1"))
	assert.False(t, server.Exists("token:test"))
}
