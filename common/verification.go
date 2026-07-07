package common

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

type verificationValue struct {
	code string
	time time.Time
}

const (
	EmailVerificationPurpose = "v"
	PasswordResetPurpose     = "r"
	PhoneVerificationPurpose = "p"
)

var verificationMutex sync.Mutex
var verificationMap map[string]verificationValue
var verificationMapMaxSize = 10
var VerificationValidMinutes = 10

func GenerateVerificationCode(length int) string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	if length == 0 {
		return code
	}
	return code[:length]
}

func RegisterVerificationCodeWithKey(key string, code string, purpose string) {
	normalizedKey := normalizeVerificationKey(key)
	if registerVerificationCodeInRedis(normalizedKey, code, purpose) {
		return
	}

	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap[purpose+normalizedKey] = verificationValue{
		code: code,
		time: time.Now(),
	}
	if len(verificationMap) > verificationMapMaxSize {
		removeExpiredPairs()
	}
}

func VerifyCodeWithKey(key string, code string, purpose string) bool {
	normalizedKey := normalizeVerificationKey(key)
	if storedCode, ok := getVerificationCodeFromRedis(normalizedKey, purpose); ok {
		return code == storedCode
	}

	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	value, okay := verificationMap[purpose+normalizedKey]
	now := time.Now()
	if !okay || int(now.Sub(value.time).Seconds()) >= VerificationValidMinutes*60 {
		return false
	}
	return code == value.code
}

func DeleteKey(key string, purpose string) {
	normalizedKey := normalizeVerificationKey(key)
	deleteVerificationCodeFromRedis(normalizedKey, purpose)

	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	delete(verificationMap, purpose+normalizedKey)
}

func normalizeVerificationKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func redisVerificationKey(key string, purpose string) string {
	return "verification:" + purpose + ":" + key
}

func registerVerificationCodeInRedis(key string, code string, purpose string) bool {
	if !RedisEnabled || RDB == nil {
		return false
	}
	err := RDB.Set(context.Background(), redisVerificationKey(key, purpose), code, time.Duration(VerificationValidMinutes)*time.Minute).Err()
	return err == nil
}

func getVerificationCodeFromRedis(key string, purpose string) (string, bool) {
	if !RedisEnabled || RDB == nil {
		return "", false
	}
	code, err := RDB.Get(context.Background(), redisVerificationKey(key, purpose)).Result()
	if err != nil {
		if err == redis.Nil {
			return "", false
		}
		return "", false
	}
	return code, true
}

func deleteVerificationCodeFromRedis(key string, purpose string) {
	if !RedisEnabled || RDB == nil {
		return
	}
	_ = RDB.Del(context.Background(), redisVerificationKey(key, purpose)).Err()
}

// no lock inside, so the caller must lock the verificationMap before calling!
func removeExpiredPairs() {
	now := time.Now()
	for key := range verificationMap {
		if int(now.Sub(verificationMap[key].time).Seconds()) >= VerificationValidMinutes*60 {
			delete(verificationMap, key)
		}
	}
}

func init() {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap = make(map[string]verificationValue)
}
