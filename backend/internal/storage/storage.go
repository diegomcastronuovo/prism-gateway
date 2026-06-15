package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/google/uuid"
)

// RequestLog represents one row in the request_log table.
type RequestLog struct {
	ID                         uuid.UUID
	RequestID                  string // Stable per-request identifier (groups attempts)
	Attempt                    int    // 1-based attempt number
	Timestamp                  time.Time
	TenantID                   string
	Model                      string
	Provider                   string
	Strategy                   string
	Status                     string // "ok" or "error"
	LatencyMs                  int
	Error                      string
	FallbackUsed               bool
	PIIWebhookRequestDecision  *string         // nullable: "allow", "reject", "modify"
	PIIWebhookResponseDecision *string         // nullable: "allow", "reject", "modify"
	DecisionReason             string          // NEW v2: routing decision explanation
	ErrorType                  string          // NEW v2: classified error type
	DecisionSnapshot           json.RawMessage // NEW v2: full decision context (JSON)
	Metadata                   json.RawMessage // optional cost-allocation tags from request body
	RoutingSnapshot            json.RawMessage // flat routing decision (success rows only)
	// Router-only timing breakdown (ms).
	// router_pre_ms: before upstream call starts.
	// llm_latency_ms: time spent waiting on provider/LLM.
	// router_post_ms: after upstream done, before response completion/log.
	RouterPreMS  *int
	LLMLatencyMS *int
	RouterPostMS *int
	// router_pre stage breakdown (ms). NULL means stage not reached.
	PreDecodeMS       *int
	PreAuthzMS        *int
	PreTenantConfigMS *int
	PrePIIMS          *int
	PreRateLimitMS    *int
	PreModelFilterMS  *int
	PreRoutingMS      *int
	PreRequestBuildMS *int
	// pre_tenant_config sub-breakdown (ms).
	CfgToolRoutesMS               *int
	CfgDynamicRoutesMS            *int
	CfgBudgetPressureMS           *int
	CfgSemanticMS                 *int
	CfgModelResolutionMS          *int
	ToolRoutesEmbeddingModelMS    *int
	ToolRoutesEmbeddingGenerateMS *int
	ToolRoutesSemanticDBMS        *int
	ToolRoutesMatchEvalMS         *int
	APIKeyID                      *uuid.UUID // optional; for FinOps/usage attribution
	APIKeyName                    *string    // optional; for display
	JWTSub                        *string    // optional; JWT claims.sub when auth is JWT
	// Optional business-context headers (migration 050). All nullable.
	CustomerID      *string
	Channel         *string
	InteractionType *string
	AgentID         *string
	Department      *string
	TicketID        *string
	CustomerSegment *string
	Language        *string
	// Extended business-context headers (migration 051). All nullable.
	Intent       *string
	ExperimentID *string
	AutonomyLevel *string
	PolicyID     *string
	RiskLevel    *string
	RevenueImpact *string
	Currency     *string
	// CachedTokens is the number of prompt tokens served from the provider's cache.
	CachedTokens int `json:"cached_tokens"`
	// ToolCostUSD is the cost attributed to tool calls (web search, container, etc.)
	// as a subset of the total request cost.
	ToolCostUSD float64 `json:"tool_cost_usd"`
}

// UsageRecord represents one row in the usage table.
type UsageRecord struct {
	ID               uuid.UUID
	Timestamp        time.Time
	TenantID         string
	Model            string
	Provider         string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CostUSD          float64
	RequestID        string
	APIKeyID         *uuid.UUID // optional; for FinOps/usage attribution
	APIKeyName       *string    // optional; for display
	JWTSub           *string    // optional; JWT claims.sub when auth is JWT
}

// BudgetCheck holds the result of a budget check query.
type BudgetCheck struct {
	MonthSpendUSD float64
	// ReservationID is the UUID of the budget_reservations row inserted by
	// CheckAndReserveBudget. It is uuid.Nil when no row was inserted (e.g.
	// report_only mode, DB error fail-open, or NopStorage).
	ReservationID uuid.UUID
}

// ModelStatDaily represents daily aggregate statistics for a model
type ModelStatDaily struct {
	Date         time.Time
	TenantID     string
	Model        string
	RequestCount int
	SuccessCount int
	ErrorCount   int
	AvgLatencyMs float64
	TotalCostUSD float64
}

// UsageSummary represents monthly usage aggregates
type UsageSummary struct {
	TenantID       string
	Month          string
	TotalRequests  int
	TotalCost      float64
	ModelBreakdown map[string]ModelUsage
}

type ModelUsage struct {
	Requests int
	Cost     float64
}

// BudgetForecast represents projected spending
type BudgetForecast struct {
	TenantID       string
	Month          string
	CurrentSpend   float64
	ProjectedSpend float64
	DaysElapsed    int
	DaysInMonth    int
	IsOverBudget   bool
	BudgetLimit    float64
}

// UsageDetailRow represents a single usage record with linked request data
type UsageDetailRow struct {
	RequestID        uuid.UUID
	Timestamp        time.Time
	TenantID         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	CostUSD          float64
	Status           string // "ok" or "error" (from request_log)
	LatencyMs        int
}

// SmartImpactData aggregates data for ROI calculation
type SmartImpactData struct {
	TenantID        string
	PeriodStart     time.Time
	PeriodEnd       time.Time
	TotalRequests   int
	SuccessRequests int
	ErrorRequests   int
	TotalCostUSD    float64
	AvgLatencyMs    float64
	UsageDetails    []UsageDetailRow // Individual requests with token counts
}

// BillingLineItem is one row in a streaming billing export (group_by=none).
type BillingLineItem struct {
	Timestamp        time.Time
	RequestID        string
	TenantID         string
	Model            string
	Provider         string
	Status           string
	TotalTokens      int
	PromptTokens     int
	CompletionTokens int
	CostUSD          float64
	Project          string
	CostCenter       string
	Env              string
	Application      string
}

