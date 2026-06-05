package model

import "testing"

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
			if got := cacheTokensFromLogOther(tt.other); got != tt.want {
				t.Fatalf("cacheTokensFromLogOther() = %d, want %d", got, tt.want)
			}
		})
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

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "gpt-test", "alice", "primary", 0, "", "")
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

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "alice", "", 0, "", "")
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
