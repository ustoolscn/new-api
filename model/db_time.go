package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// GetDBTimestamp returns a UNIX timestamp from database time.
// Falls back to application time on error.
func GetDBTimestamp() int64 {
	return GetDBTimestampTx(nil)
}

// GetDBTimestampTx is like GetDBTimestamp but reuses the provided transaction
// connection when present. This avoids SQLite single-connection deadlocks when
// callers already hold an open transaction on the only available connection.
func GetDBTimestampTx(tx *gorm.DB) int64 {
	query := DB
	if tx != nil {
		query = tx
	}
	if query == nil {
		return common.GetTimestamp()
	}

	var ts int64
	var err error
	switch {
	case common.UsingMainDatabase(common.DatabaseTypePostgreSQL):
		err = query.Raw("SELECT EXTRACT(EPOCH FROM NOW())::bigint").Scan(&ts).Error
	case common.UsingMainDatabase(common.DatabaseTypeSQLite):
		err = query.Raw("SELECT strftime('%s','now')").Scan(&ts).Error
	default:
		err = query.Raw("SELECT UNIX_TIMESTAMP()").Scan(&ts).Error
	}
	if err != nil || ts <= 0 {
		return common.GetTimestamp()
	}
	return ts
}
