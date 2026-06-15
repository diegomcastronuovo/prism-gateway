package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type ListenNotifyConfig struct {
	Enabled bool `yaml:"enabled"`
}

type DynamicConfig struct {
	Enabled           bool               `yaml:"enabled"`
	Source            string             `yaml:"source"` // "postgres"
	RefreshIntervalMs int                `yaml:"refresh_interval_ms"`
	CacheTTLms        int                `yaml:"cache_ttl_ms"`
	ListenNotify      ListenNotifyConfig `yaml:"listen_notify"`
	SeedMode          string             `yaml:"seed_mode"` // "never" | "if_empty" | "always"
}

type ServerConfig struct {
	Host             string `yaml:"host"`
	Port             int    `yaml:"port"`
	RequestTimeoutMs int    `yaml:"request_timeout_ms"`
	LogMode          string `yaml:"log_mode"` // Global default: "metadata_only", "redacted", "full"
	GRPCPort         int    `yaml:"grpc_port"`  // ext_proc gRPC listener port (default 6666; 0 = disabled)
	GRPCTLSCert      string `yaml:"grpc_tls_cert"` // path to TLS cert file (overrides GRPC_TLS_CERT env)
	GRPCTLSKey       string `yaml:"grpc_tls_key"`  // path to TLS key file (overrides GRPC_TLS_KEY env)
	// AsyncSemaphoreCapacity sets the maximum number of concurrent fire-and-forget
	// cache goroutines. Zero or unset defaults to 500.
	AsyncSemaphoreCapacity int `yaml:"async_semaphore_capacity"`
}

type ProviderConfig struct {
	Type      string `yaml:"type"`
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
	// Enabled controls whether this provider is active. Nil means enabled (default).
	Enabled *bool `yaml:"enabled"`
	// APIKey holds a credential stored directly in config (not via env var).
	// Persisted in global config JSON; admin GET responses must redact api_key.
	APIKey string `yaml:"api_key" json:"api_key,omitempty"`
	// AWS Bedrock (type aws_bedrock): explicit credentials and region per provider instance.
	AwsAccessKeyID     string `yaml:"aws_access_key_id,omitempty" json:"aws_access_key_id,omitempty"`
	AwsSecretAccessKey string `yaml:"aws_secret_access_key,omitempty" json:"aws_secret_access_key,omitempty"`
	AwsRegion          string `yaml:"aws_region,omitempty" json:"aws_region,omitempty"`
}

type Pricing struct {
	PromptPer1M                 float64 `yaml:"prompt_per_1m"                    json:"prompt_per_1m"`
	CachedInputPer1M            float64 `yaml:"cached_input_per_1m"              json:"cached_input_per_1m"`
	CompletionPer1M             float64 `yaml:"completion_per_1m"               json:"completion_per_1m"`
	LongContext                 bool    `yaml:"long_context"                    json:"long_context"`
	LongContextStartTokens      int     `yaml:"long_context_start_tokens"       json:"long_context_start_tokens"`
	LongContextPromptPer1M      float64 `yaml:"long_context_prompt_per_1m"      json:"long_context_prompt_per_1m"`
	LongContextCachedInputPer1M float64 `yaml:"long_context_cached_input_per_1m" json:"long_context_cached_input_per_1m"`
	LongContextCompletionPer1M  float64 `yaml:"long_context_completion_per_1m"  json:"long_context_completion_per_1m"`
	// Anthropic ephemeral cache write pricing (5-min and 1-hour TTL tiers).
	CacheWrite5mPer1M float64 `yaml:"cache_write_5m_per_1m" json:"cache_write_5m_per_1m,omitempty"`
	CacheWrite1hPer1M float64 `yaml:"cache_write_1h_per_1m" json:"cache_write_1h_per_1m,omitempty"`
	// GeoMultiplierUS is applied when the provider reports inference_geo == "us".
	// A value of 0 or 1 means no surcharge.
	GeoMultiplierUS float64 `yaml:"geo_multiplier_us" json:"geo_multiplier_us,omitempty"`
}

type MockConfig struct {
	Enabled       bool    `yaml:"enabled"`
	DelayMinMs    int     `yaml:"delay_min_ms"`
	DelayMaxMs    int     `yaml:"delay_max_ms"`
	ErrorRate     float64 `yaml:"error_rate"`
	ErrorStatus   int     `yaml:"error_status"`
	ErrorMessage  string  `yaml:"error_message"`
	FixedResponse string  `yaml:"fixed_response"`
}

// MLExecutionConfig defines how to call an ML model endpoint.
type MLExecutionConfig struct {
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	Protocol string `yaml:"protocol" json:"protocol"` // MVP: "http" only
}

// MLObservableField defines a single field to log for observability.
type MLObservableField struct {
	Path string `yaml:"path" json:"path"` // dot-separated JSON path, e.g. "input.features"
	Type string `yaml:"type" json:"type"` // "text" | "json" | "number"
	Role string `yaml:"role" json:"role"` // "input" | "output"
}

// MLObservableConfig defines which fields to extract and log from ML request/response.
type MLObservableConfig struct {
	Fields []MLObservableField `yaml:"fields" json:"fields"`
}

type ModelConfig struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	// ProviderModelID is the upstream native model identifier (e.g. Bedrock modelId).
	ProviderModelID string     `yaml:"provider_model_id,omitempty" json:"provider_model_id,omitempty"`
	Type            string     `yaml:"type"` // "" | "chat" | "embedding" | "ml"; empty/omitted means "chat"
	Pricing         Pricing    `yaml:"pricing"`
	Mock            MockConfig `yaml:"mock"`
	// InfrastructureMonthlyUSD is the fixed monthly infrastructure cost for this model.
	// Used for FinOps amortization alongside token-based variable cost.
	// Defaults to 0 if omitted; must be non-negative.
	InfrastructureMonthlyUSD float64 `yaml:"infrastructure_monthly_usd,omitempty" json:"infrastructure_monthly_usd,omitempty"`
	// MarkupPercentage is applied over effective cost to derive price.
	// price = effective_cost * (1 + markup_percentage / 100).
	// Defaults to 0 if omitted; must be non-negative.
	MarkupPercentage float64 `yaml:"markup_percentage,omitempty" json:"markup_percentage,omitempty"`
	// BaseURL overrides the provider's base_url for this specific model.
	// Useful when multiple local LLM instances run on different endpoints.
	// If empty, the provider's base_url is used (default behaviour).
	BaseURL string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	// DestEndpoint is the host:port injected as x-gateway-destination-endpoint in ext_proc responses.
	// Used by agentgateway GIE/InferencePool routing to direct traffic to a specific pod or service.
	// Example: "10.244.0.9:8080" (pod IP) or "model-svc.default.svc.cluster.local:8080".
	// If empty, the host:port is derived from BaseURL (or the provider's base_url).
	DestEndpoint string `yaml:"dest_endpoint,omitempty" json:"dest_endpoint,omitempty"`
	// Execution defines how to call the ML model (only used when Type = "ml").
	Execution *MLExecutionConfig `yaml:"execution,omitempty" json:"execution,omitempty"`
	// Observable defines which fields to log for observability (only used when Type = "ml").
	Observable *MLObservableConfig `yaml:"observable,omitempty" json:"observable,omitempty"`
	// Enabled controls whether the model can be used at runtime.
	// nil or true = enabled (default); false = disabled (returns 403 on any request).
	// All existing models remain usable after deploy (nil == enabled).
	// No explicit json tag: marshals as "Enabled" (PascalCase), consistent with Name/Provider/Type.
	// Backward compat: UnmarshalJSON normalises legacy "enabled" → "Enabled".
	Enabled *bool `yaml:"enabled,omitempty" json:",omitempty"`
	// AutoEnable is reserved for future MRM-driven automatic enable/disable logic.
	// Not enforced in this phase.
	// No explicit json tag: marshals as "AutoEnable" (PascalCase).
	// Backward compat: UnmarshalJSON normalises legacy "auto_enable" → "AutoEnable".
	AutoEnable *bool `yaml:"auto_enable,omitempty" json:",omitempty"`
}

