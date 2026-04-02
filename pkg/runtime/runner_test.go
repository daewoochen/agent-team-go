package runtime

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daewoochen/agent-team-go/pkg/model"
	"github.com/daewoochen/agent-team-go/pkg/observe"
	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type flakyProvider struct {
	failuresRemaining int
}

func (p *flakyProvider) Generate(context.Context, model.Prompt) (string, error) {
	if p.failuresRemaining > 0 {
		p.failuresRemaining--
		return "", errors.New("temporary upstream failure")
	}
	return "Recovered on retry", nil
}

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
		Models: spec.ModelConfig{
			DefaultModel: "mock/generalist",
			Providers: map[string]spec.ProviderSpec{
				"mock": {
					Kind: "mock",
				},
			},
		},
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
			{Name: "captain", Role: "captain", Goal: "Lead delivery", Model: "mock/captain", RequiredSkills: []string{"github"}},
			{Name: "planner", Role: "planner", Goal: "Plan the work", Model: "mock/planner"},
			{Name: "researcher", Role: "researcher", Goal: "Research risks", Model: "mock/researcher"},
			{Name: "reviewer", Role: "reviewer", Goal: "Review quality", Model: "mock/reviewer"},
		},
		Channels: []spec.ChannelConfig{
			{Kind: "cli", Enabled: true},
		},
		Policies: spec.PolicySpec{
			AllowExternalSkillInstall:  true,
			RequireApprovalForGitWrite: true,
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
	if len(result.WorkItems) == 0 {
		t.Fatalf("expected work items")
	}
	if len(result.ModelBindings) == 0 {
		t.Fatalf("expected model bindings")
	}
	if len(result.Deliveries) != 1 {
		t.Fatalf("expected 1 prepared delivery, got %d", len(result.Deliveries))
	}
	if result.CheckpointPath == "" {
		t.Fatalf("expected checkpoint path")
	}
	if _, err := os.Stat(result.ReplayPath); err != nil {
		t.Fatalf("expected replay log to exist: %v", err)
	}
	if _, err := os.Stat(result.CheckpointPath); err != nil {
		t.Fatalf("expected checkpoint to exist: %v", err)
	}
}

func TestRunnerRetriesFailedWorkItem(t *testing.T) {
	tmpDir := t.TempDir()
	localSkillDir := filepath.Join(tmpDir, "skills", "github")
	if err := os.MkdirAll(localSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localSkillDir, "skill.yaml"), []byte("name: github\nversion: 0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	team := &spec.TeamSpec{
		Name:        "retry-team",
		Description: "Demo",
		BaseDir:     tmpDir,
		Models: spec.ModelConfig{
			DefaultModel: "mock/generalist",
			Providers: map[string]spec.ProviderSpec{
				"mock": {
					Kind: "mock",
				},
				"flaky": {
					Kind: "flaky",
				},
			},
		},
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
			{Name: "captain", Role: "captain", Goal: "Lead delivery", Model: "mock/captain", RequiredSkills: []string{"github"}},
			{Name: "planner", Role: "planner", Goal: "Plan the work", Model: "mock/planner"},
			{Name: "researcher", Role: "researcher", Goal: "Research risks", Model: "flaky/researcher", MaxAttempts: 2},
			{Name: "reviewer", Role: "reviewer", Goal: "Review quality", Model: "mock/reviewer"},
		},
		Channels: []spec.ChannelConfig{
			{Kind: "cli", Enabled: true},
		},
	}

	runner := NewRunner(tmpDir)
	provider := &flakyProvider{failuresRemaining: 1}
	runner.modelFactory.Register("flaky", func(_ *http.Client, _ spec.ProviderSpec) (model.Provider, error) {
		return provider, nil
	})

	result, err := runner.Run(context.Background(), team, "Ship a public MVP")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	retried := false
	for _, event := range result.Events {
		if event.Type == "work_item.retry_scheduled" {
			retried = true
			break
		}
	}
	if !retried {
		t.Fatalf("expected retry event in run trace")
	}

	researchIdx := indexWorkItem(result.WorkItems, "researcher-001")
	if researchIdx < 0 {
		t.Fatalf("expected researcher work item")
	}
	if result.WorkItems[researchIdx].Status != StatusCompleted {
		t.Fatalf("expected researcher work item to complete, got %s", result.WorkItems[researchIdx].Status)
	}
	if result.WorkItems[researchIdx].Attempt != 2 {
		t.Fatalf("expected researcher attempt to be 2, got %d", result.WorkItems[researchIdx].Attempt)
	}
}