// BillingGroupedRow is one aggregate row in a grouped billing export.
type BillingGroupedRow struct {
	GroupKey         string  `json:"group_key"`
	RequestsCount    int     `json:"requests_count"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// UsageByTagRow is one row in a usage-by-tag aggregation.
type UsageByTagRow struct {
	Value       string
	Requests    int
	TotalTokens int
	CostUSD     float64
}

// AuditRecord represents a complete audit trail entry (request + usage)
type AuditRecord struct {
	RequestID                  uuid.UUID
	Timestamp                  time.Time
	TenantID                   string
	Model                      string
	Provider                   string
	Strategy                   string
	Status                     string
	LatencyMs                  int
	PromptTokens               int
	CompletionTokens           int
	TotalTokens                int
	CostUSD                    float64
	FallbackUsed               bool
	PIIWebhookRequestDecision  *string
	PIIWebhookResponseDecision *string
}

// BudgetAlert represents a budget threshold alert
type BudgetAlert struct {
	ID              uuid.UUID
	TenantID        string
	Threshold       float64
	Month           string // YYYY-MM
	TriggeredAt     time.Time
	CurrentSpendUSD float64
	BudgetLimitUSD  float64
}

// CostAnomaly represents a detected cost anomaly
type CostAnomaly struct {
	ID             uuid.UUID
	TenantID       string
	DetectedAt     time.Time
	Date           string // YYYY-MM-DD
	DailySpendUSD  float64
	BaselineAvgUSD float64
	Multiplier     float64
}

// ConfigChange represents a configuration change audit log entry
type ConfigChange struct {
	ID          string          `json:"id"`
	Timestamp   time.Time       `json:"timestamp"`
	TenantID    string          `json:"tenant_id"`
	ActorSub    string          `json:"actor_sub"`
	ActorRoles  []string        `json:"actor_roles"`
	FromVersion int             `json:"from_version"`
	ToVersion   int             `json:"to_version"`
	Summary     string          `json:"summary"`
	Diff        json.RawMessage `json:"diff"`
}

// APIKeyRecord represents a complete API key record (internal use, includes hash)
type APIKeyRecord struct {
	ID         uuid.UUID       `json:"id"`
	TenantID   string          `json:"tenant_id"`
	Name       string          `json:"name"`
	KeyHash    string          `json:"-"` // Never marshal - security sensitive
	Prefix     string          `json:"prefix"`
	Scopes     []string        `json:"scopes"`
	CreatedAt  time.Time       `json:"created_at"`
	ExpiresAt  *time.Time      `json:"expires_at,omitempty"`
	RevokedAt  *time.Time      `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time      `json:"last_used_at,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

// APIKeyMeta represents public API key metadata (no hash/plaintext)
type APIKeyMeta struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   string     `json:"tenant_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// APIKeyCreateResult includes plaintext key (returned only once on creation/rotation)
type APIKeyCreateResult struct {
	APIKeyMeta
	Key string `json:"key"` // Plaintext - ONLY returned on create/rotate
}

// TenantUsageOverview holds aggregated request/token/cost totals for a tenant.
type TenantUsageOverview struct {
	TotalRequests int64
	TotalTokens   int64
	TotalCostUSD  float64
}

// RequestListRow is one row returned by ListRecentRequests.
type RequestListRow struct {
	RequestID      string
	TenantID       string
	Model          string
	Provider       string
	Status         string
	CreatedAt      time.Time
	LatencyMs      int
	Strategy       string
	FallbackUsed   bool
	ErrorType      *string
	DecisionReason *string
	CacheHit       bool
}

// ComplianceRetentionConfig holds retention day counts for each log type.
type ComplianceRetentionConfig struct {
	ConversationLogsDays int `json:"conversation_logs_days"`
	RequestLogsDays      int `json:"request_logs_days"`
	ComplianceLogsDays   int `json:"compliance_logs_days"`
}

// ComplianceGlobalConfig is the persisted compliance module configuration (SPEC_132).
type ComplianceGlobalConfig struct {
	ConversationLogsMode string                    `json:"conversation_logs_mode"`
	Retention            ComplianceRetentionConfig `json:"retention"`
	RetentionAction      string                    `json:"retention_action"`
}

// DefaultComplianceGlobalConfig returns the hardcoded default compliance config
// used when no row exists in the compliance_config table.
func DefaultComplianceGlobalConfig() ComplianceGlobalConfig {
	return ComplianceGlobalConfig{
		ConversationLogsMode: "redacted",
		Retention: ComplianceRetentionConfig{
			ConversationLogsDays: 30,
			RequestLogsDays:      30,
			ComplianceLogsDays:   90,
		},
		RetentionAction: "inform",
	}
}

// ComplianceEventLog represents one row in compliance_event_log.
type ComplianceEventLog struct {
	ID          uuid.UUID
	TenantID    string
	RequestID   string
	EventType   string
	ActionTaken string
	Metadata    json.RawMessage
	CreatedAt   time.Time
}

// ComplianceEventFilter filters compliance events by optional fields.
type ComplianceEventFilter struct {
	From      *time.Time
	To        *time.Time
	TenantID  *string
	RequestID *string
	EventType *string
}

// ConversationLog represents one row in conversation_log.
type ConversationLog struct {
	ID                  uuid.UUID
	RequestID           string
	TenantID            string
	JWTSub              *string
	WorkflowID          *string // non-nil when request is part of a DecisionOps workflow
	ConversationID      *string // non-nil when request is part of a DecisionOps workflow
	CustomerID          *string // optional; from X-Customer-Id header (migration 050)
	PromptPreview       string
	ResponsePreview     string
	PromptRedacted      *string
	ResponseRedacted    *string
	PromptFull          *string
	ResponseFull        *string
	EncKeyVersion       string `json:"-"`
	PromptRedactedEnc   []byte `json:"-"`
	ResponseRedactedEnc []byte `json:"-"`
	PromptFullEnc       []byte `json:"-"`
	ResponseFullEnc     []byte `json:"-"`
	PIIDetected         bool
	LoggingMode         string
	CreatedAt           time.Time
}

// ConversationLogFilter filters conversation logs by optional fields.
type ConversationLogFilter struct {
	From           *time.Time
	To             *time.Time
	TenantID       *string
	JWTSub         *string
	WorkflowID     *string
	ConversationID *string
}

// LicenseRecord holds the active license JWT token persisted in the database.
type LicenseRecord struct {
	Token       string
	InstalledAt time.Time
}

// Storage is the interface for persisting request logs and usage records.
type Storage interface {
	LogRequest(ctx context.Context, log RequestLog) error
	LogConversation(ctx context.Context, row ConversationLog) error
	LogComplianceEvent(ctx context.Context, event ComplianceEventLog) error
	SaveUsage(ctx context.Context, usage UsageRecord) error
	// CheckAndReserveBudget atomically checks the current month spend for a tenant
	// and inserts a cost reservation if within budget. Uses advisory locks to prevent
	// concurrent overspend. Returns the current month spend and the reservation UUID.
	// If the spend + estimatedCost > limit, returns ErrBudgetExceeded.
	CheckAndReserveBudget(ctx context.Context, tenantID string, monthStart, monthEnd time.Time, limitUSD, estimatedCost float64) (BudgetCheck, error)
	// ReleaseReservation deletes a budget_reservations row by its primary key.
	// Called after a request completes successfully to avoid double-counting
	// the reservation against confirmed usage. Fail-safe: no error if not found.
	ReleaseReservation(ctx context.Context, reservationID uuid.UUID) error
	// PurgeExpiredReservations deletes all budget_reservations rows past their expires_at.
	// Returns the number of rows deleted.
	PurgeExpiredReservations(ctx context.Context) (int64, error)
	// GetMonthlyReservedSpend returns the sum of all in-flight budget reservations
	// for a tenant within the given month window.
	GetMonthlyReservedSpend(ctx context.Context, tenantID string, monthStart, monthEnd time.Time) (float64, error)

	// Smart routing methods
	UpsertModelStatDaily(ctx context.Context, stat ModelStatDaily) error
	GetModelStats(ctx context.Context, tenantID string, windowDays int) ([]ModelStatDaily, error)
	GetUsageSummary(ctx context.Context, tenantID string, month time.Time) (UsageSummary, error)
	GetBudgetForecast(ctx context.Context, tenantID string, month time.Time, budgetLimit float64) (BudgetForecast, error)

	// GetSmartImpactData retrieves detailed usage data for ROI calculation
	// Returns individual request records with tokens and status for baseline simulation
	GetSmartImpactData(ctx context.Context, tenantID string, from, to time.Time) (SmartImpactData, error)

	// GetAuditRecords retrieves audit trail for export (90-day max window)
	GetAuditRecords(ctx context.Context, tenantID string, from, to time.Time) ([]AuditRecord, error)

	// StreamBillingLineItems iterates request-level billing rows for a tenant/month
	// without loading them all into memory. fn is called once per row; if fn returns
	// a non-nil error the iteration stops and that error is returned.
	StreamBillingLineItems(ctx context.Context, tenantID string, from, to time.Time,
		fn func(BillingLineItem) error) error

	// GetBillingGrouped returns aggregated billing rows for the given groupBy field.
	// groupBy must be one of: "model", "provider", "project", "cost_center", "env", "application".
	GetBillingGrouped(ctx context.Context, tenantID string, from, to time.Time,
		groupBy string) ([]BillingGroupedRow, error)

	// GetUsageByTag aggregates usage for a tenant/month grouped by the given
	// metadata tag key. tag must match ^[a-zA-Z][a-zA-Z0-9_]{0,63}$.
	GetUsageByTag(ctx context.Context, tenantID string, from, to time.Time,
		tag string) ([]UsageByTagRow, error)

	// DeleteOldRecords removes request_log and usage entries older than cutoff
	// Returns number of request_log rows deleted
	DeleteOldRecords(ctx context.Context, tenantID string, cutoffDate time.Time) (int, error)

	// InsertBudgetAlert tries to insert an alert. Returns (true, nil) if inserted,
	// (false, nil) if duplicate (UNIQUE constraint), or (false, err) on error.
	InsertBudgetAlert(ctx context.Context, alert BudgetAlert) (bool, error)

	// GetBudgetAlerts retrieves alerts for a tenant in a specific month
	GetBudgetAlerts(ctx context.Context, tenantID, month string) ([]BudgetAlert, error)

	// InsertCostAnomaly records a detected cost anomaly
	InsertCostAnomaly(ctx context.Context, anomaly CostAnomaly) error

	// GetCostAnomalies retrieves anomalies for a tenant within window
	GetCostAnomalies(ctx context.Context, tenantID string, windowDays int) ([]CostAnomaly, error)

	// Dynamic tenant configuration methods
	// GetTenantConfig retrieves tenant configuration from database
	// Returns (configJSON, version, exists, error)
	GetTenantConfig(ctx context.Context, tenantID string) (json.RawMessage, int, bool, error)

	// PutTenantConfig updates tenant configuration with optimistic concurrency control
	// Returns new version on success, error on version conflict or other failures
	PutTenantConfig(ctx context.Context, tenantID string, ifMatchVersion int, newConfigJSON json.RawMessage, actorSub string, actorRoles []string, summary string, diffJSON json.RawMessage) (int, error)

	// PatchTenantConfig applies JSON Merge Patch to tenant configuration
	// Returns new version on success, error on version conflict or other failures
	PatchTenantConfig(ctx context.Context, tenantID string, ifMatchVersion int, mergePatchJSON json.RawMessage, actorSub string, actorRoles []string) (int, error)

	// ListTenantConfigChanges retrieves configuration change history
	ListTenantConfigChanges(ctx context.Context, tenantID string, limit int) ([]ConfigChange, error)

	// SeedTenantConfig initializes tenant config in DB with version=0 if it doesn't exist
	// Used for seeding from YAML - does NOT create config_change_log entry
	// Returns true if seeded, false if already exists or if seed was skipped
	SeedTenantConfig(ctx context.Context, tenantID string, configJSON json.RawMessage) (bool, error)

	// GetGlobalConfig retrieves the current active global config from database.
	// Returns (configJSON, version, exists, error).
	GetGlobalConfig(ctx context.Context) (json.RawMessage, int, bool, error)

	// PutGlobalConfig replaces the global configuration using optimistic locking.
	// ifMatchVersion must match the current active_version in global_active_config.
	// Returns (newVersion, error). Returns ErrVersionConflict on mismatch.
	PutGlobalConfig(ctx context.Context, ifMatchVersion int, configJSON json.RawMessage, actorSub string, actorRoles []string) (int, error)

	// PatchGlobalConfig applies a JSON Merge Patch (RFC 7396) to the current global config.
	// ifMatchVersion must match the current active_version.
	// Returns (newVersion, error). Returns ErrVersionConflict on mismatch.
	PatchGlobalConfig(ctx context.Context, ifMatchVersion int, mergePatchJSON json.RawMessage, actorSub string, actorRoles []string) (int, error)

	// RollbackGlobalConfig sets global_active_config to a previously stored version.
	// ifMatchVersion must match the current active_version (optimistic lock).
	// targetVersion must exist in global_config_versions.
	// Returns error. Returns ErrVersionConflict on mismatch.
	RollbackGlobalConfig(ctx context.Context, ifMatchVersion, targetVersion int, actorSub string, actorRoles []string) error

	// SeedGlobalConfig seeds the global config if not present (or always, depending on caller logic).
	// Returns true if a new version was inserted, false if skipped.
	SeedGlobalConfig(ctx context.Context, configJSON json.RawMessage) (bool, error)

	// SeedTenantVersionedConfig seeds tenant_config_versions + tenant_active_config for a tenant.
	// seedMode: "if_empty" → only when no version exists; "always" → always insert new version.
	// Returns true if a new version was inserted.
	SeedTenantVersionedConfig(ctx context.Context, tenantID string, configJSON json.RawMessage, seedMode string) (bool, error)

	// SeedAPIKeyFromYAML inserts a YAML-defined plaintext API key into api_keys (idempotent).
	// Returns true if inserted, false if already existed.
	SeedAPIKeyFromYAML(ctx context.Context, tenantID, apiKey string) (bool, error)

	// API Key management methods

	// CreateAPIKey generates a new API key with SHA256 hashing
	// Returns plaintext key ONLY once in APIKeyCreateResult
	CreateAPIKey(ctx context.Context, tenantID, name string, scopes []string, expiresAt *time.Time, actorSub string, actorRoles []string) (APIKeyCreateResult, error)

	// CountAPIKeys returns the total number of API keys across all tenants
	// Used by bootstrap endpoint to detect if api_keys table is empty (first-time setup)
	CountAPIKeys(ctx context.Context) (int, error)

	// ListAPIKeys retrieves all API keys for a tenant (never includes plaintext or hash)
	ListAPIKeys(ctx context.Context, tenantID string) ([]APIKeyMeta, error)

	// ListAPIKeysPaged retrieves API keys for a tenant with pagination support.
	// If includeRevoked is false, only active (non-revoked) keys are returned.
	// Returns (keys, hasMore, error). hasMore is true if there are more pages.
	ListAPIKeysPaged(ctx context.Context, tenantID string, includeRevoked bool, limit, offset int) ([]APIKeyMeta, bool, error)

	// RevokeAPIKey marks an API key as revoked (sets revoked_at timestamp)
	// Returns the revocation timestamp
	RevokeAPIKey(ctx context.Context, tenantID string, keyID uuid.UUID, actorSub string, actorRoles []string) (*time.Time, error)

	// RotateAPIKey atomically creates a new key and revokes the old one
	// Returns (oldKeyID, newKeyResult) where newKeyResult includes plaintext ONLY once
	RotateAPIKey(ctx context.Context, tenantID string, keyID uuid.UUID, actorSub string, actorRoles []string) (uuid.UUID, APIKeyCreateResult, error)

	// LookupAPIKeyByHash retrieves an active API key by its SHA256 hash
	// Returns (record, found, error). Returns found=false if key doesn't exist, is revoked, or is expired
	LookupAPIKeyByHash(ctx context.Context, keyHash string) (APIKeyRecord, bool, error)

	// TouchAPIKeyLastUsed updates the last_used_at timestamp (best-effort, non-blocking)
	TouchAPIKeyLastUsed(ctx context.Context, keyID uuid.UUID, ts time.Time) error

	// GetNearestSemanticAnchor returns the nearest semantic anchor for the given embedding
	// using cosine distance via pgvector. Filtered by tenantID and modality.
	// Returns (name, routeGroup, preferredModels, distance, found, error).
	// On not-found or error, callers must fail open (skip semantic routing).
	GetNearestSemanticAnchor(ctx context.Context, tenantID string, embedding []float64, modality string) (name, routeGroup string, preferredModels []string, distance float64, found bool, err error)

	// UpsertSemanticAnchor inserts a new semantic anchor for a tenant.
	// Returns ErrAnchorAlreadyExists if (tenant_id, name) already exists.
	UpsertSemanticAnchor(ctx context.Context, tenantID, name string, embedding []float64, routeGroup string, preferredModels []string, anchorText *string, modality string) error

	// ListSemanticAnchorsSorted returns all semantic anchors for a tenant ordered by cosine
	// distance (ascending) to the given embedding. Filtered by modality. Returns at most topK results.
	// Distance and dims are computed in SQL via pgvector.
	ListSemanticAnchorsSorted(ctx context.Context, tenantID string, embedding []float64, topK int, modality string) ([]SemanticAnchorRow, error)

	// ListSemanticAnchorsPaged returns a paginated list of anchors for a tenant ordered by name.
	// If includeAnchorText is false, AnchorText is set to nil in the results.
	// hasMore is true when there are more pages after this one.
	ListSemanticAnchorsPaged(ctx context.Context, tenantID string, includeAnchorText bool, limit, offset int) ([]SemanticAnchorMeta, bool, error)

	// UpdateSemanticAnchor applies a partial update to an existing anchor.
	// Returns (true, nil) on success, (false, nil) if not found, or (false, err) on error.
	UpdateSemanticAnchor(ctx context.Context, tenantID, name string, patch SemanticAnchorPatch) (bool, error)

	// DeleteSemanticAnchor removes an anchor. Returns (true, nil) on success,
	// (false, nil) if not found, or (false, err) on error.
	DeleteSemanticAnchor(ctx context.Context, tenantID, name string) (bool, error)

	// FindNearestSemanticCache returns the nearest eligible cache entry for the given
	// tenant and query vector. Returns found=false if nothing exceeds the threshold.
	FindNearestSemanticCache(ctx context.Context, tenantID string, queryVector []float64, scope SemanticCacheScope, model string, routeGroup string, threshold float64) (*SemanticCacheEntry, bool, error)

	// InsertSemanticCacheEntry stores a new cache entry.
	InsertSemanticCacheEntry(ctx context.Context, entry SemanticCacheInsert) error

	// TouchSemanticCacheHit updates last_hit_at and increments hit_count.
	TouchSemanticCacheHit(ctx context.Context, id uuid.UUID, ts time.Time) error

	// PruneExpiredSemanticCache removes expired entries for a tenant.
	PruneExpiredSemanticCache(ctx context.Context, tenantID string) error

	// GetRoutingSnapshot retrieves the routing snapshot for a successful request.
	// Returns (tenantID, snapshotJSON, found, error).
	// Only returns rows with status='ok' and non-null routing_snapshot.
	GetRoutingSnapshot(ctx context.Context, requestID string) (tenantID string, snapshot json.RawMessage, found bool, err error)

	// GetReplayDiagnostics returns full diagnostics for a successful request (replay endpoint).
	GetReplayDiagnostics(ctx context.Context, requestID string) (ReplayDiagnostics, bool, error)

	// GetReplayRequests retrieves successful request_log rows (with routing_snapshot)
	// joined with usage data for traffic replay simulation.
	// Returns at most limit rows ordered by timestamp descending.
	GetReplayRequests(ctx context.Context, tenantID string, from, to time.Time, limit int) ([]ReplayRow, error)

	// GetMonthlySpend returns the total cost_usd for a tenant in [from, to) — read-only, no reservation.
	// Used for report_only budget enforcement (no budget_reservations row created).
	GetMonthlySpend(ctx context.Context, tenantID string, from, to time.Time) (float64, error)

	// GetTagMonthlySpend returns the total cost_usd for requests where metadata->>tagKey == tagValue.
	// Used for tag-level budget enforcement.
	GetTagMonthlySpend(ctx context.Context, tenantID, tagKey, tagValue string, from, to time.Time) (float64, error)

	// CreateSemanticRoute inserts all utterance rows for a named route inside a transaction.
	// Returns ErrRouteAlreadyExists if any row for (tenant_id, name) already exists.
	CreateSemanticRoute(ctx context.Context, tenantID, name, description, action string,
		threshold float64, utterances []string, embeddings [][]float64) error

	// GetNearestSemanticRoute finds the closest utterance embedding across all routes for
	// the tenant. Returns (match, true, nil) on hit; (zero, false, nil) if no routes exist.
	GetNearestSemanticRoute(ctx context.Context, tenantID string, embedding []float64) (SemanticRouteMatch, bool, error)

	// DeleteSemanticRoute removes all utterance rows for (tenant_id, name).
	// Returns (true, nil) if at least one row was deleted, (false, nil) if not found.
	DeleteSemanticRoute(ctx context.Context, tenantID, name string) (bool, error)

	// ListSemanticRoutes returns one row per distinct (name, action) for the tenant.
	ListSemanticRoutes(ctx context.Context, tenantID string) ([]SemanticRouteRow, error)

	// GetSemanticRoute returns the named route with all its utterances.
	// Returns (detail, true, nil) if found, (zero, false, nil) if not found.
	GetSemanticRoute(ctx context.Context, tenantID, name string) (SemanticRouteDetail, bool, error)

	// UpdateSemanticRoute applies a partial update to an existing route atomically.
	// If patch.Utterances is non-nil, existing utterance rows are deleted and replaced using
	// the provided embeddings (must match len). Returns (true, nil) on success,
	// (false, nil) if the route does not exist.
	UpdateSemanticRoute(ctx context.Context, tenantID, name string, patch SemanticRoutePatch, embeddings [][]float64) (bool, error)

	// GetModelMRM returns the MRM metadata for a model.
	// Returns (metadata, true, nil) if found, (nil, false, nil) if not found.
	GetModelMRM(ctx context.Context, modelID string) (map[string]interface{}, bool, error)

	// PatchModelMRM merges patch into the existing mrm_metadata for modelID.
	// Creates the row if it does not exist (upsert). Returns the merged result.
	PatchModelMRM(ctx context.Context, modelID string, patch map[string]interface{}) (map[string]interface{}, error)

	// GetComplianceConfig returns the current compliance configuration.
	// Returns the default config when no row exists in the compliance_config table.
	GetComplianceConfig(ctx context.Context) (ComplianceGlobalConfig, error)

	// PatchComplianceConfig merges the provided partial config into the stored one and saves it.
	// Only non-zero fields in patch are applied; zero fields are left unchanged.
	PatchComplianceConfig(ctx context.Context, patch ComplianceGlobalConfig) (ComplianceGlobalConfig, error)

	// Compliance retention helpers (SPEC_137).
	// Count* methods return the number of rows strictly older than cutoff without deleting them.
	CountExpiredConversationLogs(ctx context.Context, cutoff time.Time) (int64, error)
	CountExpiredRequestLogs(ctx context.Context, cutoff time.Time) (int64, error)
	CountExpiredComplianceEvents(ctx context.Context, cutoff time.Time) (int64, error)
	// Delete* methods delete rows strictly older than cutoff and return the number deleted.
	DeleteExpiredConversationLogs(ctx context.Context, cutoff time.Time) (int64, error)
	DeleteExpiredRequestLogs(ctx context.Context, cutoff time.Time) (int64, error)
	DeleteExpiredComplianceEvents(ctx context.Context, cutoff time.Time) (int64, error)

	// CreateTenant creates a new tenant with empty initial configuration.
	// Returns ErrTenantAlreadyExists if the tenant already exists in the database.
	CreateTenant(ctx context.Context, tenantID string, initialConfig json.RawMessage, actorSub string, actorRoles []string) error

	// DeleteTenant removes all configuration data for a tenant from the database.
	// Returns (true, nil) if deleted, (false, nil) if the tenant was not found in the DB.
	DeleteTenant(ctx context.Context, tenantID string) (bool, error)

	// ListTenants returns all tenant IDs present in tenant_active_config (DB-created tenants).
	// Used to merge dynamic tenants with YAML-defined ones in catalog responses.
	ListTenants(ctx context.Context) ([]string, error)

	// GetTenantUsageOverview returns total request count, token count, and cost for a
	// tenant within [from, to). Uses two queries: request_log for counts, usage for tokens/cost.
	GetTenantUsageOverview(ctx context.Context, tenantID string, from, to time.Time) (TenantUsageOverview, error)

	// GetModelRequestCounts returns the total request count per model aggregated from
	// model_stats_daily over the last windowDays days (across all tenants).
	GetModelRequestCounts(ctx context.Context, windowDays int) (map[string]int64, error)

	// ListRecentRequests returns recent request_log rows (attempt=1 only) in the last windowHours,
	// ordered by timestamp descending. tenantID="" means all tenants. windowHours <= 0 defaults to 24.
	// Returns (rows, hasMore, error). hasMore is true if there are more pages.
	ListRecentRequests(ctx context.Context, tenantID string, windowHours, limit, offset int) ([]RequestListRow, bool, error)

	// InsertModelBenchmark stores a single benchmark result row.
	InsertModelBenchmark(ctx context.Context, row ModelBenchmarkRow) error

	// GetModelBenchmarkAggregates returns per-model aggregate stats for the given
	// window (e.g. last 24 h). windowHours <= 0 defaults to 24.
	GetModelBenchmarkAggregates(ctx context.Context, windowHours int) ([]BenchmarkAggregate, error)

	// DeleteOldModelBenchmarks removes rows older than retainDays days.
	// Returns the number of rows deleted.
	DeleteOldModelBenchmarks(ctx context.Context, retainDays int) (int64, error)

	// TruncateModelBenchmarks deletes all rows from model_benchmarks.
	// Used by admins to reset stale historical data.
	TruncateModelBenchmarks(ctx context.Context) (int64, error)

	// InsertAnthropicMessageLog stores one audit row for POST /v1/messages (SPEC_154).
	// Writes to anthropic_message_log only — never touches any existing table.
	InsertAnthropicMessageLog(ctx context.Context, row AnthropicMessageLog) error

	// GetClaudeCodeMonthlySpend returns the sum of cost from anthropic_message_log
	// for a tenant in [from, to) where cost IS NOT NULL. Used for SPEC_163 budget enforcement.
	GetClaudeCodeMonthlySpend(ctx context.Context, tenantID string, from, to time.Time) (float64, error)

	// SPEC_164: FinOps query methods for GET /admin/claude_code/usage.
	GetClaudeCodeUsageSummary(ctx context.Context, f ClaudeCodeUsageFilter) (ClaudeCodeUsageSummary, error)
	GetClaudeCodeUsageTimeseries(ctx context.Context, f ClaudeCodeUsageFilter) ([]ClaudeCodeTimeseriesBucket, error)
	GetClaudeCodeUsageRows(ctx context.Context, f ClaudeCodeUsageFilter) ([]ClaudeCodeUsageRow, int64, error)

	// ListAnomalies returns paginated rows from cost_anomalies for the admin API.
	ListAnomalies(ctx context.Context, filter AnomalyListFilter) ([]AnomalyListRow, int, error)

	// GetAnomalyExplanations returns top drivers (model, provider, api_key) per anomaly for the last windowDays.
	GetAnomalyExplanations(ctx context.Context, windowDays int) ([]AnomalyExplanation, error)

	// GetAPIKeyUsage returns summary and per-key rows for the admin FinOps API.
	GetAPIKeyUsage(ctx context.Context, filter APIKeyUsageFilter) (APIKeyUsageSummary, []APIKeyUsageRow, error)

	// GetAPIKeyUsageDetail returns full drilldown for one API key.
	GetAPIKeyUsageDetail(ctx context.Context, apiKeyID uuid.UUID, windowHours, limit, offset int) (APIKeyUsageDetailSummary, []APIKeyUsageByModelRow, []APIKeyUsageByProviderRow, []APIKeyUsageRecentRow, int, LatencyStats, []ErrorCountRow, error)

	// GetAPIKeyModelBreakdown returns per-(api_key_id, model) usage rows for the same window as GetAPIKeyUsage.
	// Used by handlers to compute effective cost per request including infrastructure allocation.
	GetAPIKeyModelBreakdown(ctx context.Context, filter APIKeyUsageFilter) ([]APIKeyModelUsageRow, error)

	// GetTenantModelRequestCounts returns total request counts per model for a tenant in the given window.
	// Used as the denominator for infrastructure cost allocation.
	// If tenantID is "", returns counts across all tenants keyed by model name.
	GetTenantModelRequestCounts(ctx context.Context, tenantID string, since time.Time) (map[string]int64, error)

	// GetAnomalyStats returns aggregate anomaly stats for the dashboard.
	GetAnomalyStats(ctx context.Context, windowHours int, tenantID, model, provider string) (AnomalyStats, error)

	// ListConfigHistory returns config change history (global or tenant) for GET /admin/config/history.
	ListConfigHistory(ctx context.Context, filter ConfigHistoryFilter) ([]ConfigHistoryRow, error)

	// ListConfigVersions returns version timeline for GET /admin/config/versions.
	ListConfigVersions(ctx context.Context, filter ConfigVersionFilter) ([]ConfigVersionRow, error)

	// ListRequestLogRecent returns paginated request_log rows with optional filters.
	ListRequestLogRecent(ctx context.Context, filter RequestLogRecentFilter, limit, offset int) ([]RequestLogRecentRow, int, error)
	// ListComplianceEvents returns paginated compliance_event_log rows with optional filters.
	ListComplianceEvents(ctx context.Context, filter ComplianceEventFilter, limit, offset int) ([]ComplianceEventLog, int, error)
	// ListConversations returns paginated conversation_log rows with optional filters.
	ListConversations(ctx context.Context, filter ConversationLogFilter, limit, offset int) ([]ConversationLog, int, error)

	// ListAPIKeyRawUsage returns raw per-request usage rows for admin API key requests.
	ListAPIKeyRawUsage(ctx context.Context, filter APIKeyRawUsageFilter) ([]APIKeyRawUsageRow, int, error)

	// GetJWTSubModelBreakdown returns per-(jwt_sub, tenant_id, model) rows for monetization computation.
	GetJWTSubModelBreakdown(ctx context.Context, filter JWTSubUsageFilter) ([]JWTSubModelUsageRow, error)

	// GetJWTSubUsage returns aggregated usage grouped by jwt_sub.
	GetJWTSubUsage(ctx context.Context, filter JWTSubUsageFilter) ([]JWTSubUsageRow, int, error)

	// GetJWTSubUsageDetail returns summary and breakdown for one jwt_sub.
	GetJWTSubUsageDetail(ctx context.Context, jwtSub string, filter JWTSubUsageDetailFilter) (JWTSubUsageDetailSummary, []JWTSubUsageBreakdownRow, error)

	// ListJWTSubRawUsage returns raw per-request usage rows attributed to jwt_sub.
	ListJWTSubRawUsage(ctx context.Context, filter JWTSubRawUsageFilter) ([]JWTSubRawUsageRow, int, error)

	// GetRequestStats returns aggregated request telemetry for the given window and optional tenant.
	GetRequestStats(ctx context.Context, tenantID string, windowHours int, bucket string) (RequestStats, error)

	// GetRouterPerformance returns router-only performance metrics for a time window.
	GetRouterPerformance(ctx context.Context, filter RouterPerformanceFilter) (RouterPerformanceMetrics, error)

	// GetSemanticRoutingStats returns semantic routing analytics (coverage, top routes, top anchors).
	GetSemanticRoutingStats(ctx context.Context, tenantID string, windowDays int) (SemanticRoutingStats, error)

	// GetConfigAtVersion returns the config JSON at a specific version for diff/view.
	GetConfigAtVersion(ctx context.Context, scope, tenantID string, version int) (json.RawMessage, error)

	// ApplyGlobalConfigVersion sets the active global config to an existing version.
	ApplyGlobalConfigVersion(ctx context.Context, version int, actorSub string, actorRoles []string) error

	// GetAPIKeyMetaByID returns key metadata by id. Returns found=false if key does not exist.
	GetAPIKeyMetaByID(ctx context.Context, id uuid.UUID) (APIKeyMeta, bool, error)

	// GetSemanticCacheStats returns aggregated semantic cache analytics.
	GetSemanticCacheStats(ctx context.Context, tenantID string, limit int) (SemanticCacheStats, error)

	// GetCacheSavings estimates cost savings from semantic cache.
	GetCacheSavings(ctx context.Context, tenantID string) (CacheSavings, error)

	// GetSemanticCorrelation correlates semantic cache hits and request counts by route_group.
	GetSemanticCorrelation(ctx context.Context, tenantID string, windowDays int) (SemanticCorrelation, error)

	// StoreLicense persists the raw license JWT token.
	// Replaces any previously stored license (singleton row).
	StoreLicense(ctx context.Context, token string) error

	// GetLicense retrieves the stored license JWT token.
	// Returns found=false (not an error) when no license has been stored yet.
	GetLicense(ctx context.Context) (token string, found bool, err error)

	// CreateDecisionWorkflow inserts a new workflow row.
	// Returns ErrWorkflowAlreadyExists on duplicate workflow_id.
	CreateDecisionWorkflow(ctx context.Context, row DecisionWorkflowRow) error

	// GetDecisionWorkflow fetches a workflow by ID.
	// Returns (row, true, nil) if found, (zero, false, nil) if not found.
	GetDecisionWorkflow(ctx context.Context, workflowID string) (DecisionWorkflowRow, bool, error)

	// UpdateDecisionWorkflow replaces description, max_cost, max_steps and updated_at/by.
	// Returns ErrWorkflowNotFound if the row does not exist.
	UpdateDecisionWorkflow(ctx context.Context, row DecisionWorkflowRow) error

	// DeleteDecisionWorkflow removes a workflow row.
	// Returns ErrWorkflowNotFound if the row does not exist.
	DeleteDecisionWorkflow(ctx context.Context, workflowID string) error

	// ListDecisionWorkflows returns all workflows ordered by workflow_id.
	ListDecisionWorkflows(ctx context.Context) ([]DecisionWorkflowRow, error)

	// WorkflowConversation methods (SPEC_169)

	// UpsertWorkflowConversation inserts or updates a conversation row.
	// Atomically: accumulated_steps += 1, accumulated_cost_usd += costUSD, last_activity_at = NOW().
	// entryModel is stored only on INSERT (first call); subsequent updates preserve the original value.
	// Returns the updated row.
	UpsertWorkflowConversation(ctx context.Context, tenantID, workflowID, conversationID, entryModel string, costUSD float64, attrs WorkflowContextAttrs) (WorkflowConversationRow, error)

	// GetWorkflowConversation returns (row, true, nil) or (zero, false, nil) if not found.
	GetWorkflowConversation(ctx context.Context, tenantID, workflowID, conversationID string) (WorkflowConversationRow, bool, error)

	// GetConversationDialog returns conversation metadata + all request-response steps ordered ASC.
	// Returns (nil, nil) if not found.
	GetConversationDialog(ctx context.Context, tenantID, workflowID, conversationID string) (*ConversationDialog, error)

	// ListDialogExportRows returns step-level rows for the dialog CSV export.
	// tenantID="" returns rows for all tenants. limit max is 50000.
	ListDialogExportRows(ctx context.Context, tenantID string, since time.Time, limit int) ([]DialogExportRow, error)

	// SetWorkflowConversationTier updates current_tier.
	SetWorkflowConversationTier(ctx context.Context, tenantID, workflowID, conversationID string, tier int) error

	// SetWorkflowConversationBlocked sets blocked = true.
	SetWorkflowConversationBlocked(ctx context.Context, tenantID, workflowID, conversationID string) error

	// SetWorkflowConversationPolicyActions stores the per-policy action results from the last evaluation.
	SetWorkflowConversationPolicyActions(ctx context.Context, tenantID, workflowID, conversationID, stepsAction, costAction string) error

	// DeleteExpiredWorkflowConversations removes rows where last_activity_at < cutoff.
	DeleteExpiredWorkflowConversations(ctx context.Context, cutoff time.Time) (int64, error)

	// ListWorkflowConversations returns active conversation rows for analytics.
	ListWorkflowConversations(ctx context.Context, workflowID, tenantID string, since time.Time, limit, offset int) ([]WorkflowConversationRow, int, error)

	// GetWorkflowAnalytics returns per-workflow aggregated stats since `since` (SPEC_171).
	GetWorkflowAnalytics(ctx context.Context, since time.Time) ([]WorkflowAnalyticsRow, error)

	// GetWorkflowTenantBreakdown returns per-workflow per-tenant aggregates since `since` (SPEC_171).
	GetWorkflowTenantBreakdown(ctx context.Context, since time.Time) ([]WorkflowTenantRow, error)

	// GetWorkflowFinOps returns per-conversation cost data for FinOps savings analytics (SPEC_172/173).
	// Queries both workflow_conversation_snapshots (historical) and workflow_conversations (active).
	GetWorkflowFinOps(ctx context.Context, since time.Time) ([]WorkflowFinOpsRow, error)

	// GetConversationStepCosts returns per-step cost breakdown for one conversation (SPEC_172).
	GetConversationStepCosts(ctx context.Context, workflowID, conversationID string) ([]ConversationStepCost, error)

	// UpsertWorkflowConversationSnapshot writes or updates a conversation snapshot (SPEC_173).
	// Used on block (immediate) and by the TTL cleanup job (on expiry).
	UpsertWorkflowConversationSnapshot(ctx context.Context, s WorkflowConversationSnapshot) error

	// SnapshotAndDeleteExpiredConversations snapshots expired rows then deletes them (SPEC_173).
	// Replaces DeleteExpiredWorkflowConversations in the scheduler.
	SnapshotAndDeleteExpiredConversations(ctx context.Context, cutoff time.Time) (int64, error)

	// DeleteOldWorkflowConversationSnapshots removes snapshot rows older than cutoff (SPEC_173).
	DeleteOldWorkflowConversationSnapshots(ctx context.Context, cutoff time.Time) (int64, error)

	// InsertCustomerInteraction inserts a single customer interaction row.
	// If row.ID is uuid.Nil the database generates the UUID via gen_random_uuid().
	InsertCustomerInteraction(ctx context.Context, row CustomerInteraction) error

	// InsertCustomerInteractionBatch inserts multiple rows inside a single transaction.
	// All-or-nothing: any error rolls back the entire batch.
	InsertCustomerInteractionBatch(ctx context.Context, rows []CustomerInteraction) error

	// ListCustomerInteractions returns a paginated list of customer interactions
	// matching the given filter. Returns (rows, totalCount, error).
	ListCustomerInteractions(ctx context.Context, filter CustomerInteractionFilter, limit, offset int) ([]CustomerInteraction, int, error)

	// Model catalog methods.

	// ListModelCatalog returns all entries matching the given filter.
	ListModelCatalog(ctx context.Context, filter ModelCatalogFilter) ([]ModelCatalogEntry, error)

	// GetModelCatalogEntry returns a single entry by (provider, id).
	// Returns (entry, true, nil) if found, (zero, false, nil) if not found.
	GetModelCatalogEntry(ctx context.Context, provider, id string) (ModelCatalogEntry, bool, error)

	// CreateModelCatalogEntry inserts a new entry.
	// Returns an error containing "already exists" on duplicate (provider, id).
	CreateModelCatalogEntry(ctx context.Context, entry ModelCatalogEntry) (ModelCatalogEntry, error)

	// UpdateModelCatalogEntry replaces a catalog entry.
	// Returns (updated, true, nil) on success, (zero, false, nil) if not found.
	UpdateModelCatalogEntry(ctx context.Context, entry ModelCatalogEntry) (ModelCatalogEntry, bool, error)

	// DeleteModelCatalogEntry removes an entry.
	// Returns (true, nil) if deleted, (false, nil) if not found.
	DeleteModelCatalogEntry(ctx context.Context, provider, id string) (bool, error)

	// ListCatalogPricing returns a minimal pricing projection of all model_catalog rows.
	// Used by config.CatalogEnricher to enrich ModelConfig.Pricing at cache-load time.
	ListCatalogPricing(ctx context.Context) ([]config.CatalogPricingRow, error)

	// Tool catalog methods.

	// ListToolCatalog returns all entries matching the given filter.
	ListToolCatalog(ctx context.Context, filter ToolCatalogFilter) ([]ToolCatalogEntry, error)

	// GetToolCatalogEntry returns a single entry by (provider, id).
	// Returns (entry, nil) if found, (nil, nil) if not found.
	GetToolCatalogEntry(ctx context.Context, provider, id string) (*ToolCatalogEntry, error)

	// CreateToolCatalogEntry inserts a new entry.
	// Returns ErrToolAlreadyExists on duplicate (provider, id).
	CreateToolCatalogEntry(ctx context.Context, entry ToolCatalogEntry) error

	// UpdateToolCatalogEntry replaces a tool catalog entry.
	// Returns an error if not found.
	UpdateToolCatalogEntry(ctx context.Context, provider, id string, entry ToolCatalogEntry) error

	// DeleteToolCatalogEntry removes an entry by (provider, id).
	DeleteToolCatalogEntry(ctx context.Context, provider, id string) error

	// ListToolCatalogPricing returns a minimal pricing projection of active tool_catalog rows.
	// Used by config.ToolCatalogEnricher to enrich GlobalConfig.ToolPricing at cache-load time.
	ListToolCatalogPricing(ctx context.Context) ([]config.ToolCatalogPricingRow, error)

	// PingDB sends a trivial query to the database and returns an error if unreachable.
	PingDB(ctx context.Context) error

	// ListTables returns all base table names in the public schema.
	ListTables(ctx context.Context) ([]string, error)

	// EncryptionConfigured reports whether field-level encryption is available.
	// Returns false when LOG_ENC_KEY_V* env vars are not set.
	EncryptionConfigured() bool
}

// SemanticAnchorRow is one row returned by ListSemanticAnchorsSorted.
type SemanticAnchorRow struct {
	Name            string
	RouteGroup      string
	PreferredModels []string
	Distance        float64
	VectorDims      int
	AnchorText      *string
	Modality        string
}

// SemanticAnchorMeta is one row returned by ListSemanticAnchorsPaged.
type SemanticAnchorMeta struct {
	Name            string
	RouteGroup      string
	PreferredModels []string
	AnchorText      *string
	VectorDims      int
	Modality        string
}

// SemanticAnchorPatch holds the fields that may be updated via PATCH.
// Only non-nil fields are written to the database.
type SemanticAnchorPatch struct {
	RouteGroup      *string
	PreferredModels *[]string
	AnchorText      *string
	Embedding       *[]float64 // set internally when anchor_text triggers re-embedding
	Modality        *string
}

// SemanticCacheScope controls which entries are eligible for a cache hit.
type SemanticCacheScope string

const (
	SemanticCacheScopeModel      SemanticCacheScope = "model"
	SemanticCacheScopeRouteGroup SemanticCacheScope = "route_group"
)

// SemanticCacheEntry is returned by FindNearestSemanticCache on a hit.
type SemanticCacheEntry struct {
	ID           uuid.UUID
	TenantID     string
	Model        string
	RouteGroup   string
	ResponseJSON json.RawMessage
	Similarity   float64 // 1 - cosine_distance
}

// SemanticCacheInsert holds data for a new cache entry.
type SemanticCacheInsert struct {
	TenantID     string
	Model        string
	RouteGroup   string
	Embedding    []float64
	RequestText  string
	ResponseJSON json.RawMessage
	ExpiresAt    time.Time
}

// ReplayRow is one row returned by GetReplayRequests for traffic replay simulation.
type ReplayRow struct {
	RequestID        string
	Timestamp        time.Time
	TenantID         string
	Model            string // originally selected model
	Strategy         string // routing strategy used
	PromptTokens     int    // from usage (0 if no usage row)
	CompletionTokens int
	CostUSD          float64         // original cost
	RoutingSnapshot  json.RawMessage // parsed to extract CandidateModels, RouteGroup
}

// ErrBudgetExceeded is returned when a tenant's monthly budget has been exceeded.
var ErrBudgetExceeded = fmt.Errorf("monthly budget exceeded")

// ErrAnchorAlreadyExists is returned when a semantic anchor with the same (tenant_id, name) already exists.
var ErrAnchorAlreadyExists = fmt.Errorf("anchor already exists")

// ErrRouteAlreadyExists is returned when a semantic route with the same (tenant_id, name) already exists.
var ErrRouteAlreadyExists = fmt.Errorf("semantic route already exists")

// ErrWorkflowAlreadyExists is returned when a decision workflow with the same workflow_id already exists.
var ErrWorkflowAlreadyExists = fmt.Errorf("workflow already exists")

// ErrWorkflowNotFound is returned when a decision workflow with the given workflow_id does not exist.
var ErrWorkflowNotFound = fmt.Errorf("workflow not found")

// DecisionWorkflowRow represents one row in the decision_workflows table (SPEC_167).
type DecisionWorkflowRow struct {
	WorkflowID          string
	WorkflowDescription string
	WorkflowMaxCost     float64
	WorkflowMaxSteps    int
	CreatedAt           time.Time
	UpdatedAt           time.Time
	CreatedBy           string
	UpdatedBy           string
}

// WorkflowConversationRow is one row of workflow_conversations (SPEC_169).
type WorkflowConversationRow struct {
	ConversationID     string
	TenantID           string
	WorkflowID         string
	CurrentTier        int // 0=premium 1=normal 2=low
	AccumulatedSteps   int
	AccumulatedCostUSD float64
	Blocked            bool
	LastActivityAt     time.Time
	CreatedAt          time.Time
	// Added migration 046
	StepsAction string // last steps-policy action: "none"|"degrade"|"block"
	CostAction  string // last cost-policy action:  "none"|"degrade"|"block"
	EntryModel  string // model selected on first call; never overwritten
	// Added migration 050 — optional business-context headers (preserve first-seen values)
	CustomerID      *string
	Channel         *string
	InteractionType *string
	Department      *string
	CustomerSegment *string
	// Added migration 051 — extended business-context headers (preserve first-seen values)
	Intent       *string
	ExperimentID *string
	AutonomyLevel *string
	RiskLevel    *string
}

// DialogStep is one request-response turn in a conversation.
type DialogStep struct {
	Timestamp       time.Time
	RequestID       string
	Model           string
	Provider        string
	LatencyMs       int
	Status          string
	PromptPreview   string
	ResponsePreview string
	PIIDetected     bool
}

// DialogExportRow is one step row for the dialog export CSV.
type DialogExportRow struct {
	// Conversation-level fields
	ConversationID     string
	WorkflowID         string
	TenantID           string
	EntryModel         string
	AccumulatedCostUSD float64
	Blocked            bool
	StepsAction        string
	CostAction         string
	CreatedAt          time.Time
	LastActivityAt     time.Time
	CustomerID         *string
	Channel            *string
	InteractionType    *string
	Department         *string
	CustomerSegment    *string
	Intent             *string
	ExperimentID       *string
	AutonomyLevel      *string
	RiskLevel          *string
	// Step-level fields
	StepNumber      int
	StepTimestamp   time.Time
	StepModel       string
	StepProvider    string
	StepLatencyMs   int
	StepStatus      string
	PromptPreview   string
	ResponsePreview string
	PIIDetected     bool
}

// ConversationDialog holds metadata + all steps for a conversation.
type ConversationDialog struct {
	ConversationID     string
	WorkflowID         string
	TenantID           string
	EntryModel         string
	AccumulatedSteps   int
	AccumulatedCostUSD float64
	CurrentTier        int
	Blocked            bool
	StepsAction        string
	CostAction         string
	LastActivityAt     time.Time
	CreatedAt          time.Time
	// Context headers
	CustomerID      *string
	Channel         *string
	InteractionType *string
	Department      *string
	CustomerSegment *string
	Intent          *string
	ExperimentID    *string
	AutonomyLevel   *string
	RiskLevel       *string
	// Steps ordered ASC by timestamp
	Steps []DialogStep
}

// WorkflowContextAttrs carries optional business-context headers for a conversation.
// All fields are nullable — populated only if the caller sends the corresponding HTTP header.
type WorkflowContextAttrs struct {
	CustomerID      *string
	Channel         *string
	InteractionType *string
	Department      *string
	CustomerSegment *string
	// Extended business-context headers (migration 051).
	Intent       *string
	ExperimentID *string
	AutonomyLevel *string
	RiskLevel    *string
}

// WorkflowAnalyticsRow holds per-workflow aggregated stats for SPEC_171.
type WorkflowAnalyticsRow struct {
	WorkflowID         string
	TotalConversations int
	TotalSteps         int64
	AvgSteps           float64
	P50Steps           float64
	P90Steps           float64
	P95Steps           float64
	MaxStepsObserved   int
	MinStepsObserved   int
	TotalCostUSD       float64
	AvgCostUSD         float64
	DegradeStepsCount  int
	DegradeCostCount   int
	BlockStepsCount    int
	BlockCostCount     int
	TotalBlocked       int
	TierPremium        int
	TierNormal         int
	TierLow            int
}

// WorkflowTenantRow holds per-workflow per-tenant aggregates for SPEC_171.
type WorkflowTenantRow struct {
	WorkflowID   string
	TenantID     string
	Conversations int
	TotalSteps   int64
	AvgSteps     float64
	TotalCostUSD float64
	DegradeCount int
	BlockCount   int
}

// WorkflowConversationSnapshot is one row in workflow_conversation_snapshots (SPEC_173).
// Written when a conversation is blocked (immediate) or when the TTL cleanup job expires it.
type WorkflowConversationSnapshot struct {
	ConversationID     string
	WorkflowID         string
	TenantID           string
	EntryModel         string
	FinalTier          int
	AccumulatedSteps   int
	AccumulatedCostUSD float64
	StepsAction        string
	CostAction         string
	Blocked            bool
	// Cost fields pre-calculated from usage at snapshot time.
	// nil means "not available yet" — GetWorkflowFinOps will recalculate from usage.
	ActualCostUSD    *float64
	AvgStepCostUSD   *float64
	MaxStepCostUSD   *float64
	FirstHalfAvgUSD  *float64
	SecondHalfAvgUSD *float64
	LastActivityAt   time.Time
}

// WorkflowFinOpsRow holds per-conversation cost data for SPEC_172 FinOps analytics.
// Produced by joining usage (actual costs) with workflow_conversations (policy state).
type WorkflowFinOpsRow struct {
	ConversationID     string
	WorkflowID         string
	TenantID           string
	EntryModel         string
	CurrentTier        int
	AccumulatedSteps   int
	AccumulatedCostUSD float64
	StepsAction        string
	CostAction         string
	Blocked            bool
	ActualCostUSD      float64 // SUM(cost_usd) from usage for this conversation
	AvgStepCostUSD     float64 // AVG(cost_usd) per step from usage
	MaxStepCostUSD     float64 // MAX(cost_usd) across steps (for worst-case block savings)
	FirstHalfAvgUSD    float64 // avg cost of first half of steps (trajectory detection)
	SecondHalfAvgUSD   float64 // avg cost of second half of steps
	LastActivityAt     time.Time
}

// ConversationStepCost holds per-step cost for one conversation (SPEC_172 drill-down).
type ConversationStepCost struct {
	Step    int     `json:"step"`
	Model   string  `json:"model"`
	CostUSD float64 `json:"cost_usd"`
}

// SemanticRouteRow is one distinct route (name-level, not utterance-level).
type SemanticRouteRow struct {
	Name        string
	Description string
	Action      string
	Threshold   float64
	CreatedAt   time.Time
}

// ModelBenchmarkRow represents one row in the model_benchmarks table.
type ModelBenchmarkRow struct {
	ID               uuid.UUID
	Ts               time.Time
	Provider         string
	Model            string
	Success          bool
	LatencyMs        int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	CostUSD          float64
	ErrorType        string
	BenchmarkName    string
}

// BenchmarkAggregate holds computed stats for one model over a time window.
type BenchmarkAggregate struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
	SuccessRate  float64 `json:"success_rate"`
	AvgCostUSD   float64 `json:"avg_cost_usd"`
	Samples      int     `json:"samples"`
}

