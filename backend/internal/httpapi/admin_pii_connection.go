package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	piiTestRateLimitPerMinute = 5
	piiTestRateLimitWindow    = time.Minute
)

// piiTestRateLimiter limits PII test-connection calls per tenant (e.g. 5/min).
type piiTestRateLimiter struct {
	mu       sync.Mutex
	byTenant map[string][]time.Time
}

func newPIITestRateLimiter() *piiTestRateLimiter {
	return &piiTestRateLimiter{byTenant: make(map[string][]time.Time)}
}

// allow returns true if the tenant is under the limit (5 per minute).
func (l *piiTestRateLimiter) allow(tenantID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-piiTestRateLimitWindow)
	times := l.byTenant[tenantID]
	// Drop entries older than 1 minute
	var kept []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= piiTestRateLimitPerMinute {
		l.byTenant[tenantID] = kept
		return false
	}
	l.byTenant[tenantID] = append(kept, now)
	return true
}

// piiTestConnectionRequest is the body for POST /admin/tenants/{tenant_id}/pii/test-connection
type piiTestConnectionRequest struct {
	RequestURL  string `json:"request_url"`
	ResponseURL string `json:"response_url"`
	TimeoutMs   int    `json:"timeout_ms"`
	APIKey      string `json:"api_key,omitempty"`
}

// piiTestEndpointResult is one of request_endpoint or response_endpoint in the response
type piiTestEndpointResult struct {
	Status     string `json:"status"` // "ok", "error", "timeout"
	StatusCode int    `json:"status_code,omitempty"`
	LatencyMs  int64  `json:"latency_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

// piiTestConnectionResponse is the 200 response body
type piiTestConnectionResponse struct {
	RequestEndpoint  piiTestEndpointResult `json:"request_endpoint"`
	ResponseEndpoint piiTestEndpointResult `json:"response_endpoint"`
}

var piiTestPayload = map[string]interface{}{
	"test":   true,
	"source": "router-admin-test",
}

// AdminPIITestConnection handles POST /admin/tenants/{tenant_id}/pii/test-connection.
// It validates the body, then POSTs a minimal test payload to request_url and response_url
// and returns latency and status. Does not store or modify any configuration.
func (h *Handlers) AdminPIITestConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.PathValue("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id is required", "invalid_request_error")
		return
	}

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	// Rate limit: 5 tests per minute per tenant
	if h.piiTestLimiter != nil && !h.piiTestLimiter.allow(tenantID) {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded: max 5 PII connection tests per minute per tenant", "rate_limit_error")
		return
	}

	var body piiTestConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error")
		return
	}

	// Validation: URLs and timeout
	if body.RequestURL == "" {
		writeError(w, http.StatusBadRequest, "request_url is required", "invalid_request_error")
		return
	}
	if body.ResponseURL == "" {
		writeError(w, http.StatusBadRequest, "response_url is required", "invalid_request_error")
		return
	}
	reqURL, err := url.Parse(body.RequestURL)
	if err != nil || reqURL.Scheme == "" || reqURL.Host == "" {
		writeError(w, http.StatusBadRequest, "request_url must be a valid URL", "invalid_request_error")
		return
	}
	respURL, err := url.Parse(body.ResponseURL)
	if err != nil || respURL.Scheme == "" || respURL.Host == "" {
		writeError(w, http.StatusBadRequest, "response_url must be a valid URL", "invalid_request_error")
		return
	}
	timeoutMs := body.TimeoutMs
	if timeoutMs < 500 || timeoutMs > 10000 {
		writeError(w, http.StatusBadRequest, "timeout_ms must be between 500 and 10000", "invalid_request_error")
		return
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	payloadBytes, _ := json.Marshal(piiTestPayload)

	// DEBUG: Log what we received
	h.log.InfoContext(ctx, "PII test connection request",
		"request_url", body.RequestURL,
		"response_url", body.ResponseURL,
		"timeout_ms", body.TimeoutMs,
		"api_key_length", len(body.APIKey),
		"api_key_empty", body.APIKey == "",
	)

	// Call request_url then response_url (no retries)
	requestResult := postAndMeasure(ctx, body.RequestURL, payloadBytes, timeout, body.APIKey)
	responseResult := postAndMeasure(ctx, body.ResponseURL, payloadBytes, timeout, body.APIKey)

	out := piiTestConnectionResponse{
		RequestEndpoint:  requestResult,
		ResponseEndpoint: responseResult,
	}

	h.log.InfoContext(ctx, "PII test connection",
		"tenant_id", tenantID,
		"request_url", body.RequestURL,
		"response_url", body.ResponseURL,
		"request_status", requestResult.Status,
		"request_latency_ms", requestResult.LatencyMs,
		"response_status", responseResult.Status,
		"response_latency_ms", responseResult.LatencyMs,
	)

	writeJSON(w, http.StatusOK, out)
}

// postAndMeasure performs a single POST and returns status, status_code, latency_ms, and optional error.
func postAndMeasure(ctx context.Context, targetURL string, body []byte, timeout time.Duration, apiKey string) piiTestEndpointResult {
	start := time.Now()
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return piiTestEndpointResult{Status: "error", Error: "invalid_request"}
	}
	req.Header.Set("Content-Type", "application/json")

	// Add API Key if provided
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, err := client.Do(req)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		var urlErr *url.Error
		if ctx.Err() != nil {
			return piiTestEndpointResult{Status: "error", LatencyMs: latencyMs, Error: "context_canceled"}
		}
		if errors.As(err, &urlErr) && urlErr.Timeout() {
			return piiTestEndpointResult{Status: "timeout", LatencyMs: latencyMs}
		}
		errStr := err.Error()
		if errStr == "" {
			errStr = "connection_refused"
		}
		return piiTestEndpointResult{Status: "error", LatencyMs: latencyMs, Error: errStr}
	}
	defer resp.Body.Close()

	statusCode := resp.StatusCode
	status := "ok"
	if statusCode < 200 || statusCode >= 300 {
		status = "error"
	}
	return piiTestEndpointResult{
		Status:     status,
		StatusCode: statusCode,
		LatencyMs:  latencyMs,
	}
}
