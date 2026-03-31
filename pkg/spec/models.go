package spec

import (
	"fmt"
	"os"
	"strings"
)

type ResolvedProvider struct {
	Name      string
	Kind      string
	BaseURL   string
	APIKeyEnv string
	HasAPIKey bool
}

func (t *TeamSpec) ResolveProviders() []ResolvedProvider {
	out := make([]ResolvedProvider, 0, len(t.Models.Providers))
	for name, provider := range t.Models.Providers {
		key := provider.APIKeyValue()
		out = append(out, ResolvedProvider{
			Name:      name,
			Kind:      provider.Kind,
			BaseURL:   provider.BaseURL,
			APIKeyEnv: provider.APIKeyEnv,
			HasAPIKey: key != "",
		})
	}
	return out
}

func (t *TeamSpec) ValidateModelCredentials() error {
	for name, provider := range t.Models.Providers {
		if strings.TrimSpace(provider.APIKey) != "" {
			continue
		}
		if strings.TrimSpace(provider.APIKeyEnv) == "" {
			continue
		}
		if strings.TrimSpace(os.Getenv(provider.APIKeyEnv)) == "" {
			return fmt.Errorf("model provider %q is missing env var %s", name, provider.APIKeyEnv)
		}
	}
	return nil
}

func (t *TeamSpec) ExplainModelSetup() string {
	lines := []string{
		"Model configuration",
		"",
		"Define providers in team.yaml under models.providers.",
		"Store API keys in environment variables and reference them with api_key_env.",
	}
	if strings.TrimSpace(t.Models.DefaultModel) != "" {
		lines = append(lines, fmt.Sprintf("Default model: %s", t.Models.DefaultModel))
	}
	if len(t.Models.Providers) == 0 {
		lines = append(lines, "No providers are configured yet.")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "", "Providers:")
	for name, provider := range t.Models.Providers {
		env := provider.APIKeyEnv
		if env == "" {
			env = "<inline-or-unset>"
		}
		lines = append(lines, fmt.Sprintf("- %s (%s) key via %s", name, provider.Kind, env))
	}
	return strings.Join(lines, "\n")
}

func (p ProviderSpec) APIKeyValue() string {
	key := strings.TrimSpace(p.APIKey)
	if key == "" && strings.TrimSpace(p.APIKeyEnv) != "" {
		key = os.Getenv(p.APIKeyEnv)
	}
	return key
}