// SemanticRouteDetail is a route with its full utterance list, used by PATCH and reads.
type SemanticRouteDetail struct {
	Name        string
	Description string
	Action      string
	Threshold   float64
	Utterances  []string
}

// SemanticRoutePatch holds the fields that may be updated via PATCH.
// Nil pointer fields are preserved from the current persisted state.
// Utterances nil = preserve existing; non-nil = replace entirely.
type SemanticRoutePatch struct {
	Description *string
	Action      *string
	Threshold   *float64
	Utterances  []string // nil = unchanged; non-nil = full replacement
}

// SemanticRouteMatch is returned by GetNearestSemanticRoute on a hit.
type SemanticRouteMatch struct {
	Name       string
	Action     string
	Similarity float64 // 1 - cosine_distance
	Threshold  float64 // stored per-route
}

// ReplayDiagnostics holds full diagnostics for a successful request (replay endpoint).
type ReplayDiagnostics struct {
	TenantID         string
	RoutingSnapshot  json.RawMessage
	DecisionReason   *string
	DecisionSnapshot json.RawMessage
	Strategy         string
}

// AnomalyListFilter filters ListAnomalies by window and optional tenant/model/provider/status; supports pagination.
type AnomalyListFilter struct {
	WindowHours int
	TenantID    string
	Model       string
	Provider    string
	Status      string
	Limit       int
	Offset      int
}

