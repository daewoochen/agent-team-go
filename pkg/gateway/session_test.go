package gateway

import (
	"strings"
	"testing"
	"time"
)

func TestSessionStoreSaveLoadDelete(t *testing.T) {
	store := NewSessionStore(t.TempDir(), 4)
	msg := InboundMessage{Channel: "telegram", Target: "12345", UserID: "7"}

	session, err := store.Load(msg)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	session.PreferredProfile = "research"
	store.AppendTurn(session, "user", "Compare runtimes", "")
	store.AppendTurn(session, "assistant", "Here is the first summary", "run-1")
	if err := store.Save(session); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := store.Load(msg)
	if err != nil {
		t.Fatalf("second Load returned error: %v", err)
	}
	if loaded.PreferredProfile != "research" || len(loaded.Turns) != 2 {
		t.Fatalf("unexpected loaded session: %+v", loaded)
	}

	if err := store.Delete(msg); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	reset, err := store.Load(msg)
	if err != nil {
		t.Fatalf("Load after delete returned error: %v", err)
	}
	if len(reset.Turns) != 0 {
		t.Fatalf("expected empty session after delete, got %+v", reset)
	}
}

func TestBuildSessionContext(t *testing.T) {
	session := &Session{
		Turns: []SessionTurn{
			{Role: "user", Text: "First request"},
			{Role: "assistant", Text: "First answer"},
			{Role: "user", Text: "Second request"},
		},
	}
	context := BuildSessionContext(session, 2)
	if !strings.Contains(context, "assistant: First answer") || !strings.Contains(context, "user: Second request") {
		t.Fatalf("unexpected context %q", context)
	}
}

func TestParseCommand(t *testing.T) {
	cmd, ok := ParseCommand("/profile research Compare runtimes")
	if !ok {
		t.Fatalf("expected command to parse")
	}
	if cmd.Name != "profile" || cmd.Profile != "research" || cmd.Task != "Compare runtimes" {
		t.Fatalf("unexpected command %+v", cmd)
	}
}

func TestHandleCommandProfileWithTask(t *testing.T) {
	summary, continueTask, continueProfile, reset, err := HandleCommand(Command{
		Name:    "profile",
		Profile: "assistant",
		Task:    "Draft the launch update",
	}, &Session{})
	if err != nil {
		t.Fatalf("HandleCommand returned error: %v", err)
	}
	if summary != "" || continueTask != "Draft the launch update" || continueProfile != "assistant" || reset {
		t.Fatalf("unexpected command handling result summary=%q continueTask=%q continueProfile=%q reset=%t", summary, continueTask, continueProfile, reset)
	}
}

func TestSessionStoreListSorted(t *testing.T) {
	store := NewSessionStore(t.TempDir(), 4)

	first := &Session{
		ID:        "telegram:1",
		Channel:   "telegram",
		Target:    "1",
		UpdatedAt: time.Now().Add(-time.Minute),
	}
	second := &Session{
		ID:               "feishu:2",
		Channel:          "feishu",
		Target:           "2",
		PreferredProfile: "research",
	}
	if err := store.Save(first); err != nil {
		t.Fatalf("save first session: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := store.Save(second); err != nil {
		t.Fatalf("save second session: %v", err)
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].Target != "2" || sessions[1].Target != "1" {
		t.Fatalf("expected newest session first, got %+v", sessions)
	}
}
