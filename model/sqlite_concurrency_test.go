package model

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteCustomPathConcurrencyDefaults(t *testing.T) {
	oldPath := common.SQLitePath
	t.Cleanup(func() { common.SQLitePath = oldPath })
	t.Setenv("SQL_DSN", "local")
	common.SQLitePath = filepath.Join(t.TempDir(), "desktop.db")
	db, kind, err := chooseDB("SQL_DSN", false)
	require.NoError(t, err)
	assert.Equal(t, common.DatabaseTypeSQLite, kind)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	// Verify driver behavior on two separate pooled connections, not DSN text.
	ctx := context.Background()
	first, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	defer first.Close()
	second, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	defer second.Close()
	for _, conn := range []*sql.Conn{first, second} {
		var mode string
		var timeout int
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode))
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout))
		assert.Equal(t, "wal", mode)
		assert.Equal(t, 30000, timeout)
	}
	_, err = second.ExecContext(ctx, "PRAGMA busy_timeout=0")
	require.NoError(t, err)
	tx, err := first.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = second.ExecContext(ctx, "BEGIN IMMEDIATE")
	require.Error(t, err, "the first transaction must acquire the writer lock immediately")
	assert.Contains(t, err.Error(), "locked")
	require.NoError(t, tx.Rollback())
	_, err = second.ExecContext(ctx, "BEGIN IMMEDIATE")
	require.NoError(t, err)
	_, err = second.ExecContext(ctx, "ROLLBACK")
	require.NoError(t, err)
}
