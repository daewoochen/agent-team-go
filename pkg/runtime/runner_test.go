package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

func TestRunnerRun(t *testing.T) {
	tmpDir := t.TempDir()
	localSkillDir := filepath.Join(tmpDir, "skills", "github")
	if err := os.MkdirAll(localSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localSkillDir, "skill.yaml"), []byte("name: github\nversion: 0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	team := &spec.TeamSpec{
		Name:        "software-team",
		Description: "Demo",
		BaseDir:     tmpDir,
		Skills: []spec.SkillRequirement{
			{
				Name:    "github",
				Version: ">=0.1.0",
				Source: spec.SkillSource{
					Type: "local",
					Path: "./skills/github",
				},
			},
		},
		Agents: []spec.AgentSpec{
			{Name: "captain", Role: "captain", Goal: "Lead delivery", RequiredSkills: []string{"github"}},
			{Name: "planner", Role: "planner", Goal: "Plan the work"},
			{Name: "researcher", Role: "researcher", Goal: "Research risks"},
			{Name: "reviewer", Role: "reviewer", Goal: "Review quality"},
		},
		Channels: []spec.ChannelConfig{
			{Kind: "cli", Enabled: true},
		},
		Policies: spec.PolicySpec{
			AllowExternalSkillInstall: true,
		},
	}

	runner := NewRunner(tmpDir)
	result, err := runner.Run(context.Background(), team, "Ship a public MVP")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ReplayPath == "" {
		t.Fatalf("expected replay path")
	}
	if len(result.Events) == 0 {
		t.Fatalf("expected events")
	}
	if _, err := os.Stat(result.ReplayPath); err != nil {
		t.Fatalf("expected replay log to exist: %v", err)
	}
}
