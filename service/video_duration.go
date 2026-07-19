package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	videoProbeTimeout      = 5 * time.Second
	videoProbeInitialBytes = int64(64 << 10)
	videoProbeChunkBytes   = int64(128 << 10)
	videoProbeMaxBytes     = int64(2 << 20)
	videoProbeMaxRequests  = 3
	videoProbeConcurrency  = 8
)

var videoProbeLimiter = make(chan struct{}, videoProbeConcurrency)

type videoProbeSegment struct {
	start int64
	data  []byte
}

type videoRangeProbe struct {
	ctx       context.Context
	client    *http.Client
	url       string
	totalSize int64
	requests  int
	bytesRead int64
	segments  []videoProbeSegment
}

func ProbeVideoDurationURL(ctx context.Context, rawURL string) (float64, error) {
	if err := ValidateSSRFProtectedFetchURL(rawURL); err != nil {
		return 0, fmt.Errorf("input video URL rejected: %w", err)
	}
	client := GetSSRFProtectedHTTPClient()
	if client == nil {
		client = newProtectedFetchHTTPClient()
	}
	return probeVideoDurationURLWithClient(ctx, rawURL, client)
}

func probeVideoDurationURLWithClient(ctx context.Context, rawURL string, client *http.Client) (float64, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return 0, fmt.Errorf("input video must be an HTTP or HTTPS URL")
	}

	probeCtx, cancel := context.WithTimeout(ctx, videoProbeTimeout)
	defer cancel()
	select {
	case videoProbeLimiter <- struct{}{}:
		defer func() { <-videoProbeLimiter }()
	case <-probeCtx.Done():
		return 0, fmt.Errorf("video duration probe timed out: %w", probeCtx.Err())
	}

	probe := &videoRangeProbe{ctx: probeCtx, client: client, url: rawURL}
	initial, contentType, err := probe.fetch(0, videoProbeInitialBytes)
	if err != nil {
		return 0, err
	}
	if isHLSContent(initial, contentType, parsedURL.Path) {
		return probeHLSDuration(probeCtx, client, rawURL, initial, 0)
	}
	if len(initial) >= 4 && binary.BigEndian.Uint32(initial[:4]) == 0x1a45dfa3 {
		return probeWebMDuration(initial)
	}
	return probeMP4Duration(probe, initial)
}

func (p *videoRangeProbe) fetch(start, length int64) ([]byte, string, error) {
	if length <= 0 || start < 0 {
		return nil, "", fmt.Errorf("invalid video probe range")
	}
	for _, segment := range p.segments {
		end := start + length
		segmentEnd := segment.start + int64(len(segment.data))
		if start >= segment.start && end <= segmentEnd {
			offset := start - segment.start
			return segment.data[offset : offset+length], "", nil
		}
	}
	if p.requests >= videoProbeMaxRequests {
		return nil, "", fmt.Errorf("video metadata was not found within %d range requests", videoProbeMaxRequests)
	}
	remaining := videoProbeMaxBytes - p.bytesRead
	if remaining <= 0 {
		return nil, "", fmt.Errorf("video metadata exceeds the %d byte probe limit", videoProbeMaxBytes)
	}
	if length < videoProbeChunkBytes {
		length = videoProbeChunkBytes
	}
	if length > remaining {
		length = remaining
	}
	if p.totalSize > 0 && start+length > p.totalSize {
		length = p.totalSize - start
	}
	if length <= 0 {
		return nil, "", io.ErrUnexpectedEOF
	}

	req, err := http.NewRequestWithContext(p.ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, start+length-1))
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read input video metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && !(resp.StatusCode == http.StatusOK && start == 0) {
		return nil, "", fmt.Errorf("input video server does not support the required range request: status %d", resp.StatusCode)
	}
	if total, ok := parseContentRangeTotal(resp.Header.Get("Content-Range")); ok {
		p.totalSize = total
	} else if resp.StatusCode == http.StatusOK && resp.ContentLength > 0 {
		p.totalSize = resp.ContentLength
	}

	limited := io.LimitReader(resp.Body, length+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read input video metadata: %w", err)
	}
	if int64(len(data)) > length {
		data = data[:length]
	}
	if len(data) == 0 {
		return nil, "", io.ErrUnexpectedEOF
	}
	p.requests++
	p.bytesRead += int64(len(data))
	p.segments = append(p.segments, videoProbeSegment{start: start, data: data})
	return data, resp.Header.Get("Content-Type"), nil
}

