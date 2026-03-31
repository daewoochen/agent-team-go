package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/daewoochen/agent-team-go/pkg/agents"
	"github.com/daewoochen/agent-team-go/pkg/channels"
	"github.com/daewoochen/agent-team-go/pkg/model"
	"github.com/daewoochen/agent-team-go/pkg/observe"
	"github.com/daewoochen/agent-team-go/pkg/policy"
	"github.com/daewoochen/agent-team-go/pkg/skills"
	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type Runner struct {
	workDir      string
	installer    *skills.Installer
	modelFactory *model.Factory
}

func NewRunner(workDir string) *Runner {
	return &Runner{
		workDir:      filepath.Clean(workDir),
		installer:    skills.NewInstaller(filepath.Join(filepath.Clean(workDir), ".agentteam", "skills")),
		modelFactory: model.NewFactory(),
	}
}

func (r *Runner) Run(ctx context.Context, team *spec.TeamSpec, task string) (*RunResult, error) {
	if err := team.Validate(); err != nil {
		return nil, err
	}
	if err := channels.ValidateTeam(team); err != nil {
		return nil, err
	}
	if err := team.ValidateModelCredentials(); err != nil {
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
	events := make([]RunEvent, 0, 24)
	artifacts := make([]Artifact, 0, 12)
	workItems := make([]WorkItem, 0, 8)
	approvals := buildApprovals(team)
	modelBindings := buildModelBindings(team)
	now := func() time.Time { return time.Now().UTC() }
	appendEvent := func(event RunEvent) {
		event.Timestamp = now()
		events = append(events, event)
	}
	var err error

	captain, ok := agents.FindByRole(team, "captain")
	if !ok {
		return nil, fmt.Errorf("captain agent is required")
	}
	appendEvent(RunEvent{
		Type:    "run.started",
		Actor:   captain.Name,
		Message: fmt.Sprintf("Captain received task: %s", task),
	})
	for _, binding := range modelBindings {
		appendEvent(RunEvent{
			Type:    "model.bound",
			Actor:   binding.Agent,
			Message: fmt.Sprintf("%s uses %s", binding.Agent, binding.Model),
		})
	}
	for _, approval := range approvals {
		approvalCopy := approval
		appendEvent(RunEvent{
			Type:     "approval.requested",
			Actor:    captain.Name,
			Message:  fmt.Sprintf("Approval requested for %s", approval.Action),
			Approval: &approvalCopy,
		})
	}

	planner, hasPlanner := agents.FindByRole(team, "planner")
	planSummary := "Work directly from captain judgment."
	if hasPlanner {
		workItem := WorkItem{
			ID:                 "plan-001",
			Objective:          "Break the incoming task into executable work items.",
			Inputs:             []string{task},
			AcceptanceCriteria: "A captain-readable execution plan with clear specialist ownership.",
			Status:             StatusRunning,
		}
		workItems = append(workItems, workItem)
		delegation := Delegation{
			From:              captain.Name,
			To:                planner.Name,
			TaskID:            workItem.ID,
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
			WorkItem:   &workItem,
		})
		planSummary, err = r.generateAgentOutput(ctx, team, planner, "You are the planning agent for a multi-agent team.", buildPlanPrompt(task))
		if err != nil {
			return nil, err
		}
		specialistItems := buildWorkItems(team, task)
		for _, item := range specialistItems {
			item.Status = StatusPending
			workItems = append(workItems, item)
			itemCopy := item
			appendEvent(RunEvent{
				Type:     "work_item.created",
				Actor:    planner.Name,
				Message:  fmt.Sprintf("Planner created %s", item.ID),
				WorkItem: &itemCopy,
			})
		}
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
		workItems[0].Status = StatusCompleted
		if err := r.writeCheckpoint(runID, task, workItems, approvals, artifacts); err != nil {
			return nil, err
		}
	}

	for _, agent := range team.Agents {
		role := agent.Role
		if role == "captain" || role == "planner" {
			continue
		}
		itemIndex := indexWorkItem(workItems, role+"-001")
		if itemIndex >= 0 {
			if err := Transition(workItems[itemIndex].Status, StatusRunning); err == nil {
				workItems[itemIndex].Status = StatusRunning
			}
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
			Content:  "",
		}
		artifact.Content, err = r.generateAgentOutput(ctx, team, agent, specialistSystemPrompt(role), specialistPrompt(role, task, planSummary))
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
		appendEvent(RunEvent{
			Type:     "artifact.created",
			Actor:    agent.Name,
			Message:  fmt.Sprintf("%s delivered their artifact.", strings.Title(role)),
			Artifact: &artifact,
		})
		if itemIndex >= 0 {
			if err := Transition(workItems[itemIndex].Status, StatusCompleted); err == nil {
				workItems[itemIndex].Status = StatusCompleted
				itemCopy := workItems[itemIndex]
				appendEvent(RunEvent{
					Type:     "work_item.completed",
					Actor:    agent.Name,
					Message:  fmt.Sprintf("%s completed %s", agent.Name, itemCopy.ID),
					WorkItem: &itemCopy,
				})
			}
		}
		if err := r.writeCheckpoint(runID, task, workItems, approvals, artifacts); err != nil {
			return nil, err
		}
	}

	summary, err := r.generateAgentOutput(ctx, team, captain, "You are the captain who synthesizes all specialist work into a final answer.", summarizePrompt(task, planSummary, artifacts))
	if err != nil {
		return nil, err
	}
	appendEvent(RunEvent{
		Type:    "run.completed",
		Actor:   captain.Name,
		Message: "Captain assembled the final response.",
	})

	result := &RunResult{
		RunID:         runID,
		Summary:       summary,
		Events:        events,
		Artifacts:     artifacts,
		WorkItems:     workItems,
		Approvals:     approvals,
		ModelBindings: modelBindings,
	}
	result.ReplayPath = filepath.Join(r.workDir, ".agentteam", "runs", runID+".json")
	result.CheckpointPath = filepath.Join(r.workDir, ".agentteam", "checkpoints", runID+".json")
	if err := observe.WriteJSON(result.ReplayPath, result); err != nil {
		return nil, err
	}
	if err := r.writeCheckpoint(runID, task, workItems, approvals, artifacts); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return result, nil
	}
}