// IsEnabled returns true when the model is enabled (nil = true = default).
func (m *ModelConfig) IsEnabled() bool {
	return m.Enabled == nil || *m.Enabled
}

// UnmarshalJSON implements json.Unmarshaler so ModelConfig accepts canonical "provider_model_id"
// and legacy "ProviderModelID" (PascalCase) from stored global config / admin JSON.
// Uses a map merge + decode into a defined type alias to avoid recursive UnmarshalJSON calls.
func (m *ModelConfig) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw != nil {
		// Normalise legacy snake/camel variants so the canonical PascalCase field
		// wins, consistent with Name/Provider/Type (no explicit json tag).
		if _, hasSnake := raw["provider_model_id"]; !hasSnake {
			if v, ok := raw["ProviderModelID"]; ok {
				raw["provider_model_id"] = v
			}
		}
		// "enabled" (old lowercase tag) → "Enabled" (canonical PascalCase).
		if _, hasPascal := raw["Enabled"]; !hasPascal {
			if v, ok := raw["enabled"]; ok {
				raw["Enabled"] = v
				delete(raw, "enabled")
			}
		}
		// "auto_enable" (old snake_case tag) → "AutoEnable" (canonical PascalCase).
		if _, hasPascal := raw["AutoEnable"]; !hasPascal {
			if v, ok := raw["auto_enable"]; ok {
				raw["AutoEnable"] = v
				delete(raw, "auto_enable")
			}
		}
	}
	normalized, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	// noCustom is a distinct type without UnmarshalJSON — default struct decoding.
	type noCustom ModelConfig
	if err := json.Unmarshal(normalized, (*noCustom)(m)); err != nil {
		return err
	}
	return nil
}

type FallbackConfig struct {
	Enabled     bool `yaml:"enabled" json:"enabled"`
	TimeoutMs   int  `yaml:"timeout_ms" json:"timeout_ms"`
	MaxAttempts int  `yaml:"max_attempts" json:"max_attempts"`
}

// SmartWeights defines the scoring weights for smart routing
type SmartWeights struct {
	Cost    float64 `yaml:"cost" json:"cost"`       // 0.0 to 1.0
	Latency float64 `yaml:"latency" json:"latency"` // 0.0 to 1.0
	Errors  float64 `yaml:"errors" json:"errors"`   // 0.0 to 1.0
}

// PromptLengthCondition defines character-count based comparisons for smart routing
type PromptLengthCondition struct {
	GT  *int `yaml:"gt,omitempty" json:"gt,omitempty"`
	GTE *int `yaml:"gte,omitempty" json:"gte,omitempty"`
	LT  *int `yaml:"lt,omitempty" json:"lt,omitempty"`
	LTE *int `yaml:"lte,omitempty" json:"lte,omitempty"`
}

// SemanticSimilarityCondition matches when the prompt embedding is close to a stored anchor.
type SemanticSimilarityCondition struct {
	Threshold float64 `yaml:"threshold" json:"threshold"`
}

// SmartRuleCondition defines when a rule applies
type SmartRuleCondition struct {
	MaxPromptTokens    *int                         `yaml:"max_prompt_tokens,omitempty" json:"max_prompt_tokens,omitempty"`
	Contains           []string                     `yaml:"contains,omitempty" json:"contains,omitempty"`
	PromptLength       *PromptLengthCondition       `yaml:"prompt_length,omitempty" json:"prompt_length,omitempty"`
	SemanticSimilarity *SemanticSimilarityCondition `yaml:"semantic_similarity,omitempty" json:"semantic_similarity,omitempty"`
}

// SmartRule defines preference rules for model selection (legacy format)
type SmartRule struct {
	Name         string             `yaml:"name" json:"name"`
	When         SmartRuleCondition `yaml:"when" json:"when"`
	PreferModels []string           `yaml:"prefer_models" json:"prefer_models"`
}

// SmartConstraints defines resource constraints for smart routing v2
type SmartConstraints struct {
	MaxCostPer1M *float64 `yaml:"max_cost_per_1m,omitempty" json:"max_cost_per_1m,omitempty"`
	MaxLatencyMs *float64 `yaml:"max_latency_ms,omitempty" json:"max_latency_ms,omitempty"`
}

// SmartAction defines actions to take when a stage rule matches
type SmartAction struct {
	Block          bool              `yaml:"block,omitempty" json:"block,omitempty"`
	Reason         string            `yaml:"reason,omitempty" json:"reason,omitempty"`
	PreferModels   []string          `yaml:"prefer_models,omitempty" json:"prefer_models,omitempty"`
	BanModels      []string          `yaml:"ban_models,omitempty" json:"ban_models,omitempty"`
	SetConstraints *SmartConstraints `yaml:"set_constraints,omitempty" json:"set_constraints,omitempty"`
	UseAnchor      bool              `yaml:"use_anchor,omitempty" json:"use_anchor,omitempty"`
}

// SmartStageRule defines a single rule within a stage
type SmartStageRule struct {
	When   SmartRuleCondition `yaml:"when" json:"when"`
	Action SmartAction        `yaml:"action" json:"action"`
}

// SmartStage represents a named group of rules evaluated in order
type SmartStage struct {
	Name  string           `yaml:"name" json:"name"`
	Rules []SmartStageRule `yaml:"rules" json:"rules"`
}

// CostOptimizerConfig controls runtime cost-aware scoring inside smart routing.
// Cost estimation is always active when pricing is configured; this struct allows
// tuning the completion token estimate used when max_tokens is absent from the request.
type CostOptimizerConfig struct {
	// DefaultCompletionTokensEst is the fallback completion token count for cost
	// estimation when max_tokens is not provided by the client. Defaults to 300.
	DefaultCompletionTokensEst int `yaml:"default_completion_tokens_estimate" json:"default_completion_tokens_estimate"`
}

