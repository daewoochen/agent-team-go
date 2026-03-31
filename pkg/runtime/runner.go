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

type executionState struct {
	runID          string
	task           string
	status         RunStatus
	pausedReason   string
	planSummary    string
	summary        string
	events         []RunEvent
	artifacts      []Artifact
	workItems      []WorkItem
	approvals      []ApprovalRequest
	modelBindings  []ModelBinding
	deliveries     []channels.Delivery
	replayPath     string
	checkpointPath string
}

func NewRunner(workDir string) *Runner {
	return &Runner{
		workDir:      filepath.Clean(workDir),
		installer:    skills.NewInstaller(filepath.Join(filepath.Clean(workDir), ".agentteam", "skills")),
		modelFactory: model.NewFactory(),
	}
}

func (r *Runner) Run(ctx context.Context, team *spec.TeamSpec, task string) (*RunResult, error) {
	return r.run(ctx, team, nil, task)
}

func (r *Runner) Resume(ctx context.Context, team *spec.TeamSpec, checkpointPath string) (*RunResult, error) {
	var checkpoint Checkpoint
	if err := observe.ReadJSON(checkpointPath, &checkpoint); err != nil {
		return nil, err
	}
	if checkpoint.RunID == "" {
		return nil, fmt.Errorf("checkpoint %s is missing run_id", checkpointPath)
	}
	if checkpoint.Task == "" {
		return nil, fmt.Errorf("checkpoint %s is missing task", checkpointPath)
	}
	if checkpoint.Status != RunStatusWaitingApproval {
		return nil, fmt.Errorf("checkpoint %s is not resumable because status is %s", checkpointPath, checkpoint.Status)
	}
	return r.run(ctx, team, &checkpoint, checkpoint.Task)
}

