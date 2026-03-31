package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearch(t *testing.T) {
	results := Search("telegram")
	if len(results) == 0 {
		t.Fatalf("expected search results")
	}
	if results[0].Name != "telegram-messenger" {
		t.Fatalf("unexpected first search result %q", results[0].Name)
	}
}

func TestListInstalled(t *testing.T) {
	baseDir := t.TempDir()
	skillDir := filepath.Join(baseDir, "research-kit")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, manifestFile), []byte("name: research-kit\nversion: 0.1.0\ndescription: Research helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	installer := NewInstaller(baseDir)
	installed, err := installer.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled returned error: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("expected 1 installed skill, got %d", len(installed))
	}
	if installed[0].Name != "research-kit" {
		t.Fatalf("unexpected installed skill %q", installed[0].Name)
	}
}

func TestScaffold(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "skills", "writer")
	out, err := Scaffold("writer", "Creates concise drafts.", targetDir)
	if err != nil {
		t.Fatalf("Scaffold returned error: %v", err)
	}
	if out != targetDir {
		t.Fatalf("unexpected scaffold output dir %q", out)
	}
	if _, err := os.Stat(filepath.Join(targetDir, manifestFile)); err != nil {
		t.Fatalf("expected manifest to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "prompt.md")); err != nil {
		t.Fatalf("expected prompt.md to exist: %v", err)
	}
}