// SmartConfig holds smart routing configuration
type SmartConfig struct {
	Weights       SmartWeights        `yaml:"weights" json:"weights"`
	Rules         []SmartRule         `yaml:"rules" json:"rules"`                   // Legacy flat rules
	Stages        []SmartStage        `yaml:"stages" json:"stages"`                 // NEW v2 stages
	CostOptimizer CostOptimizerConfig `yaml:"cost_optimizer" json:"cost_optimizer"` // runtime cost tuning
}

// SemanticRoutingConfig holds tenant-level semantic routing settings.
// ThresholdDefault is a convenience override: when a stage rule's
// semantic_similarity.threshold is not set (0), this value is used as fallback
// before the global default of 0.60.
// EmbeddingModel is the embedding model used for semantic routing, semantic cache,
// and similarity test operations. When set, it takes precedence over the first
// global embedding model. Recommended to set explicitly per tenant for determinism.
// Phase 1 (safe rollout): if not set, falls back to the first global embedding model
// with a warning. Phase 2 will enforce this as required.
type SemanticRoutingConfig struct {
	ThresholdDefault float64 `yaml:"threshold_default" json:"threshold_default"`
	EmbeddingModel   string  `yaml:"embedding_model"   json:"embedding_model"`
}

// ModalityEmbeddingConfig maps a modality (e.g. "text", "image") to its embedding model.
type ModalityEmbeddingConfig struct {
	EmbeddingModel string `yaml:"embedding_model" json:"embedding_model"`
}

// SemanticCacheConfig holds tenant-level semantic caching settings.
type SemanticCacheConfig struct {
	Enabled             bool    `yaml:"enabled"                json:"enabled"`
	Threshold           float64 `yaml:"threshold"              json:"threshold"`   // 0.92
	TTLSeconds          int     `yaml:"ttl_seconds"            json:"ttl_seconds"` // 86400
	MaxEntriesPerTenant int     `yaml:"max_entries_per_tenant" json:"max_entries_per_tenant"`
	Scope               string  `yaml:"scope"                  json:"scope"`           // "model" | "route_group"
	EmbeddingModel      string  `yaml:"embedding_model"        json:"embedding_model"` // "" = first embedding model
}

// Routing strategy constants (SPEC_169: added StrategyDecisionOps).
const (
	StrategyRoundRobin   = "round_robin"
	StrategyLatencyBased = "latency_based"
	StrategyCostBased    = "cost_based"
	StrategyHeaderBased  = "header_based"
	StrategySmart        = "smart"
	StrategyDecisionOps  = "decision_ops" // SPEC_169
)

type RoutingConfig struct {
	Strategy   string                `yaml:"strategy" json:"strategy"`
	RouteGroup string                `yaml:"route_group" json:"route_group,omitempty"`
	Smart      SmartConfig           `yaml:"smart" json:"smart"`
	Fallback   FallbackConfig        `yaml:"fallback" json:"fallback"`
	Semantic   SemanticRoutingConfig `yaml:"semantic" json:"semantic"`
}

// PrecedenceConfig defines precedence rules for model selection
type PrecedenceConfig struct {
	Model          string `yaml:"model" json:"model"`                     // "header" | "body" (default: "header")
	ConflictPolicy string `yaml:"conflict_policy" json:"conflict_policy"` // "error" | "ignore_group" | "ignore_model"
}

// VirtualModelConfig defines a single virtual model alias.
// Clients can send the alias name as the `model` field; the router resolves
// it to real candidates without exposing internal model names.
type VirtualModelConfig struct {
	Enabled               bool   `yaml:"enabled" json:"enabled"`
	RouteGroup            string `yaml:"route_group" json:"route_group"`
	ExposeAliasInResponse bool   `yaml:"expose_alias_in_response" json:"expose_alias_in_response"`
}

type SelectionConfig struct {
	HeaderModelKey string                        `yaml:"header_model_key" json:"header_model_key"`
	HeaderRouteKey string                        `yaml:"header_route_key" json:"header_route_key"`
	RouteGroups    map[string][]string           `yaml:"route_groups" json:"route_groups"`
	Precedence     PrecedenceConfig              `yaml:"precedence" json:"precedence"`
	VirtualModels  map[string]VirtualModelConfig `yaml:"virtual_models" json:"virtual_models,omitempty"`
	// SPEC_151: per-tenant controls for header override acceptance.
	// Both default to false (opt-in). Missing fields in stored config are treated as false.
	AllowModelOverride      bool `yaml:"allow_model_override" json:"allow_model_override"`
	AllowRouteGroupOverride bool `yaml:"allow_route_group_override" json:"allow_route_group_override"`
}

// TrafficSplitEntry defines one candidate in a weighted traffic split.
type TrafficSplitEntry struct {
	Model  string `yaml:"model" json:"model"`
	Weight int    `yaml:"weight" json:"weight"`
}

type RegexBlockHookConfig struct {
	Patterns []string `yaml:"patterns" json:"patterns"`
}

type PIIAllowRoutingConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Model   string `yaml:"model" json:"model"`
}

type PIIHookConfig struct {
	Enabled   bool                   `yaml:"enabled" json:"enabled"`
	URL       string                 `yaml:"url" json:"url"`
	TimeoutMs int                    `yaml:"timeout_ms" json:"timeout_ms"`
	FailOpen  bool                   `yaml:"fail_open" json:"fail_open"`
	AllowPII  *PIIAllowRoutingConfig `yaml:"allow_pii,omitempty" json:"allow_pii,omitempty"`
}

// ExternalPIIHookConfig configures webhook-based PII validation
type ExternalPIIHookConfig struct {
	Enabled      bool         `yaml:"enabled" json:"enabled"`
	BaseURL      string       `yaml:"base_url" json:"base_url"`
	Request      WebhookPhase `yaml:"request" json:"request"`
	Response     WebhookPhase `yaml:"response" json:"response"`
	TimeoutMs    int          `yaml:"timeout_ms" json:"timeout_ms"`
	FailMode     string       `yaml:"fail_mode" json:"fail_mode"` // "fail_open" or "fail_closed"
	MaxBodyBytes int          `yaml:"max_body_bytes" json:"max_body_bytes"`
	Auth         WebhookAuth  `yaml:"auth" json:"auth"`
	// APIKey is the API key used to authenticate requests to the webhook endpoint
	APIKey       string       `yaml:"api_key" json:"api_key,omitempty"`
}

type WebhookPhase struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Path    string `yaml:"path" json:"path"`
}

type WebhookAuth struct {
	Type   string `yaml:"type" json:"type"`     // "none", "bearer", "api_key"
	Token  string `yaml:"token" json:"token"`   // Token value (can be env var reference)
	Header string `yaml:"header" json:"header"` // For api_key: which header to use (default X-API-Key)
}

type WebhookAuthConfig struct {
	Type     string `yaml:"type" json:"type"`           // "none", "bearer", "api_key"
	TokenEnv string `yaml:"token_env" json:"token_env"` // Environment variable name
	Header   string `yaml:"header" json:"header"`       // For api_key: header name (default X-API-Key)
}