// AnomalyListRow is one row returned by ListAnomalies.
type AnomalyListRow struct {
	AnomalyID       string
	Timestamp       time.Time
	TenantID        string
	Model           string
	Provider        string
	ExpectedCostUSD float64
	ObservedCostUSD float64
	DeviationPct    float64
	Status          string
	AnomalyType     string
}

// ModelDriver is one top driver by model for anomaly explanation.
type ModelDriver struct {
	Model      string
	DeltaSpend float64
}

// ProviderDriver is one top driver by provider for anomaly explanation.
type ProviderDriver struct {
	Provider   string
	DeltaSpend float64
}

// APIKeyDriver is one top driver by API key name for anomaly explanation.
type APIKeyDriver struct {
	APIKeyName string
	DeltaSpend float64
}

// AnomalyTopDrivers holds top drivers per dimension for an anomaly.
type AnomalyTopDrivers struct {
	Models    []ModelDriver
	Providers []ProviderDriver
	APIKeys   []APIKeyDriver
}

// AnomalyExplanation holds one anomaly with top cost drivers (model, provider, api_key).
type AnomalyExplanation struct {
	TenantID      string
	ObservedSpend float64
	ExpectedSpend float64
	Multiplier    float64
	TopDrivers    AnomalyTopDrivers
}

