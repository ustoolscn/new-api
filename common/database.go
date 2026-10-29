package common

import (
	"fmt"
	"net/url"
	"strings"
)

type DatabaseType string

const (
	DatabaseTypeMySQL      DatabaseType = "mysql"
	DatabaseTypeSQLite     DatabaseType = "sqlite"
	DatabaseTypePostgreSQL DatabaseType = "postgres"
	DatabaseTypeClickHouse DatabaseType = "clickhouse"
)

var mainDatabaseType = DatabaseTypeSQLite
var logDatabaseType = DatabaseTypeSQLite

func MainDatabaseType() DatabaseType {
	return mainDatabaseType
}

func LogDatabaseType() DatabaseType {
	return logDatabaseType
}

func SetMainDatabaseType(databaseType DatabaseType) {
	mainDatabaseType = databaseType
}

func SetLogDatabaseType(databaseType DatabaseType) {
	logDatabaseType = databaseType
}

func SetDatabaseTypes(mainType DatabaseType, logType DatabaseType) {
	mainDatabaseType = mainType
	logDatabaseType = logType
}

func UsingMainDatabase(databaseType DatabaseType) bool {
	return mainDatabaseType == databaseType
}

func UsingLogDatabase(databaseType DatabaseType) bool {
	return logDatabaseType == databaseType
}

var SQLitePath = "one-api.db"

// SQLiteDSN applies concurrency defaults to both built-in and custom paths,
// including Electron's SQLITE_PATH, while preserving explicit driver options.
func SQLiteDSN(path string) (string, error) {
	filename, rawQuery, _ := strings.Cut(path, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("invalid SQLite DSN query: %w", err)
	}
	hasBusyTimeout, hasJournalMode := false, false
	for _, pragma := range query["_pragma"] {
		name := strings.ToLower(strings.TrimSpace(pragma))
		if i := strings.IndexAny(name, "=("); i >= 0 {
			name = strings.TrimSpace(name[:i])
		}
		hasBusyTimeout = hasBusyTimeout || name == "busy_timeout"
		hasJournalMode = hasJournalMode || name == "journal_mode"
	}
	if !hasBusyTimeout {
		timeout := query.Get("_busy_timeout")
		if timeout == "" {
			timeout = "30000"
		}
		query.Add("_pragma", "busy_timeout("+timeout+")")
	}
	query.Del("_busy_timeout")
	if !hasJournalMode {
		query.Add("_pragma", "journal_mode(WAL)")
	}
	if !query.Has("_txlock") {
		query.Set("_txlock", "immediate")
	}
	return filename + "?" + query.Encode(), nil
}