type AlertsConfig struct {
	Enabled          bool              `yaml:"enabled" json:"enabled"`
	BudgetThresholds []float64         `yaml:"budget_thresholds" json:"budget_thresholds"` // [0.7, 0.85, 1.0]
	WebhookURL       string            `yaml:"webhook_url" json:"webhook_url"`
	WebhookTimeoutMs int               `yaml:"webhook_timeout_ms" json:"webhook_timeout_ms"`
	WebhookAuth      WebhookAuthConfig `yaml:"webhook_auth" json:"webhook_auth"`
}

type HooksConfig struct {
	RegexBlock *RegexBlockHookConfig `yaml:"regex_block" json:"regex_block,omitempty"`
	PII        *PIIHookConfig        `yaml:"pii" json:"pii,omitempty"`
}

type GlobalHooksConfig struct {
	RegexBlock RegexBlockHookConfig `yaml:"regex_block"`
}

type BudgetsConfig struct {
	MonthlyUSD float64 `yaml:"monthly_usd" json:"monthly_usd"`
	Timezone   string  `yaml:"timezone" json:"timezone"`
}

type BudgetEnforcementThresholds struct {
	WarnPct float64 `yaml:"warn_pct" json:"warn_pct"` // default 0.80
	HardPct float64 `yaml:"hard_pct" json:"hard_pct"` // default 1.00
}

type TagBudgetsConfig struct {
	Enabled         bool                          `yaml:"enabled" json:"enabled"`
	Keys            []string                      `yaml:"keys" json:"keys"`
	MonthlyUSDByTag map[string]map[string]float64 `yaml:"monthly_usd_by_tag" json:"monthly_usd_by_tag"`
}

type BudgetEnforcementConfig struct {
	Enabled            bool                        `yaml:"enabled" json:"enabled"`
	Mode               string                      `yaml:"mode" json:"mode"`                 // "report_only"|"block"|"degrade"
	BlockStatus        int                         `yaml:"block_status" json:"block_status"` // default 402
	DegradeRouteGroup  string                      `yaml:"degrade_route_group" json:"degrade_route_group"`
	IncludeCostInError bool                        `yaml:"include_cost_in_error" json:"include_cost_in_error"`
	Thresholds         BudgetEnforcementThresholds `yaml:"thresholds" json:"thresholds"`
	TagBudgets         TagBudgetsConfig            `yaml:"tag_budgets" json:"tag_budgets"`
	Events             BudgetEventsConfig          `yaml:"events" json:"events"`
}

// BudgetEventsConfig controls emission of budget WARN events to Redis Streams.
// If Redis.Addr is empty, event emission is disabled (noop).
type BudgetEventsConfig struct {
	Redis BudgetEventsRedisConfig `yaml:"redis" json:"redis"`
}

// BudgetEventsRedisConfig mirrors the Redis connection fields used by other Redis features.
type BudgetEventsRedisConfig struct {
	Addr     string `yaml:"addr" json:"addr"`
	Password string `yaml:"password" json:"password"`
	DB       int    `yaml:"db" json:"db"`
}

type RateLimitConfig struct {
	RPM   int    `yaml:"rpm" json:"rpm"`     // requests per minute
	Burst int    `yaml:"burst" json:"burst"` // max burst tokens
	Scope string `yaml:"scope" json:"scope"` // "tenant" (default) | "jwt_sub" | "api_key"
}

type ComplianceConfig struct {
	RetentionDays int    `yaml:"retention_days" json:"retention_days"` // Default: 90, Range: 7-365
	LogMode       string `yaml:"log_mode" json:"log_mode"`             // "metadata_only", "redacted", "full"
}

type TenantLoggingConfig struct {
	Mode          string `yaml:"mode" json:"mode"`                     // "disabled" | "metadata_only" | "redacted" | "full"
	RetentionDays int    `yaml:"retention_days" json:"retention_days"` // retention hint for cleanup jobs
}

type ConversationLoggingConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type TenantConfig struct {
	ID                 string                             `yaml:"id" json:"id"`
	// Environment classifies the tenant as DEV, STAGING, or PROD.
	// Immutable after creation. Defaults to DEV if omitted.
	Environment        string                             `yaml:"environment,omitempty" json:"environment,omitempty"`
	APIKeys            []string                           `yaml:"api_keys" json:"api_keys,omitempty"`
	AllowedModels      []string                           `yaml:"allowed_models" json:"allowed_models"`
	Routing            RoutingConfig                      `yaml:"routing" json:"routing"`
	Selection          SelectionConfig                    `yaml:"selection" json:"selection"`
	SemanticCache      SemanticCacheConfig                `yaml:"semantic_cache" json:"semantic_cache"`
	SemanticModalities map[string]ModalityEmbeddingConfig `yaml:"semantic_modalities" json:"semantic_modalities,omitempty"`
	Hooks              HooksConfig                        `yaml:"hooks" json:"hooks"`
	Budgets            BudgetsConfig                      `yaml:"budgets" json:"budgets"`
	BudgetEnforcement  BudgetEnforcementConfig            `yaml:"budget_enforcement" json:"budget_enforcement"`
	RateLimit          RateLimitConfig                    `yaml:"rate_limit" json:"rate_limit"`
	Compliance         ComplianceConfig                   `yaml:"compliance" json:"compliance"`
	Logging            TenantLoggingConfig                `yaml:"logging" json:"logging"`
	Alerts             AlertsConfig                       `yaml:"alerts" json:"alerts"`
	ToolRoutingEnabled *bool                              `yaml:"tool_routing_enabled,omitempty" json:"tool_routing_enabled,omitempty"`
	// TrafficSplit maps a split key (virtual alias, route group, or model name)
	// to a weighted list of candidate models for canary/gradual rollout.
	TrafficSplit map[string][]TrafficSplitEntry `yaml:"traffic_split" json:"traffic_split,omitempty"`
	// MaxOutputTokens caps the number of output tokens sent to LLM providers.
	// nil, missing, or 0 means unrestricted. Positive value injects max_tokens
	// into LLM requests (type "llm" or ""). Not applied to embedding or ml models.
	MaxOutputTokens *int `yaml:"max_output_tokens,omitempty" json:"max_output_tokens,omitempty"`
	// ClaudeCode controls whether this tenant is allowed to use the Claude Code
	// Anthropic Messages endpoint. Nil or Enabled=false → access denied (SPEC_161).
	ClaudeCode *ClaudeCodeTenantConfig `yaml:"claude_code,omitempty" json:"claude_code,omitempty"`
	// DecisionOps holds per-tenant DecisionOps permission settings (SPEC_167).
	DecisionOps *TenantDecisionOpsConfig `yaml:"decision_ops,omitempty" json:"decision_ops,omitempty"`
}

// TenantWorkflowModelsHierarchy defines the 3-tier model selection for a workflow
// within a tenant. All three IDs must appear in the tenant's AllowedModels list.
// They may reference the same model (valid when the tenant only has one model).
type TenantWorkflowModelsHierarchy struct {
	// Premium is the highest-capability model (tier 1).
	Premium string `yaml:"premium" json:"premium"`
	// Normal is the default model (tier 2).
	Normal string `yaml:"normal" json:"normal"`
	// Low is the cost-optimized model (tier 3).
	Low string `yaml:"low" json:"low"`
}

