package service

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mp4TestBox(boxType string, payload []byte) []byte {
	box := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(box[:4], uint32(len(box)))
	copy(box[4:8], boxType)
	copy(box[8:], payload)
	return box
}

func mp4TestMovie(duration, timescale uint32) []byte {
	mvhd := make([]byte, 20)
	binary.BigEndian.PutUint32(mvhd[12:16], timescale)
	binary.BigEndian.PutUint32(mvhd[16:20], duration)
	return mp4TestBox("moov", mp4TestBox("mvhd", mvhd))
}

func newRangeVideoServer(t *testing.T, data []byte, contentType string) (*httptest.Server, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	requests := &atomic.Int64{}
	bytesServed := &atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		rangeHeader := r.Header.Get("Range")
		if !strings.HasPrefix(rangeHeader, "bytes=") {
			http.Error(w, "missing range", http.StatusBadRequest)
			return
		}
		parts := strings.Split(strings.TrimPrefix(rangeHeader, "bytes="), "-")
		if len(parts) != 2 {
			http.Error(w, "invalid range", http.StatusBadRequest)
			return
		}
		start, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			http.Error(w, "invalid range start", http.StatusBadRequest)
			return
		}
		end, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			http.Error(w, "invalid range end", http.StatusBadRequest)
			return
		}
		if start >= int64(len(data)) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if end >= int64(len(data)) {
			end = int64(len(data)) - 1
		}
		chunk := data[start : end+1]
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(chunk)
		bytesServed.Add(int64(len(chunk)))
	}))
	t.Cleanup(server.Close)
	return server, requests, bytesServed
}

func TestProbeVideoDurationURLReadsMP4MovieMetadataAtFileEnd(t *testing.T) {
	ftyp := mp4TestBox("ftyp", []byte("isom\x00\x00\x00\x00isom"))
	mdat := mp4TestBox("mdat", make([]byte, 512<<10))
	data := append(append(ftyp, mdat...), mp4TestMovie(12_345, 1_000)...)
	server, requests, bytesServed := newRangeVideoServer(t, data, "video/mp4")

	duration, err := probeVideoDurationURLWithClient(context.Background(), server.URL+"/input.mp4", server.Client())

	require.NoError(t, err)
	assert.InDelta(t, 12.345, duration, 0.0001)
	assert.LessOrEqual(t, requests.Load(), int64(2))
	assert.Less(t, bytesServed.Load(), int64(300<<10))
}

func TestProbeVideoDurationURLReadsWebMMetadata(t *testing.T) {
	data := []byte{0x1a, 0x45, 0xdf, 0xa3, 0x80, 0x2a, 0xd7, 0xb1, 0x83, 0x0f, 0x42, 0x40, 0x44, 0x89, 0x88}
	durationBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(durationBytes, math.Float64bits(7_250))
	data = append(data, durationBytes...)
	server, _, _ := newRangeVideoServer(t, data, "video/webm")

	duration, err := probeVideoDurationURLWithClient(context.Background(), server.URL+"/input.webm", server.Client())

	require.NoError(t, err)
	assert.InDelta(t, 7.25, duration, 0.0001)
}

func TestProbeVideoDurationURLReadsVODHLSPlaylist(t *testing.T) {
	playlist := []byte("#EXTM3U\n#EXT-X-TARGETDURATION:5\n#EXTINF:4.25,\na.ts\n#EXTINF:5.5,\nb.ts\n#EXT-X-ENDLIST\n")
	server, requests, bytesServed := newRangeVideoServer(t, playlist, "application/vnd.apple.mpegurl")

	duration, err := probeVideoDurationURLWithClient(context.Background(), server.URL+"/input.m3u8", server.Client())

	require.NoError(t, err)
	assert.InDelta(t, 9.75, duration, 0.0001)
	assert.Equal(t, int64(1), requests.Load())
	assert.Equal(t, int64(len(playlist)), bytesServed.Load())
}

func TestProbeVideoDurationURLRejectsNonHTTPResource(t *testing.T) {
	_, err := probeVideoDurationURLWithClient(context.Background(), "data:video/mp4;base64,AAAA", http.DefaultClient)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP or HTTPS URL")
}
