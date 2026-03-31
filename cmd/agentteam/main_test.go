package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvIfPresent(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("OPENAI_API_KEY=test-key\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Unsetenv("OPENAI_API_KEY")

	if err := loadDotEnvIfPresent(envPath); err != nil {
		t.Fatalf("loadDotEnvIfPresent returned error: %v", err)
	}
	if got := os.Getenv("OPENAI_API_KEY"); got != "test-key" {
		t.Fatalf("unexpected env value %q", got)
	}
}