// WorkflowThresholdAction defines what happens when a metric (steps or cost)
// reaches a given percentage of its global limit.
//
// ToDo values:
//
//	"degrade"  — switch the model hierarchy one tier down
//	"block"    — reject the next request with HTTP 429
//	"nothing"  — log/observe only; no operational change
type WorkflowThresholdAction struct {
	ToDo              string  `yaml:"to_do" json:"to_do"`                             // "degrade" | "block" | "nothing"
	PercentageToShoot float64 `yaml:"percentage_to_shoot" json:"percentage_to_shoot"` // 0 < x <= 100
}

// WorkflowThresholdPolicy defines two ordered threshold actions for a metric.
// Action1 fires first (lower percentage); Action2 fires second (higher percentage).
// Constraint: Action1.PercentageToShoot < Action2.PercentageToShoot.
type WorkflowThresholdPolicy struct {
	Action1 WorkflowThresholdAction `yaml:"action_1" json:"action_1"`
	Action2 WorkflowThresholdAction `yaml:"action_2" json:"action_2"`
}

// ExternalClassifierConfig holds connection settings for an external classification
// service that selects the initial model tier for a workflow request.
// APIKey is optional; if set it is sent as X-API-Key on classifier requests.
// Same APIKey handling pattern as ExternalPIIHookConfig.APIKey.
type ExternalClassifierConfig struct {
	Host   string `yaml:"host" json:"host"`
	Port   int    `yaml:"port" json:"port"`
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}

// TenantWorkflowAssignment assigns a global workflow to a tenant and configures
// how the tenant uses it. WorkflowID must reference a row in decision_workflows.
//
// The global workflow defines hard limits (workflow_max_steps, workflow_max_cost).
// StepsPolicy and CostPolicy define threshold actions as percentages of those limits.
type TenantWorkflowAssignment struct {
	// WorkflowID references decision_workflows.workflow_id.
	WorkflowID string `yaml:"workflow_id" json:"workflow_id"`
	// ModelsHierarchy defines the three model tiers for this workflow in this tenant.
	ModelsHierarchy TenantWorkflowModelsHierarchy `yaml:"models_hierarchy" json:"models_hierarchy"`
	// StepsPolicy defines threshold actions based on step count consumption.
	// Percentages are relative to decision_workflows.workflow_max_steps.
	StepsPolicy *WorkflowThresholdPolicy `yaml:"steps_policy,omitempty" json:"steps_policy,omitempty"`
	// CostPolicy defines threshold actions based on USD cost consumption.
	// Percentages are relative to decision_workflows.workflow_max_cost.
	CostPolicy *WorkflowThresholdPolicy `yaml:"cost_policy,omitempty" json:"cost_policy,omitempty"`
	// EnableExternalClassifier enables the external classifier for this assignment.
	// When true, ExternalClassifier must be present with host and port.
	EnableExternalClassifier bool `yaml:"enable_external_classifier" json:"enable_external_classifier"`
	// ExternalClassifier is required when EnableExternalClassifier == true.
	ExternalClassifier *ExternalClassifierConfig `yaml:"external_classifier,omitempty" json:"external_classifier,omitempty"`
}

// TenantDecisionOpsConfig holds per-tenant DecisionOps runtime settings (SPEC_168).
type TenantDecisionOpsConfig struct {
	// Enabled gates all DecisionOps functionality for this tenant.
	Enabled bool `yaml:"enabled" json:"enabled"`
	// EnabledWorkflows lists the workflows this tenant is allowed to execute,
	// with per-tenant threshold policies, model hierarchy and optional classifier.
	EnabledWorkflows []TenantWorkflowAssignment `yaml:"enabled_workflows" json:"enabled_workflows"`
}

// ClaudeCodeTenantConfig holds the tenant-level Claude Code permission flag and
// optional per-tenant budget for /v1/messages requests (SPEC_163).
type ClaudeCodeTenantConfig struct {
	Enabled       bool    `yaml:"enabled" json:"enabled"`
	MonthlyBudget float64 `yaml:"monthly_budget,omitempty" json:"monthly_budget,omitempty"`
	// Timezone is the IANA timezone used to determine the monthly billing cycle.
	// Defaults to "America/Buenos_Aires" when empty.
	Timezone string `yaml:"timezone,omitempty" json:"timezone,omitempty"`
}

// EffectiveMaxOutputTokens returns the active output token cap for a tenant.
// Returns 0 when unrestricted (nil, zero, or negative value).
func (t TenantConfig) EffectiveMaxOutputTokens() int {
	if t.MaxOutputTokens == nil || *t.MaxOutputTokens <= 0 {
		return 0
	}
	return *t.MaxOutputTokens
}

type DatabaseConfig struct {
	MaxOpenConns int `yaml:"max_open_conns"`
	MaxIdleConns int `yaml:"max_idle_conns"`
}

// RBACConfig defines role-based access control settings
type RBACConfig struct {
	UserRoles    []string `yaml:"user_roles" json:"user_roles,omitempty"`
	AdminRoles   []string `yaml:"admin_roles" json:"admin_roles,omitempty"`
	FinanceRoles []string `yaml:"finance_roles" json:"finance_roles,omitempty"`
}

// JWTConfig defines JWT authentication settings
type JWTConfig struct {
	Issuer           string            `yaml:"issuer" json:"issuer,omitempty"`
	IssuerPublic     string            `yaml:"issuer_public" json:"issuer_public,omitempty"` // Fallback issuer for multi-URL scenarios (e.g., Docker internal vs public)
	Audience         string            `yaml:"audience" json:"audience,omitempty"`
	JWKSURL          string            `yaml:"jwks_url" json:"jwks_url,omitempty"`
	ClockSkewSeconds int               `yaml:"clock_skew_seconds" json:"clock_skew_seconds,omitempty"`
	RequiredClaims   map[string]string `yaml:"required_claims" json:"required_claims,omitempty"`
	CacheTTLMinutes  int               `yaml:"cache_ttl_minutes" json:"cache_ttl_minutes,omitempty"`
	RBAC             RBACConfig        `yaml:"rbac" json:"rbac,omitempty"`
}

// AuthConfig defines authentication and authorization settings
type AuthConfig struct {
	Mode string    `yaml:"mode" json:"mode,omitempty"` // "api_key", "jwt", "both"
	JWT  JWTConfig `yaml:"jwt" json:"jwt,omitempty"`
	// RBAC at top level for YAML backward compatibility (auth.rbac).
	// When present, applyDefaults copies it into JWT.RBAC for API shape (auth.jwt.rbac).
	RBAC RBACConfig `yaml:"rbac" json:"-"`
}

type GlobalRateLimitConfig struct {
	Backend string             `yaml:"backend"` // "in_memory" (default) | "redis"
	Redis   RedisLimiterConfig `yaml:"redis"`
}

