package system_setting

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prefixStrings(prefixes []netip.Prefix) []string {
	values := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		values = append(values, prefix.String())
	}
	return values
}

func TestCompileClientIPSettingNormalizesRules(t *testing.T) {
	snapshot, err := compileClientIPSetting(ClientIPSetting{
		BlacklistEnabled: true,
		Blacklist: []string{
			" 203.0.113.7 ",
			"203.0.113.7/32",
			"2001:db8::/48",
		},
		TrustedProxies: []string{
			"127.0.0.1",
			"10.0.0.0/8",
		},
	})

	require.NoError(t, err)
	assert.True(t, snapshot.BlacklistEnabled)
	assert.Equal(t, []string{"203.0.113.7/32", "2001:db8::/48"}, prefixStrings(snapshot.Blacklist))
	assert.Equal(t, []string{"127.0.0.1/32", "10.0.0.0/8"}, prefixStrings(snapshot.TrustedProxies))
}

func TestCompileClientIPSettingRejectsInvalidRule(t *testing.T) {
	_, err := compileClientIPSetting(ClientIPSetting{
		Blacklist: []string{"not-an-ip"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-an-ip")
}

func TestUpdateAndSyncClientIPSettingKeepsPreviousSnapshotOnInvalidRule(t *testing.T) {
	originalSetting := GetClientIPSetting()
	originalSnapshot := GetClientIPSnapshot()
	t.Cleanup(func() {
		defaultClientIPSetting = originalSetting
		clientIPSnapshot.Store(originalSnapshot)
	})

	defaultClientIPSetting = ClientIPSetting{
		BlacklistEnabled: true,
		Blacklist:        []string{"203.0.113.7"},
	}
	require.NoError(t, UpdateAndSyncClientIPSetting())
	validSnapshot := GetClientIPSnapshot()

	defaultClientIPSetting = ClientIPSetting{
		BlacklistEnabled: true,
		Blacklist:        []string{"invalid"},
	}
	require.Error(t, UpdateAndSyncClientIPSetting())

	assert.Equal(t, validSnapshot, GetClientIPSnapshot())
}

func TestDefaultClientIPSettingIsDisabled(t *testing.T) {
	setting := defaultClientIPSetting

	assert.False(t, setting.BlacklistEnabled)
	assert.Empty(t, setting.Blacklist)
	assert.Empty(t, setting.TrustedProxies)
}