// APIKeyUsageFilter filters GetAPIKeyUsage by window and optional tenant/provider/model/status.
type APIKeyUsageFilter struct {
	WindowHours int
	TenantID    string
	Provider    string
	Model       string
	Status      string
	APIKeyName  string
	Limit       int
	Offset      int
}

// APIKeyUsageSummary is the aggregate summary from GetAPIKeyUsage.
type APIKeyUsageSummary struct {
	TotalActiveAPIKeys int
	TotalRequests      int
	TotalSpend         float64
	AvgSuccessRate     float64
	HighestSpendKey    string
	MostActiveKey      string
}

// APIKeyModelUsageRow is one (api_key_id, model) breakdown row used for effective cost computation.
type APIKeyModelUsageRow struct {
	APIKeyID uuid.UUID
	TenantID string
	Model    string
	Requests int
	Spend    float64
}

// APIKeyUsageRow is one per-key row from GetAPIKeyUsage.
type APIKeyUsageRow struct {
	APIKeyID     uuid.UUID
	APIKeyName   string
	TenantID     string
	Requests     int
	Spend        float64
	SuccessRate  float64
	AvgLatencyMs float64
	TopModel     string
	TopProvider  string
	LastSeen     time.Time
}

// APIKeyUsageDetailSummary is the summary for one API key drilldown.
type APIKeyUsageDetailSummary struct {
	Requests     int
	Spend        float64
	AvgLatencyMs float64
	SuccessRate  float64
	TopModel     string
	TopProvider  string
	LastSeen     time.Time
}

// APIKeyUsageByModelRow is one row of usage by model for an API key.
type APIKeyUsageByModelRow struct {
	Model    string
	Requests int
	Spend    float64
}

// APIKeyUsageByProviderRow is one row of usage by provider for an API key.
type APIKeyUsageByProviderRow struct {
	Provider string
	Requests int
}

// APIKeyUsageRecentRow is one recent request row for an API key.
type APIKeyUsageRecentRow struct {
	Timestamp time.Time
	RequestID string
	Model     string
	Provider  string
	Status    string
	LatencyMs int
	CostUSD   float64
}

// LatencyStats holds P50/P95/Max latency for an API key window.
type LatencyStats struct {
	P50 int
	P95 int
	Max int
}

// ErrorCountRow is one row of error counts by type.
type ErrorCountRow struct {
	ErrorType string
	Count     int
}

// AnomalyStatsSummary is the summary part of AnomalyStats.
type AnomalyStatsSummary struct {
	ActiveAnomalies int
	CostSpike24hUSD float64
	AffectedTenants int
	AffectedModels  int
}

// AnomalyTimelineBucket is one time bucket in anomaly stats timeline.
type AnomalyTimelineBucket struct {
	Bucket          time.Time
	Anomalies       int
	ObservedCostUSD float64
}

// AnomalyTopTenant is one tenant in top-tenants by anomaly count.
type AnomalyTopTenant struct {
	TenantID  string
	Anomalies int
}

// AnomalyDeviationBucket is one bucket in the deviation histogram.
type AnomalyDeviationBucket struct {
	Range string
	Count int
}

// AnomalyStats is the full anomaly dashboard stats (summary, timeline, top tenants, histogram).
type AnomalyStats struct {
	WindowHours        int
	Summary            AnomalyStatsSummary
	Timeline           []AnomalyTimelineBucket
	TopTenants         []AnomalyTopTenant
	DeviationHistogram []AnomalyDeviationBucket
}

// ConfigHistoryFilter filters ListConfigHistory by scope (global/tenant) and pagination.
type ConfigHistoryFilter struct {
	Scope    string // "global" or "tenant"
	TenantID string
	Limit    int
	Offset   int
	// ExcludeGlobal, when true, never reads global_config_change_log (RBAC for local_admin/user/finance JWT).
	ExcludeGlobal bool
	// AllowedTenantIDs, when non-empty with ExcludeGlobal, restricts tenant rows to these IDs.
	AllowedTenantIDs []string
}

// ConfigHistoryRow is one row from ListConfigHistory.
type ConfigHistoryRow struct {
	Scope       string
	TenantID    string
	ChangedAt   time.Time
	ChangedBy   string
	FromVersion int
	ToVersion   int
	IsRollback  bool
	ChangeType  string
}

// ConfigVersionFilter filters ListConfigVersions by scope and pagination.
type ConfigVersionFilter struct {
	Scope    string
	TenantID string
	Limit    int
	Offset   int
}

// ConfigVersionRow is one row from ListConfigVersions.
type ConfigVersionRow struct {
	Version   int
	CreatedAt time.Time
}

// RequestLogRecentFilter filters ListRequestLogRecent by time range and optional fields.
type RequestLogRecentFilter struct {
	From           *time.Time
	To             *time.Time
	TenantID       *string
	JWTSub         *string
	Model          *string
	Provider       *string
	Status         *string
	FallbackUsed   *bool
	Strategy       *string
	// SPEC_170: workflow/conversation drill-down filters
	WorkflowID     *string
	ConversationID *string
}

// RequestLogRecentRow is one row from ListRequestLogRecent.
type RequestLogRecentRow struct {
	RequestID        string
	Timestamp        time.Time
	TenantID         string
	JWTSub           string
	APIKeyID         string
	APIKeyName       string
	Model            string
	Provider         string
	Strategy         string
	LatencyMs        int
	Status           string
	FallbackUsed     bool
	Attempt          int
	DecisionReason   string
	ErrorType        string
	Error            string
	RoutingSnapshot  json.RawMessage
	DecisionSnapshot json.RawMessage
	Metadata         json.RawMessage
	// SPEC_170: populated when request originated from a decision_ops workflow
	WorkflowID     string
	ConversationID string
}