func parseContentRangeTotal(value string) (int64, bool) {
	slash := strings.LastIndex(value, "/")
	if slash < 0 || slash == len(value)-1 || value[slash+1:] == "*" {
		return 0, false
	}
	total, err := strconv.ParseInt(value[slash+1:], 10, 64)
	return total, err == nil && total > 0
}

func probeMP4Duration(probe *videoRangeProbe, initial []byte) (float64, error) {
	if len(initial) < 8 {
		return 0, fmt.Errorf("unsupported or truncated input video")
	}

	offset := int64(0)
	for boxCount := 0; boxCount < 64; boxCount++ {
		header, _, err := probe.fetch(offset, 16)
		if err != nil {
			return 0, err
		}
		size, boxType, headerSize, err := parseMP4BoxHeader(header, probe.totalSize-offset)
		if err != nil {
			return 0, err
		}
		if boxType == "moov" {
			readSize := size
			if readSize > videoProbeChunkBytes {
				readSize = videoProbeChunkBytes
			}
			moov, _, err := probe.fetch(offset, readSize)
			if err != nil {
				return 0, err
			}
			return parseMP4MovieDuration(moov, headerSize)
		}
		if size <= 0 {
			return 0, fmt.Errorf("invalid MP4 box size")
		}
		offset += size
		if probe.totalSize > 0 && offset >= probe.totalSize {
			break
		}
	}
	return 0, fmt.Errorf("MP4 movie metadata was not found within probe limits")
}

func parseMP4BoxHeader(data []byte, remaining int64) (int64, string, int64, error) {
	if len(data) < 8 {
		return 0, "", 0, io.ErrUnexpectedEOF
	}
	size := int64(binary.BigEndian.Uint32(data[:4]))
	headerSize := int64(8)
	if size == 1 {
		if len(data) < 16 {
			return 0, "", 0, io.ErrUnexpectedEOF
		}
		size = int64(binary.BigEndian.Uint64(data[8:16]))
		headerSize = 16
	} else if size == 0 {
		size = remaining
	}
	if size < headerSize {
		return 0, "", 0, fmt.Errorf("invalid MP4 box size")
	}
	return size, string(data[4:8]), headerSize, nil
}

func parseMP4MovieDuration(moov []byte, moovHeaderSize int64) (float64, error) {
	offset := moovHeaderSize
	for offset+8 <= int64(len(moov)) {
		size, boxType, headerSize, err := parseMP4BoxHeader(moov[offset:], int64(len(moov))-offset)
		if err != nil {
			return 0, err
		}
		if boxType == "mvhd" {
			payloadStart := offset + headerSize
			payloadEnd := offset + size
			if payloadEnd > int64(len(moov)) {
				return 0, fmt.Errorf("MP4 mvhd metadata exceeds probe limit")
			}
			return parseMP4MVHDDuration(moov[payloadStart:payloadEnd])
		}
		offset += size
	}
	return 0, fmt.Errorf("MP4 mvhd metadata was not found within probe limits")
}

func parseMP4MVHDDuration(payload []byte) (float64, error) {
	if len(payload) < 20 {
		return 0, io.ErrUnexpectedEOF
	}
	var timescale uint32
	var duration uint64
	if payload[0] == 1 {
		if len(payload) < 32 {
			return 0, io.ErrUnexpectedEOF
		}
		timescale = binary.BigEndian.Uint32(payload[20:24])
		duration = binary.BigEndian.Uint64(payload[24:32])
	} else {
		timescale = binary.BigEndian.Uint32(payload[12:16])
		duration = uint64(binary.BigEndian.Uint32(payload[16:20]))
	}
	if timescale == 0 || duration == 0 {
		return 0, fmt.Errorf("MP4 duration metadata is unavailable")
	}
	seconds := float64(duration) / float64(timescale)
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, fmt.Errorf("MP4 duration metadata is invalid")
	}
	return seconds, nil
}

