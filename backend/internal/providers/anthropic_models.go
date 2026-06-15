package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ClaudeModelItem is one entry from the Anthropic /v1/models response,
// normalized to only the fields we expose (SPEC_159).
type ClaudeModelItem struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name,omitempty"`
}

// FetchClaudeModels calls the Anthropic GET /v1/models endpoint and returns
// the normalized list. baseURL must NOT have a trailing slash.
// apiKey is sent as the x-api-key header; empty key is omitted.
// Returns an error on any non-200 status or network failure.
func FetchClaudeModels(ctx context.Context, baseURL, apiKey string) ([]ClaudeModelItem, error) {
	url := strings.TrimRight(baseURL, "/") + "/v1/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, truncateBody(string(body), 300))
	}

	// Anthropic /v1/models shape: {"data":[{"id":"...","type":"model","display_name":"..."}]}
	var raw struct {
		Data []struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	items := make([]ClaudeModelItem, 0, len(raw.Data))
	for _, m := range raw.Data {
		items = append(items, ClaudeModelItem{
			ID:          m.ID,
			Type:        m.Type,
			DisplayName: m.DisplayName,
		})
	}
	return items, nil
}
