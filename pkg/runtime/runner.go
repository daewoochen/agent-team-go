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
	events := make([]RunEvent, 0, 32)
	artifacts := make([]Artifact, 0, 16)
	workItems := make([]WorkItem, 0, 8)
	approvals := buildApprovals(team)
	modelBindings := buildModelBindings(team)
	deliveries := make([]channels.Delivery, 0, len(team.Channels))
	now := func() time.Time { return time.Now().UTC() }
	appendEvent := func(event RunEvent) {
		event.Timestamp = now()
		events = append(events, event)
	}
	writeCheckpoint := func() error {
		return r.writeCheckpoint(runID, task, workItems, approvals, artifacts)
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

	planSummary := "Work directly from captain judgment."
	planner, hasPlanner := agents.FindByRole(team, "planner")
	if hasPlanner {
		plannerItem := WorkItem{
			ID:                 "plan-001",
			Owner:              planner.Name,
			Objective:          "Break the incoming task into executable work items.",
			Inputs:             []string{task},
			AcceptanceCriteria: "A captain-readable execution plan with clear specialist ownership.",
			Status:             StatusPending,
			MaxAttempts:        team.ResolveMaxAttempts(planner),
		}
		workItems = append(workItems, plannerItem)
		plannerDelegation := Delegation{
			From:              captain.Name,
			To:                planner.Name,
			TaskID:            plannerItem.ID,
			Budget:            1,
			Deadline:          now().Add(30 * time.Minute).Format(time.RFC3339),
			ExpectedArtifacts: []string{"execution-plan.md"},
			Reason:            "Break the request into executable work items.",
		}
		plannerItemCopy := plannerItem
		appendEvent(RunEvent{
			Type:       "delegation.created",
			Actor:      captain.Name,
			Message:    "Captain delegated planning to planner.",
			Delegation: &plannerDelegation,
			WorkItem:   &plannerItemCopy,
		})
		appendEvent(RunEvent{
			Type:     "work_item.created",
			Actor:    planner.Name,
			Message:  fmt.Sprintf("Planner work item %s was created", plannerItem.ID),
			WorkItem: &plannerItemCopy,
		})

		planArtifact, err := r.executeWorkItem(
			ctx,
			team,
			planner,
			&workItems[0],
			"You are the planning agent for a multi-agent team.",
			buildPlanPrompt(task),
			"execution-plan.md",
			appendEvent,
		)
		if err == nil {
			artifacts = append(artifacts, planArtifact)
			planSummary = planArtifact.Content
		} else {
			planSummary = fmt.Sprintf("Planner failed after %d attempt(s). Captain should continue with direct judgment.\n\nReason: %s", workItems[0].Attempt, workItems[0].Error)
		}
		if err := writeCheckpoint(); err != nil {
			return nil, err
		}
	}

	specialistItems := buildWorkItems(team, task)
	for _, item := range specialistItems {
		workItems = append(workItems, item)
		itemCopy := item
		appendEvent(RunEvent{
			Type:     "work_item.created",
			Actor:    item.Owner,
			Message:  fmt.Sprintf("Work item %s was created", item.ID),
			WorkItem: &itemCopy,
		})
	}

	for {
		progress := false
		for i := range workItems {
			item := &workItems[i]
			if item.Status != StatusPending || item.ID == "plan-001" {
				continue
			}

			depState, depRef := dependencyState(workItems, *item)
			switch depState {
			case dependencyWaiting:
				continue
			case dependencyBlocked:
				item.Status = StatusFailed
				item.Error = fmt.Sprintf("blocked by dependency %s", depRef)
				itemCopy := *item
				appendEvent(RunEvent{
					Type:     "work_item.blocked",
					Actor:    item.Owner,
					Message:  fmt.Sprintf("%s was blocked by %s", item.ID, depRef),
					WorkItem: &itemCopy,
				})
				progress = true
				if err := writeCheckpoint(); err != nil {
					return nil, err
				}
				continue
			}

			agent, ok := findAgentByName(team, item.Owner)
			if !ok {
				item.Status = StatusFailed
				item.Error = "owner agent not found"
				itemCopy := *item
				appendEvent(RunEvent{
					Type:     "work_item.failed",
					Actor:    item.Owner,
					Message:  fmt.Sprintf("%s failed because owner agent %s does not exist", item.ID, item.Owner),
					WorkItem: &itemCopy,
				})
				progress = true
				if err := writeCheckpoint(); err != nil {
					return nil, err
				}
				continue
			}

			delegation := Delegation{
				From:              captain.Name,
				To:                agent.Name,
				TaskID:            item.ID,
				Budget:            1,
				Deadline:          now().Add(45 * time.Minute).Format(time.RFC3339),
				ExpectedArtifacts: []string{fmt.Sprintf("%s-report.md", agent.Role)},
				Reason:            fmt.Sprintf("Contribute %s-specific output to the final result.", agent.Role),
			}
			delegationCopy := delegation
			appendEvent(RunEvent{
				Type:       "delegation.created",
				Actor:      captain.Name,
				Message:    fmt.Sprintf("Captain delegated %s to %s.", item.ID, agent.Name),
				Delegation: &delegationCopy,
			})

			artifact, err := r.executeWorkItem(
				ctx,
				team,
				agent,
				item,
				specialistSystemPrompt(agent.Role),
				specialistPrompt(agent.Role, task, planSummary),
				fmt.Sprintf("%s-report.md", agent.Role),
				appendEvent,
			)
			if err == nil {
				artifacts = append(artifacts, artifact)
			}
			progress = true
			if err := writeCheckpoint(); err != nil {
				return nil, err
			}
		}

		if !progress {
			break
		}
	}

	for i := range workItems {
		item := &workItems[i]
		if item.Status != StatusPending {
			continue
		}
		item.Status = StatusFailed
		item.Error = "scheduler made no further progress"
		itemCopy := *item
		appendEvent(RunEvent{
			Type:     "work_item.blocked",
			Actor:    item.Owner,
			Message:  fmt.Sprintf("%s remained pending because the scheduler made no progress", item.ID),
			WorkItem: &itemCopy,
		})
	}

	summary, err := r.generateAgentOutput(
		ctx,
		team,
		captain,
		"You are the captain who synthesizes all specialist work into a final answer.",
		summarizePrompt(task, planSummary, workItems, artifacts),
	)
	if err != nil {
		return nil, err
	}

	deliveries, err = channels.BuildTeamDeliveries(team, channels.DeliveryContext{
		TeamName: team.Name,
		RunID:    runID,
		Task:     task,
		Summary:  summary,
	})
	if err != nil {
		return nil, err
	}
	for _, delivery := range deliveries {
		deliveryCopy := delivery
		appendEvent(RunEvent{
			Type:     "channel.delivery.prepared",
			Actor:    captain.Name,
			Message:  fmt.Sprintf("Prepared %s delivery", delivery.Channel),
			Delivery: &deliveryCopy,
		})
	}

	completionType := "run.completed"
	completionMessage := "Captain assembled the final response."
	if countFailedWorkItems(workItems) > 0 {
		completionType = "run.completed_with_failures"
		completionMessage = fmt.Sprintf("Captain assembled the final response with %d failed work item(s).", countFailedWorkItems(workItems))
	}
	appendEvent(RunEvent{
		Type:    completionType,
		Actor:   captain.Name,
		Message: completionMessage,
	})

	result := &RunResult{
		RunID:          runID,
		Summary:        summary,
		Events:         events,
		Artifacts:      artifacts,
		WorkItems:      workItems,
		Approvals:      approvals,
		ModelBindings:  modelBindings,
		Deliveries:     deliveries,
		ReplayPath:     filepath.Join(r.workDir, ".agentteam", "runs", runID+".json"),
		CheckpointPath: filepath.Join(r.workDir, ".agentteam", "checkpoints", runID+".json"),
	}
	if err := observe.WriteJSON(result.ReplayPath, result); err != nil {
		return nil, err
	}
	if err := writeCheckpoint(); err != nil {
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
			Owner:              agent.Name,
			Objective:          workObjective(agent.Role, task),
			Inputs:             []string{task},
			AcceptanceCriteria: workAcceptance(agent.Role),
			Status:             StatusPending,
			MaxAttempts:        team.ResolveMaxAttempts(agent),
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

func summarizePrompt(task, plan string, workItems []WorkItem, artifacts []Artifact) string {
	lines := []string{
		fmt.Sprintf("Task: %s", task),
		"",
		"Planning baseline:",
		plan,
		"",
		"Work item status:",
		workItemStatusSummary(workItems),
		"",
		"Artifacts:",
	}
	for _, artifact := range artifacts {
		lines = append(lines, fmt.Sprintf("## %s from %s\n%s", artifact.Name, artifact.Producer, artifact.Content))
	}
	lines = append(lines, "", "Produce the final captain summary in markdown. Mention the replayable nature of the run and any degraded areas.")
	return strings.Join(lines, "\n")
}

func (r *Runner) executeWorkItem(
	ctx context.Context,
	team *spec.TeamSpec,
	agent spec.AgentSpec,
	item *WorkItem,
	systemPrompt string,
	input string,
	artifactName string,
	appendEvent func(RunEvent),
) (Artifact, error) {
	if item.MaxAttempts <= 0 {
		item.MaxAttempts = 1
	}

	var lastErr error
	for attempt := item.Attempt + 1; attempt <= item.MaxAttempts; attempt++ {
		_ = Transition(item.Status, StatusRunning)
		item.Status = StatusRunning
		item.Attempt = attempt
		item.Error = ""

		itemCopy := *item
		eventType := "work_item.started"
		message := fmt.Sprintf("%s started %s (attempt %d/%d)", agent.Name, item.ID, attempt, item.MaxAttempts)
		if attempt > 1 {
			eventType = "work_item.retrying"
			message = fmt.Sprintf("%s retried %s (attempt %d/%d)", agent.Name, item.ID, attempt, item.MaxAttempts)
		}
		appendEvent(RunEvent{
			Type:     eventType,
			Actor:    agent.Name,
			Message:  message,
			WorkItem: &itemCopy,
		})

		output, err := r.generateAgentOutput(ctx, team, agent, systemPrompt, input)
		if err == nil {
			artifact := Artifact{
				Name:     artifactName,
				Producer: agent.Name,
				Content:  output,
			}
			appendEvent(RunEvent{
				Type:     "artifact.created",
				Actor:    agent.Name,
				Message:  fmt.Sprintf("%s delivered %s.", agent.Name, artifactName),
				Artifact: &artifact,
			})
			_ = Transition(item.Status, StatusCompleted)
			item.Status = StatusCompleted
			item.Error = ""
			itemCopy = *item
			appendEvent(RunEvent{
				Type:     "work_item.completed",
				Actor:    agent.Name,
				Message:  fmt.Sprintf("%s completed %s", agent.Name, item.ID),
				WorkItem: &itemCopy,
			})
			return artifact, nil
		}

		lastErr = err
		item.Error = err.Error()
		if attempt < item.MaxAttempts {
			_ = Transition(item.Status, StatusPending)
			item.Status = StatusPending
			itemCopy = *item
			appendEvent(RunEvent{
				Type:     "work_item.retry_scheduled",
				Actor:    agent.Name,
				Message:  fmt.Sprintf("%s scheduled a retry for %s after: %s", agent.Name, item.ID, item.Error),
				WorkItem: &itemCopy,
			})
			continue
		}

		_ = Transition(item.Status, StatusFailed)
		item.Status = StatusFailed
		itemCopy = *item
		appendEvent(RunEvent{
			Type:     "work_item.failed",
			Actor:    agent.Name,
			Message:  fmt.Sprintf("%s failed %s after %d attempt(s): %s", agent.Name, item.ID, item.Attempt, item.Error),
			WorkItem: &itemCopy,
		})
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("work item %s failed without a concrete error", item.ID)
	}
	return Artifact{}, lastErr
}

func (r *Runner) writeCheckpoint(runID, task string, workItems []WorkItem, approvals []ApprovalRequest, artifacts []Artifact) error {
	completed := make([]string, 0, len(workItems))
	pending := make([]string, 0, len(workItems))
	failed := make([]string, 0, len(workItems))
	for _, item := range workItems {
		switch item.Status {
		case StatusCompleted:
			completed = append(completed, item.ID)
		case StatusFailed:
			failed = append(failed, item.ID)
		default:
			pending = append(pending, item.ID)
		}
	}
	sort.Strings(completed)
	sort.Strings(pending)
	sort.Strings(failed)

	checkpoint := Checkpoint{
		RunID:              runID,
		Task:               task,
		Timestamp:          time.Now().UTC(),
		CompletedWorkItems: completed,
		PendingWorkItems:   pending,
		FailedWorkItems:    failed,
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

func findAgentByName(team *spec.TeamSpec, name string) (spec.AgentSpec, bool) {
	for _, agent := range team.Agents {
		if agent.Name == name {
			return agent, true
		}
	}
	return spec.AgentSpec{}, false
}

type depState string

const (
	dependencyReady   depState = "ready"
	dependencyWaiting depState = "waiting"
	dependencyBlocked depState = "blocked"
)

func dependencyState(items []WorkItem, item WorkItem) (depState, string) {
	if len(item.Dependencies) == 0 {
		return dependencyReady, ""
	}

	for _, dep := range item.Dependencies {
		idx := indexWorkItem(items, dep)
		if idx < 0 {
			return dependencyBlocked, dep
		}
		switch items[idx].Status {
		case StatusCompleted:
			continue
		case StatusFailed:
			return dependencyBlocked, dep
		default:
			return dependencyWaiting, dep
		}
	}
	return dependencyReady, ""
}

func workItemStatusSummary(items []WorkItem) string {
	if len(items) == 0 {
		return "- no work items"
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		line := fmt.Sprintf("- %s (%s): status=%s attempt=%d/%d", item.ID, item.Owner, item.Status, item.Attempt, item.MaxAttempts)
		if item.Error != "" {
			line += fmt.Sprintf(" error=%s", item.Error)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func countFailedWorkItems(items []WorkItem) int {
	count := 0
	for _, item := range items {
		if item.Status == StatusFailed {
			count++
		}
	}
	return count
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
