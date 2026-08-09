package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLegacyOAuthMigrationTestDB(t *testing.T) {
	t.Helper()
	originalDB := DB
	originalDatabaseType := common.MainDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Token{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	t.Cleanup(func() {
		DB = originalDB
		common.SetMainDatabaseType(originalDatabaseType)
		common.RedisEnabled = originalRedisEnabled
	})
}

func TestMigrateLegacyOAuthTokensRevokesOnlyMarkedTokens(t *testing.T) {
	setupLegacyOAuthMigrationTestDB(t)

	for _, column := range legacyOAuthTokenColumns {
		require.NoError(t, DB.Exec("ALTER TABLE tokens ADD COLUMN `"+column+"` varchar(256)").Error)
	}

	tokens := []*Token{
		{UserId: 1, Key: "legacy-high-codex", Status: common.TokenStatusEnabled},
		{UserId: 2, Key: "legacy-hi-codex", Status: common.TokenStatusEnabled},
		{UserId: 3, Key: "ordinary-oauth-source", Status: common.TokenStatusEnabled},
		{UserId: 4, Key: "ordinary-oauth-scope", Status: common.TokenStatusEnabled},
		{UserId: 5, Key: "ordinary-api-key", Status: common.TokenStatusEnabled},
	}
	for _, token := range tokens {
		require.NoError(t, DB.Create(token).Error)
	}
	require.NoError(t, DB.Exec(
		"UPDATE tokens SET source = ?, oauth_client_id = ?, oauth_scopes = ? WHERE id = ?",
		legacyOAuthTokenSource, legacyOAuthTokenClientIDOld, legacyOAuthTokenScope, tokens[0].Id,
	).Error)
	require.NoError(t, DB.Exec(
		"UPDATE tokens SET source = ?, oauth_client_id = ?, oauth_scopes = ? WHERE id = ?",
		legacyOAuthTokenSource, legacyOAuthTokenClientIDNew, legacyOAuthTokenScope, tokens[1].Id,
	).Error)
	require.NoError(t, DB.Exec(
		"UPDATE tokens SET source = ?, oauth_client_id = ?, oauth_scopes = ? WHERE id = ?",
		"manual", legacyOAuthTokenClientIDOld, legacyOAuthTokenScope, tokens[2].Id,
	).Error)
	require.NoError(t, DB.Exec(
		"UPDATE tokens SET source = ?, oauth_client_id = ?, oauth_scopes = ? WHERE id = ?",
		legacyOAuthTokenSource, legacyOAuthTokenClientIDNew, "api_keys:read account:read", tokens[3].Id,
	).Error)

	require.NoError(t, migrateLegacyOAuthTokens())

	var storedTokens []Token
	require.NoError(t, DB.Select("id, status").Find(&storedTokens).Error)
	statuses := make(map[int]int, len(storedTokens))
	for _, token := range storedTokens {
		statuses[token.Id] = token.Status
	}
	assert.Equal(t, common.TokenStatusDisabled, statuses[tokens[0].Id])
	assert.Equal(t, common.TokenStatusDisabled, statuses[tokens[1].Id])
	assert.Equal(t, common.TokenStatusEnabled, statuses[tokens[2].Id])
	assert.Equal(t, common.TokenStatusEnabled, statuses[tokens[3].Id])
	assert.Equal(t, common.TokenStatusEnabled, statuses[tokens[4].Id])

	// A second startup migration is a no-op and leaves all decisions unchanged.
	require.NoError(t, migrateLegacyOAuthTokens())
	var migrated Token
	require.NoError(t, DB.First(&migrated, tokens[0].Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, migrated.Status)
}

func TestMigrateLegacyOAuthTokensSkipsFreshSchema(t *testing.T) {
	setupLegacyOAuthMigrationTestDB(t)
	token := &Token{UserId: 1, Key: "ordinary-api-key", Status: common.TokenStatusEnabled}
	require.NoError(t, DB.Create(token).Error)

	require.NoError(t, migrateLegacyOAuthTokens())
	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	assert.Equal(t, common.TokenStatusEnabled, stored.Status)
}
