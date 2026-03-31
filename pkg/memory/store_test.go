package memory

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreAppendAndLoad(t *testing.T) {
	store := NewFileStore("demo", filepath.Join(t.TempDir(), "memory.json"), 2)
	if err := store.Append(Entry{RunID: "run-1", Task: "Task 1", Status: "completed", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if err := store.Append(Entry{RunID: "run-2", Task: "Task 2", Status: "completed", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if err := store.Append(Entry{RunID: "run-3", Task: "Task 3", Status: "completed", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(snapshot.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(snapshot.Entries))
	}
	if snapshot.Entries[0].RunID != "run-2" || snapshot.Entries[1].RunID != "run-3" {
		t.Fatalf("unexpected retained entries: %+v", snapshot.Entries)
	}
}

func TestBuildContext(t *testing.T) {
	context := BuildContext([]Entry{
		{Status: "completed", Task: "Task 1", Summary: "First summary"},
		{Status: "completed", Task: "Task 2", Summary: "Second summary"},
	}, 1)
	if !strings.Contains(context, "Task 2") {
		t.Fatalf("expected context to include latest task, got %q", context)
	}
}
