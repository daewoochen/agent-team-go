package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/daewoochen/agent-team-go/pkg/autoteam"
	"github.com/daewoochen/agent-team-go/pkg/channels"
	"github.com/daewoochen/agent-team-go/pkg/runtime"
	"github.com/daewoochen/agent-team-go/pkg/spec"
)

func TestTelegramWebhookRunsAutoTeamAndDelivers(t *testing.T) {
	server := NewServer(t.TempDir(), "auto", true)
	t.Setenv("TELEGRAM_BOT_TOKEN", "bot-token")

	var gotTask string
	var gotTarget string
	server.BuildTeam = func(task string, opts autoteam.Options) (*spec.TeamSpec, autoteam.Profile, error) {
		gotTask = task
		return &spec.TeamSpec{
			Name: "auto-research-team",
			Channels: []spec.ChannelConfig{
				{Kind: "cli", Enabled: true},
			},
		}, autoteam.ProfileResearch, nil
	}
	server.RunTask = func(_ context.Context, team *spec.TeamSpec, task string) (*runtime.RunResult, error) {
		for _, channel := range team.Channels {
			if channel.Kind == "telegram" && len(channel.AllowFrom) > 0 {
				gotTarget = channel.AllowFrom[0]
			}
		}
		return &runtime.RunResult{
			RunID:      "run-1",
			Status:     runtime.RunStatusCompleted,
			Summary:    "done",
			ReplayPath: ".agentteam/runs/run-1.json",
			Deliveries: []channels.Delivery{{Channel: "telegram", Target: "12345", Title: "[TG] demo", Body: "done"}},
		}, nil
	}
	server.Deliveries = func(_ context.Context, _ *spec.TeamSpec, deliveries []channels.Delivery) ([]channels.DeliveryReport, error) {
		return []channels.DeliveryReport{{Channel: deliveries[0].Channel, Target: deliveries[0].Target, Status: "delivered"}}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{"message":{"text":"Compare the top Go agent runtimes","chat":{"id":12345},"from":{"id":7}}}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", rec.Code, rec.Body.String())
	}
	if gotTask != "Compare the top Go agent runtimes" {
		t.Fatalf("unexpected task %q", gotTask)
	}
	if gotTarget != "12345" {
		t.Fatalf("unexpected telegram target %q", gotTarget)
	}

	var response WebhookResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !response.OK || response.Profile != string(autoteam.ProfileResearch) {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.SessionID == "" {
		t.Fatalf("expected session id in response: %+v", response)
	}
}

func TestFeishuChallenge(t *testing.T) {
	server := NewServer(t.TempDir(), "auto", true)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/feishu", strings.NewReader(`{"challenge":"abc123"}`))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "{\"challenge\":\"abc123\"}" {
		t.Fatalf("unexpected challenge response %q", rec.Body.String())
	}
}

func TestFeishuWebhookRunsAndParsesText(t *testing.T) {
	server := NewServer(t.TempDir(), "incident", false)

	var gotTask string
	server.BuildTeam = func(task string, opts autoteam.Options) (*spec.TeamSpec, autoteam.Profile, error) {
		gotTask = task
		return &spec.TeamSpec{Name: "auto-incident-team"}, autoteam.ProfileIncident, nil
	}
	server.RunTask = func(_ context.Context, team *spec.TeamSpec, task string) (*runtime.RunResult, error) {
		gotTask = task
		return &runtime.RunResult{
			RunID:      "run-2",
			Status:     runtime.RunStatusCompleted,
			Summary:    "incident handled",
			ReplayPath: ".agentteam/runs/run-2.json",
		}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/webhooks/feishu", strings.NewReader(`{
	  "header":{"event_type":"im.message.receive_v1"},
	  "event":{
	    "sender":{"sender_id":{"open_id":"ou_x"}},
	    "message":{"chat_id":"oc_abc","message_type":"text","content":"{\"text\":\"Handle the sev1 incident\"}"}
	  }
	}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", rec.Code, rec.Body.String())
	}
	if gotTask != "Handle the sev1 incident" {
		t.Fatalf("unexpected task %q", gotTask)
	}
}

func TestTelegramWebhookProfileCommandPersistsPreference(t *testing.T) {
	server := NewServer(t.TempDir(), "auto", false)

	runCount := 0
	gotProfile := ""
	server.BuildTeam = func(task string, opts autoteam.Options) (*spec.TeamSpec, autoteam.Profile, error) {
		gotProfile = opts.Profile
		return &spec.TeamSpec{Name: "auto-research-team"}, autoteam.ProfileResearch, nil
	}
	server.RunTask = func(_ context.Context, _ *spec.TeamSpec, _ string) (*runtime.RunResult, error) {
		runCount++
		return &runtime.RunResult{
			RunID:   "run-profile",
			Status:  runtime.RunStatusCompleted,
			Summary: "research done",
		}, nil
	}

	firstReq := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{"message":{"text":"/profile research","chat":{"id":12345},"from":{"id":7}}}`))
	firstRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstRec, firstReq)

	if firstRec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", firstRec.Code, firstRec.Body.String())
	}
	if runCount != 0 {
		t.Fatalf("expected no run for profile command, got %d", runCount)
	}

	var firstResponse WebhookResponse
	if err := json.Unmarshal(firstRec.Body.Bytes(), &firstResponse); err != nil {
		t.Fatalf("failed to decode first response: %v", err)
	}
	if !strings.Contains(firstResponse.Summary, "Preferred profile set to research") {
		t.Fatalf("unexpected summary %q", firstResponse.Summary)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{"message":{"text":"Compare the top Go agent runtimes","chat":{"id":12345},"from":{"id":7}}}`))
	secondRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(secondRec, secondReq)

	if secondRec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if runCount != 1 {
		t.Fatalf("expected one run after profile command, got %d", runCount)
	}
	if gotProfile != "research" {
		t.Fatalf("expected saved profile to be reused, got %q", gotProfile)
	}

	session, err := server.Sessions.Load(InboundMessage{Channel: "telegram", Target: "12345"})
	if err != nil {
		t.Fatalf("failed to load saved session: %v", err)
	}
	if session.PreferredProfile != "research" {
		t.Fatalf("expected research profile in saved session, got %+v", session)
	}
}

