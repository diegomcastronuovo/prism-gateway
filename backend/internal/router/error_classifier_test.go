package router

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

func TestErrorClassifier(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name           string
		err            error
		expectedType   ErrorType
		expectedRetry  bool
	}{
		{
			name:          "context deadline exceeded",
			err:           context.DeadlineExceeded,
			expectedType:  ErrorTypeTimeout,
			expectedRetry: true,
		},
		{
			name:          "HTTP 429 rate limit",
			err:           &providers.UpstreamError{StatusCode: 429, Body: "Rate limit exceeded"},
			expectedType:  ErrorTypeRateLimited,
			expectedRetry: true,
		},
		{
			name:          "HTTP 500 upstream error",
			err:           &providers.UpstreamError{StatusCode: 500, Body: "Internal server error"},
			expectedType:  ErrorTypeUpstream5xx,
			expectedRetry: true,
		},
		{
			name:          "HTTP 502 bad gateway",
			err:           &providers.UpstreamError{StatusCode: 502, Body: "Bad gateway"},
			expectedType:  ErrorTypeUpstream5xx,
			expectedRetry: true,
		},
		{
			name:          "HTTP 503 service unavailable",
			err:           &providers.UpstreamError{StatusCode: 503, Body: "Service unavailable"},
			expectedType:  ErrorTypeUpstream5xx,
			expectedRetry: true,
		},
		{
			name:          "HTTP 401 unauthorized",
			err:           &providers.UpstreamError{StatusCode: 401, Body: "Unauthorized"},
			expectedType:  ErrorTypeAuth,
			expectedRetry: false,
		},
		{
			name:          "HTTP 403 forbidden",
			err:           &providers.UpstreamError{StatusCode: 403, Body: "Forbidden"},
			expectedType:  ErrorTypeAuth,
			expectedRetry: false,
		},
		{
			name:          "HTTP 400 bad request",
			err:           &providers.UpstreamError{StatusCode: 400, Body: "Bad request"},
			expectedType:  ErrorTypeInvalidRequest,
			expectedRetry: false,
		},
		{
			name:          "network timeout error",
			err:           &timeoutError{},
			expectedType:  ErrorTypeTimeout,
			expectedRetry: true,
		},
		{
			name:          "DNS error",
			err:           &net.DNSError{Err: "no such host", Name: "invalid.example.com"},
			expectedType:  ErrorTypeNetwork,
			expectedRetry: true,
		},
		{
			name:          "connection refused (string match)",
			err:           errors.New("dial tcp: connection refused"),
			expectedType:  ErrorTypeNetwork,
			expectedRetry: true,
		},
		{
			name:          "timeout in error message",
			err:           errors.New("request timeout after 30s"),
			expectedType:  ErrorTypeTimeout,
			expectedRetry: true,
		},
		{
			name:          "unknown error",
			err:           errors.New("something went wrong"),
			expectedType:  ErrorTypeUnknown,
			expectedRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errType := classifier.Classify(tt.err)
			if errType != tt.expectedType {
				t.Errorf("Classify() = %v, want %v", errType, tt.expectedType)
			}

			retryable := classifier.IsRetryable(errType)
			if retryable != tt.expectedRetry {
				t.Errorf("IsRetryable(%v) = %v, want %v", errType, retryable, tt.expectedRetry)
			}
		})
	}
}

// Mock timeout error for testing
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