func probeWebMDuration(data []byte) (float64, error) {
	timecodeScale := uint64(1_000_000)
	var duration float64
	for i := 0; i < len(data); i++ {
		switch {
		case i+3 < len(data) && bytes.Equal(data[i:i+3], []byte{0x2a, 0xd7, 0xb1}):
			value, consumed, ok := readEBMLElementUint(data[i+3:])
			if ok {
				timecodeScale = value
				i += 2 + consumed
			}
		case i+2 < len(data) && bytes.Equal(data[i:i+2], []byte{0x44, 0x89}):
			value, consumed, ok := readEBMLElementFloat(data[i+2:])
			if ok {
				duration = value
				i += 1 + consumed
			}
		}
	}
	seconds := duration * float64(timecodeScale) / 1_000_000_000
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, fmt.Errorf("WebM duration metadata was not found within probe limits")
	}
	return seconds, nil
}

func readEBMLVint(data []byte) (uint64, int, bool) {
	if len(data) == 0 || data[0] == 0 {
		return 0, 0, false
	}
	mask := byte(0x80)
	length := 1
	for length <= 8 && data[0]&mask == 0 {
		mask >>= 1
		length++
	}
	if length > 8 || len(data) < length {
		return 0, 0, false
	}
	value := uint64(data[0] & (mask - 1))
	for i := 1; i < length; i++ {
		value = value<<8 | uint64(data[i])
	}
	return value, length, true
}

func readEBMLElementUint(data []byte) (uint64, int, bool) {
	size, sizeLength, ok := readEBMLVint(data)
	if !ok || size == 0 || size > 8 || len(data) < sizeLength+int(size) {
		return 0, 0, false
	}
	var value uint64
	for _, b := range data[sizeLength : sizeLength+int(size)] {
		value = value<<8 | uint64(b)
	}
	return value, sizeLength + int(size), true
}

func readEBMLElementFloat(data []byte) (float64, int, bool) {
	size, sizeLength, ok := readEBMLVint(data)
	if !ok || (size != 4 && size != 8) || len(data) < sizeLength+int(size) {
		return 0, 0, false
	}
	payload := data[sizeLength : sizeLength+int(size)]
	if size == 4 {
		return float64(math.Float32frombits(binary.BigEndian.Uint32(payload))), sizeLength + 4, true
	}
	return math.Float64frombits(binary.BigEndian.Uint64(payload)), sizeLength + 8, true
}

func isHLSContent(data []byte, contentType, path string) bool {
	contentType = strings.ToLower(contentType)
	return bytes.HasPrefix(bytes.TrimSpace(data), []byte("#EXTM3U")) ||
		strings.Contains(contentType, "mpegurl") || strings.HasSuffix(strings.ToLower(path), ".m3u8")
}

func probeHLSDuration(ctx context.Context, client *http.Client, playlistURL string, data []byte, depth int) (float64, error) {
	if depth > 1 {
		return 0, fmt.Errorf("nested HLS master playlists are not supported")
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var duration float64
	var variant string
	expectVariant := false
	hasEndList := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "#EXTINF:"):
			value := strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ",")
			if comma := strings.Index(value, ","); comma >= 0 {
				value = value[:comma]
			}
			seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil || seconds < 0 {
				return 0, fmt.Errorf("invalid HLS segment duration")
			}
			duration += seconds
		case line == "#EXT-X-ENDLIST":
			hasEndList = true
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			expectVariant = true
		case expectVariant && line != "" && !strings.HasPrefix(line, "#"):
			variant = line
			expectVariant = false
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if duration > 0 {
		if !hasEndList {
			return 0, fmt.Errorf("live HLS input videos are not supported")
		}
		return duration, nil
	}
	if variant == "" {
		return 0, fmt.Errorf("HLS playlist duration was not found")
	}
	base, err := url.Parse(playlistURL)
	if err != nil {
		return 0, err
	}
	reference, err := url.Parse(variant)
	if err != nil {
		return 0, err
	}
	variantURL := base.ResolveReference(reference).String()
	if err := ValidateSSRFProtectedFetchURL(variantURL); err != nil {
		return 0, fmt.Errorf("HLS variant URL rejected: %w", err)
	}
	probe := &videoRangeProbe{ctx: ctx, client: client, url: variantURL}
	variantData, _, err := probe.fetch(0, videoProbeInitialBytes)
	if err != nil {
		return 0, err
	}
	return probeHLSDuration(ctx, client, variantURL, variantData, depth+1)
}
