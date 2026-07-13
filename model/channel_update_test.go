package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelUpdateFieldsPersistsZeroValuesAndLeavesOmittedFields(t *testing.T) {
	originalDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	t.Cleanup(func() { DB = originalDB })

	baseURL := "https://api.example.com"
	autoBan := 1
	channel := Channel{
		Key: "secret", Name: "original", Weight: uintPtr(10), BaseURL: &baseURL,
		Models: "gpt-4o", Group: "default", AutoBan: &autoBan, OtherInfo: "metadata",
	}
	require.NoError(t, db.Create(&channel).Error)

	empty := ""
	zero := uint(0)
	autoBan = 0
	channel.Name = "must-not-change"
	channel.Weight = &zero
	channel.BaseURL = &empty
	channel.AutoBan = &autoBan
	channel.OtherInfo = ""
	require.NoError(t, channel.UpdateFields("weight", "base_url", "auto_ban", "other_info"))

	var persisted Channel
	require.NoError(t, db.First(&persisted, channel.Id).Error)
	assert.Equal(t, "original", persisted.Name)
	require.NotNil(t, persisted.Weight)
	assert.Zero(t, *persisted.Weight)
	require.NotNil(t, persisted.BaseURL)
	assert.Empty(t, *persisted.BaseURL)
	require.NotNil(t, persisted.AutoBan)
	assert.Zero(t, *persisted.AutoBan)
	assert.Empty(t, persisted.OtherInfo)
}

func uintPtr(value uint) *uint { return &value }
