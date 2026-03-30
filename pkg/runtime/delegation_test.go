package runtime

import "testing"

func TestTransition(t *testing.T) {
	if err := Transition(StatusPending, StatusRunning); err != nil {
		t.Fatalf("expected pending -> running to be valid: %v", err)
	}
	if err := Transition(StatusRunning, StatusCompleted); err != nil {
		t.Fatalf("expected running -> completed to be valid: %v", err)
	}
	if err := Transition(StatusCompleted, StatusRunning); err == nil {
		t.Fatalf("expected completed -> running to be invalid")
	}
}
