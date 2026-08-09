package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	legacyOAuthTokenSource       = "oauth"
	legacyOAuthTokenScope        = "api"
	legacyOAuthTokenClientIDOld  = "high-codex"
	legacyOAuthTokenClientIDNew  = "hi-codex"
	legacyOAuthTokenColumnSource = "source"
)

var legacyOAuthTokenColumns = []string{
	legacyOAuthTokenColumnSource,
	"oauth_client_id",
	"oauth_scopes",
}

// legacyOAuthToken is intentionally limited to the fields needed to revoke a
// token and remove its cached copy. The OAuth marker columns are not part of
// Token anymore, but may remain in existing databases after AutoMigrate.
type legacyOAuthToken struct {
	Id  int    `gorm:"column:id"`
	Key string `gorm:"column:key"`
}

// migrateLegacyOAuthTokens revokes tokens created by the unreleased OAuth
// API-key flow. That flow stored ordinary sk- credentials in tokens and marked
// them with source/oauth_client_id/oauth_scopes. Requiring all three marker
// columns and their exact historical values prevents ordinary API keys from
// being touched. The migration is safe to run repeatedly.
func migrateLegacyOAuthTokens() error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	if !DB.Migrator().HasTable(&Token{}) {
		return nil
	}
	for _, column := range legacyOAuthTokenColumns {
		if !DB.Migrator().HasColumn(&Token{}, column) {
			return nil
		}
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	var tokens []legacyOAuthToken
	keyColumn := legacyOAuthTokenKeyColumn()
	query := tx.Unscoped().Table("tokens").
		Select("id, "+keyColumn).
		Where(legacyOAuthTokenColumnSource+" = ?", legacyOAuthTokenSource).
		Where("oauth_client_id IN ?", []string{legacyOAuthTokenClientIDOld, legacyOAuthTokenClientIDNew}).
		Where("oauth_scopes = ?", legacyOAuthTokenScope)
	if err := query.Find(&tokens).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to find legacy OAuth tokens: %w", err)
	}
	if len(tokens) == 0 {
		tx.Rollback()
		return nil
	}

	ids := make([]int, 0, len(tokens))
	for _, token := range tokens {
		ids = append(ids, token.Id)
	}
	if err := tx.Unscoped().Model(&Token{}).
		Where("id IN ?", ids).
		Update("status", common.TokenStatusDisabled).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to revoke legacy OAuth tokens: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit legacy OAuth token migration: %w", err)
	}

	cacheTokens := make([]Token, 0, len(tokens))
	for _, token := range tokens {
		cacheTokens = append(cacheTokens, Token{Id: token.Id, Key: token.Key})
	}
	if err := invalidateTokensCache(cacheTokens); err != nil {
		return fmt.Errorf("failed to invalidate legacy OAuth token cache: %w", err)
	}
	common.SysLog(fmt.Sprintf("revoked %d legacy OAuth token(s)", len(tokens)))
	return nil
}

func legacyOAuthTokenKeyColumn() string {
	if commonKeyCol != "" {
		return commonKeyCol
	}
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return `"key"`
	}
	return "`key`"
}
