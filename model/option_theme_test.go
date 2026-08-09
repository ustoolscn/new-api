package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInitOptionMapNormalizesLegacyFrontendThemeOption(t *testing.T) {
	originalDB := DB
	originalOptionMap := common.OptionMap
	originalIsMasterNode := common.IsMasterNode
	common.IsMasterNode = true
	legacyFrontendThemeOptionOnce = sync.Once{}
	t.Cleanup(func() {
		DB = originalDB
		common.IsMasterNode = originalIsMasterNode
		legacyFrontendThemeOptionOnce = sync.Once{}
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	tests := []struct {
		name  string
		value *string
	}{
		{name: "absent"},
		{name: "classic", value: stringPtr("classic")},
		{name: "invalid", value: stringPtr("unknown")},
		{name: "default", value: stringPtr("default")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacyFrontendThemeOptionOnce = sync.Once{}
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&Option{}))
			DB = db
			if tt.value != nil {
				require.NoError(t, db.Create(&Option{Key: legacyFrontendThemeOptionKey, Value: *tt.value}).Error)
			}

			InitOptionMap()

			var option Option
			require.NoError(t, db.First(&option, "key = ?", legacyFrontendThemeOptionKey).Error)
			assert.Equal(t, defaultFrontendThemeValue, option.Value)
			common.OptionMapRWMutex.RLock()
			assert.Equal(t, defaultFrontendThemeValue, common.OptionMap[legacyFrontendThemeOptionKey])
			common.OptionMapRWMutex.RUnlock()
		})
	}
}

func TestInitOptionMapContinuesWhenLegacyThemeNormalizationFails(t *testing.T) {
	originalDB := DB
	originalOptionMap := common.OptionMap
	originalIsMasterNode := common.IsMasterNode
	common.IsMasterNode = true
	legacyFrontendThemeOptionOnce = sync.Once{}
	t.Cleanup(func() {
		DB = originalDB
		common.IsMasterNode = originalIsMasterNode
		legacyFrontendThemeOptionOnce = sync.Once{}
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: legacyFrontendThemeOptionKey, Value: "classic"}).Error)
	DB = db
	legacyFrontendThemeOptionOnce = sync.Once{}

	writeErr := errors.New("read-only database")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail_theme_normalization", func(tx *gorm.DB) {
		tx.AddError(writeErr)
	}))

	InitOptionMap()

	var option Option
	require.NoError(t, db.First(&option, "key = ?", legacyFrontendThemeOptionKey).Error)
	assert.Equal(t, "classic", option.Value)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, defaultFrontendThemeValue, common.OptionMap[legacyFrontendThemeOptionKey])
	common.OptionMapRWMutex.RUnlock()
}

func TestInitOptionMapHandlesNilDatabase(t *testing.T) {
	originalDB := DB
	originalOptionMap := common.OptionMap
	originalIsMasterNode := common.IsMasterNode
	common.IsMasterNode = true
	legacyFrontendThemeOptionOnce = sync.Once{}
	t.Cleanup(func() {
		DB = originalDB
		common.IsMasterNode = originalIsMasterNode
		legacyFrontendThemeOptionOnce = sync.Once{}
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	DB = nil
	legacyFrontendThemeOptionOnce = sync.Once{}
	InitOptionMap()

	common.OptionMapRWMutex.RLock()
	assert.Equal(t, defaultFrontendThemeValue, common.OptionMap[legacyFrontendThemeOptionKey])
	common.OptionMapRWMutex.RUnlock()
}

func TestInitOptionMapSlaveSkipsLegacyThemePersistence(t *testing.T) {
	originalDB := DB
	originalOptionMap := common.OptionMap
	originalIsMasterNode := common.IsMasterNode
	common.IsMasterNode = false
	legacyFrontendThemeOptionOnce = sync.Once{}
	t.Cleanup(func() {
		DB = originalDB
		common.IsMasterNode = originalIsMasterNode
		legacyFrontendThemeOptionOnce = sync.Once{}
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: legacyFrontendThemeOptionKey, Value: "classic"}).Error)
	DB = db

	InitOptionMap()

	var option Option
	require.NoError(t, db.First(&option, "key = ?", legacyFrontendThemeOptionKey).Error)
	assert.Equal(t, "classic", option.Value)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, defaultFrontendThemeValue, common.OptionMap[legacyFrontendThemeOptionKey])
	common.OptionMapRWMutex.RUnlock()
}

func stringPtr(value string) *string {
	return &value
}
