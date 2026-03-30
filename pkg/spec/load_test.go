package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTeam(t *testing.T) {
	tmpDir := t.TempDir()
	teamPath := filepath.Join(tmpDir, "team.yaml")
	content := `
name: demo
description: Demo team
skills:
  - name: github
    version: ">=0.1.0"
    source:
      type: registry
agents:
  - name: captain
    role: captain
    goal: Lead delivery
    required_skills: [github]
channels:
  - kind: cli
    enabled: true
`
	if err := os.WriteFile(teamPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	team, err := LoadTeam(teamPath)
	if err != nil {
		t.Fatalf("LoadTeam returned error: %v", err)
	}

	if team.Name != "demo" {
		t.Fatalf("unexpected team name %q", team.Name)
	}
	if team.BaseDir != tmpDir {
		t.Fatalf("unexpected base dir %q", team.BaseDir)
	}
	if len(team.RequiredSkillRequirements()) != 1 {
		t.Fatalf("expected 1 skill requirement, got %d", len(team.RequiredSkillRequirements()))
	}
}
