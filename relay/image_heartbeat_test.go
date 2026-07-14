package relay

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

			stop := startImageJSONHeartbeat(c, &relaycommon.RelayInfo{}, time.Millisecond, func() {})
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
		stop := startImageJSONHeartbeat(c, &relaycommon.RelayInfo{IsStream: test.isStream}, time.Millisecond, func() {})
		time.Sleep(5 * time.Millisecond)
		stop()
		require.Empty(t, recorder.Body.String())
	}
}

func TestImageHeartbeatContinuesUntilResponseBodyEnds(t *testing.T) {
	var stopped atomic.Int32
	body := &imageHeartbeatReadCloser{
		ReadCloser: io.NopCloser(strings.NewReader("image-data")),
		stop: func() {
			stopped.Add(1)
		},
	}

	buffer := make([]byte, 1)
	n, err := body.Read(buffer)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Zero(t, stopped.Load(), "the first upstream body byte must not stop heartbeats")

	_, err = io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, int32(1), stopped.Load(), "EOF should stop heartbeats exactly once")
}

type failingHeartbeatWriter struct {
	gin.ResponseWriter
}

func (w *failingHeartbeatWriter) Write([]byte) (int, error) {
	return 0, errors.New("client disconnected")
}

func TestImageHeartbeatWriteFailureCancelsUpstream(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Writer = &failingHeartbeatWriter{ResponseWriter: c.Writer}

	upstreamCanceled := make(chan struct{})
	stop := startImageJSONHeartbeat(c, &relaycommon.RelayInfo{}, time.Millisecond, func() {
		close(upstreamCanceled)
	})
	defer stop()

	select {
	case <-upstreamCanceled:
	case <-time.After(time.Second):
		t.Fatal("heartbeat write failure did not cancel the upstream request")
	}
}
