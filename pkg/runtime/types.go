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

type RunEvent struct {
	Timestamp  time.Time   `json:"timestamp"`
	Type       string      `json:"type"`
	Actor      string      `json:"actor"`
	Message    string      `json:"message"`
	Delegation *Delegation `json:"delegation,omitempty"`
	Artifact   *Artifact   `json:"artifact,omitempty"`
}

type RunResult struct {
	RunID      string     `json:"run_id"`
	Summary    string     `json:"summary"`
	Events     []RunEvent `json:"events"`
	Artifacts  []Artifact `json:"artifacts"`
	ReplayPath string     `json:"replay_path"`
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
