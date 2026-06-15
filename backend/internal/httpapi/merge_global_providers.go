package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// MergeGlobalProvidersFromStore loads the active global config from the store and merges
// its providers map into cfg.Providers (YAML keys are the base; global keys override or add).
// Used at startup so providers.BuildFromConfig registers DB-only providers (e.g. aws_bedrock).
func MergeGlobalProvidersFromStore(ctx context.Context, cfg *config.Config, store storage.Storage, log *slog.Logger) {
	if cfg == nil || store == nil || !cfg.DynamicConfig.Enabled {
		return
	}
	raw, _, exists, err := store.GetGlobalConfig(ctx)
	if err != nil {
		if log != nil {
			log.WarnContext(ctx, "merge global providers: get global config failed", "error", err)
		}
		return
	}
	if !exists || len(raw) == 0 {
		return
	}
	var gc config.GlobalConfig
	if err := json.Unmarshal(raw, &gc); err != nil {
		if log != nil {
			log.WarnContext(ctx, "merge global providers: unmarshal failed", "error", err)
		}
		return
	}
	config.MergeGlobalProvidersIntoConfig(cfg, &gc)
	if log != nil && len(gc.Providers) > 0 {
		log.InfoContext(ctx, "merged global providers into runtime config for registry",
			"provider_keys", len(gc.Providers))
	}
}
