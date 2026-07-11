package common

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveClientIPIgnoresForwardedHeaderFromUntrustedPeer(t *testing.T) {
	got, err := ResolveClientIP("198.51.100.20:443", "203.0.113.9", "", nil)

	require.NoError(t, err)
	assert.Equal(t, "198.51.100.20", got.String())
}

func TestResolveClientIPWalksTrustedChainRightToLeft(t *testing.T) {
	trusted := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.0.2.0/24"),
	}

	got, err := ResolveClientIP(
		"10.0.0.5:443",
		"203.0.113.9, 192.0.2.8",
		"",
		trusted,
	)

	require.NoError(t, err)
	assert.Equal(t, "203.0.113.9", got.String())
}

func TestResolveClientIPUsesRealIPForTrustedPeerWithoutForwardedFor(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	got, err := ResolveClientIP("10.0.0.5:443", "", "203.0.113.9", trusted)

	require.NoError(t, err)
	assert.Equal(t, "203.0.113.9", got.String())
}

func TestResolveClientIPSupportsIPv6RemoteAddress(t *testing.T) {
	got, err := ResolveClientIP("[2001:db8::5]:443", "", "", nil)

	require.NoError(t, err)
	assert.Equal(t, "2001:db8::5", got.String())
}

func TestResolveClientIPRejectsMalformedForwardedEntryFromTrustedPeer(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	_, err := ResolveClientIP("10.0.0.5:443", "203.0.113.9, invalid", "", trusted)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestResolveClientIPRejectsInvalidRemoteAddress(t *testing.T) {
	_, err := ResolveClientIP("invalid", "", "", nil)

	require.Error(t, err)
}
