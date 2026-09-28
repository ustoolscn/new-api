package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthOptionalAPIKeysWriteScope(t *testing.T) {
	for _, tc := range []struct {
		scope        string
		valid, write bool
	}{
		{OAuthPublicScope, true, false},
		{" account:read  api_keys:read ", true, false},
		{"api_keys:write account:read api_keys:read", true, true},
		{"api_keys:write api_keys:read", false, false},
		{"api_keys:write account:read", false, false},
		{"api_keys:write", false, false},
		{OAuthPublicScope + " api_keys:group:write", false, false},
		{OAuthPublicScope + " api_keys:write api_keys:write", false, false},
		{"api_keys:read api_keys:read api_keys:write", false, false},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			normalized, err := NormalizeOAuthScope(tc.scope)
			if !tc.valid {
				require.Error(t, err)
				assert.False(t, OAuthScopeAllows(tc.scope, OAuthScopeAPIKeysWrite))
				return
			}
			require.NoError(t, err)
			want := OAuthPublicScope
			if tc.write {
				want += " " + OAuthScopeAPIKeysWrite
			}
			assert.Equal(t, want, normalized)
			assert.True(t, OAuthScopeAllows(tc.scope, OAuthScopeAPIKeysRead, OAuthScopeAccountRead))
			assert.Equal(t, tc.write, OAuthScopeAllows(tc.scope, OAuthScopeAPIKeysWrite))
		})
	}
}
