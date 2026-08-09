package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// OAuthAccessToken stores the metadata for a dedicated Hi Codex bearer token.
// The plaintext token is deliberately never persisted; TokenHash is the
// SHA-256 digest of the complete oa- prefixed token returned to the client.
type OAuthAccessToken struct {
	Id        int    `json:"-"`
	TokenHash string `json:"-" gorm:"type:char(64);uniqueIndex;not null"`
	UserId    int    `json:"-" gorm:"index;not null"`
	ClientID  string `json:"-" gorm:"type:varchar(64);index;not null"`
	Scope     string `json:"-" gorm:"type:varchar(256);not null"`
	CreatedAt int64  `json:"-" gorm:"bigint;not null"`
	ExpiresAt int64  `json:"-" gorm:"bigint;not null;index"`
	RevokedAt *int64 `json:"-" gorm:"bigint;index"`
}

// GetOAuthAccessTokenByValue looks up a dedicated OAuth bearer by hashing the
// supplied opaque value. Callers must validate the oa- prefix at the boundary.
func GetOAuthAccessTokenByValue(value string) (*OAuthAccessToken, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	if !strings.HasPrefix(value, "oa-") || len(value) <= len("oa-") {
		return nil, gorm.ErrRecordNotFound
	}
	var token OAuthAccessToken
	if err := DB.Where("token_hash = ?", HashOAuthValue(value)).First(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

// ValidateOAuthAccessTokenScope performs the scope check used by the OAuth
// middleware while keeping malformed legacy rows fail-closed.
func ValidateOAuthAccessTokenScope(token *OAuthAccessToken, requiredScopes ...string) error {
	if token == nil {
		return errors.New("oauth access token is nil")
	}
	if !OAuthScopeAllows(token.Scope, requiredScopes...) {
		return errors.New("oauth access token scope is insufficient")
	}
	return nil
}
