package runtime

import (
	"fmt"
	"time"
)

type WorkStatus string

const (
	StatusPending   WorkStatus = "pending"
	StatusRunning   WorkStatus = "running"
	StatusCompleted WorkStatus = "completed"
	StatusFailed    WorkStatus = "failed"
)

type WorkItem struct {
	ID                 string     `json:"id"`
	Objective          string     `json:"objective"`
	Inputs             []string   `json:"inputs"`
	AcceptanceCriteria string     `json:"acceptance_criteria"`
	Dependencies       []string   `json:"dependencies"`
	Status             WorkStatus `json:"status"`
}

type Delegation struct {
	From              string   `json:"from"`
	To                string   `json:"to"`
	TaskID            string   `json:"task_id"`
	Budget            int      `json:"budget"`
	Deadline          string   `json:"deadline"`
	ExpectedArtifacts []string `json:"expected_artifacts"`
	Reason            string   `json:"reason"`
}

type Artifact struct {
	Name     string `json:"name"`
	Producer string `json:"producer"`
	Content  string `json:"content"`
}

type ApprovalRequest struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Reason    string `json:"reason"`
	Approved  bool   `json:"approved"`
	PolicyRef string `json:"policy_ref"`
}

type ModelBinding struct {
	Agent      string `json:"agent"`
	Model      string `json:"model"`
	Provider   string `json:"provider"`
	ProviderOK bool   `json:"provider_ok"`
	APIKeyEnv  string `json:"api_key_env,omitempty"`
	HasAPIKey  bool   `json:"has_api_key"`
}

type Checkpoint struct {
	RunID              string            `json:"run_id"`
	Task               string            `json:"task"`
	Timestamp          time.Time         `json:"timestamp"`
	CompletedWorkItems []string          `json:"completed_work_items"`
	PendingWorkItems   []string          `json:"pending_work_items"`
	Approvals          []ApprovalRequest `json:"approvals"`
	Artifacts          []Artifact        `json:"artifacts"`
}

type RunEvent struct {
	Timestamp  time.Time        `json:"timestamp"`
	Type       string           `json:"type"`
	Actor      string           `json:"actor"`
	Message    string           `json:"message"`
	Delegation *Delegation      `json:"delegation,omitempty"`
	Artifact   *Artifact        `json:"artifact,omitempty"`
	Approval   *ApprovalRequest `json:"approval,omitempty"`
	WorkItem   *WorkItem        `json:"work_item,omitempty"`
}

type RunResult struct {
	RunID          string            `json:"run_id"`
	Summary        string            `json:"summary"`
	Events         []RunEvent        `json:"events"`
	Artifacts      []Artifact        `json:"artifacts"`
	WorkItems      []WorkItem        `json:"work_items"`
	Approvals      []ApprovalRequest `json:"approvals"`
	ModelBindings  []ModelBinding    `json:"model_bindings"`
	ReplayPath     string            `json:"replay_path"`
	CheckpointPath string            `json:"checkpoint_path"`
}

func Transition(current, next WorkStatus) error {
	switch current {
	case StatusPending:
		if next == StatusRunning || next == StatusFailed {
			return nil
		}
	case StatusRunning:
		if next == StatusCompleted || next == StatusFailed {
			return nil
		}
	case StatusCompleted, StatusFailed:
	}
	return fmt.Errorf("invalid transition from %s to %s", current, next)
}
