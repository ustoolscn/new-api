package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type heartbeatSignalWriter struct {
	gin.ResponseWriter
	wrote chan struct{}
}

func (w *heartbeatSignalWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	select {
	case w.wrote <- struct{}{}:
	default:
	}
	return n, err
}

func TestImageJSONHeartbeatKeepsFinalResponseValid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, path, nil)
			signal := &heartbeatSignalWriter{ResponseWriter: c.Writer, wrote: make(chan struct{}, 1)}
			c.Writer = signal

			stop := startImageJSONHeartbeat(c, &relaycommon.RelayInfo{}, time.Millisecond)
			select {
			case <-signal.wrote:
			case <-time.After(time.Second):
				t.Fatal("image heartbeat was not written")
			}
			stop()
			_, err := io.WriteString(c.Writer, `{"created":1,"data":[]}`)
			require.NoError(t, err)

			var response map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
			require.Equal(t, "no", recorder.Header().Get("X-Accel-Buffering"))
		})
	}
}

func TestImageJSONHeartbeatSkipsStreamingAndOtherRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		path     string
		isStream bool
	}{
		{path: "/v1/images/generations", isStream: true},
		{path: "/v1/chat/completions", isStream: false},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, test.path, nil)
		stop := startImageJSONHeartbeat(c, &relaycommon.RelayInfo{IsStream: test.isStream}, time.Millisecond)
		time.Sleep(5 * time.Millisecond)
		stop()
		require.Empty(t, recorder.Body.String())
	}
}
