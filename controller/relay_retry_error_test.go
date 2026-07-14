package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryStopsAfterClientCancellation(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	requestCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil).WithContext(requestCtx)
	cancel()

	err := types.NewOpenAIError(errors.New("upstream unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	require.False(t, shouldRetry(c, err, 2))
}

func TestKeepLastRelayErrorOnChannelExhausted(t *testing.T) {
	lastErr := types.NewOpenAIError(errors.New("upstream real error"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	channelErr := types.NewError(errors.New("group has no available channel"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())

	got := keepLastRelayErrorOnChannelExhausted(lastErr, channelErr)
	if got != lastErr {
		t.Fatalf("expected last relay error to be preserved, got %v", got)
	}
}

func TestKeepLastRelayErrorOnChannelExhaustedReturnsChannelErrorWithoutLastError(t *testing.T) {
	channelErr := types.NewError(errors.New("group has no available channel"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())

	got := keepLastRelayErrorOnChannelExhausted(nil, channelErr)
	if got != channelErr {
		t.Fatalf("expected channel error when no previous relay error exists, got %v", got)
	}
}
