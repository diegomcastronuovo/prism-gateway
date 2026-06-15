package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const testConnectionTimeout = 4 * time.Second
const testConnectionMaxRedirects = 2

// testConnectionRequest is the body for POST /admin/test-connection
type testConnectionRequest struct {
	URL  string `json:"url"`
	Type string `json:"type"` // "basic" or "jwks", default "basic"
}

// testConnectionResponse is the response for POST /admin/test-connection
type testConnectionResponse struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

// AdminTestConnection handles POST /admin/test-connection.
// Tests connectivity to an external URL (e.g. Keycloak JWKS). Used by the frontend "Test Connection" button.
func (h *Handlers) AdminTestConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	var body testConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error")
		return
	}
	body.URL = strings.TrimSpace(body.URL)
	if body.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required", "invalid_request_error")
		return
	}

	// Only allow HTTP/HTTPS
	parsed, err := url.Parse(body.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid url: "+err.Error(), "invalid_request_error")
		return
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		// allowed
	default:
		writeError(w, http.StatusBadRequest, "url must use http or https", "invalid_request_error")
		return
	}

	validationType := strings.ToLower(strings.TrimSpace(body.Type))
	if validationType == "" {
		validationType = "basic"
	}
	if validationType != "basic" && validationType != "jwks" {
		writeError(w, http.StatusBadRequest, "type must be basic or jwks", "invalid_request_error")
		return
	}

	client := &http.Client{
		Timeout: testConnectionTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= testConnectionMaxRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, body.URL, nil)
	if err != nil {
		h.log.WarnContext(ctx, "test-connection: failed to create request", "error", err, "url", body.URL)
		writeJSON(w, http.StatusOK, testConnectionResponse{OK: false, Status: 0, Error: "failed to create request"})
		return
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		h.log.WarnContext(ctx, "test-connection: request failed", "error", err, "url", body.URL)
		writeJSON(w, http.StatusOK, testConnectionResponse{OK: false, Status: 0, Error: "failed to fetch url"})
		return
	}
	defer resp.Body.Close()
	statusCode := resp.StatusCode

	if validationType == "basic" {
		ok := statusCode >= 200 && statusCode < 300
		writeJSON(w, http.StatusOK, testConnectionResponse{OK: ok, Status: statusCode})
		return
	}

	// jwks: status must be 200, body must be JSON with "keys" array of length > 0
	if statusCode != 200 {
		writeJSON(w, http.StatusOK, testConnectionResponse{OK: false, Status: statusCode, Error: "expected status 200 for jwks"})
		return
	}
	var jwks struct {
		Keys []interface{} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		h.log.WarnContext(ctx, "test-connection: invalid jwks json", "error", err, "url", body.URL)
		writeJSON(w, http.StatusOK, testConnectionResponse{OK: false, Status: statusCode, Error: "invalid json"})
		return
	}
	if jwks.Keys == nil || len(jwks.Keys) == 0 {
		writeJSON(w, http.StatusOK, testConnectionResponse{OK: false, Status: statusCode, Error: "jwks must contain non-empty keys array"})
		return
	}
	writeJSON(w, http.StatusOK, testConnectionResponse{OK: true, Status: statusCode})
}