func TestRunnerBlocksDependentWorkItemAfterFailure(t *testing.T) {
	tmpDir := t.TempDir()
	team := &spec.TeamSpec{
		Name:        "blocked-team",
		Description: "Demo",
		BaseDir:     tmpDir,
		Models: spec.ModelConfig{
			DefaultModel: "mock/generalist",
			Providers: map[string]spec.ProviderSpec{
				"mock": {
					Kind: "mock",
				},
				"flaky": {
					Kind: "flaky",
				},
			},
		},
		Agents: []spec.AgentSpec{
			{Name: "captain", Role: "captain", Goal: "Lead delivery", Model: "mock/captain"},
			{Name: "planner", Role: "planner", Goal: "Plan the work", Model: "mock/planner"},
			{Name: "researcher", Role: "researcher", Goal: "Research risks", Model: "flaky/researcher", MaxAttempts: 1},
			{Name: "reviewer", Role: "reviewer", Goal: "Review quality", Model: "mock/reviewer"},
		},
		Channels: []spec.ChannelConfig{
			{Kind: "cli", Enabled: true},
		},
	}

	runner := NewRunner(tmpDir)
	provider := &flakyProvider{failuresRemaining: 2}
	runner.modelFactory.Register("flaky", func(_ *http.Client, _ spec.ProviderSpec) (model.Provider, error) {
		return provider, nil
	})

	result, err := runner.Run(context.Background(), team, "Ship a public MVP")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	researchIdx := indexWorkItem(result.WorkItems, "researcher-001")
	if researchIdx < 0 {
		t.Fatalf("expected researcher work item")
	}
	if result.WorkItems[researchIdx].Status != StatusFailed {
		t.Fatalf("expected researcher to fail, got %s", result.WorkItems[researchIdx].Status)
	}

	reviewerIdx := indexWorkItem(result.WorkItems, "reviewer-001")
	if reviewerIdx < 0 {
		t.Fatalf("expected reviewer work item")
	}
	if result.WorkItems[reviewerIdx].Status != StatusFailed {
		t.Fatalf("expected reviewer to be blocked and failed, got %s", result.WorkItems[reviewerIdx].Status)
	}

	blocked := false
	for _, event := range result.Events {
		if event.Type == "work_item.blocked" && event.WorkItem != nil && event.WorkItem.ID == "reviewer-001" {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatalf("expected blocked work item event for reviewer")
	}
}

func TestRunnerPausesAndResumesAfterManualApproval(t *testing.T) {
	tmpDir := t.TempDir()
	team := &spec.TeamSpec{
		Name:        "approval-team",
		Description: "Demo",
		BaseDir:     tmpDir,
		Models: spec.ModelConfig{
			DefaultModel: "mock/generalist",
			Providers: map[string]spec.ProviderSpec{
				"mock": {
					Kind: "mock",
				},
			},
		},
		Agents: []spec.AgentSpec{
			{Name: "captain", Role: "captain", Goal: "Lead delivery", Model: "mock/captain"},
			{Name: "planner", Role: "planner", Goal: "Plan the work", Model: "mock/planner"},
			{Name: "researcher", Role: "researcher", Goal: "Research risks", Model: "mock/researcher"},
		},
		Channels: []spec.ChannelConfig{
			{Kind: "cli", Enabled: true},
		},
		Policies: spec.PolicySpec{
			RequireApprovalForMessages: true,
			ApprovalMode:               "manual",
		},
	}

	runner := NewRunner(tmpDir)
	initial, err := runner.Run(context.Background(), team, "Ship a public MVP")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if initial.Status != RunStatusWaitingApproval {
		t.Fatalf("expected waiting approval status, got %s", initial.Status)
	}
	if pending := countPendingApprovals(initial.Approvals); pending == 0 {
		t.Fatalf("expected pending approvals")
	}

	var checkpoint Checkpoint
	if err := observe.ReadJSON(initial.CheckpointPath, &checkpoint); err != nil {
		t.Fatalf("failed to load checkpoint: %v", err)
	}
	for i := range checkpoint.Approvals {
		checkpoint.Approvals[i].Approved = true
		checkpoint.Approvals[i].Decision = ApprovalApproved
		checkpoint.Approvals[i].Note = "Use a conservative rollout and mention the operator review."
	}
	if err := observe.WriteJSON(initial.CheckpointPath, &checkpoint); err != nil {
		t.Fatalf("failed to write checkpoint: %v", err)
	}

	resumed, err := runner.Resume(context.Background(), team, initial.CheckpointPath)
	if err != nil {
		t.Fatalf("Resume returned error: %v", err)
	}
	if resumed.Status != RunStatusCompleted {
		t.Fatalf("expected completed status, got %s", resumed.Status)
	}

	resumedEvent := false
	for _, event := range resumed.Events {
		if event.Type == "run.resumed" {
			resumedEvent = true
			break
		}
	}
	if !resumedEvent {
		t.Fatalf("expected run.resumed event")
	}
	if !strings.Contains(resumed.Summary, "conservative rollout") {
		t.Fatalf("expected resumed summary to include operator note, got %q", resumed.Summary)
	}
}

func TestRunnerEndsRejectedWhenApprovalIsRejected(t *testing.T) {
	tmpDir := t.TempDir()
	team := &spec.TeamSpec{
		Name:        "approval-team",
		Description: "Demo",
		BaseDir:     tmpDir,
		Models: spec.ModelConfig{
			DefaultModel: "mock/generalist",
			Providers: map[string]spec.ProviderSpec{
				"mock": {
					Kind: "mock",
				},
			},
		},
		Agents: []spec.AgentSpec{
			{Name: "captain", Role: "captain", Goal: "Lead delivery", Model: "mock/captain"},
			{Name: "planner", Role: "planner", Goal: "Plan the work", Model: "mock/planner"},
		},
		Channels: []spec.ChannelConfig{
			{Kind: "cli", Enabled: true},
		},
		Policies: spec.PolicySpec{
			RequireApprovalForMessages: true,
			ApprovalMode:               "manual",
		},
	}

	runner := NewRunner(tmpDir)
	initial, err := runner.Run(context.Background(), team, "Ship a public MVP")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var checkpoint Checkpoint
	if err := observe.ReadJSON(initial.CheckpointPath, &checkpoint); err != nil {
		t.Fatalf("failed to load checkpoint: %v", err)
	}
	checkpoint.Approvals[0].Approved = false
	checkpoint.Approvals[0].Decision = ApprovalRejected
	checkpoint.Approvals[0].Note = "Need a safer rollout first."
	if err := observe.WriteJSON(initial.CheckpointPath, &checkpoint); err != nil {
		t.Fatalf("failed to write checkpoint: %v", err)
	}

	resumed, err := runner.Resume(context.Background(), team, initial.CheckpointPath)
	if err != nil {
		t.Fatalf("Resume returned error: %v", err)
	}
	if resumed.Status != RunStatusRejected {
		t.Fatalf("expected rejected status, got %s", resumed.Status)
	}
	if !strings.Contains(resumed.Summary, "safer rollout") {
		t.Fatalf("expected rejection summary to include note, got %q", resumed.Summary)
	}
}

func TestRunnerRevisionRequestLoopsBackToWaitingApproval(t *testing.T) {
	tmpDir := t.TempDir()
	team := &spec.TeamSpec{
		Name:        "revision-team",
		Description: "Demo",
		BaseDir:     tmpDir,
		Models: spec.ModelConfig{
			DefaultModel: "mock/generalist",
			Providers: map[string]spec.ProviderSpec{
				"mock": {
					Kind: "mock",
				},
			},
		},
		Agents: []spec.AgentSpec{
			{Name: "captain", Role: "captain", Goal: "Lead delivery", Model: "mock/captain"},
			{Name: "planner", Role: "planner", Goal: "Plan the work", Model: "mock/planner"},
			{Name: "researcher", Role: "researcher", Goal: "Research risks", Model: "mock/researcher"},
		},
		Channels: []spec.ChannelConfig{
			{Kind: "cli", Enabled: true},
		},
		Policies: spec.PolicySpec{
			RequireApprovalForMessages: true,
			ApprovalMode:               "manual",
		},
	}

	runner := NewRunner(tmpDir)
	initial, err := runner.Run(context.Background(), team, "Ship a public MVP")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var checkpoint Checkpoint
	if err := observe.ReadJSON(initial.CheckpointPath, &checkpoint); err != nil {
		t.Fatalf("failed to load checkpoint: %v", err)
	}
	checkpoint.Approvals[0].Approved = false
	checkpoint.Approvals[0].Decision = ApprovalChanges
	checkpoint.Approvals[0].Note = "Add rollback guidance and a safer communication plan."
	if err := observe.WriteJSON(initial.CheckpointPath, &checkpoint); err != nil {
		t.Fatalf("failed to write checkpoint: %v", err)
	}

	revised, err := runner.Resume(context.Background(), team, initial.CheckpointPath)
	if err != nil {
		t.Fatalf("Resume returned error: %v", err)
	}
	if revised.Status != RunStatusWaitingApproval {
		t.Fatalf("expected waiting approval status after revision, got %s", revised.Status)
	}
	if pending := countPendingApprovals(revised.Approvals); pending == 0 {
		t.Fatalf("expected reopened approvals after revision")
	}
	if !strings.Contains(revised.Summary, "rollback guidance") {
		t.Fatalf("expected revised summary to include operator feedback, got %q", revised.Summary)
	}

	revisionEvent := false
	for _, event := range revised.Events {
		if event.Type == "run.revision_requested" {
			revisionEvent = true
			break
		}
	}
	if !revisionEvent {
		t.Fatalf("expected run.revision_requested event")
	}

	var revisedCheckpoint Checkpoint
	if err := observe.ReadJSON(initial.CheckpointPath, &revisedCheckpoint); err != nil {
		t.Fatalf("failed to reload checkpoint: %v", err)
	}
	for i := range revisedCheckpoint.Approvals {
		revisedCheckpoint.Approvals[i].Approved = true
		revisedCheckpoint.Approvals[i].Decision = ApprovalApproved
	}
	if err := observe.WriteJSON(initial.CheckpointPath, &revisedCheckpoint); err != nil {
		t.Fatalf("failed to write revised checkpoint: %v", err)
	}

	completed, err := runner.Resume(context.Background(), team, initial.CheckpointPath)
	if err != nil {
		t.Fatalf("second Resume returned error: %v", err)
	}
	if completed.Status != RunStatusCompleted {
		t.Fatalf("expected completed status after approval, got %s", completed.Status)
	}
}
