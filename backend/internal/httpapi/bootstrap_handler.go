package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// BootstrapAPIKeyRequest is the request to create the first admin API key
type BootstrapAPIKeyRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

// BootstrapAPIKeyResponse is the response with the plaintext key (only shown once)
type BootstrapAPIKeyResponse struct {
	ID      string   `json:"id"`
	Prefix  string   `json:"prefix"`
	Key     string   `json:"key"`
	Scopes  []string `json:"scopes"`
	Message string   `json:"message"`
}

// BootstrapAPIKeyHandler creates the first admin API key.
// This endpoint is only available when api_keys table is empty (fail-safe).
// It's protected by X-Bootstrap-Key environment variable to prevent unauthorized access.
// The plaintext key is ONLY returned once on creation.
func BootstrapAPIKeyHandler(store storage.Storage, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// 1. Validate X-Bootstrap-Key header
		bootstrapKey := r.Header.Get("X-Bootstrap-Key")
		expectedKey := os.Getenv("BOOTSTRAP_KEY")

		if bootstrapKey == "" || expectedKey == "" || bootstrapKey != expectedKey {
			log.WarnContext(ctx, "bootstrap: invalid bootstrap key provided")
			writeJSONError(w, http.StatusUnauthorized, "invalid bootstrap key", "authentication_error")
			return
		}

		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request")
			return
		}

		// 2. Check if api_keys table is empty (first time only)
		count, err := store.CountAPIKeys(ctx)
		if err != nil {
			log.ErrorContext(ctx, "bootstrap: failed to count existing api keys", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "internal error", "internal_error")
			return
		}

		if count > 0 {
			log.WarnContext(ctx, "bootstrap: attempted when api keys already exist", "existing_count", count)
			writeJSONError(w, http.StatusForbidden, "bootstrap already completed (api keys already exist)", "forbidden")
			return
		}

		// 3. Parse request
		var req BootstrapAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
			return
		}

		// Set defaults
		if req.Name == "" {
			req.Name = "Bootstrap Admin Key"
		}
		if len(req.Scopes) == 0 {
			req.Scopes = []string{"admin_write"}
		}

		// Validate scopes
		validScopes := map[string]bool{"admin_read": true, "admin_write": true}
		for _, scope := range req.Scopes {
			if !validScopes[scope] {
				writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid scope: %s (valid: admin_read, admin_write)", scope), "invalid_request")
				return
			}
		}

		// 4. Create API key via storage
		// Use a special tenant ID for bootstrap keys
		tenantID := "system:bootstrap"
		result, err := store.CreateAPIKey(
			ctx,
			tenantID,
			req.Name,
			req.Scopes,
			nil, // no expiration
			"bootstrap",
			[]string{"system"},
		)
		if err != nil {
			log.ErrorContext(ctx, "bootstrap: failed to create api key", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to create api key", "internal_error")
			return
		}

		// 5. Return response with plaintext key (only shown once)
		response := BootstrapAPIKeyResponse{
			ID:      result.ID.String(),
			Prefix:  result.Prefix,
			Key:     result.Key,
			Scopes:  result.Scopes,
			Message: "✅ API key created successfully. Store it securely - it will never be shown again.",
		}

		log.InfoContext(ctx, "bootstrap: api key created",
			"key_prefix", result.Prefix,
			"scopes", result.Scopes,
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}
}
