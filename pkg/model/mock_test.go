package model

import (
	"context"
	"strings"
	"testing"
)

func TestMockProviderGenerate(t *testing.T) {
	provider := NewMockProvider()
	out, err := provider.Generate(context.Background(), Prompt{
		AgentName: "captain",
		Goal:      "Lead delivery",
		Input:     "Ship a release",
		ModelRef:  "mock/captain",
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !strings.Contains(out, "captain") {
		t.Fatalf("expected output to mention agent, got %q", out)
	}
}
