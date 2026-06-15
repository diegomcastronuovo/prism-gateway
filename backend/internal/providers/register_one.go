package providers

import (
	"fmt"
	"os"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

func apiKeyFromEnv(pc config.ProviderConfig) string {
	if pc.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(pc.APIKeyEnv)
}

// instantiateProviders builds chat and optional embedding implementations for one ProviderConfig.
func instantiateProviders(pc config.ProviderConfig) (Provider, EmbeddingProvider, error) {
	apiKey := apiKeyFromEnv(pc)
	switch pc.Type {
	case "openai", "local":
		p := NewOpenAI(pc.BaseURL, apiKey)
		return p, p, nil
	case "anthropic":
		return NewAnthropic(pc.BaseURL, apiKey), nil, nil
	case "gemini":
		p := NewGemini(pc.BaseURL, apiKey)
		return p, p, nil
	case "xai":
		return NewXAI(pc.BaseURL, apiKey), nil, nil
	case "cohere":
		return nil, NewCohereEmbedding(pc.BaseURL, apiKey), nil
	case "aws_bedrock":
		p, err := NewBedrock(pc)
		if err != nil {
			return nil, nil, err
		}
		return p, nil, nil
	default:
		p := NewOpenAI(pc.BaseURL, apiKey)
		return p, p, nil
	}
}

// ProviderPairFromConfig is used for lazy resolution (providers added in DB after process start).
func ProviderPairFromConfig(pc config.ProviderConfig) (Provider, EmbeddingProvider, error) {
	return instantiateProviders(pc)
}

// RegisterOne registers a single provider entry into reg (same semantics as BuildFromConfig loop).
func RegisterOne(reg *Registry, name string, pc config.ProviderConfig) error {
	chat, emb, err := instantiateProviders(pc)
	if err != nil {
		return fmt.Errorf("provider %q (%s): %w", name, pc.Type, err)
	}
	if chat != nil {
		reg.Register(name, chat)
	}
	if emb != nil {
		reg.RegisterEmbedding(name, emb)
	}
	// Auto-register NativeMessagesProvider if the chat provider supports it.
	// Currently only *Anthropic implements this; zero impact on other providers.
	if nmp, ok := chat.(NativeMessagesProvider); ok {
		reg.RegisterNativeMessages(name, nmp)
	}
	return nil
}
