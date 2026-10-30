package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamAvailabilityClassification(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		code    string
		kind    string
		message string
		want    bool
	}{
		{"invalid format", 400, "", "", "malformed input", false},
		{"validation", 422, "", "", "invalid messages", false},
		{"oversized body", 413, "", "", "too large", false},
		{"context limit with nonstandard status", 500, "context_length_exceeded", "", "", false},
		{"Claude invalid request", 400, "", "invalid_request_error", "", false},
		{"Gemini safety", 500, "prompt_blocked", "", "", false},
		{"OpenAI safety", 403, "content_policy_violation", "", "", false},
		{"Azure safety", 403, "", "", "response rejected by content management policy", false},
		{"Grok safety", 500, "", "", "Failed check: SAFETY_CHECK_TYPE", false},
		{"moderation service outage", 503, "", "", "safety service unavailable", true},
		{"upstream internal error", 500, "server_error", "", "", true},
		{"upstream timeout", 504, "", "", "", true},
		{"upstream limit", 429, "", "", "", true},
		{"upstream credentials", 401, "invalid_api_key", "invalid_request_error", "", true},
		{"upstream permissions", 403, "permission_denied", "", "", true},
		{"upstream balance with bad request status", 400, "insufficient_quota", "invalid_request_error", "", true},
		{"broad type does not hide server failure", 502, "", "invalid_request_error", "", true},
		{"broad code does not hide auth failure", 401, "invalid_request", "", "", true},
		{"precise validation with nonstandard forbidden", 403, "context_length_exceeded", "", "", false},
		{"policy service outage", 503, "", "", "content management policy service unavailable", true},
		{"server failure quoting policy text", 500, "server_error", "", "could not load rejected by moderation records", true},
		{"stream image moderation", 502, "content_moderation_failed", "upstream_error", "", false},
		{"DashScope moderation", 403, "DataInspectionFailed", "", "", false},
		{"Doubao moderation", 403, "InputTextSensitiveMaterial", "", "", false},
		{"embedded validation error in 200", 200, "", "invalid_request_error", "", false},
		{"empty response remains failure", 500, "empty_response", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := types.WithOpenAIError(types.OpenAIError{Code: tt.code, Type: tt.kind, Message: tt.message}, tt.status)
			assert.Equal(t, tt.want, IsUpstreamAvailabilityError(err))
		})
	}
	assert.False(t, IsUpstreamAvailabilityError(nil))
	for _, code := range []types.ErrorCode{types.ErrorCodeInvalidRequest, types.ErrorCodeSensitiveWordsDetected,
		types.ErrorCodeReadRequestBodyFailed, types.ErrorCodeConvertRequestFailed,
		types.ErrorCodeInsufficientUserQuota, types.ErrorCodeGetChannelFailed} {
		assert.False(t, IsUpstreamAvailabilityError(types.NewError(errors.New("local failure"), code)), string(code))
	}
	assert.False(t, IsUpstreamAvailabilityError(types.NewOpenAIError(errors.New("invalid input"), types.ErrorCodeConvertRequestFailed, 500)))
	assert.False(t, IsUpstreamAvailabilityError(types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed)))
	assert.True(t, IsUpstreamAvailabilityError(types.NewError(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed)))
	assert.False(t, IsUpstreamAvailabilityError(types.NewOpenAIError(context.Canceled, types.ErrorCodeDoRequestFailed, 500)))
	assert.True(t, IsUpstreamAvailabilityError(types.NewOpenAIError(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed, 500)))
	maskedCancel := types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed, types.ErrOptionWithHideErrMsg("upstream error: do request failed"))
	require.EqualError(t, maskedCancel, "upstream error: do request failed")
	assert.False(t, IsUpstreamAvailabilityError(maskedCancel))
	maskedTimeout := types.NewError(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed, types.ErrOptionWithHideErrMsg("upstream error: do request failed"))
	assert.True(t, IsUpstreamAvailabilityError(maskedTimeout))
}

func TestAvailabilityClassifiesWrappedAWSErrors(t *testing.T) {
	for _, tt := range []struct {
		code string
		want bool
	}{
		{"ValidationException", false},
		{"ThrottlingException", true},
		{"ServiceQuotaExceededException", true},
		{"ModelNotReadyException", true},
	} {
		t.Run(tt.code, func(t *testing.T) {
			cause := &smithy.GenericAPIError{Code: tt.code, Message: "provider response"}
			err := types.NewOpenAIError(cause, types.ErrorCodeAwsInvokeError, 400)
			require.ErrorIs(t, err, cause)
			assert.Equal(t, tt.want, IsUpstreamAvailabilityError(err))
		})
	}
}

func TestAvailabilityFromUpstreamResponse(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"Gemini validation", 400, `{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"invalid contents"}}`, false},
		{"Gemini capacity", 400, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"capacity exhausted"}}`, true},
		{"Claude outage", 529, `{"error":{"type":"overloaded_error","message":"overloaded"}}`, true},
		{"Azure policy", 403, `{"error":{"code":"content_filter","message":"blocked"}}`, false},
		{"plain gateway failure", 502, `<html>Bad Gateway</html>`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, showBody := range []bool{false, true} {
				response := &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}
				err := RelayErrorHandler(context.Background(), response, showBody)
				require.NotNil(t, err)
				assert.Equal(t, tt.want, IsUpstreamAvailabilityError(err))
				ResetStatusCode(err, `{"400":500,"403":500,"502":400,"529":400}`)
				assert.Equal(t, tt.want, IsUpstreamAvailabilityError(err))
			}
		})
	}
}

func TestAvailabilityUsesStatusBeforeChannelMapping(t *testing.T) {
	for _, tt := range []struct {
		status  int
		mapping string
		want    bool
	}{
		{400, `{"400":500}`, false},
		{503, `{"503":400}`, true},
	} {
		err := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, tt.status)
		ResetStatusCode(err, tt.mapping)
		require.NotEqual(t, tt.status, err.StatusCode)
		assert.Equal(t, tt.want, IsUpstreamAvailabilityError(err))
	}
}
