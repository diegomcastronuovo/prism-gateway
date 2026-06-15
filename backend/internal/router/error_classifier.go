package router

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// ErrorType represents classified error categories
type ErrorType string

const (
	ErrorTypeTimeout       ErrorType = "timeout"
	ErrorTypeRateLimited   ErrorType = "rate_limited"
	ErrorTypeUpstream5xx   ErrorType = "upstream_5xx"
	ErrorTypeNetwork       ErrorType = "network"
	ErrorTypeAuth          ErrorType = "auth"
	ErrorTypeInvalidRequest ErrorType = "invalid_request"
	ErrorTypeUnknown       ErrorType = "unknown"
)

// ErrorClassifier classifies errors into retryable categories
type ErrorClassifier struct{}

// NewErrorClassifier creates an error classifier
func NewErrorClassifier() *ErrorClassifier {
	return &ErrorClassifier{}
}

// Classify determines the error type from an error
func (ec *ErrorClassifier) Classify(err error) ErrorType {
	if err == nil {
		return ErrorTypeUnknown
	}

	// Check for context deadline exceeded (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorTypeTimeout
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrorTypeTimeout
		}
		return ErrorTypeNetwork
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return ErrorTypeNetwork
	}

	// Check for UpstreamError (provider-specific errors)
	var upstreamErr *providers.UpstreamError
	if errors.As(err, &upstreamErr) {
		statusCode := upstreamErr.StatusCode

		switch {
		case statusCode == 429:
			return ErrorTypeRateLimited
		case statusCode == 401 || statusCode == 403:
			return ErrorTypeAuth
		case statusCode == 400:
			return ErrorTypeInvalidRequest
		case statusCode >= 500 && statusCode < 600:
			return ErrorTypeUpstream5xx
		}
	}

	// Fallback: check error message for keywords
	errMsg := strings.ToLower(err.Error())

	if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded") {
		return ErrorTypeTimeout
	}

	if strings.Contains(errMsg, "429") || strings.Contains(errMsg, "rate limit") {
		return ErrorTypeRateLimited
	}

	if strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "no such host") {
		return ErrorTypeNetwork
	}

	if strings.Contains(errMsg, "401") || strings.Contains(errMsg, "403") || strings.Contains(errMsg, "unauthorized") {
		return ErrorTypeAuth
	}

	if strings.Contains(errMsg, "400") || strings.Contains(errMsg, "bad request") {
		return ErrorTypeInvalidRequest
	}

	if strings.Contains(errMsg, "500") || strings.Contains(errMsg, "502") || strings.Contains(errMsg, "503") {
		return ErrorTypeUpstream5xx
	}

	return ErrorTypeUnknown
}

// IsRetryable determines if an error type should trigger fallback retry
func (ec *ErrorClassifier) IsRetryable(errType ErrorType) bool {
	switch errType {
	case ErrorTypeTimeout, ErrorTypeRateLimited, ErrorTypeUpstream5xx, ErrorTypeNetwork, ErrorTypeUnknown:
		return true
	case ErrorTypeAuth, ErrorTypeInvalidRequest:
		return false
	default:
		return false
	}
}
