package service

import (
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayClientsPreserveProxyAndIsolateTLS(t *testing.T) {
	oldHeader, oldTLS := common.RelayResponseHeaderTimeout, common.TLSInsecureSkipVerify
	oldClient, oldProtected := httpClient, ssrfProtectedHTTPClient
	t.Cleanup(func() {
		ResetProxyClientCache()
		common.RelayResponseHeaderTimeout, common.TLSInsecureSkipVerify = oldHeader, oldTLS
		httpClient, ssrfProtectedHTTPClient = oldClient, oldProtected
	})
	ResetProxyClientCache()
	common.RelayResponseHeaderTimeout = 1800
	common.TLSInsecureSkipVerify = true
	InitHttpClient()
	for _, proxyURL := range []string{"", "http://127.0.0.1:18080", "https://127.0.0.1:18443", "socks5://127.0.0.1:11080", "socks5h://127.0.0.1:11081"} {
		t.Run(proxyURL, func(t *testing.T) {
			client, err := GetHttpClientWithProxy(proxyURL)
			require.NoError(t, err)
			transport, ok := client.Transport.(*http.Transport)
			require.True(t, ok)
			assert.Equal(t, 30*time.Minute, transport.ResponseHeaderTimeout)
			require.NotNil(t, transport.TLSClientConfig)
			assert.NotSame(t, common.InsecureTLSConfig, transport.TLSClientConfig)
			transport.TLSClientConfig.ServerName = "isolated.example"
			assert.NotEqual(t, "isolated.example", common.InsecureTLSConfig.ServerName)
			if proxyURL == "http://127.0.0.1:18080" || proxyURL == "https://127.0.0.1:18443" {
				req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
				require.NoError(t, err)
				actual, err := transport.Proxy(req)
				require.NoError(t, err)
				assert.Equal(t, proxyURL, actual.String())
			} else if proxyURL != "" {
				assert.Nil(t, transport.Proxy)
				assert.NotNil(t, transport.DialContext)
			}
		})
	}
}

func TestRelayResponseHeaderTimeoutBounds(t *testing.T) {
	previous := common.RelayResponseHeaderTimeout
	t.Cleanup(func() { common.RelayResponseHeaderTimeout = previous })
	for _, tc := range []struct {
		seconds int
		want    time.Duration
	}{
		{0, 0}, {-1, 0}, {45, 45 * time.Second},
		{math.MaxInt, time.Duration(math.MaxInt64/int64(time.Second)) * time.Second},
	} {
		common.RelayResponseHeaderTimeout = tc.seconds
		transport := newRelayHTTPTransport()
		assert.Equal(t, tc.want, transport.ResponseHeaderTimeout)
	}
}
