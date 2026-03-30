package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

func TestInstallRegistrySkill(t *testing.T) {
	installer := NewInstaller(t.TempDir())
	result, err := installer.Install(spec.SkillRequirement{
		Name:    "research-kit",
		Version: "latest",
		Source: spec.SkillSource{
			Type: "registry",
		},
	}, "")
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if !result.Installed {
		t.Fatalf("expected skill to be installed")
	}

	if _, err := os.Stat(filepath.Join(result.InstallDir, manifestFile)); err != nil {
		t.Fatalf("manifest not installed: %v", err)
	}
}

func TestInstallLocalSkill(t *testing.T) {
	src := filepath.Join(t.TempDir(), "github")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, manifestFile), []byte("name: github\nversion: 0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	installer := NewInstaller(filepath.Join(t.TempDir(), ".agentteam", "skills"))
	result, err := installer.Install(spec.SkillRequirement{
		Name: "github",
		Source: spec.SkillSource{
			Type: "local",
			Path: src,
		},
	}, "")
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.InstallDir, manifestFile)); err != nil {
		t.Fatalf("expected manifest in install dir: %v", err)
	}
}
