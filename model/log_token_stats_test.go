package model

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCacheTokensFromLogOther(t *testing.T) {
	tests := []struct {
		name  string
		other string
		want  int
	}{
		{
			name:  "empty payload",
			other: "",
			want:  0,
		},
		{
			name: "cache read and aggregate cache write",
			other: `{
				"cache_tokens": 12,
				"cache_creation_tokens": 34
			}`,
			want: 46,
		},
		{
			name: "split cache writes override aggregate cache write",
			other: `{
				"cache_tokens": 12,
				"cache_creation_tokens": 999,
				"cache_creation_tokens_5m": 34,
				"cache_creation_tokens_1h": 56
			}`,
			want: 102,
		},
		{
			name:  "invalid payload",
			other: "{",
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cacheTokenStatsFromLogOther(tt.other).Total; got != tt.want {
				t.Fatalf("cache token total = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSumUsedQuotaPreservesAnthropicInputWhenModelUsesMixedFormats(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	logs := []Log{
		{
			Username: "alice", Type: LogTypeConsume, ModelName: "shared-model", CreatedAt: now,
			PromptTokens: 50, CompletionTokens: 100, Quota: 10,
			Other: `{"usage_semantic":"anthropic","cache_tokens":10000,"cache_write_tokens":2000,"cache_creation_tokens":2000,"cache_creation_tokens_5m":500}`,
		},
		{
			Username: "alice", Type: LogTypeConsume, ModelName: "shared-model", CreatedAt: now - 120,
			PromptTokens: 10, CompletionTokens: 20, Quota: 10,
			Other: `{"claude":true,"cache_tokens":1000}`,
		},
		{
			Username: "alice", Type: LogTypeConsume, ModelName: "shared-model", CreatedAt: now,
			PromptTokens: 400, CompletionTokens: 10, Quota: 10,
			Other: `{"usage_semantic":"openai","claude":true,"cache_tokens":300}`,
		},
		{
			Username: "alice", Type: LogTypeConsume, ModelName: "other-model", CreatedAt: now,
			PromptTokens: 25, CompletionTokens: 5, Quota: 1,
			Other: `{"cache_tokens":5}`,
		},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "alice", "", 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, 31, stat.Quota)
	require.Equal(t, 485, stat.PromptTokens)
	require.Equal(t, 180, stat.InputTokens)
	require.Equal(t, 13305, stat.CacheTokens)
	require.Equal(t, 135, stat.CompletionTokens)
	require.Equal(t, 3, stat.Rpm)
	require.Equal(t, 12590, stat.Tpm)
	require.Equal(t, []ModelTokenStat{
		{ModelName: "shared-model", Quota: 30, PromptTokens: 460, InputTokens: 160, CacheTokens: 13300, CompletionTokens: 130},
		{ModelName: "other-model", Quota: 1, PromptTokens: 25, InputTokens: 20, CacheTokens: 5, CompletionTokens: 5},
	}, stat.ModelStats)
}

func TestRecordConsumeLogExportsFullAnthropicTokensToRankings(t *testing.T) {
	truncateTables(t)
	oldDataExportEnabled := common.DataExportEnabled
	common.DataExportEnabled = true
	CacheQuotaDataLock.Lock()
	oldQuotaDataCache := CacheQuotaData
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()
	t.Cleanup(func() {
		common.DataExportEnabled = oldDataExportEnabled
		CacheQuotaDataLock.Lock()
		CacheQuotaData = oldQuotaDataCache
		CacheQuotaDataLock.Unlock()
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	ctx.Set("username", "alice")

	RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ModelName: "messages-model", PromptTokens: 50, CompletionTokens: 100,
		Other: map[string]interface{}{
			"usage_semantic": "anthropic", "cache_tokens": 10000,
			"cache_write_tokens": 2000, "cache_creation_tokens": 2000, "cache_creation_tokens_5m": 500,
		},
	})
	RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ModelName: "chat-model", PromptTokens: 400, CompletionTokens: 10,
		Other: map[string]interface{}{"usage_semantic": "openai", "claude": true, "cache_tokens": 300},
	})
	RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ModelName: "cache-only-model",
		Other:     map[string]interface{}{"claude": true, "cache_tokens": 8, "cache_creation_tokens_5m": 2},
	})
	SaveQuotaDataCache()

	// Aggregated rows written before the fix still contribute their stored counts.
	require.NoError(t, DB.Create(&QuotaData{ModelName: "old-model", CreatedAt: 3600, TokenUsed: 20}).Error)

	totals, err := GetRankingQuotaTotals(0, 0)
	require.NoError(t, err)
	require.Equal(t, []RankingQuotaTotal{
		{ModelName: "messages-model", TotalTokens: 12150},
		{ModelName: "chat-model", TotalTokens: 410},
		{ModelName: "old-model", TotalTokens: 20},
		{ModelName: "cache-only-model", TotalTokens: 10},
	}, totals)

	buckets, err := GetRankingQuotaBuckets(0, 0, 3600)
	require.NoError(t, err)
	require.Len(t, buckets, len(totals))
	byModel := make(map[string]int64, len(buckets))
	for _, bucket := range buckets {
		byModel[bucket.ModelName] = bucket.Tokens
	}
	for _, total := range totals {
		require.Equal(t, total.TotalTokens, byModel[total.ModelName], total.ModelName)
	}
}