func buildPlanPrompt(task string) string {
	return strings.Join([]string{
		fmt.Sprintf("Create a concise execution plan for this task: %s", task),
		"Return a short markdown plan with 3 numbered steps.",
	}, "\n")
}

func buildWorkItems(team *spec.TeamSpec, task string) []WorkItem {
	items := make([]WorkItem, 0, len(team.Agents))
	lastSpecialistID := ""
	for _, agent := range team.Agents {
		if agent.Role == "captain" || agent.Role == "planner" {
			continue
		}

		item := WorkItem{
			ID:                 fmt.Sprintf("%s-001", agent.Role),
			Objective:          workObjective(agent.Role, task),
			Inputs:             []string{task},
			AcceptanceCriteria: workAcceptance(agent.Role),
			Status:             StatusPending,
		}
		if lastSpecialistID != "" && agent.Role == "reviewer" {
			item.Dependencies = []string{lastSpecialistID}
		}
		items = append(items, item)
		lastSpecialistID = item.ID
	}
	return items
}

func workObjective(role, task string) string {
	switch role {
	case "researcher":
		return fmt.Sprintf("Research risks, assumptions, and dependencies for %s", task)
	case "coder":
		return fmt.Sprintf("Shape the implementation path for %s", task)
	case "reviewer":
		return fmt.Sprintf("Review the plan and outputs for %s", task)
	default:
		return fmt.Sprintf("Contribute role-specific output for %s", task)
	}
}

func workAcceptance(role string) string {
	switch role {
	case "researcher":
		return "A concise risk brief for the captain."
	case "coder":
		return "A concrete implementation note or patch strategy."
	case "reviewer":
		return "A clear go/no-go review checklist."
	default:
		return "A useful artifact for the captain."
	}
}

func specialistPrompt(role, task, plan string) string {
	switch role {
	case "researcher":
		return fmt.Sprintf("Task: %s\n\nPlan basis:\n%s\n\nProduce a concise research brief with assumptions, dependencies, and risks.", task, plan)
	case "coder":
		return fmt.Sprintf("Task: %s\n\nPlan basis:\n%s\n\nProduce implementation notes for the MVP path.", task, plan)
	case "reviewer":
		return fmt.Sprintf("Task: %s\n\nPlan basis:\n%s\n\nProduce a review checklist focused on quality, safety, and release readiness.", task, plan)
	default:
		return fmt.Sprintf("Task: %s\n\nPlan basis:\n%s\n\nProduce a concise artifact for this role.", task, plan)
	}
}

func specialistSystemPrompt(role string) string {
	return fmt.Sprintf("You are the %s agent in a multi-agent team. Be concise, specific, and execution-focused.", role)
}

