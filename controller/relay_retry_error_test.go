package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"
)

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
