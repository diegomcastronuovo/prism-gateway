package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	redis "github.com/redis/go-redis/v9"
)

type healthStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type tableHealthStatus struct {
	Status   string   `json:"status"`
	Missing  []string `json:"missing,omitempty"`
	Found    int      `json:"found"`
	Expected int      `json:"expected"`
	Error    string   `json:"error,omitempty"`
}

type systemHealthResponse struct {
	Gateway  healthStatus      `json:"gateway"`
	Postgres healthStatus      `json:"postgres"`
	Redis    healthStatus      `json:"redis"`
	Keycloak healthStatus      `json:"keycloak"`
	Tables   tableHealthStatus `json:"tables"`
}

// AdminSystemHealth runs connectivity and schema checks against all system dependencies.
func (h *Handlers) AdminSystemHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	resp := systemHealthResponse{
		Gateway: healthStatus{Status: "ok"},
	}

	// Postgres ping
	if err := h.store.PingDB(ctx); err != nil {
		resp.Postgres = healthStatus{Status: "error", Error: err.Error()}
	} else {
		resp.Postgres = healthStatus{Status: "ok"}
	}

	// Table existence check
	if resp.Postgres.Status == "ok" {
		resp.Tables = checkTables(ctx, h.store)
	} else {
		resp.Tables = tableHealthStatus{Status: "error", Error: "postgres unavailable"}
	}

	// Redis ping
	resp.Redis = checkRedis(ctx, h.cfg.RateLimit.Redis.Addr, h.cfg.CircuitBreaker.Redis.Addr)

	// Keycloak ping
	resp.Keycloak = checkKeycloak(ctx, h.cfg.Auth.JWT.Issuer, h.cfg.Auth.Mode)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func checkTables(ctx context.Context, store storage.Storage) tableHealthStatus {
	existing, err := store.ListTables(ctx)
	if err != nil {
		return tableHealthStatus{Status: "error", Error: err.Error()}
	}

	expected := storage.ExpectedTables()
	existingSet := make(map[string]struct{}, len(existing))
	for _, t := range existing {
		existingSet[t] = struct{}{}
	}

	var missing []string
	for _, t := range expected {
		if _, ok := existingSet[t]; !ok {
			missing = append(missing, t)
		}
	}

	status := "ok"
	if len(missing) > 0 {
		status = "error"
	}
	return tableHealthStatus{
		Status:   status,
		Missing:  missing,
		Found:    len(existing),
		Expected: len(expected),
	}
}

func checkRedis(ctx context.Context, addrs ...string) healthStatus {
	addr := ""
	for _, a := range addrs {
		if a != "" {
			addr = a
			break
		}
	}
	if addr == "" {
		return healthStatus{Status: "disabled"}
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:        addr,
		DialTimeout: 3 * time.Second,
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return healthStatus{Status: "error", Error: err.Error()}
	}
	return healthStatus{Status: "ok"}
}

func checkKeycloak(ctx context.Context, issuer, authMode string) healthStatus {
	if authMode != "jwt" && authMode != "both" {
		return healthStatus{Status: "disabled"}
	}
	if issuer == "" {
		return healthStatus{Status: "error", Error: "issuer not configured"}
	}

	// localhost/127.0.0.1 URLs are browser-facing and unreachable from inside the container.
	// In K8s, configure the issuer with the internal service DNS instead.
	if strings.Contains(issuer, "localhost") || strings.Contains(issuer, "127.0.0.1") {
		return healthStatus{Status: "disabled", Error: "issuer is a localhost URL — not reachable from container; use internal service URL in K8s"}
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer, nil)
	if err != nil {
		return healthStatus{Status: "error", Error: err.Error()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return healthStatus{Status: "error", Error: err.Error()}
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return healthStatus{Status: "error", Error: http.StatusText(resp.StatusCode)}
	}
	return healthStatus{Status: "ok"}
}