// SmartRoutingMetricsConfig configures the metrics store used for smart routing decisions.
type SmartRoutingMetricsConfig struct {
	Backend string             `yaml:"backend"` // "in_memory" (default) | "redis"
	Redis   RedisLimiterConfig `yaml:"redis"`
}

// SmartRoutingConfig is the top-level config block for distributed smart routing.
type SmartRoutingConfig struct {
	MetricsStore SmartRoutingMetricsConfig `yaml:"metrics_store"`
}

// BenchmarkRequestMessage is one message in the benchmark prompt.
type BenchmarkRequestMessage struct {
	Role    string `yaml:"role" json:"role"`
	Content string `yaml:"content" json:"content"`
}

// BenchmarkRequestConfig holds the standard benchmark payload sent to each model.
type BenchmarkRequestConfig struct {
	Messages []BenchmarkRequestMessage `yaml:"messages" json:"messages"`
}

// BenchmarkStorageConfig controls retention of raw benchmark rows.
type BenchmarkStorageConfig struct {
	RetainDays int `yaml:"retain_days" json:"retain_days"`
}

// BenchmarkRoutingIntegrationConfig enables feeding benchmark data into smart routing.
type BenchmarkRoutingIntegrationConfig struct {
	Enabled         bool `yaml:"enabled" json:"enabled"`
	WeightLatency   bool `yaml:"weight_latency" json:"weight_latency"`
	WeightErrorRate bool `yaml:"weight_error_rate" json:"weight_error_rate"`
}

// BenchmarkingConfig holds the top-level benchmarking section from config.yaml.
type BenchmarkingConfig struct {
	Enabled            bool                              `yaml:"enabled" json:"enabled"`
	IntervalMinutes    int                               `yaml:"interval_minutes" json:"interval_minutes"`
	TimeoutMs          int                               `yaml:"timeout_ms" json:"timeout_ms"`
	MaxConcurrency     int                               `yaml:"max_concurrency" json:"max_concurrency"`
	FailOpen           bool                              `yaml:"fail_open" json:"fail_open"`
	Request            BenchmarkRequestConfig            `yaml:"request" json:"request"`
	Storage            BenchmarkStorageConfig            `yaml:"storage" json:"storage"`
	RoutingIntegration BenchmarkRoutingIntegrationConfig `yaml:"routing_integration" json:"routing_integration"`
}

type RedisLimiterConfig struct {
	Addr          string `yaml:"addr"`
	Password      string `yaml:"password"`
	DB            int    `yaml:"db"`
	DialTimeoutMs int    `yaml:"dial_timeout_ms"`
	OpTimeoutMs   int    `yaml:"op_timeout_ms"`
	KeyPrefix     string `yaml:"key_prefix"`
	FailOpen      bool   `yaml:"fail_open"`
}

type CircuitBreakerRedisConfig struct {
	Addr          string `yaml:"addr" json:"addr"`
	Password      string `yaml:"password" json:"password"`
	DB            int    `yaml:"db" json:"db"`
	DialTimeoutMs int    `yaml:"dial_timeout_ms" json:"dial_timeout_ms"`
	OpTimeoutMs   int    `yaml:"op_timeout_ms" json:"op_timeout_ms"`
	KeyPrefix     string `yaml:"key_prefix" json:"key_prefix"`
	FailOpen      bool   `yaml:"fail_open" json:"fail_open"`
}

type CircuitBreakerProviderConfig struct {
	Enabled                  *bool    `yaml:"enabled,omitempty"`
	WindowSeconds            *int     `yaml:"window_seconds,omitempty"`
	MinRequests              *int     `yaml:"min_requests,omitempty"`
	FailureRateThreshold     *float64 `yaml:"failure_rate_threshold,omitempty"`
	OpenCooldownSeconds      *int     `yaml:"open_cooldown_seconds,omitempty"`
	HalfOpenMaxInflight      *int     `yaml:"half_open_max_inflight,omitempty"`
	HalfOpenSuccessesToClose *int     `yaml:"half_open_successes_to_close,omitempty"`
}

type CircuitBreakerDefaultsConfig struct {
	Enabled                  bool    `yaml:"enabled"`
	WindowSeconds            int     `yaml:"window_seconds"`
	BucketSizeSeconds        int     `yaml:"bucket_size_seconds"`
	MinRequests              int     `yaml:"min_requests"`
	FailureRateThreshold     float64 `yaml:"failure_rate_threshold"`
	OpenCooldownSeconds      int     `yaml:"open_cooldown_seconds"`
	HalfOpenMaxInflight      int     `yaml:"half_open_max_inflight"`
	HalfOpenSuccessesToClose int     `yaml:"half_open_successes_to_close"`
}

type CircuitBreakerConfig struct {
	Backend   string                                  `yaml:"backend"` // "redis" | "in_memory"
	Redis     CircuitBreakerRedisConfig               `yaml:"redis"`
	Defaults  CircuitBreakerDefaultsConfig            `yaml:"defaults"`
	Providers map[string]CircuitBreakerProviderConfig `yaml:"providers,omitempty"`
}

// ProviderConfig merges per-provider overrides on top of defaults.
func (cb *CircuitBreakerConfig) ProviderConfig(provider string) CircuitBreakerDefaultsConfig {
	cfg := cb.Defaults
	if ov, ok := cb.Providers[provider]; ok {
		if ov.Enabled != nil {
			cfg.Enabled = *ov.Enabled
		}
		if ov.WindowSeconds != nil {
			cfg.WindowSeconds = *ov.WindowSeconds
		}
		if ov.MinRequests != nil {
			cfg.MinRequests = *ov.MinRequests
		}
		if ov.FailureRateThreshold != nil {
			cfg.FailureRateThreshold = *ov.FailureRateThreshold
		}
		if ov.OpenCooldownSeconds != nil {
			cfg.OpenCooldownSeconds = *ov.OpenCooldownSeconds
		}
		if ov.HalfOpenMaxInflight != nil {
			cfg.HalfOpenMaxInflight = *ov.HalfOpenMaxInflight
		}
		if ov.HalfOpenSuccessesToClose != nil {
			cfg.HalfOpenSuccessesToClose = *ov.HalfOpenSuccessesToClose
		}
	}
	return cfg
}

// configBlock holds the optional top-level `config:` YAML block.
type configBlock struct {
	Version string `yaml:"version"`
}

// LicenseConfig holds paths to the license JWT and its verification public key.
type LicenseConfig struct {
	LicenseFile   string `yaml:"license_file"`
	PublicKeyFile string `yaml:"public_key_file"`
}

