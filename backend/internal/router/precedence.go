package router

import (
	"fmt"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// PrecedenceResult holds the outcome of precedence resolution (P0-P5)
type PrecedenceResult struct {
	ForcedModel      string              // Empty if no forced model
	Candidates       []config.ModelConfig // Filtered candidate pool
	DecisionReason   string              // Human-readable reason (e.g., "explicit_header|group:cheap")
	RequestedSource  string              // "none" | "body" | "header"
	RequestedModel   string              // The model requested (if any)
	RouteGroupUsed   string              // The route group name (if any)
}

// PrecedenceResolver implements the P0-P5 precedence hierarchy
type PrecedenceResolver struct {
	tenantConfig *config.TenantConfig
	globalModels []config.ModelConfig
	targetType   string // "chat" (default) or "embedding"
}

// NewPrecedenceResolver creates a chat-mode resolver for a tenant.
// Embedding-type models are excluded from the candidate pool (P0).
func NewPrecedenceResolver(tenant *config.TenantConfig, globalModels []config.ModelConfig) *PrecedenceResolver {
	return &PrecedenceResolver{
		tenantConfig: tenant,
		globalModels: globalModels,
		targetType:   "chat",
	}
}

// NewEmbeddingPrecedenceResolver creates an embedding-mode resolver for a tenant.
// Only models with type=="embedding" are included in the candidate pool (P0).
func NewEmbeddingPrecedenceResolver(tenant *config.TenantConfig, globalModels []config.ModelConfig) *PrecedenceResolver {
	return &PrecedenceResolver{
		tenantConfig: tenant,
		globalModels: globalModels,
		targetType:   "embedding",
	}
}

// Resolve applies the P0-P5 precedence algorithm
func (pr *PrecedenceResolver) Resolve(bodyModel, headerModel, routeGroup string) (PrecedenceResult, error) {
	result := PrecedenceResult{
		RequestedSource: "none",
		RouteGroupUsed:  routeGroup,
	}

	// P0: Security - filter to allowed models
	allowedSet := make(map[string]bool, len(pr.tenantConfig.AllowedModels))
	for _, m := range pr.tenantConfig.AllowedModels {
		allowedSet[m] = true
	}

	candidates := make([]config.ModelConfig, 0, len(pr.tenantConfig.AllowedModels))
	for _, m := range pr.globalModels {
		if !allowedSet[m.Name] {
			continue
		}
		if pr.targetType == "embedding" {
			// Embedding endpoint: only include explicit embedding models.
			if m.Type == "embedding" {
				candidates = append(candidates, m)
			}
		} else {
			// Chat endpoint: exclude embedding-only models.
			// Models with type="embedding" must not be routed through ChatCompletions;
			// they are handled exclusively by the /v1/embeddings endpoint.

			// Modified by hand  DIEGO
			// if m.Type == "" || m.Type == "chat" {
			if m.Type == "" || m.Type == "chat" || m.Type == "llm"{
				candidates = append(candidates, m)
			}
		}
	}

	if len(candidates) == 0 {
		return result, &ErrNoAllowedModels{TenantID: pr.tenantConfig.ID}
	}

	// P1: Parse Intent - determine requested model and source
	requestedModel := ""
	requestedSource := "none"

	// P2: Resolve Model Precedence
	precedenceMode := pr.tenantConfig.Selection.Precedence.Model
	if precedenceMode == "" {
		precedenceMode = "header" // Default
	}

	if precedenceMode == "header" {
		// Header wins over body
		if headerModel != "" {
			requestedModel = headerModel
			requestedSource = "header"
		} else if bodyModel != "" {
			requestedModel = bodyModel
			requestedSource = "body"
		}
	} else {
		// Body wins over header (legacy mode)
		if bodyModel != "" {
			requestedModel = bodyModel
			requestedSource = "body"
		} else if headerModel != "" {
			requestedModel = headerModel
			requestedSource = "header"
		}
	}

	result.RequestedSource = requestedSource
	result.RequestedModel = requestedModel

	// P3: Apply Route Group filtering
	if routeGroup != "" {
		groupModels, exists := pr.tenantConfig.Selection.RouteGroups[routeGroup]
		if !exists {
			return result, &ErrRouteGroupNotFound{RouteGroup: routeGroup}
		}

		// Filter candidates to route group
		groupSet := make(map[string]bool, len(groupModels))
		for _, m := range groupModels {
			groupSet[m] = true
		}

		filteredCandidates := make([]config.ModelConfig, 0)
		for _, c := range candidates {
			if groupSet[c.Name] {
				filteredCandidates = append(filteredCandidates, c)
			}
		}

		// Check for conflict between forced model and route group
		if requestedModel != "" && !groupSet[requestedModel] {
			// Conflict: forced model not in route group
			conflictPolicy := pr.tenantConfig.Selection.Precedence.ConflictPolicy
			if conflictPolicy == "" {
				conflictPolicy = "error" // Default
			}

			switch conflictPolicy {
			case "error":
				return result, &ErrModelNotInRouteGroup{Model: requestedModel, RouteGroup: routeGroup}
			case "ignore_group":
				// Keep original candidates, ignore route group
				result.DecisionReason = fmt.Sprintf("explicit_%s|group_ignored", requestedSource)
				result.ForcedModel = requestedModel
				result.Candidates = candidates
				return result, nil
			case "ignore_model":
				// Ignore forced model, use route group
				requestedModel = ""
				requestedSource = "none"
				result.DecisionReason = fmt.Sprintf("group:%s|model_ignored", routeGroup)
				result.ForcedModel = ""
				result.RequestedModel = ""
				result.RequestedSource = "none"
				result.Candidates = filteredCandidates
				return result, nil
			default:
				return result, fmt.Errorf("invalid conflict_policy: %s", conflictPolicy)
			}
		}

		candidates = filteredCandidates

		if len(candidates) == 0 {
			return result, fmt.Errorf("route group '%s' resulted in empty candidate pool", routeGroup)
		}
	}

	// P4: Validate forced model is in candidate pool
	if requestedModel != "" {
		found := false
		for _, c := range candidates {
			if c.Name == requestedModel {
				found = true
				break
			}
		}

		if !found {
			return result, &ErrModelNotAllowed{Model: requestedModel, TenantID: pr.tenantConfig.ID}
		}

		result.ForcedModel = requestedModel
		if routeGroup != "" {
			result.DecisionReason = fmt.Sprintf("explicit_%s|group:%s", requestedSource, routeGroup)
		} else {
			result.DecisionReason = fmt.Sprintf("explicit_%s", requestedSource)
		}
	} else {
		// No forced model - strategy will select from candidates
		if routeGroup != "" {
			result.DecisionReason = fmt.Sprintf("group:%s|strategy", routeGroup)
		} else {
			result.DecisionReason = "strategy"
		}
	}

	result.Candidates = candidates
	return result, nil
}