func summarizePrompt(task, plan string, artifacts []Artifact) string {
	lines := []string{
		fmt.Sprintf("Task: %s", task),
		"",
		"Planning baseline:",
		plan,
		"",
		"Artifacts:",
	}
	for _, artifact := range artifacts {
		lines = append(lines, fmt.Sprintf("## %s from %s\n%s", artifact.Name, artifact.Producer, artifact.Content))
	}
	lines = append(lines, "", "Produce the final captain summary in markdown. Mention the replayable nature of the run.")
	return strings.Join(lines, "\n")
}

func (r *Runner) writeCheckpoint(runID, task string, workItems []WorkItem, approvals []ApprovalRequest, artifacts []Artifact) error {
	completed := make([]string, 0, len(workItems))
	pending := make([]string, 0, len(workItems))
	for _, item := range workItems {
		switch item.Status {
		case StatusCompleted:
			completed = append(completed, item.ID)
		default:
			pending = append(pending, item.ID)
		}
	}
	sort.Strings(completed)
	sort.Strings(pending)

	checkpoint := Checkpoint{
		RunID:              runID,
		Task:               task,
		Timestamp:          time.Now().UTC(),
		CompletedWorkItems: completed,
		PendingWorkItems:   pending,
		Approvals:          approvals,
		Artifacts:          artifacts,
	}
	return observe.WriteJSON(filepath.Join(r.workDir, ".agentteam", "checkpoints", runID+".json"), checkpoint)
}

func buildApprovals(team *spec.TeamSpec) []ApprovalRequest {
	approvals := make([]ApprovalRequest, 0, 4)
	if team.Policies.RequireApprovalForGitWrite {
		approvals = append(approvals, ApprovalRequest{
			ID:        "approval-git-write",
			Action:    "git.write",
			Target:    "repository",
			Reason:    "Coder or release steps may mutate repository state.",
			Approved:  true,
			PolicyRef: "policies.require_approval_for_git_write",
		})
	}
	if team.Policies.RequireApprovalForMessages {
		approvals = append(approvals, ApprovalRequest{
			ID:        "approval-outbound-message",
			Action:    "message.send",
			Target:    "channels",
			Reason:    "Team may deliver updates to human channels.",
			Approved:  true,
			PolicyRef: "policies.require_approval_for_messages",
		})
	}
	if team.Policies.RequireApprovalForExtSkills {
		for _, skill := range team.RequiredSkillRequirements() {
			if skill.Source.Type == "git" || skill.Source.Type == "registry" {
				approvals = append(approvals, ApprovalRequest{
					ID:        "approval-skill-" + skill.Name,
					Action:    "skills.install",
					Target:    skill.Name,
					Reason:    "Skill comes from an external distribution source.",
					Approved:  true,
					PolicyRef: "policies.require_approval_for_external_skills",
				})
			}
		}
	}
	return approvals
}

func buildModelBindings(team *spec.TeamSpec) []ModelBinding {
	providers := team.ResolveProviders()
	providerMap := make(map[string]spec.ResolvedProvider, len(providers))
	for _, provider := range providers {
		providerMap[provider.Name] = provider
	}

	out := make([]ModelBinding, 0, len(team.Agents))
	for _, agent := range team.Agents {
		modelRef := team.ResolveModel(agent)
		providerName := parseProviderName(modelRef)
		provider, ok := providerMap[providerName]
		out = append(out, ModelBinding{
			Agent:      agent.Name,
			Model:      modelRef,
			Provider:   providerName,
			ProviderOK: ok,
			APIKeyEnv:  provider.APIKeyEnv,
			HasAPIKey:  provider.HasAPIKey,
		})
	}
	return out
}

func parseProviderName(modelRef string) string {
	if idx := strings.Index(modelRef, "/"); idx > 0 {
		return modelRef[:idx]
	}
	return modelRef
}

func indexWorkItem(items []WorkItem, id string) int {
	for i := range items {
		if items[i].ID == id {
			return i
		}
	}
	return -1
}

func (r *Runner) generateAgentOutput(ctx context.Context, team *spec.TeamSpec, agent spec.AgentSpec, systemPrompt, input string) (string, error) {
	modelRef := team.ResolveModel(agent)
	providerName := parseProviderName(modelRef)
	providerCfg, ok := team.ModelProvider(providerName)
	if !ok {
		return "", fmt.Errorf("agent %s references unknown provider %q", agent.Name, providerName)
	}
	provider, err := r.modelFactory.Build(providerCfg)
	if err != nil {
		return "", err
	}
	return provider.Generate(ctx, model.Prompt{
		AgentName: agent.Name,
		Role:      agent.Role,
		Goal:      agent.Goal,
		System:    systemPrompt,
		Input:     input,
		ModelRef:  modelRef,
	})
}