type Config struct {
	Auth                AuthConfig                `yaml:"auth"`
	Server              ServerConfig              `yaml:"server"`
	Database            DatabaseConfig            `yaml:"database"`
	RateLimit           GlobalRateLimitConfig     `yaml:"rate_limit"`
	SmartRouting        SmartRoutingConfig        `yaml:"smart_routing"`
	CircuitBreaker      CircuitBreakerConfig      `yaml:"circuit_breaker"`
	Providers           map[string]ProviderConfig `yaml:"providers"`
	Models              []ModelConfig             `yaml:"models"`
	Tenants             []TenantConfig            `yaml:"tenants"`
	Hooks               GlobalHooksConfig         `yaml:"hooks"`
	ConversationLogging ConversationLoggingConfig `yaml:"conversation_logging"`
	DynamicConfig       DynamicConfig             `yaml:"dynamic_config"`
	Benchmarking        BenchmarkingConfig        `yaml:"benchmarking"`
	License             LicenseConfig             `yaml:"license"`

	// Parsed from the optional `config: version:` YAML block.
	ConfigMeta configBlock `yaml:"config"`

	// Computed at load time — not from YAML.
	Version string `yaml:"-"` // resolved version; "unknown" if not specified
	SHA256  string `yaml:"-"` // hex-encoded SHA256 of the raw config file
}

// Load reads .env for secrets and config.yaml for application config.
func Load(configPath string) (*Config, error) {
	// Load .env (optional, ignore error if file doesn't exist)
	_ = godotenv.Load()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", configPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config yaml: %w", err)
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.RequestTimeoutMs == 0 {
		cfg.Server.RequestTimeoutMs = 45000
	}
	if cfg.Server.LogMode == "" {
		cfg.Server.LogMode = "metadata_only" // Privacy-safe by default
	}
	if cfg.Server.GRPCPort == 0 {
		cfg.Server.GRPCPort = 7777
	}
	if cfg.Database.MaxOpenConns == 0 {
		cfg.Database.MaxOpenConns = 10
	}
	if cfg.Database.MaxIdleConns == 0 {
		cfg.Database.MaxIdleConns = 5
	}

	// Set dynamic config defaults
	if cfg.DynamicConfig.CacheTTLms == 0 {
		cfg.DynamicConfig.CacheTTLms = 5000 // 5s default per spec
	}

	// Set rate limit defaults
	if cfg.RateLimit.Backend == "" {
		cfg.RateLimit.Backend = "in_memory" // Backward compatible
	}
	if cfg.RateLimit.Redis.DialTimeoutMs == 0 {
		cfg.RateLimit.Redis.DialTimeoutMs = 200
	}
	if cfg.RateLimit.Redis.OpTimeoutMs == 0 {
		cfg.RateLimit.Redis.OpTimeoutMs = 100
	}
	if cfg.RateLimit.Redis.KeyPrefix == "" {
		cfg.RateLimit.Redis.KeyPrefix = "rl:"
	}

	// Set smart routing metrics store defaults
	if cfg.SmartRouting.MetricsStore.Backend == "" {
		cfg.SmartRouting.MetricsStore.Backend = "in_memory"
	}
	if cfg.SmartRouting.MetricsStore.Redis.DialTimeoutMs == 0 {
		cfg.SmartRouting.MetricsStore.Redis.DialTimeoutMs = 200
	}
	if cfg.SmartRouting.MetricsStore.Redis.OpTimeoutMs == 0 {
		cfg.SmartRouting.MetricsStore.Redis.OpTimeoutMs = 100
	}
	if cfg.SmartRouting.MetricsStore.Redis.KeyPrefix == "" {
		cfg.SmartRouting.MetricsStore.Redis.KeyPrefix = "sr:"
	}

	// Set benchmarking defaults
	if cfg.Benchmarking.IntervalMinutes == 0 {
		cfg.Benchmarking.IntervalMinutes = 30
	}
	if cfg.Benchmarking.TimeoutMs == 0 {
		cfg.Benchmarking.TimeoutMs = 15000
	}
	if cfg.Benchmarking.MaxConcurrency == 0 {
		cfg.Benchmarking.MaxConcurrency = 2
	}
	if !cfg.Benchmarking.Enabled {
		cfg.Benchmarking.FailOpen = true // default fail-open when not explicitly set
	}
	if len(cfg.Benchmarking.Request.Messages) == 0 {
		cfg.Benchmarking.Request.Messages = []BenchmarkRequestMessage{
			{Role: "user", Content: "Say hello in one short sentence."},
		}
	}
	if cfg.Benchmarking.Storage.RetainDays == 0 {
		cfg.Benchmarking.Storage.RetainDays = 30
	}
	// Set license config defaults.
	// LICENSE_FILE="" (explicit empty string) disables file-based license fallback — use this
	// in Kubernetes/OpenShift where /app/config is read-only and the DB is the source of truth.
	if cfg.License.LicenseFile == "" {
		if v, ok := os.LookupEnv("LICENSE_FILE"); ok {
			// Explicit env var set (even if empty) — honour it (empty = disable file).
			cfg.License.LicenseFile = v
		} else {
			// No env var at all: use local default for docker-compose / bare-metal.
			cfg.License.LicenseFile = "./config/license.jwt"
		}
	}
	if cfg.License.PublicKeyFile == "" {
		if v := os.Getenv("LICENSE_PUBLIC_KEY_FILE"); v != "" {
			cfg.License.PublicKeyFile = v
		} else {
			cfg.License.PublicKeyFile = "./config/public_key.pem"
		}
	}

	// Set circuit breaker defaults
	if cfg.CircuitBreaker.Backend == "" {
		cfg.CircuitBreaker.Backend = "in_memory"
	}
	if cfg.CircuitBreaker.Redis.DialTimeoutMs == 0 {
		cfg.CircuitBreaker.Redis.DialTimeoutMs = 200
	}
	if cfg.CircuitBreaker.Redis.OpTimeoutMs == 0 {
		cfg.CircuitBreaker.Redis.OpTimeoutMs = 100
	}
	if cfg.CircuitBreaker.Redis.KeyPrefix == "" {
		cfg.CircuitBreaker.Redis.KeyPrefix = "cb:"
	}
	d := &cfg.CircuitBreaker.Defaults
	if d.WindowSeconds == 0 {
		d.WindowSeconds = 60
	}
	if d.BucketSizeSeconds == 0 {
		d.BucketSizeSeconds = 5
	}
	if d.MinRequests == 0 {
		d.MinRequests = 20
	}
	if d.FailureRateThreshold == 0 {
		d.FailureRateThreshold = 0.5
	}
	if d.OpenCooldownSeconds == 0 {
		d.OpenCooldownSeconds = 30
	}
	if d.HalfOpenMaxInflight == 0 {
		d.HalfOpenMaxInflight = 1
	}
	if d.HalfOpenSuccessesToClose == 0 {
		d.HalfOpenSuccessesToClose = 3
	}
	if !d.Enabled {
		d.Enabled = true // default on
	}

	// Set authentication defaults
	if cfg.Auth.Mode == "" {
		cfg.Auth.Mode = "api_key" // Backward compatible
	}
	if cfg.Auth.JWT.ClockSkewSeconds == 0 {
		cfg.Auth.JWT.ClockSkewSeconds = 60
	}
	if cfg.Auth.JWT.CacheTTLMinutes == 0 {
		cfg.Auth.JWT.CacheTTLMinutes = 10
	}
	if cfg.Auth.JWT.RequiredClaims == nil {
		cfg.Auth.JWT.RequiredClaims = map[string]string{
			"tenant_id": "tenant_id",
			"roles":     "roles",
		}
	}
	if cfg.Auth.JWT.JWKSURL == "" && cfg.Auth.JWT.Issuer != "" {
		cfg.Auth.JWT.JWKSURL = cfg.Auth.JWT.Issuer + "/protocol/openid-connect/certs"
	}
	// Data-plane /v1 JWT RBAC and admin JWT fallbacks use the same vocabulary as the rest of the gateway:
	// user, admin, local_admin, financial (not Keycloak-style tenant_user / org_admin / platform_admin).
	if len(cfg.Auth.RBAC.UserRoles) == 0 {
		cfg.Auth.RBAC.UserRoles = []string{"user", "admin", "local_admin", "financial"}
	}
	if len(cfg.Auth.RBAC.AdminRoles) == 0 {
		cfg.Auth.RBAC.AdminRoles = []string{"admin"}
	}
	// Keep auth.jwt.rbac in sync with auth.rbac for API shape and server code
	if len(cfg.Auth.JWT.RBAC.UserRoles) == 0 && len(cfg.Auth.RBAC.UserRoles) > 0 {
		cfg.Auth.JWT.RBAC = cfg.Auth.RBAC
	}
	if len(cfg.Auth.RBAC.UserRoles) == 0 && len(cfg.Auth.JWT.RBAC.UserRoles) > 0 {
		cfg.Auth.RBAC = cfg.Auth.JWT.RBAC
	}

	// Set defaults for tenants
	for i := range cfg.Tenants {
		if cfg.Tenants[i].Compliance.RetentionDays == 0 {
			cfg.Tenants[i].Compliance.RetentionDays = 90
		}
		if cfg.Tenants[i].Compliance.LogMode == "" {
			cfg.Tenants[i].Compliance.LogMode = cfg.Server.LogMode // Inherit global
		}
		if cfg.Tenants[i].Logging.Mode == "" {
			cfg.Tenants[i].Logging.Mode = cfg.Tenants[i].Compliance.LogMode
		}
		if cfg.Tenants[i].Logging.RetentionDays == 0 {
			cfg.Tenants[i].Logging.RetentionDays = cfg.Tenants[i].Compliance.RetentionDays
		}
		if cfg.Tenants[i].RateLimit.Scope == "" {
			cfg.Tenants[i].RateLimit.Scope = "tenant" // Default to tenant-level scoping
		}

		// Set routing v2 precedence defaults
		if cfg.Tenants[i].Selection.Precedence.Model == "" {
			cfg.Tenants[i].Selection.Precedence.Model = "header" // NEW PRO default: header wins over body
		}
		if cfg.Tenants[i].Selection.Precedence.ConflictPolicy == "" {
			cfg.Tenants[i].Selection.Precedence.ConflictPolicy = "error" // Strict by default
		}

		// Set semantic cache defaults
		if cfg.Tenants[i].SemanticCache.TTLSeconds == 0 {
			cfg.Tenants[i].SemanticCache.TTLSeconds = 86400 // 24h default
		}

		// Set alert defaults if alerts are enabled
		if cfg.Tenants[i].Alerts.Enabled {
			if len(cfg.Tenants[i].Alerts.BudgetThresholds) == 0 {
				cfg.Tenants[i].Alerts.BudgetThresholds = []float64{0.7, 0.85, 1.0}
			}
			if cfg.Tenants[i].Alerts.WebhookTimeoutMs == 0 {
				cfg.Tenants[i].Alerts.WebhookTimeoutMs = 2000
			}
		}
	}

	// Compute SHA256 of the raw config file for traceability.
	sum := sha256.Sum256(data)
	cfg.SHA256 = hex.EncodeToString(sum[:])

	// Resolve config version; default to "unknown" if the block is absent.
	cfg.Version = cfg.ConfigMeta.Version
	if cfg.Version == "" {
		cfg.Version = "unknown"
	}

	return &cfg, nil
}

