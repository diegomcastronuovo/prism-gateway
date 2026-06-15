package config

// MergeGlobalProvidersIntoConfig merges gc.Providers into cfg.Providers in place.
func MergeGlobalProvidersIntoConfig(cfg *Config, gc *GlobalConfig) {
	if cfg == nil || gc == nil || len(gc.Providers) == 0 {
		return
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig, len(gc.Providers))
	}
	for name, pc := range gc.Providers {
		cfg.Providers[name] = pc
	}
}