// APIKeyRawUsageFilter filters ListAPIKeyRawUsage by time range and optional tenant/model/key name.
type APIKeyRawUsageFilter struct {
	From       *time.Time
	To         *time.Time
	TenantID   string
	APIKeyName string
	Model      string
	Provider   string
	Status     string
	Limit      int
	Offset     int
}

// APIKeyRawUsageRow is one raw usage row from ListAPIKeyRawUsage.
type APIKeyRawUsageRow struct {
	Timestamp        time.Time
	TenantID         string
	APIKeyID         uuid.UUID
	APIKeyName       string
	RequestID        string
	Model            string
	Provider         string
	Status           string
	LatencyMs        int
	CostUSD          float64
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// JWTSubUsageFilter filters aggregated JWT subject usage.
type JWTSubUsageFilter struct {
	From      *time.Time
	To        *time.Time
	TenantID  string
	Model     string
	Provider  string
	Limit     int
	Offset    int
	SortBy    string
	SortOrder string
}

// JWTSubModelUsageRow is one (jwt_sub, tenant_id, model) breakdown row for monetization computation.
type JWTSubModelUsageRow struct {
	JWTSub   string
	TenantID string
	Model    string
	Requests int
	Spend    float64
}

// JWTSubUsageRow is one aggregated usage row grouped by (jwt_sub, tenant_id).
type JWTSubUsageRow struct {
	JWTSub           string
	TenantID         string
	Requests         int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	TotalCostUSD     float64
	FirstSeen        time.Time
	LastSeen         time.Time
}

// JWTSubUsageDetailFilter filters detailed usage for one jwt_sub.
type JWTSubUsageDetailFilter struct {
	From     *time.Time
	To       *time.Time
	TenantID string
	GroupBy  string
}

// JWTSubUsageDetailSummary is the summary for one jwt_sub.
type JWTSubUsageDetailSummary struct {
	Requests         int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	TotalCostUSD     float64
}

// JWTSubUsageBreakdownRow is one breakdown row grouped by model/provider/day.
type JWTSubUsageBreakdownRow struct {
	Group        string
	Requests     int
	TotalTokens  int
	TotalCostUSD float64
}

// JWTSubRawUsageFilter filters raw request rows by jwt_sub.
type JWTSubRawUsageFilter struct {
	From     *time.Time
	To       *time.Time
	TenantID string
	JWTSub   string
	Model    string
	Provider string
	Status   string
	Limit    int
	Offset   int
}

// JWTSubRawUsageRow is one raw request row attributed to jwt_sub.
type JWTSubRawUsageRow struct {
	Timestamp        time.Time
	TenantID         string
	JWTSub           string
	RequestID        string
	Model            string
	Provider         string
	Status           string
	LatencyMs        int
	CostUSD          float64
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// RequestStatsSummary is the summary part of RequestStats.
type RequestStatsSummary struct {
	TotalRequests    int
	FallbackRate     float64
	FallbackRequests int
	CacheHitRate     *float64
	SuccessRate      float64
	AvgLatencyMs     float64
}

// TrafficBucket is one time bucket in request stats traffic.
type TrafficBucket struct {
	Bucket    time.Time
	Requests  int
	Successes int
	Errors    int
}

// ProviderHealthRow is one row of provider health in RequestStats.
type ProviderHealthRow struct {
	Provider      string
	TotalRequests int
	AvgLatencyMs  float64
	SuccessRate   float64
}

// RequestStats is aggregated request telemetry (summary, traffic over time, provider health, status breakdown).
type RequestStats struct {
	WindowHours     int
	Summary         RequestStatsSummary
	TrafficOverTime []TrafficBucket
	ProviderHealth  []ProviderHealthRow
	StatusBreakdown map[string]int
}

// RouterPerformanceFilter scopes router performance metrics.
type RouterPerformanceFilter struct {
	From     *time.Time
	To       *time.Time
	TenantID *string
	Model    *string
	Provider *string
	Status   *string
	Bucket   string
}

// RouterPerformanceSummary is the summary metrics for router performance.
type RouterPerformanceSummary struct {
	Requests int

	AvgRouterPreMs float64
	MinRouterPreMs float64
	MaxRouterPreMs float64
	P50RouterPreMs float64
	P95RouterPreMs float64

	AvgLLMLatencyMs float64
	MinLLMLatencyMs float64
	MaxLLMLatencyMs float64
	P50LLMLatencyMs float64
	P95LLMLatencyMs float64

	AvgRouterPostMs float64
	MinRouterPostMs float64
	MaxRouterPostMs float64
	P50RouterPostMs float64
	P95RouterPostMs float64

	AvgTotalLatencyMs float64
	P50TotalLatencyMs float64
	P95TotalLatencyMs float64

	SuccessRate float64
	ErrorRate   float64

	AvgPreTenantConfigMs    float64
	AvgCfgToolRoutesMs      float64
	AvgCfgDynamicRoutesMs   float64
	AvgCfgDecisionOpsMs     float64
	AvgCfgBudgetPressureMs  float64
	AvgCfgSemanticMs        float64
	AvgCfgModelResolutionMs float64
}

// RouterPerformanceTimeseriesRow is one time bucket of router performance.
type RouterPerformanceTimeseriesRow struct {
	BucketStart       time.Time
	Requests          int
	AvgRouterPreMs    float64
	AvgLLMLatencyMs   float64
	AvgRouterPostMs   float64
	AvgTotalLatencyMs float64
	P95RouterPreMs    float64
	P95LLMLatencyMs   float64
	P95RouterPostMs   float64
}

// RouterPreBreakdownAvgMs holds averaged router pre-stage breakdowns.
type RouterPreBreakdownAvgMs struct {
	TenantConfig    float64
	ToolRoutes      float64
	DynamicRoutes   float64
	DecisionOps     float64
	BudgetPressure  float64
	Semantic        float64
	ModelResolution float64
}

// RouterToolRoutesBreakdownAvgMs holds averaged tool routes breakdowns.
type RouterToolRoutesBreakdownAvgMs struct {
	EmbeddingModel    float64
	EmbeddingGenerate float64
	SemanticDB        float64
	MatchEval         float64
}

// RouterPerformanceBreakdowns is the breakdowns section of router performance.
type RouterPerformanceBreakdowns struct {
	PreBreakdownAvgMs        RouterPreBreakdownAvgMs
	ToolRoutesBreakdownAvgMs RouterToolRoutesBreakdownAvgMs
}

// RouterPerformanceMetrics is the full router performance response data.
type RouterPerformanceMetrics struct {
	Summary    RouterPerformanceSummary
	Timeseries []RouterPerformanceTimeseriesRow
	Breakdowns RouterPerformanceBreakdowns
}

// SemanticRoutingCoverage is the coverage part of SemanticRoutingStats.
type SemanticRoutingCoverage struct {
	TotalRequests   int
	MatchedRequests int
	CoverageRate    float64
}

// SemanticRoutingTopRoute is one top route in SemanticRoutingStats.
type SemanticRoutingTopRoute struct {
	RouteGroup string
	Matches    int
}

// SemanticRoutingTopAnchor is one top anchor in SemanticRoutingStats.
type SemanticRoutingTopAnchor struct {
	Anchor  string
	Matches int
}

// SemanticRoutingStats holds semantic routing analytics (coverage, top routes, top anchors).
type SemanticRoutingStats struct {
	TopRoutes  []SemanticRoutingTopRoute
	TopAnchors []SemanticRoutingTopAnchor
	Coverage   SemanticRoutingCoverage
}

// ErrConfigVersionNotFound is returned when the requested config version does not exist.
var ErrConfigVersionNotFound = fmt.Errorf("config version not found")

// SemanticCorrelationByRouteGroup is one route_group row in SemanticCorrelation.
type SemanticCorrelationByRouteGroup struct {
	RouteGroup    string
	CacheHits     int64
	TotalRequests int
	HitRate       float64
}

// SemanticCorrelation correlates semantic cache hits and request counts by route_group.
type SemanticCorrelation struct {
	ByRouteGroup []SemanticCorrelationByRouteGroup
}

// SemanticCacheStatsSummary is the summary part of SemanticCacheStats.
type SemanticCacheStatsSummary struct {
	TotalEntries    int
	TotalHits       int64
	AvgHitsPerEntry float64
	ActiveEntries   int
	ExpiredEntries  int
}

// SemanticCacheTopPrompt is one top prompt row.
type SemanticCacheTopPrompt struct {
	RequestText string
	HitCount    int64
	LastHitAt   *time.Time
	ExpiresAt   time.Time
	Model       string
	RouteGroup  string
}

// SemanticCacheTopModel is one top model row.
type SemanticCacheTopModel struct {
	Model     string
	Entries   int
	TotalHits int64
}

// SemanticCacheTopRouteGroup is one top route group row.
type SemanticCacheTopRouteGroup struct {
	RouteGroup string
	Entries    int
	TotalHits  int64
}

// SemanticCacheExpiration holds active/expired counts.
type SemanticCacheExpiration struct {
	Active  int
	Expired int
}

// SemanticCacheStats holds semantic cache analytics (summary, top prompts/models/route groups).
type SemanticCacheStats struct {
	Summary        SemanticCacheStatsSummary
	TopPrompts     []SemanticCacheTopPrompt
	TopModels      []SemanticCacheTopModel
	TopRouteGroups []SemanticCacheTopRouteGroup
	Expiration     SemanticCacheExpiration
}

// CacheSavingsSummary is the summary part of CacheSavings.
type CacheSavingsSummary struct {
	TotalHits             int64
	EstimatedCostSavedUSD float64
	AvgCostPerRequest     float64
}

// CacheSavingsByModel is one model row in CacheSavings.
type CacheSavingsByModel struct {
	Model    string
	SavedUSD float64
}

// CacheSavings holds estimated cost savings from semantic cache.
type CacheSavings struct {
	Summary CacheSavingsSummary
	ByModel []CacheSavingsByModel
}

// ErrVersionConflict is returned when optimistic concurrency check fails
type ErrVersionConflict struct {
	Expected int
	Current  int
}

func (e ErrVersionConflict) Error() string {
	return fmt.Sprintf("version conflict: expected %d, current %d", e.Expected, e.Current)
}

// ErrTenantAlreadyExists is returned when creating a tenant that already exists.
type ErrTenantAlreadyExists struct {
	TenantID string
}

func (e ErrTenantAlreadyExists) Error() string {
	return fmt.Sprintf("tenant %q already exists", e.TenantID)
}

// CustomerInteraction is one row in the customer_interactions table.
type CustomerInteraction struct {
	ID           uuid.UUID `json:"id"`
	TenantID     string    `json:"tenant_id"`
	SourceSystem string    `json:"source_system"`
	OccurredAt   time.Time `json:"occurred_at"`
	CreatedAt    time.Time `json:"created_at"`
	// Context fields — all nullable.
	CustomerID      *string `json:"customer_id,omitempty"`
	Channel         *string `json:"channel,omitempty"`
	InteractionType *string `json:"interaction_type,omitempty"`
	AgentID         *string `json:"agent_id,omitempty"`
	Department      *string `json:"department,omitempty"`
	TicketID        *string `json:"ticket_id,omitempty"`
	CustomerSegment *string `json:"customer_segment,omitempty"`
	Language        *string `json:"language,omitempty"`
	Intent          *string `json:"intent,omitempty"`
	ExperimentID    *string `json:"experiment_id,omitempty"`
	AutonomyLevel   *string `json:"autonomy_level,omitempty"`
	PolicyID        *string `json:"policy_id,omitempty"`
	RiskLevel       *string `json:"risk_level,omitempty"`
	RevenueImpact   *string `json:"revenue_impact,omitempty"`
	Currency        *string `json:"currency,omitempty"`
	// Outcome fields — all nullable.
	Outcome          *string `json:"outcome,omitempty"`
	ResolutionTimeMS *int    `json:"resolution_time_ms,omitempty"`
	CSAT             *string `json:"csat,omitempty"`
	FeedbackLoop     *bool   `json:"feedback_loop,omitempty"`
	// Links to AI conversations — nullable.
	WorkflowID     *string `json:"workflow_id,omitempty"`
	ConversationID *string `json:"conversation_id,omitempty"`
	// Free-form metadata.
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// CustomerInteractionFilter restricts the rows returned by ListCustomerInteractions.
type CustomerInteractionFilter struct {
	TenantID     *string
	CustomerID   *string
	Channel      *string
	SourceSystem *string
	From         *time.Time
	To           *time.Time
}

// ModelCatalogEntry represents a single row in the model_catalog table.
type ModelCatalogEntry struct {
	ID                           string    `json:"id"`
	Provider                     string    `json:"provider"`
	DisplayName                  string    `json:"display_name"`
	Type                         string    `json:"type"`
	PromptPer1M                  float64   `json:"prompt_per_1m"`
	CachedInputPer1M             float64   `json:"cached_input_per_1m"`
	CompletionPer1M              float64   `json:"completion_per_1m"`
	InfrastructureMonthlyUSD     float64   `json:"infrastructure_monthly_usd"`
	IsActive                     bool      `json:"is_active"`
	LongContext                  bool      `json:"long_context"`
	LongContextStartTokens       int       `json:"long_context_start_tokens"`
	LongContextPromptPer1M       float64   `json:"long_context_prompt_per_1m"`
	LongContextCachedInputPer1M  float64   `json:"long_context_cached_input_per_1m"`
	LongContextCompletionPer1M   float64   `json:"long_context_completion_per_1m"`
	CreatedAt                    time.Time `json:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at"`
}

// ModelCatalogFilter restricts the rows returned by ListModelCatalog.
type ModelCatalogFilter struct {
	Provider *string
	Type     *string
	IsActive *bool
}

// ToolCatalogEntry represents one row in the tool_catalog table.
type ToolCatalogEntry struct {
	ID           string  `db:"id"             json:"id"`
	Provider     string  `db:"provider"       json:"provider"`
	DisplayName  string  `db:"display_name"   json:"display_name"`
	ToolType     string  `db:"tool_type"      json:"tool_type"`
	Unit         string  `db:"unit"           json:"unit"`
	PricePerUnit float64 `db:"price_per_unit" json:"price_per_unit"`
	IsActive     bool    `db:"is_active"      json:"is_active"`
}

// ToolCatalogFilter holds optional filters for ListToolCatalog.
type ToolCatalogFilter struct {
	Provider *string
	IsActive *bool
}

// ErrToolAlreadyExists is returned when attempting to create a tool catalog
// entry whose (provider, id) primary key already exists.
var ErrToolAlreadyExists = errors.New("tool catalog entry already exists")

// ErrNotFound is a generic sentinel returned when a requested resource does
// not exist in the database.
var ErrNotFound = errors.New("not found")

// NopStorage is a no-op implementation used when no database is configured.
type NopStorage struct{}

func (NopStorage) LogRequest(ctx context.Context, log RequestLog) error { return nil }
func (NopStorage) LogConversation(ctx context.Context, row ConversationLog) error {
	return nil
}
func (NopStorage) LogComplianceEvent(ctx context.Context, event ComplianceEventLog) error {
	return nil
}
func (NopStorage) SaveUsage(ctx context.Context, usage UsageRecord) error { return nil }
func (NopStorage) CheckAndReserveBudget(ctx context.Context, tenantID string, monthStart, monthEnd time.Time, limitUSD, estimatedCost float64) (BudgetCheck, error) {
	return BudgetCheck{}, nil
}
func (NopStorage) ReleaseReservation(ctx context.Context, reservationID uuid.UUID) error {
	return nil
}
func (NopStorage) PurgeExpiredReservations(ctx context.Context) (int64, error) { return 0, nil }
func (NopStorage) GetMonthlyReservedSpend(ctx context.Context, tenantID string, monthStart, monthEnd time.Time) (float64, error) {
	return 0, nil
}
func (NopStorage) UpsertModelStatDaily(ctx context.Context, stat ModelStatDaily) error {
	return nil
}
func (NopStorage) GetModelStats(ctx context.Context, tenantID string, windowDays int) ([]ModelStatDaily, error) {
	return []ModelStatDaily{}, nil
}
func (NopStorage) GetUsageSummary(ctx context.Context, tenantID string, month time.Time) (UsageSummary, error) {
	return UsageSummary{ModelBreakdown: make(map[string]ModelUsage)}, nil
}
func (NopStorage) GetBudgetForecast(ctx context.Context, tenantID string, month time.Time, budgetLimit float64) (BudgetForecast, error) {
	return BudgetForecast{}, nil
}
func (NopStorage) GetSmartImpactData(ctx context.Context, tenantID string, from, to time.Time) (SmartImpactData, error) {
	return SmartImpactData{}, nil
}
func (NopStorage) GetAuditRecords(ctx context.Context, tenantID string, from, to time.Time) ([]AuditRecord, error) {
	return []AuditRecord{}, nil
}
func (NopStorage) StreamBillingLineItems(_ context.Context, _ string, _, _ time.Time, _ func(BillingLineItem) error) error {
	return nil
}
func (NopStorage) GetBillingGrouped(_ context.Context, _ string, _, _ time.Time, _ string) ([]BillingGroupedRow, error) {
	return []BillingGroupedRow{}, nil
}
func (NopStorage) GetUsageByTag(_ context.Context, _ string, _, _ time.Time, _ string) ([]UsageByTagRow, error) {
	return []UsageByTagRow{}, nil
}
func (NopStorage) DeleteOldRecords(ctx context.Context, tenantID string, cutoffDate time.Time) (int, error) {
	return 0, nil
}
func (NopStorage) InsertBudgetAlert(ctx context.Context, alert BudgetAlert) (bool, error) {
	return false, nil
}
func (NopStorage) GetBudgetAlerts(ctx context.Context, tenantID, month string) ([]BudgetAlert, error) {
	return []BudgetAlert{}, nil
}
func (NopStorage) InsertCostAnomaly(ctx context.Context, anomaly CostAnomaly) error {
	return nil
}
func (NopStorage) GetCostAnomalies(ctx context.Context, tenantID string, windowDays int) ([]CostAnomaly, error) {
	return []CostAnomaly{}, nil
}

// Dynamic config methods (NopStorage - no database)
func (NopStorage) GetTenantConfig(ctx context.Context, tenantID string) (json.RawMessage, int, bool, error) {
	return nil, 0, false, nil
}

func (NopStorage) PutTenantConfig(ctx context.Context, tenantID string, ifMatchVersion int, newConfigJSON json.RawMessage, actorSub string, actorRoles []string, summary string, diffJSON json.RawMessage) (int, error) {
	return 0, fmt.Errorf("dynamic config not supported without database")
}

func (NopStorage) PatchTenantConfig(ctx context.Context, tenantID string, ifMatchVersion int, mergePatchJSON json.RawMessage, actorSub string, actorRoles []string) (int, error) {
	return 0, fmt.Errorf("dynamic config not supported without database")
}

func (NopStorage) ListTenantConfigChanges(ctx context.Context, tenantID string, limit int) ([]ConfigChange, error) {
	return nil, nil
}

func (NopStorage) SeedTenantConfig(ctx context.Context, tenantID string, configJSON json.RawMessage) (bool, error) {
	return false, nil // No database - can't seed
}

func (NopStorage) GetGlobalConfig(ctx context.Context) (json.RawMessage, int, bool, error) {
	return nil, 0, false, nil
}

func (NopStorage) PutGlobalConfig(_ context.Context, _ int, _ json.RawMessage, _ string, _ []string) (int, error) {
	return 0, fmt.Errorf("global config not supported without database")
}

func (NopStorage) PatchGlobalConfig(_ context.Context, _ int, _ json.RawMessage, _ string, _ []string) (int, error) {
	return 0, fmt.Errorf("global config not supported without database")
}

func (NopStorage) RollbackGlobalConfig(_ context.Context, _ int, _ int, _ string, _ []string) error {
	return fmt.Errorf("global config not supported without database")
}

func (NopStorage) SeedGlobalConfig(ctx context.Context, configJSON json.RawMessage) (bool, error) {
	return false, nil
}

func (NopStorage) SeedTenantVersionedConfig(ctx context.Context, tenantID string, configJSON json.RawMessage, seedMode string) (bool, error) {
	return false, nil
}

func (NopStorage) SeedAPIKeyFromYAML(ctx context.Context, tenantID, apiKey string) (bool, error) {
	return false, nil
}

// API Key methods (NopStorage - no database)
func (NopStorage) CreateAPIKey(ctx context.Context, tenantID, name string, scopes []string, expiresAt *time.Time, actorSub string, actorRoles []string) (APIKeyCreateResult, error) {
	return APIKeyCreateResult{}, fmt.Errorf("api key management not supported without database")
}

func (NopStorage) CountAPIKeys(ctx context.Context) (int, error) {
	return 0, nil // NopStorage pretends table is empty
}

func (NopStorage) ListAPIKeys(ctx context.Context, tenantID string) ([]APIKeyMeta, error) {
	return []APIKeyMeta{}, nil
}

func (NopStorage) ListAPIKeysPaged(ctx context.Context, tenantID string, includeRevoked bool, limit, offset int) ([]APIKeyMeta, bool, error) {
	return []APIKeyMeta{}, false, nil
}

func (NopStorage) RevokeAPIKey(ctx context.Context, tenantID string, keyID uuid.UUID, actorSub string, actorRoles []string) (*time.Time, error) {
	return nil, fmt.Errorf("api key management not supported without database")
}

func (NopStorage) RotateAPIKey(ctx context.Context, tenantID string, keyID uuid.UUID, actorSub string, actorRoles []string) (uuid.UUID, APIKeyCreateResult, error) {
	return uuid.Nil, APIKeyCreateResult{}, fmt.Errorf("api key management not supported without database")
}

func (NopStorage) LookupAPIKeyByHash(ctx context.Context, keyHash string) (APIKeyRecord, bool, error) {
	return APIKeyRecord{}, false, nil
}

func (NopStorage) TouchAPIKeyLastUsed(ctx context.Context, keyID uuid.UUID, ts time.Time) error {
	return nil
}

func (NopStorage) GetNearestSemanticAnchor(_ context.Context, _ string, _ []float64, _ string) (string, string, []string, float64, bool, error) {
	return "", "", nil, 0, false, nil
}

func (NopStorage) UpsertSemanticAnchor(_ context.Context, _, _ string, _ []float64, _ string, _ []string, _ *string, _ string) error {
	return fmt.Errorf("semantic anchors not supported without database")
}

func (NopStorage) ListSemanticAnchorsSorted(_ context.Context, _ string, _ []float64, _ int, _ string) ([]SemanticAnchorRow, error) {
	return []SemanticAnchorRow{}, nil
}

func (NopStorage) ListSemanticAnchorsPaged(_ context.Context, _ string, _ bool, _, _ int) ([]SemanticAnchorMeta, bool, error) {
	return []SemanticAnchorMeta{}, false, nil
}

func (NopStorage) UpdateSemanticAnchor(_ context.Context, _, _ string, _ SemanticAnchorPatch) (bool, error) {
	return false, fmt.Errorf("semantic anchors not supported without database")
}

func (NopStorage) DeleteSemanticAnchor(_ context.Context, _, _ string) (bool, error) {
	return false, fmt.Errorf("semantic anchors not supported without database")
}

func (NopStorage) GetRoutingSnapshot(_ context.Context, _ string) (string, json.RawMessage, bool, error) {
	return "", nil, false, nil
}

func (NopStorage) FindNearestSemanticCache(_ context.Context, _ string, _ []float64, _ SemanticCacheScope, _, _ string, _ float64) (*SemanticCacheEntry, bool, error) {
	return nil, false, nil
}
func (NopStorage) InsertSemanticCacheEntry(_ context.Context, _ SemanticCacheInsert) error {
	return nil
}
func (NopStorage) TouchSemanticCacheHit(_ context.Context, _ uuid.UUID, _ time.Time) error {
	return nil
}
func (NopStorage) PruneExpiredSemanticCache(_ context.Context, _ string) error { return nil }
func (NopStorage) GetReplayRequests(_ context.Context, _ string, _, _ time.Time, _ int) ([]ReplayRow, error) {
	return []ReplayRow{}, nil
}

func (NopStorage) GetMonthlySpend(_ context.Context, _ string, _, _ time.Time) (float64, error) {
	return 0, nil
}

func (NopStorage) GetTagMonthlySpend(_ context.Context, _, _, _ string, _, _ time.Time) (float64, error) {
	return 0, nil
}

func (NopStorage) CreateSemanticRoute(_ context.Context, _, _, _, _ string, _ float64, _ []string, _ [][]float64) error {
	return fmt.Errorf("semantic routes not supported without database")
}

func (NopStorage) GetNearestSemanticRoute(_ context.Context, _ string, _ []float64) (SemanticRouteMatch, bool, error) {
	return SemanticRouteMatch{}, false, nil
}

func (NopStorage) DeleteSemanticRoute(_ context.Context, _, _ string) (bool, error) {
	return false, fmt.Errorf("semantic routes not supported without database")
}

func (NopStorage) ListSemanticRoutes(_ context.Context, _ string) ([]SemanticRouteRow, error) {
	return []SemanticRouteRow{}, nil
}

func (NopStorage) GetSemanticRoute(_ context.Context, _, _ string) (SemanticRouteDetail, bool, error) {
	return SemanticRouteDetail{}, false, nil
}

func (NopStorage) UpdateSemanticRoute(_ context.Context, _, _ string, _ SemanticRoutePatch, _ [][]float64) (bool, error) {
	return false, fmt.Errorf("semantic routes not supported without database")
}

func (NopStorage) CreateTenant(_ context.Context, _ string, _ json.RawMessage, _ string, _ []string) error {
	return fmt.Errorf("tenant management not supported without database")
}

func (NopStorage) DeleteTenant(_ context.Context, _ string) (bool, error) {
	return false, fmt.Errorf("tenant management not supported without database")
}

func (NopStorage) GetTenantUsageOverview(_ context.Context, _ string, _, _ time.Time) (TenantUsageOverview, error) {
	return TenantUsageOverview{}, nil
}

func (NopStorage) GetModelRequestCounts(_ context.Context, _ int) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (NopStorage) ListRecentRequests(_ context.Context, _ string, _, _, _ int) ([]RequestListRow, bool, error) {
	return []RequestListRow{}, false, nil
}

func (NopStorage) ListTenants(_ context.Context) ([]string, error) {
	return []string{}, nil
}

func (NopStorage) InsertModelBenchmark(_ context.Context, _ ModelBenchmarkRow) error {
	return nil
}

func (NopStorage) GetClaudeCodeMonthlySpend(_ context.Context, _ string, _, _ time.Time) (float64, error) {
	return 0, nil
}

func (NopStorage) GetClaudeCodeUsageSummary(_ context.Context, _ ClaudeCodeUsageFilter) (ClaudeCodeUsageSummary, error) {
	return ClaudeCodeUsageSummary{}, nil
}

func (NopStorage) GetClaudeCodeUsageTimeseries(_ context.Context, _ ClaudeCodeUsageFilter) ([]ClaudeCodeTimeseriesBucket, error) {
	return nil, nil
}

func (NopStorage) GetClaudeCodeUsageRows(_ context.Context, _ ClaudeCodeUsageFilter) ([]ClaudeCodeUsageRow, int64, error) {
	return nil, 0, nil
}

func (NopStorage) InsertAnthropicMessageLog(_ context.Context, _ AnthropicMessageLog) error {
	return nil
}

func (NopStorage) GetModelBenchmarkAggregates(_ context.Context, _ int) ([]BenchmarkAggregate, error) {
	return []BenchmarkAggregate{}, nil
}

func (NopStorage) DeleteOldModelBenchmarks(_ context.Context, _ int) (int64, error) {
	return 0, nil
}

func (NopStorage) TruncateModelBenchmarks(_ context.Context) (int64, error) {
	return 0, nil
}

func (NopStorage) GetReplayDiagnostics(_ context.Context, _ string) (ReplayDiagnostics, bool, error) {
	return ReplayDiagnostics{}, false, nil
}

func (NopStorage) ListDialogExportRows(_ context.Context, _ string, _ time.Time, _ int) ([]DialogExportRow, error) {
	return nil, nil
}

func (NopStorage) ListAnomalies(_ context.Context, _ AnomalyListFilter) ([]AnomalyListRow, int, error) {
	return nil, 0, nil
}

func (NopStorage) GetAnomalyExplanations(_ context.Context, _ int) ([]AnomalyExplanation, error) {
	return nil, nil
}

func (NopStorage) GetAPIKeyUsage(_ context.Context, _ APIKeyUsageFilter) (APIKeyUsageSummary, []APIKeyUsageRow, error) {
	return APIKeyUsageSummary{}, nil, nil
}
func (NopStorage) GetAPIKeyModelBreakdown(_ context.Context, _ APIKeyUsageFilter) ([]APIKeyModelUsageRow, error) {
	return nil, nil
}
func (NopStorage) GetTenantModelRequestCounts(_ context.Context, _ string, _ time.Time) (map[string]int64, error) {
	return nil, nil
}

func (NopStorage) GetAPIKeyUsageDetail(_ context.Context, _ uuid.UUID, _, _, _ int) (APIKeyUsageDetailSummary, []APIKeyUsageByModelRow, []APIKeyUsageByProviderRow, []APIKeyUsageRecentRow, int, LatencyStats, []ErrorCountRow, error) {
	return APIKeyUsageDetailSummary{}, nil, nil, nil, 0, LatencyStats{}, nil, nil
}

func (NopStorage) GetAnomalyStats(_ context.Context, _ int, _, _, _ string) (AnomalyStats, error) {
	return AnomalyStats{}, nil
}

func (NopStorage) ListConfigHistory(_ context.Context, _ ConfigHistoryFilter) ([]ConfigHistoryRow, error) {
	return nil, nil
}

func (NopStorage) ListConfigVersions(_ context.Context, _ ConfigVersionFilter) ([]ConfigVersionRow, error) {
	return nil, nil
}

func (NopStorage) ListRequestLogRecent(_ context.Context, _ RequestLogRecentFilter, _, _ int) ([]RequestLogRecentRow, int, error) {
	return nil, 0, nil
}

func (NopStorage) ListComplianceEvents(_ context.Context, _ ComplianceEventFilter, _, _ int) ([]ComplianceEventLog, int, error) {
	return nil, 0, nil
}
func (NopStorage) ListConversations(_ context.Context, _ ConversationLogFilter, _, _ int) ([]ConversationLog, int, error) {
	return nil, 0, nil
}

func (NopStorage) ListAPIKeyRawUsage(_ context.Context, _ APIKeyRawUsageFilter) ([]APIKeyRawUsageRow, int, error) {
	return nil, 0, nil
}

func (NopStorage) GetJWTSubModelBreakdown(_ context.Context, _ JWTSubUsageFilter) ([]JWTSubModelUsageRow, error) {
	return nil, nil
}
func (NopStorage) GetJWTSubUsage(_ context.Context, _ JWTSubUsageFilter) ([]JWTSubUsageRow, int, error) {
	return nil, 0, nil
}

func (NopStorage) GetJWTSubUsageDetail(_ context.Context, _ string, _ JWTSubUsageDetailFilter) (JWTSubUsageDetailSummary, []JWTSubUsageBreakdownRow, error) {
	return JWTSubUsageDetailSummary{}, nil, nil
}

func (NopStorage) ListJWTSubRawUsage(_ context.Context, _ JWTSubRawUsageFilter) ([]JWTSubRawUsageRow, int, error) {
	return nil, 0, nil
}

func (NopStorage) GetRequestStats(_ context.Context, _ string, _ int, _ string) (RequestStats, error) {
	return RequestStats{}, nil
}

func (NopStorage) GetRouterPerformance(_ context.Context, _ RouterPerformanceFilter) (RouterPerformanceMetrics, error) {
	return RouterPerformanceMetrics{}, nil
}

func (NopStorage) GetSemanticRoutingStats(_ context.Context, _ string, _ int) (SemanticRoutingStats, error) {
	return SemanticRoutingStats{}, nil
}

func (NopStorage) GetConfigAtVersion(_ context.Context, _, _ string, _ int) (json.RawMessage, error) {
	return nil, ErrConfigVersionNotFound
}

func (NopStorage) ApplyGlobalConfigVersion(_ context.Context, _ int, _ string, _ []string) error {
	return fmt.Errorf("apply global config version not supported without database")
}

func (NopStorage) GetAPIKeyMetaByID(_ context.Context, _ uuid.UUID) (APIKeyMeta, bool, error) {
	return APIKeyMeta{}, false, nil
}

func (NopStorage) GetSemanticCacheStats(_ context.Context, _ string, _ int) (SemanticCacheStats, error) {
	return SemanticCacheStats{}, nil
}

func (NopStorage) GetCacheSavings(_ context.Context, _ string) (CacheSavings, error) {
	return CacheSavings{}, nil
}

func (NopStorage) GetSemanticCorrelation(_ context.Context, _ string, _ int) (SemanticCorrelation, error) {
	return SemanticCorrelation{}, nil
}

func (NopStorage) GetModelMRM(_ context.Context, _ string) (map[string]interface{}, bool, error) {
	return nil, false, nil
}

func (NopStorage) PatchModelMRM(_ context.Context, _ string, _ map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}

func (NopStorage) GetComplianceConfig(_ context.Context) (ComplianceGlobalConfig, error) {
	return DefaultComplianceGlobalConfig(), nil
}

func (NopStorage) PatchComplianceConfig(_ context.Context, _ ComplianceGlobalConfig) (ComplianceGlobalConfig, error) {
	return DefaultComplianceGlobalConfig(), nil
}

func (NopStorage) CountExpiredConversationLogs(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (NopStorage) CountExpiredRequestLogs(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (NopStorage) CountExpiredComplianceEvents(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (NopStorage) DeleteExpiredConversationLogs(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (NopStorage) DeleteExpiredRequestLogs(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (NopStorage) DeleteExpiredComplianceEvents(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (NopStorage) StoreLicense(_ context.Context, _ string) error {
	return nil
}

func (NopStorage) GetLicense(_ context.Context) (string, bool, error) {
	return "", false, nil
}

// DecisionWorkflow methods (NopStorage - no database)
func (NopStorage) CreateDecisionWorkflow(_ context.Context, _ DecisionWorkflowRow) error {
	return fmt.Errorf("decision workflows not supported without database")
}

func (NopStorage) GetDecisionWorkflow(_ context.Context, _ string) (DecisionWorkflowRow, bool, error) {
	return DecisionWorkflowRow{}, false, nil
}

func (NopStorage) UpdateDecisionWorkflow(_ context.Context, _ DecisionWorkflowRow) error {
	return fmt.Errorf("decision workflows not supported without database")
}

func (NopStorage) DeleteDecisionWorkflow(_ context.Context, _ string) error {
	return fmt.Errorf("decision workflows not supported without database")
}

func (NopStorage) ListDecisionWorkflows(_ context.Context) ([]DecisionWorkflowRow, error) {
	return []DecisionWorkflowRow{}, nil
}

func (NopStorage) UpsertWorkflowConversation(_ context.Context, _, _, _, _ string, _ float64, _ WorkflowContextAttrs) (WorkflowConversationRow, error) {
	return WorkflowConversationRow{}, nil
}
func (NopStorage) GetWorkflowConversation(_ context.Context, _, _, _ string) (WorkflowConversationRow, bool, error) {
	return WorkflowConversationRow{}, false, nil
}
func (NopStorage) GetConversationDialog(_ context.Context, _, _, _ string) (*ConversationDialog, error) {
	return nil, nil
}
func (NopStorage) SetWorkflowConversationTier(_ context.Context, _, _, _ string, _ int) error {
	return nil
}
func (NopStorage) SetWorkflowConversationBlocked(_ context.Context, _, _, _ string) error {
	return nil
}
func (NopStorage) SetWorkflowConversationPolicyActions(_ context.Context, _, _, _, _, _ string) error {
	return nil
}
func (NopStorage) DeleteExpiredWorkflowConversations(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (NopStorage) ListWorkflowConversations(_ context.Context, _, _ string, _ time.Time, _, _ int) ([]WorkflowConversationRow, int, error) {
	return nil, 0, nil
}
func (NopStorage) GetWorkflowAnalytics(_ context.Context, _ time.Time) ([]WorkflowAnalyticsRow, error) {
	return nil, nil
}
func (NopStorage) GetWorkflowTenantBreakdown(_ context.Context, _ time.Time) ([]WorkflowTenantRow, error) {
	return nil, nil
}
func (NopStorage) GetWorkflowFinOps(_ context.Context, _ time.Time) ([]WorkflowFinOpsRow, error) {
	return nil, nil
}
func (NopStorage) GetConversationStepCosts(_ context.Context, _, _ string) ([]ConversationStepCost, error) {
	return nil, nil
}
func (NopStorage) UpsertWorkflowConversationSnapshot(_ context.Context, _ WorkflowConversationSnapshot) error {
	return nil
}
func (NopStorage) SnapshotAndDeleteExpiredConversations(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (NopStorage) DeleteOldWorkflowConversationSnapshots(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// CustomerInteraction methods (NopStorage - no database)
func (NopStorage) InsertCustomerInteraction(_ context.Context, _ CustomerInteraction) error {
	return nil
}
func (NopStorage) InsertCustomerInteractionBatch(_ context.Context, _ []CustomerInteraction) error {
	return nil
}
func (NopStorage) ListCustomerInteractions(_ context.Context, _ CustomerInteractionFilter, _, _ int) ([]CustomerInteraction, int, error) {
	return []CustomerInteraction{}, 0, nil
}

// Model catalog methods (NopStorage - no database)
func (NopStorage) ListModelCatalog(_ context.Context, _ ModelCatalogFilter) ([]ModelCatalogEntry, error) {
	return nil, nil
}
func (NopStorage) GetModelCatalogEntry(_ context.Context, _, _ string) (ModelCatalogEntry, bool, error) {
	return ModelCatalogEntry{}, false, nil
}
func (NopStorage) CreateModelCatalogEntry(_ context.Context, e ModelCatalogEntry) (ModelCatalogEntry, error) {
	return e, nil
}
func (NopStorage) UpdateModelCatalogEntry(_ context.Context, e ModelCatalogEntry) (ModelCatalogEntry, bool, error) {
	return e, false, nil
}
func (NopStorage) DeleteModelCatalogEntry(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (NopStorage) ListCatalogPricing(_ context.Context) ([]config.CatalogPricingRow, error) {
	return nil, nil
}

// Tool catalog no-ops.
func (NopStorage) ListToolCatalog(_ context.Context, _ ToolCatalogFilter) ([]ToolCatalogEntry, error) {
	return nil, nil
}
func (NopStorage) GetToolCatalogEntry(_ context.Context, _, _ string) (*ToolCatalogEntry, error) {
	return nil, nil
}
func (NopStorage) CreateToolCatalogEntry(_ context.Context, _ ToolCatalogEntry) error { return nil }
func (NopStorage) UpdateToolCatalogEntry(_ context.Context, _, _ string, _ ToolCatalogEntry) error {
	return nil
}
func (NopStorage) DeleteToolCatalogEntry(_ context.Context, _, _ string) error { return nil }
func (NopStorage) ListToolCatalogPricing(_ context.Context) ([]config.ToolCatalogPricingRow, error) {
	return nil, nil
}
func (NopStorage) PingDB(_ context.Context) error { return errors.New("no database configured") }
func (NopStorage) ListTables(_ context.Context) ([]string, error) {
	return nil, errors.New("no database configured")
}
func (NopStorage) EncryptionConfigured() bool { return false }