func (r *Runner) run(ctx context.Context, team *spec.TeamSpec, checkpoint *Checkpoint, task string) (*RunResult, error) {
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

	captain, ok := agents.FindByRole(team, "captain")
	if !ok {
		return nil, fmt.Errorf("captain agent is required")
	}

	state := r.newExecutionState(team, task, checkpoint)
	now := func() time.Time { return time.Now().UTC() }
	appendEvent := func(event RunEvent) {
		event.Timestamp = now()
		state.events = append(state.events, event)
	}

	if checkpoint == nil {
		appendEvent(RunEvent{
			Type:    "run.started",
			Actor:   captain.Name,
			Message: fmt.Sprintf("Captain received task: %s", task),
		})
		for _, binding := range state.modelBindings {
			appendEvent(RunEvent{
				Type:    "model.bound",
				Actor:   binding.Agent,
				Message: fmt.Sprintf("%s uses %s", binding.Agent, binding.Model),
			})
		}
		for _, approval := range state.approvals {
			approvalCopy := approval
			appendEvent(RunEvent{
				Type:     "approval.requested",
				Actor:    captain.Name,
				Message:  fmt.Sprintf("Approval requested for %s", approval.Action),
				Approval: &approvalCopy,
			})
		}
	} else {
		appendEvent(RunEvent{
			Type:    "run.resumed",
			Actor:   captain.Name,
			Message: "Captain resumed the paused run.",
		})
	}

	if rejected := countRejectedApprovals(state.approvals); rejected > 0 {
		state.status = RunStatusRejected
		state.pausedReason = ""
		state.summary = rejectionSummary(state.approvals)
		appendEvent(RunEvent{
			Type:    "run.rejected",
			Actor:   captain.Name,
			Message: fmt.Sprintf("Run stopped because %d approval(s) were rejected.", rejected),
		})
		return r.persistState(state)
	}

	if pending := countPendingApprovals(state.approvals); pending > 0 {
		state.status = RunStatusWaitingApproval
		state.pausedReason = fmt.Sprintf("%d approval(s) still pending", pending)
		state.summary = fmt.Sprintf("Run paused waiting for %d approval(s). Use `agentteam approvals show`, `agentteam approvals approve` or `agentteam approvals reject`, then `agentteam resume` if the run should continue.", pending)
		appendEvent(RunEvent{
			Type:    "run.paused",
			Actor:   captain.Name,
			Message: state.pausedReason,
		})
		return r.persistState(state)
	}

	if checkpoint == nil {
		state.planSummary = "Work directly from captain judgment."
	} else if strings.TrimSpace(state.planSummary) == "" {
		state.planSummary = "Work directly from captain judgment."
	}
	feedbackSummary := humanFeedbackSummary(state.approvals)

	planner, hasPlanner := agents.FindByRole(team, "planner")
	if hasPlanner {
		plannerIndex := ensurePlannerWorkItem(&state.workItems, planner, task, team.ResolveMaxAttempts(planner), appendEvent)
		if state.workItems[plannerIndex].Status != StatusCompleted {
			plannerDelegation := Delegation{
				From:              captain.Name,
				To:                planner.Name,
				TaskID:            state.workItems[plannerIndex].ID,
				Budget:            1,
				Deadline:          now().Add(30 * time.Minute).Format(time.RFC3339),
				ExpectedArtifacts: []string{"execution-plan.md"},
				Reason:            "Break the request into executable work items.",
			}
			delegationCopy := plannerDelegation
			appendEvent(RunEvent{
				Type:       "delegation.created",
				Actor:      captain.Name,
				Message:    "Captain delegated planning to planner.",
				Delegation: &delegationCopy,
			})

			planArtifact, err := r.executeWorkItem(
				ctx,
				team,
				planner,
				&state.workItems[plannerIndex],
				"You are the planning agent for a multi-agent team.",
				buildPlanPrompt(task, feedbackSummary),
				"execution-plan.md",
				appendEvent,
			)
			if err == nil {
				upsertArtifact(&state.artifacts, planArtifact)
				state.planSummary = planArtifact.Content
			} else {
				state.planSummary = fmt.Sprintf("Planner failed after %d attempt(s). Captain should continue with direct judgment.\n\nReason: %s", state.workItems[plannerIndex].Attempt, state.workItems[plannerIndex].Error)
			}
			if result, err := r.persistIntermediateState(state); err != nil {
				return nil, err
			} else {
				state = result
			}
		} else if artifact, ok := artifactByName(state.artifacts, "execution-plan.md"); ok && strings.TrimSpace(state.planSummary) == "" {
			state.planSummary = artifact.Content
		}
	}

	ensureSpecialistWorkItems(&state.workItems, team, task, appendEvent)

	for {
		progress := false
		for i := range state.workItems {
			item := &state.workItems[i]
			if item.Status != StatusPending || item.ID == "plan-001" {
				continue
			}

			depState, depRef := dependencyState(state.workItems, *item)
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
				if result, err := r.persistIntermediateState(state); err != nil {
					return nil, err
				} else {
					state = result
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
				if result, err := r.persistIntermediateState(state); err != nil {
					return nil, err
				} else {
					state = result
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
				specialistPrompt(agent.Role, task, state.planSummary, feedbackSummary),
				fmt.Sprintf("%s-report.md", agent.Role),
				appendEvent,
			)
			if err == nil {
				upsertArtifact(&state.artifacts, artifact)
			}
			progress = true
			if result, err := r.persistIntermediateState(state); err != nil {
				return nil, err
			} else {
				state = result
			}
		}

		if !progress {
			break
		}
	}

	for i := range state.workItems {
		item := &state.workItems[i]
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
		summarizePrompt(task, state.planSummary, feedbackSummary, state.workItems, state.artifacts),
	)
	if err != nil {
		return nil, err
	}
	state.summary = summary

	deliveries, err := channels.BuildTeamDeliveries(team, channels.DeliveryContext{
		TeamName: team.Name,
		RunID:    state.runID,
		Task:     task,
		Summary:  state.summary,
	})
	if err != nil {
		return nil, err
	}
	state.deliveries = deliveries
	for _, delivery := range state.deliveries {
		deliveryCopy := delivery
		appendEvent(RunEvent{
			Type:     "channel.delivery.prepared",
			Actor:    captain.Name,
			Message:  fmt.Sprintf("Prepared %s delivery", delivery.Channel),
			Delivery: &deliveryCopy,
		})
	}

	state.pausedReason = ""
	if failed := countFailedWorkItems(state.workItems); failed > 0 {
		state.status = RunStatusCompletedWithFailures
		appendEvent(RunEvent{
			Type:    "run.completed_with_failures",
			Actor:   captain.Name,
			Message: fmt.Sprintf("Captain assembled the final response with %d failed work item(s).", failed),
		})
	} else {
		state.status = RunStatusCompleted
		appendEvent(RunEvent{
			Type:    "run.completed",
			Actor:   captain.Name,
			Message: "Captain assembled the final response.",
		})
	}

	return r.persistState(state)
}

func (r *Runner) newExecutionState(team *spec.TeamSpec, task string, checkpoint *Checkpoint) executionState {
	runID := time.Now().UTC().Format("20060102T150405Z")
	if checkpoint != nil && checkpoint.RunID != "" {
		runID = checkpoint.RunID
	}
	state := executionState{
		runID:          runID,
		task:           task,
		status:         RunStatusRunning,
		modelBindings:  buildModelBindings(team),
		replayPath:     filepath.Join(r.workDir, ".agentteam", "runs", runID+".json"),
		checkpointPath: filepath.Join(r.workDir, ".agentteam", "checkpoints", runID+".json"),
	}
	if checkpoint == nil {
		state.approvals = buildApprovals(team)
		return state
	}

	state.status = checkpoint.Status
	state.pausedReason = checkpoint.PausedReason
	state.planSummary = checkpoint.PlanSummary
	state.summary = checkpoint.Summary
	state.events = append([]RunEvent(nil), checkpoint.Events...)
	state.artifacts = append([]Artifact(nil), checkpoint.Artifacts...)
	state.workItems = append([]WorkItem(nil), checkpoint.WorkItems...)
	state.approvals = append([]ApprovalRequest(nil), checkpoint.Approvals...)
	state.deliveries = append([]channels.Delivery(nil), checkpoint.Deliveries...)
	return state
}

func (r *Runner) persistIntermediateState(state executionState) (executionState, error) {
	_, err := r.persistState(state)
	return state, err
}

func (r *Runner) persistState(state executionState) (*RunResult, error) {
	result := &RunResult{
		RunID:          state.runID,
		Task:           state.task,
		Status:         state.status,
		PausedReason:   state.pausedReason,
		Summary:        state.summary,
		Events:         state.events,
		Artifacts:      state.artifacts,
		WorkItems:      state.workItems,
		Approvals:      state.approvals,
		ModelBindings:  state.modelBindings,
		Deliveries:     state.deliveries,
		ReplayPath:     state.replayPath,
		CheckpointPath: state.checkpointPath,
	}
	if err := observe.WriteJSON(result.ReplayPath, result); err != nil {
		return nil, err
	}
	if err := observe.WriteJSON(result.CheckpointPath, checkpointFromState(state)); err != nil {
		return nil, err
	}
	return result, nil
}

func checkpointFromState(state executionState) Checkpoint {
	completed := make([]string, 0, len(state.workItems))
	pending := make([]string, 0, len(state.workItems))
	failed := make([]string, 0, len(state.workItems))
	for _, item := range state.workItems {
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
	return Checkpoint{
		RunID:              state.runID,
		Task:               state.task,
		Timestamp:          time.Now().UTC(),
		Status:             state.status,
		PausedReason:       state.pausedReason,
		PlanSummary:        state.planSummary,
		Summary:            state.summary,
		Events:             state.events,
		WorkItems:          state.workItems,
		CompletedWorkItems: completed,
		PendingWorkItems:   pending,
		FailedWorkItems:    failed,
		Approvals:          state.approvals,
		Artifacts:          state.artifacts,
		Deliveries:         state.deliveries,
	}
}

func buildPlanPrompt(task, feedback string) string {
	lines := []string{
		fmt.Sprintf("Create a concise execution plan for this task: %s", task),
		"Return a short markdown plan with 3 numbered steps.",
	}
	if strings.TrimSpace(feedback) != "" {
		lines = append(lines, "", "Operator feedback:", feedback)
	}
	return strings.Join(lines, "\n")
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

func specialistPrompt(role, task, plan, feedback string) string {
	prefix := []string{
		fmt.Sprintf("Task: %s", task),
		"",
		"Plan basis:",
		plan,
	}
	if strings.TrimSpace(feedback) != "" {
		prefix = append(prefix, "", "Operator feedback:", feedback)
	}
	header := strings.Join(prefix, "\n")
	switch role {
	case "researcher":
		return fmt.Sprintf("%s\n\nProduce a concise research brief with assumptions, dependencies, and risks.", header)
	case "coder":
		return fmt.Sprintf("%s\n\nProduce implementation notes for the MVP path.", header)
	case "reviewer":
		return fmt.Sprintf("%s\n\nProduce a review checklist focused on quality, safety, and release readiness.", header)
	default:
		return fmt.Sprintf("%s\n\nProduce a concise artifact for this role.", header)
	}
}

func specialistSystemPrompt(role string) string {
	return fmt.Sprintf("You are the %s agent in a multi-agent team. Be concise, specific, and execution-focused.", role)
}

func summarizePrompt(task, plan, feedback string, workItems []WorkItem, artifacts []Artifact) string {
	lines := []string{
		fmt.Sprintf("Task: %s", task),
		"",
		"Planning baseline:",
		plan,
		"",
	}
	if strings.TrimSpace(feedback) != "" {
		lines = append(lines, "Operator feedback:", feedback, "")
	}
	lines = append(lines,
		"Work item status:",
		workItemStatusSummary(workItems),
		"",
		"Artifacts:",
	)
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

func buildApprovals(team *spec.TeamSpec) []ApprovalRequest {
	mode := team.ResolveApprovalMode()
	approvals := make([]ApprovalRequest, 0, 4)
	addApproval := func(id, action, target, reason, policyRef string) {
		approval := ApprovalRequest{
			ID:        id,
			Action:    action,
			Target:    target,
			Reason:    reason,
			Approved:  mode == "auto",
			PolicyRef: policyRef,
		}
		if approval.Approved {
			approval.Decision = ApprovalApproved
		} else {
			approval.Decision = ApprovalPending
		}
		approvals = append(approvals, approval)
	}

	if team.Policies.RequireApprovalForGitWrite {
		addApproval("approval-git-write", "git.write", "repository", "Coder or release steps may mutate repository state.", "policies.require_approval_for_git_write")
	}
	if team.Policies.RequireApprovalForMessages {
		addApproval("approval-outbound-message", "message.send", "channels", "Team may deliver updates to human channels.", "policies.require_approval_for_messages")
	}
	if team.Policies.RequireApprovalForExtSkills {
		for _, skill := range team.RequiredSkillRequirements() {
			if skill.Source.Type == "git" || skill.Source.Type == "registry" {
				addApproval("approval-skill-"+skill.Name, "skills.install", skill.Name, "Skill comes from an external distribution source.", "policies.require_approval_for_external_skills")
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

func ensurePlannerWorkItem(items *[]WorkItem, planner spec.AgentSpec, task string, maxAttempts int, appendEvent func(RunEvent)) int {
	if idx := indexWorkItem(*items, "plan-001"); idx >= 0 {
		return idx
	}
	item := WorkItem{
		ID:                 "plan-001",
		Owner:              planner.Name,
		Objective:          "Break the incoming task into executable work items.",
		Inputs:             []string{task},
		AcceptanceCriteria: "A captain-readable execution plan with clear specialist ownership.",
		Status:             StatusPending,
		MaxAttempts:        maxAttempts,
	}
	*items = append(*items, item)
	itemCopy := item
	appendEvent(RunEvent{
		Type:     "work_item.created",
		Actor:    planner.Name,
		Message:  fmt.Sprintf("Planner work item %s was created", item.ID),
		WorkItem: &itemCopy,
	})
	return len(*items) - 1
}

func ensureSpecialistWorkItems(items *[]WorkItem, team *spec.TeamSpec, task string, appendEvent func(RunEvent)) {
	for _, item := range buildWorkItems(team, task) {
		if indexWorkItem(*items, item.ID) >= 0 {
			continue
		}
		*items = append(*items, item)
		itemCopy := item
		appendEvent(RunEvent{
			Type:     "work_item.created",
			Actor:    item.Owner,
			Message:  fmt.Sprintf("Work item %s was created", item.ID),
			WorkItem: &itemCopy,
		})
	}
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

func countPendingApprovals(approvals []ApprovalRequest) int {
	count := 0
	for _, approval := range approvals {
		if !approval.IsApproved() && !approval.IsRejected() {
			count++
		}
	}
	return count
}

func countRejectedApprovals(approvals []ApprovalRequest) int {
	count := 0
	for _, approval := range approvals {
		if approval.IsRejected() {
			count++
		}
	}
	return count
}

func humanFeedbackSummary(approvals []ApprovalRequest) string {
	lines := make([]string, 0, len(approvals))
	for _, approval := range approvals {
		if approval.Note == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", approval.ID, approval.Note))
	}
	return strings.Join(lines, "\n")
}

func rejectionSummary(approvals []ApprovalRequest) string {
	lines := []string{"Run stopped because one or more approvals were rejected."}
	for _, approval := range approvals {
		if !approval.IsRejected() {
			continue
		}
		line := fmt.Sprintf("- %s rejected for %s", approval.ID, approval.Action)
		if approval.Note != "" {
			line += fmt.Sprintf(": %s", approval.Note)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
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

func artifactByName(artifacts []Artifact, name string) (Artifact, bool) {
	for _, artifact := range artifacts {
		if artifact.Name == name {
			return artifact, true
		}
	}
	return Artifact{}, false
}

func upsertArtifact(artifacts *[]Artifact, next Artifact) {
	for i := range *artifacts {
		if (*artifacts)[i].Name == next.Name && (*artifacts)[i].Producer == next.Producer {
			(*artifacts)[i] = next
			return
		}
	}
	*artifacts = append(*artifacts, next)
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