// TenantByAPIKey returns the tenant config matching the given API key, or nil.
func (c *Config) TenantByAPIKey(apiKey string) *TenantConfig {
	for i := range c.Tenants {
		for _, k := range c.Tenants[i].APIKeys {
			if k == apiKey {
				return &c.Tenants[i]
			}
		}
	}
	return nil
}

// TenantByID finds tenant by ID, returns nil if not found
func (c *Config) TenantByID(id string) *TenantConfig {
	for i := range c.Tenants {
		if c.Tenants[i].ID == id {
			return &c.Tenants[i]
		}
	}
	return nil
}

// ModelByName returns the model config matching the given name, or nil.
func (c *Config) ModelByName(name string) *ModelConfig {
	for i := range c.Models {
		if c.Models[i].Name == name {
			return &c.Models[i]
		}
	}
	return nil
}

// AllowedModelsForTenant returns the full model configs allowed for a tenant.
func (c *Config) AllowedModelsForTenant(t *TenantConfig) []ModelConfig {
	allowed := make(map[string]bool, len(t.AllowedModels))
	for _, m := range t.AllowedModels {
		allowed[m] = true
	}
	var result []ModelConfig
	for _, m := range c.Models {
		if allowed[m.Name] {
			result = append(result, m)
		}
	}
	return result
}

// Storage is a minimal interface for config resolution (avoids circular import)
type Storage interface {
	GetTenantConfig(ctx context.Context, tenantID string) (json.RawMessage, int, bool, error)
	GetGlobalConfig(ctx context.Context) (json.RawMessage, int, bool, error)
}

// ResolveTenantConfig resolves tenant config from cache → DB → YAML fallback
// This enables dynamic configuration updates without service restart
func (c *Config) ResolveTenantConfig(
	ctx context.Context,
	tenantID string,
	cache *TenantConfigCache,
	store Storage,
) (*TenantConfig, error) {
	// 1. Try cache (hot path)
	if cache != nil {
		if config, _, ok := cache.Get(tenantID); ok {
			return config, nil
		}
	}

	// 2. Try DB (if available and dynamic config is enabled)
	if store != nil && c.DynamicConfig.Enabled {
		configJSON, version, exists, err := store.GetTenantConfig(ctx, tenantID)
		if err != nil {
			// Log warning but continue to YAML fallback
			// (DB errors shouldn't block requests if YAML config exists)
		} else if exists {
			var config TenantConfig
			if err := json.Unmarshal(configJSON, &config); err != nil {
				return nil, fmt.Errorf("unmarshal tenant config: %w", err)
			}

			// Set tenant ID (not stored in DB, but needed for context)
			config.ID = tenantID

			// Cache it
			if cache != nil {
				cache.Set(tenantID, &config, version)
			}

			return &config, nil
		}
	}

	// 3. Fallback to YAML
	yamlConfig := c.TenantByID(tenantID)
	if yamlConfig == nil {
		return nil, nil // Tenant not found
	}

	return yamlConfig, nil
}
