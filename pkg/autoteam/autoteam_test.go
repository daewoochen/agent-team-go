package autoteam

import (
	"os"
	"testing"
)

func TestGuess(t *testing.T) {
	cases := []struct {
		task string
		want Profile
	}{
		{task: "Compare the top Go agent runtimes and write a recommendation", want: ProfileResearch},
		{task: "Handle a sev1 incident and prepare mitigation updates", want: ProfileIncident},
		{task: "Draft a launch blog post and campaign copy", want: ProfileContent},
		{task: "Ship the MVP release plan", want: ProfileSoftware},
	}

	for _, tc := range cases {
		if got := Guess(tc.task); got != tc.want {
			t.Fatalf("Guess(%q) = %s, want %s", tc.task, got, tc.want)
		}
	}
}

func TestBuildUsesOpenAIWhenAPIKeyPresent(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", "https://example.invalid/v1")

	team, profile, err := Build("Ship the MVP release plan", Options{Profile: "auto", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if profile != ProfileSoftware {
		t.Fatalf("unexpected profile %s", profile)
	}
	if team.Models.DefaultModel != "openai/gpt-4.1-mini" {
		t.Fatalf("unexpected default model %q", team.Models.DefaultModel)
	}
	if provider, ok := team.Models.Providers["openai"]; !ok || provider.APIKeyEnv != "OPENAI_API_KEY" {
		t.Fatalf("expected openai provider with env binding")
	}
}

func TestBuildAutoEnablesConfiguredChannels(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat-1")
	t.Setenv("FEISHU_APP_ID", "app-id")
	t.Setenv("FEISHU_APP_SECRET", "app-secret")
	t.Setenv("FEISHU_CHAT_ID", "oc_x")

	team, _, err := Build("Coordinate the weekly launch response", Options{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	kinds := make(map[string]bool)
	for _, channel := range team.Channels {
		kinds[channel.Kind] = channel.Enabled
	}
	if !kinds["cli"] || !kinds["telegram"] || !kinds["feishu"] {
		t.Fatalf("expected auto team to enable cli, telegram, and feishu when env vars are present")
	}
}

func TestBuildDefaultsToMockWithoutAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	_ = os.Unsetenv("OPENAI_BASE_URL")

	team, _, err := Build("Coordinate a support response", Options{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if team.Models.DefaultModel != "mock/generalist" {
		t.Fatalf("unexpected default model %q", team.Models.DefaultModel)
	}
}