func TestTelegramWebhookUsesConversationContext(t *testing.T) {
	server := NewServer(t.TempDir(), "auto", false)

	tasks := []string{}
	server.BuildTeam = func(task string, opts autoteam.Options) (*spec.TeamSpec, autoteam.Profile, error) {
		return &spec.TeamSpec{Name: "auto-assistant-team"}, autoteam.ProfileAssistant, nil
	}
	server.RunTask = func(_ context.Context, _ *spec.TeamSpec, task string) (*runtime.RunResult, error) {
		tasks = append(tasks, task)
		summary := "first summary"
		runID := "run-1"
		if len(tasks) > 1 {
			summary = "second summary"
			runID = "run-2"
		}
		return &runtime.RunResult{
			RunID:   runID,
			Status:  runtime.RunStatusCompleted,
			Summary: summary,
		}, nil
	}

	firstReq := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{"message":{"text":"Prepare the launch memo","chat":{"id":12345},"from":{"id":7}}}`))
	firstRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("unexpected first status %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{"message":{"text":"Now turn that into a follow-up note","chat":{"id":12345},"from":{"id":7}}}`))
	secondRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("unexpected second status %d body=%s", secondRec.Code, secondRec.Body.String())
	}

	if len(tasks) != 2 {
		t.Fatalf("expected two runs, got %d", len(tasks))
	}
	if !strings.Contains(tasks[1], "Conversation context:") {
		t.Fatalf("expected session context in second task, got %q", tasks[1])
	}
	if !strings.Contains(tasks[1], "user: Prepare the launch memo") {
		t.Fatalf("expected prior user turn in second task, got %q", tasks[1])
	}
	if !strings.Contains(tasks[1], "assistant: first summary") {
		t.Fatalf("expected prior assistant turn in second task, got %q", tasks[1])
	}
	if !strings.Contains(tasks[1], "Current user request:\nNow turn that into a follow-up note") {
		t.Fatalf("expected current request in second task, got %q", tasks[1])
	}
}

func TestTelegramWebhookRunsWithoutReplySecretsWhenDeliverDisabled(t *testing.T) {
	server := NewServer(t.TempDir(), "assistant", false)

	var gotTarget string
	gotEnabled := true
	server.BuildTeam = func(task string, opts autoteam.Options) (*spec.TeamSpec, autoteam.Profile, error) {
		return &spec.TeamSpec{Name: "auto-assistant-team"}, autoteam.ProfileAssistant, nil
	}
	server.RunTask = func(_ context.Context, team *spec.TeamSpec, task string) (*runtime.RunResult, error) {
		for _, cfg := range team.Channels {
			if cfg.Kind == "telegram" && len(cfg.AllowFrom) > 0 {
				gotTarget = cfg.AllowFrom[0]
				gotEnabled = cfg.Enabled
			}
		}
		return &runtime.RunResult{
			RunID:   "run-no-deliver",
			Status:  runtime.RunStatusCompleted,
			Summary: "done without delivery credentials",
		}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{"message":{"text":"Draft the launch update","chat":{"id":12345},"from":{"id":7}}}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", rec.Code, rec.Body.String())
	}
	if gotTarget != "12345" {
		t.Fatalf("expected source target to be preserved, got %q", gotTarget)
	}
	if gotEnabled {
		t.Fatalf("expected reply channel to stay disabled when --deliver=false")
	}
}
