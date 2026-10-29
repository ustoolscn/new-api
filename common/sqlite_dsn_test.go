package common

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteDSNPreservesExplicitOptions(t *testing.T) {
	for _, path := range []string{
		`C:\data\desktop.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(DELETE)&_txlock=deferred&cache=shared`,
		`file:custom.db?_pragma=busy_timeout=5000&_pragma=journal_mode=DELETE&_txlock=deferred&cache=shared`,
	} {
		dsn, err := SQLiteDSN(path)
		require.NoError(t, err)
		filename, queryText, _ := strings.Cut(dsn, "?")
		original, originalQuery, _ := strings.Cut(path, "?")
		assert.Equal(t, original, filename)
		query, err := url.ParseQuery(queryText)
		require.NoError(t, err)
		want, err := url.ParseQuery(originalQuery)
		require.NoError(t, err)
		assert.Equal(t, want, query)
	}
	dsn, err := SQLiteDSN("custom.db?_busy_timeout=1234")
	require.NoError(t, err)
	_, queryText, _ := strings.Cut(dsn, "?")
	query, err := url.ParseQuery(queryText)
	require.NoError(t, err)
	assert.Contains(t, query["_pragma"], "busy_timeout(1234)")
	assert.False(t, query.Has("_busy_timeout"))
	_, err = SQLiteDSN("custom.db?broken=%zz")
	require.Error(t, err)
}
