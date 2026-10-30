package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/types"
)

// IsUpstreamAvailabilityError excludes client mistakes and policy rejections from
// availability samples entirely: they are neither successful nor failed service calls.
// This classification must not control retries, error logs, or billing.
func IsUpstreamAvailabilityError(err *types.NewAPIError) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if IsViolationFeeCode(err.GetErrorCode()) || HasCSAMViolationMarker(err) {
		return false
	}
	// Local validation can also be wrapped in an OpenAI-compatible envelope.
	switch err.GetErrorCode() {
	case types.ErrorCodeReadRequestBodyFailed,
		types.ErrorCodeConvertRequestFailed, types.ErrorCodeBadRequestBody,
		types.ErrorCodeAccessDenied, types.ErrorCodeCountTokenFailed,
		types.ErrorCodeInsufficientUserQuota, types.ErrorCodePreConsumeTokenQuotaFailed:
		return false
	}
	code := strings.ToLower(string(err.GetErrorCode()))
	errorType := strings.ToLower(err.ToOpenAIError().Type)
	// AWS SDK wrappers retain typed service errors even when the envelope uses
	// the generic aws_invoke_error code (including throttling returned as 400).
	var providerError interface{ ErrorCode() string }
	if errors.As(err, &providerError) {
		code = strings.ToLower(providerError.ErrorCode())
	}
	errorKinds := []string{code, errorType, strings.ToLower(err.UpstreamErrorType)}
	for _, value := range errorKinds {
		switch value {
		case "sensitive_words_detected", "prompt_blocked", "content_filter", "content_filter_error",
			"content_policy", "content_policy_violation", "moderation_blocked", "safety_violation",
			"responsibleaipolicyviolation", "data_inspection_failed", "datainspectionfailed",
			"inappropriate_content", "content_moderation_failed", "contentfilter",
			"inputtextsensitivematerial", "outputtextsensitivematerial",
			"inputimagesensitivematerial", "outputimagesensitivematerial":
			return false
		}
	}
	// Explicit upstream failure codes take precedence over broad request types
	// and message snippets, which can quote rejected input or policy text.
	for _, value := range errorKinds {
		switch value {
		case "invalid_api_key", "authentication_error", "permission_denied", "permission_error",
			"insufficient_quota", "rate_limit_exceeded", "rate_limit_error", "overloaded_error",
			"server_error", "api_error", "internal_error", "service_unavailable",
			"resource_exhausted", "unauthenticated", "internal", "unavailable", "deadline_exceeded",
			"throttlingexception", "servicequotaexceededexception", "modelnotreadyexception",
			"modeltimeoutexception", "internalserverexception", "serviceunavailableexception",
			"accessdeniedexception", "unrecognizedclientexception":
			return true
		}
	}
	// Some compatible providers return only a message for policy rejection.
	// Match explicit rejection phrases, never generic words such as "safety".
	message := strings.ToLower(err.ToOpenAIError().Message)
	for _, marker := range []string{"rejected by content management policy", "violates our content policy",
		"rejected by moderation", "blocked by safety",
		"因安全原因被拦截", "内容审核不通过", "内容审核未通过"} {
		if strings.Contains(message, marker) {
			return false
		}
	}
	// Local failures are not evidence of an upstream outage. Transport and
	// response failures are locally wrapped but do describe upstream health.
	if err.GetErrorType() == types.ErrorTypeNewAPIError {
		switch err.GetErrorCode() {
		case types.ErrorCodeDoRequestFailed, types.ErrorCodeReadResponseBodyFailed,
			types.ErrorCodeBadResponseStatusCode, types.ErrorCodeBadResponse,
			types.ErrorCodeBadResponseBody, types.ErrorCodeEmptyResponse,
			types.ErrorCodeAwsInvokeError, types.ErrorCodeChannelInvalidKey,
			types.ErrorCodeChannelResponseTimeExceeded, types.ErrorCodeChannelAwsClientError:
		default:
			return false
		}
	}
	status := err.OriginalStatusCode
	if status == 0 {
		status = err.StatusCode
	}
	// Specific validation codes remain client errors even behind a provider's
	// nonstandard status. A generic invalid_request_error alone is not enough
	// to discard a 5xx, authentication or capacity failure.
	for _, value := range errorKinds {
		switch value {
		case "invalid_argument", "validation_error", "validationexception",
			"context_length_exceeded", "invalid_prompt", "invalid_parameter", "invalid_value":
			return false
		}
	}
	// Upstream credentials, permissions and capacity are channel problems,
	// even when a provider labels them broadly as invalid_request_error.
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	}
	if status >= 500 {
		return true
	}
	if status < 400 {
		for _, value := range errorKinds {
			if value == "invalid_request" || value == "invalid_request_error" {
				return false
			}
		}
	}
	switch status {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge,
		http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		return false
	}
	return true
}
