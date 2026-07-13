package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionDoesNotChangeMemoryWhenDatabaseWriteFails(t *testing.T) {
	originalDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db
	t.Cleanup(func() { DB = originalDB })
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()

	const key = "test.persisted.option"
	common.OptionMapRWMutex.Lock()
	originalValue, existed := common.OptionMap[key]
	common.OptionMap[key] = "memory-old"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if existed {
			common.OptionMap[key] = originalValue
		} else {
			delete(common.OptionMap, key)
		}
	})

	require.NoError(t, db.Create(&Option{Key: key, Value: "database-old"}).Error)
	writeErr := errors.New("forced option update failure")
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail_option_update", func(tx *gorm.DB) {
		tx.AddError(writeErr)
	}))

	err = UpdateOption(key, "new-value")
	require.ErrorIs(t, err, writeErr)

	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "memory-old", common.OptionMap[key])
	common.OptionMapRWMutex.RUnlock()

	var persisted Option
	require.NoError(t, db.First(&persisted, "key = ?", key).Error)
	assert.Equal(t, "database-old", persisted.Value)
}

func TestUpdateOptionReturnsCreateErrorBeforeChangingMemory(t *testing.T) {
	originalDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db
	t.Cleanup(func() { DB = originalDB })
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()

	const key = "test.new.option"
	common.OptionMapRWMutex.Lock()
	delete(common.OptionMap, key)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, key)
		common.OptionMapRWMutex.Unlock()
	})

	writeErr := errors.New("forced option create failure")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail_option_create", func(tx *gorm.DB) {
		tx.AddError(writeErr)
	}))

	err = UpdateOption(key, "new-value")
	require.ErrorIs(t, err, writeErr)
	common.OptionMapRWMutex.RLock()
	_, exists := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	assert.False(t, exists)
}

func TestUpdateOptionRollsBackNewOptionWhenValueWriteFails(t *testing.T) {
	originalDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db
	t.Cleanup(func() { DB = originalDB })

	const key = "test.rollback.option"
	writeErr := errors.New("forced option value failure")
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail_new_option_update", func(tx *gorm.DB) {
		tx.AddError(writeErr)
	}))

	err = UpdateOption(key, "new-value")
	require.ErrorIs(t, err, writeErr)

	var count int64
	require.NoError(t, db.Model(&Option{}).Where("key = ?", key).Count(&count).Error)
	assert.Zero(t, count)
}
