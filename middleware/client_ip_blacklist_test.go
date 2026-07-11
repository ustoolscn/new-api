package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestClientIPBlacklistBlocksBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	router := gin.New()
	router.Use(ClientIPBlacklistWithSnapshot(func() system_setting.ClientIPSnapshot {
		return system_setting.ClientIPSnapshot{
			BlacklistEnabled: true,
			Blacklist:        []netip.Prefix{netip.MustParsePrefix("203.0.113.7/32")},
		}
	}))
	router.GET("/login", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	recorder := performClientIPRequest(router, "/login", "203.0.113.7:50000", "")

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.False(t, called)
	assert.Contains(t, recorder.Body.String(), "client_ip_blocked")
}

func TestClientIPBlacklistAllowsWhenDisabled(t *testing.T) {
	router := newClientIPTestRouter(system_setting.ClientIPSnapshot{
		BlacklistEnabled: false,
		Blacklist:        []netip.Prefix{netip.MustParsePrefix("203.0.113.7/32")},
	})

	recorder := performClientIPRequest(router, "/login", "203.0.113.7:50000", "")

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestClientIPBlacklistBlocksTrustedForwardedClient(t *testing.T) {
	router := newClientIPTestRouter(system_setting.ClientIPSnapshot{
		BlacklistEnabled: true,
		Blacklist:        []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")},
		TrustedProxies:   []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
	})

	recorder := performClientIPRequest(router, "/", "10.0.0.5:50000", "203.0.113.7")

	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestClientIPBlacklistIgnoresForwardedClientFromUntrustedPeer(t *testing.T) {
	router := newClientIPTestRouter(system_setting.ClientIPSnapshot{
		BlacklistEnabled: true,
		Blacklist:        []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")},
		TrustedProxies:   []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
	})

	recorder := performClientIPRequest(router, "/", "198.51.100.5:50000", "203.0.113.7")

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestClientIPBlacklistExemptsOnlyStatusEndpoint(t *testing.T) {
	router := newClientIPTestRouter(system_setting.ClientIPSnapshot{
		BlacklistEnabled: true,
		Blacklist:        []netip.Prefix{netip.MustParsePrefix("203.0.113.7/32")},
	})

	statusRecorder := performClientIPRequest(router, "/api/status", "203.0.113.7:50000", "")
	uptimeRecorder := performClientIPRequest(router, "/api/uptime/status", "203.0.113.7:50000", "")

	assert.Equal(t, http.StatusOK, statusRecorder.Code)
	assert.Equal(t, http.StatusForbidden, uptimeRecorder.Code)
}

func newClientIPTestRouter(snapshot system_setting.ClientIPSnapshot) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ClientIPBlacklistWithSnapshot(func() system_setting.ClientIPSnapshot {
		return snapshot
	}))
	router.GET("/*path", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router
}

func performClientIPRequest(router http.Handler, path, remoteAddr, forwardedFor string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}
