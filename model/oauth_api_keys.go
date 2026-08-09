package model

import "gorm.io/gorm"

// GetAllOAuthAPIKeys returns every non-deleted API key owned by userID in
// reverse creation order. GORM's default query scope excludes soft-deleted
// tokens, and the unbounded query is intentional: this endpoint is used by
// OAuth clients that need the complete key inventory.
func GetAllOAuthAPIKeys(userID int) ([]*Token, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var tokens []*Token
	err := DB.Where("user_id = ?", userID).Order("id DESC").Find(&tokens).Error
	return tokens, err
}
