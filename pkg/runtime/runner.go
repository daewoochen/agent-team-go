package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/daewoochen/agent-team-go/pkg/agents"
	"github.com/daewoochen/agent-team-go/pkg/channels"
	"github.com/daewoochen/agent-team-go/pkg/observe"
	"github.com/daewoochen/agent-team-go/pkg/policy"
	"github.com/daewoochen/agent-team-go/pkg/skills"
	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type Runner struct {
	workDir   string
	installer *skills.Installer
}

func NewRunner(workDir string) *Runner {
	return &Runner{
		workDir:   filepath.Clean(workDir),
		installer: skills.NewInstaller(filepath.Join(filepath.Clean(workDir), ".agentteam", "skills")),
	}
}

func (r *Runner) Run(ctx context.Context, team *spec.TeamSpec, task string) (*RunResult, error) {
	if err := team.Validate(); err != nil {
		return nil, err
	}
	if err := channels.ValidateTeam(team); err != nil {
		return nil, err
	}

	for _, skillReq := range team.RequiredSkillRequirements() {
		if err := policy.CanInstallSkill(team, skillReq); err != nil {
			return nil, err
		}
	}
	if _, err := r.installer.EnsureFromTeam(team); err != nil {
		return nil, err
	}

	runID := time.Now().UTC().Format("20060102T150405Z")
	events := make([]RunEvent, 0, 16)
	artifacts := make([]Artifact, 0, 8)
	now := func() time.Time { return time.Now().UTC() }
	appendEvent := func(event RunEvent) {
		event.Timestamp = now()
		events = append(events, event)
	}

	captain, ok := agents.FindByRole(team, "captain")
	if !ok {
		return nil, fmt.Errorf("captain agent is required")
	}
	appendEvent(RunEvent{
		Type:    "run.started",
		Actor:   captain.Name,
		Message: fmt.Sprintf("Captain received task: %s", task),
	})

	planner, hasPlanner := agents.FindByRole(team, "planner")
	planSummary := "Work directly from captain judgment."
	if hasPlanner {
		delegation := Delegation{
			From:              captain.Name,
			To:                planner.Name,
			TaskID:            "plan-001",
			Budget:            1,
			Deadline:          now().Add(30 * time.Minute).Format(time.RFC3339),
			ExpectedArtifacts: []string{"execution-plan.md"},
			Reason:            "Break the request into executable work items.",
		}
		appendEvent(RunEvent{
			Type:       "delegation.created",
			Actor:      captain.Name,
			Message:    "Captain delegated planning to planner.",
			Delegation: &delegation,
		})
		planSummary = buildPlan(task)
		artifact := Artifact{
			Name:     "execution-plan.md",
			Producer: planner.Name,
			Content:  planSummary,
		}
		artifacts = append(artifacts, artifact)
		appendEvent(RunEvent{
			Type:     "artifact.created",
			Actor:    planner.Name,
			Message:  "Planner produced the execution plan.",
			Artifact: &artifact,
		})
	}

	specialistRoles := []string{"researcher", "coder", "reviewer"}
	for _, role := range specialistRoles {
		agent, ok := agents.FindByRole(team, role)
		if !ok {
			continue
		}
		delegation := Delegation{
			From:              captain.Name,
			To:                agent.Name,
			TaskID:            fmt.Sprintf("%s-001", role),
			Budget:            1,
			Deadline:          now().Add(45 * time.Minute).Format(time.RFC3339),
			ExpectedArtifacts: []string{fmt.Sprintf("%s-report.md", role)},
			Reason:            fmt.Sprintf("Contribute %s-specific output to the final result.", role),
		}
		appendEvent(RunEvent{
			Type:       "delegation.created",
			Actor:      captain.Name,
			Message:    fmt.Sprintf("Captain delegated work to %s.", agent.Name),
			Delegation: &delegation,
		})

		artifact := Artifact{
			Name:     fmt.Sprintf("%s-report.md", role),
			Producer: agent.Name,
			Content:  specialistOutput(role, task, planSummary),
		}
		artifacts = append(artifacts, artifact)
		appendEvent(RunEvent{
			Type:     "artifact.created",
			Actor:    agent.Name,
			Message:  fmt.Sprintf("%s delivered their artifact.", strings.Title(role)),
			Artifact: &artifact,
		})
	}

	summary := summarize(task, planSummary, artifacts)
	appendEvent(RunEvent{
		Type:    "run.completed",
		Actor:   captain.Name,
		Message: "Captain assembled the final response.",
	})

	result := &RunResult{
		RunID:     runID,
		Summary:   summary,
		Events:    events,
		Artifacts: artifacts,
	}
	result.ReplayPath = filepath.Join(r.workDir, ".agentteam", "runs", runID+".json")
	if err := observe.WriteJSON(result.ReplayPath, result); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return result, nil
	}
}

func buildPlan(task string) string {
	return strings.Join([]string{
		"# Execution Plan",
		"",
		fmt.Sprintf("1. Clarify the goal behind: %s", task),
		"2. Assign research, implementation, and review work in parallel.",
		"3. Reconcile artifacts and produce a final recommendation.",
	}, "\n")
}

func specialistOutput(role, task, plan string) string {
	switch role {
	case "researcher":
		return fmt.Sprintf("Research brief for %q\n\n- Capture assumptions.\n- Identify external dependencies.\n- Surface launch risks.\n\nPlan basis:\n%s", task, plan)
	case "coder":
		return fmt.Sprintf("Implementation notes for %q\n\n- Define the MVP surface.\n- Land the CLI and runtime loop.\n- Keep extension points stable for future MCP/A2A work.", task)
	case "reviewer":
		return fmt.Sprintf("Review checklist for %q\n\n- Validate delegation traces.\n- Confirm skills auto-install path.\n- Verify channel configuration and docs quality.", task)
	default:
		return fmt.Sprintf("Artifact for %q", task)
	}
}

func summarize(task, plan string, artifacts []Artifact) string {
	lines := []string{
		fmt.Sprintf("Team completed task: %s", task),
		"",
		"Planning baseline:",
		plan,
		"",
		"Artifacts:",
	}
	for _, artifact := range artifacts {
		lines = append(lines, fmt.Sprintf("- %s from %s", artifact.Name, artifact.Producer))
	}
	lines = append(lines, "", "This run produced a replay log and structured delegation events.")
	return strings.Join(lines, "\n")
}
