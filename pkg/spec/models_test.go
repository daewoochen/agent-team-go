package spec

import (
	"os"
	"testing"
)

func TestValidateModelCredentials(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret")
	team := &TeamSpec{
		Name: "demo",
		Models: ModelConfig{
			DefaultModel: "openai/gpt-4.1-mini",
			Providers: map[string]ProviderSpec{
				"openai": {
					Kind:      "openai-compatible",
					APIKeyEnv: "OPENAI_API_KEY",
				},
			},
		},
		Agents: []AgentSpec{
			{Name: "captain", Role: "captain", Goal: "Lead", Model: "openai/gpt-4.1"},
		},
	}
	if err := team.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if err := team.ValidateModelCredentials(); err != nil {
		t.Fatalf("ValidateModelCredentials returned error: %v", err)
	}
}

func TestValidateModelCredentialsFailsWhenEnvMissing(t *testing.T) {
	_ = os.Unsetenv("MISSING_KEY")
	team := &TeamSpec{
		Name: "demo",
		Models: ModelConfig{
			DefaultModel: "openai/gpt-4.1-mini",
			Providers: map[string]ProviderSpec{
				"openai": {
					Kind:      "openai-compatible",
					APIKeyEnv: "MISSING_KEY",
				},
			},
		},
		Agents: []AgentSpec{
			{Name: "captain", Role: "captain", Goal: "Lead", Model: "openai/gpt-4.1"},
		},
	}
	if err := team.ValidateModelCredentials(); err == nil {
		t.Fatalf("expected missing env error")
	}
}