func TestSumUsedQuotaSummarizesTokensAndCacheTokens(t *testing.T) {
	truncateTables(t)

	logs := []*Log{
		{
			UserId:           1,
			Username:         "alice",
			CreatedAt:        100,
			Type:             LogTypeConsume,
			ModelName:        "gpt-test",
			TokenName:        "primary",
			Quota:            10,
			PromptTokens:     100,
			CompletionTokens: 25,
			Other:            `{"cache_tokens":15,"cache_creation_tokens":5}`,
		},
		{
			UserId:           1,
			Username:         "alice",
			CreatedAt:        101,
			Type:             LogTypeConsume,
			ModelName:        "gpt-test",
			TokenName:        "primary",
			Quota:            20,
			PromptTokens:     200,
			CompletionTokens: 50,
			Other:            `{"cache_tokens":9,"cache_creation_tokens":999,"cache_creation_tokens_5m":6,"cache_creation_tokens_1h":3}`,
		},
		{
			UserId:           1,
			Username:         "alice",
			CreatedAt:        102,
			Type:             LogTypeConsume,
			ModelName:        "gpt-test",
			TokenName:        "primary",
			Quota:            30,
			PromptTokens:     300,
			CompletionTokens: 75,
			Other:            `{}`,
		},
		{
			UserId:           2,
			Username:         "bob",
			CreatedAt:        103,
			Type:             LogTypeConsume,
			ModelName:        "gpt-test",
			TokenName:        "primary",
			Quota:            40,
			PromptTokens:     400,
			CompletionTokens: 100,
			Other:            `{"cache_tokens":100}`,
		},
	}

	for _, log := range logs {
		if err := LOG_DB.Create(log).Error; err != nil {
			t.Fatalf("insert log: %v", err)
		}
	}

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "gpt-test", "alice", "primary", 0, "", "", "")
	if err != nil {
		t.Fatalf("SumUsedQuota() error = %v", err)
	}

	if stat.Quota != 60 {
		t.Fatalf("stat.Quota = %d, want 60", stat.Quota)
	}
	if stat.PromptTokens != 600 {
		t.Fatalf("stat.PromptTokens = %d, want 600", stat.PromptTokens)
	}
	if stat.InputTokens != 576 {
		t.Fatalf("stat.InputTokens = %d, want 576", stat.InputTokens)
	}
	if stat.CompletionTokens != 150 {
		t.Fatalf("stat.CompletionTokens = %d, want 150", stat.CompletionTokens)
	}
	if stat.CacheTokens != 38 {
		t.Fatalf("stat.CacheTokens = %d, want 38", stat.CacheTokens)
	}
}

func TestSumUsedQuotaGroupsTokenStatsByModelName(t *testing.T) {
	truncateTables(t)

	logs := []*Log{
		{
			UserId:           1,
			Username:         "alice",
			CreatedAt:        100,
			Type:             LogTypeConsume,
			ModelName:        "gpt-a",
			Quota:            10,
			PromptTokens:     100,
			CompletionTokens: 10,
			Other:            `{"cache_tokens":8,"cache_creation_tokens":2}`,
		},
		{
			UserId:           1,
			Username:         "alice",
			CreatedAt:        101,
			Type:             LogTypeConsume,
			ModelName:        "gpt-b",
			Quota:            20,
			PromptTokens:     200,
			CompletionTokens: 20,
			Other:            `{"cache_tokens":20}`,
		},
		{
			UserId:           1,
			Username:         "alice",
			CreatedAt:        102,
			Type:             LogTypeConsume,
			ModelName:        "gpt-a",
			Quota:            30,
			PromptTokens:     300,
			CompletionTokens: 30,
			Other:            `{}`,
		},
	}

	for _, log := range logs {
		if err := LOG_DB.Create(log).Error; err != nil {
			t.Fatalf("insert log: %v", err)
		}
	}

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "alice", "", 0, "", "", "")
	if err != nil {
		t.Fatalf("SumUsedQuota() error = %v", err)
	}

	if len(stat.ModelStats) != 2 {
		t.Fatalf("len(stat.ModelStats) = %d, want 2", len(stat.ModelStats))
	}

	byModel := map[string]ModelTokenStat{}
	for _, item := range stat.ModelStats {
		byModel[item.ModelName] = item
	}

	gptA := byModel["gpt-a"]
	if gptA.Quota != 40 || gptA.PromptTokens != 400 || gptA.InputTokens != 392 || gptA.CacheTokens != 10 || gptA.CompletionTokens != 40 {
		t.Fatalf("gpt-a stats = %+v", gptA)
	}

	gptB := byModel["gpt-b"]
	if gptB.Quota != 20 || gptB.PromptTokens != 200 || gptB.InputTokens != 180 || gptB.CacheTokens != 20 || gptB.CompletionTokens != 20 {
		t.Fatalf("gpt-b stats = %+v", gptB)
	}
}
