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
	t.Setenv("FEISHU_APP_ID", "app-id")
	t.Setenv("FEISHU_APP_SECRET", "app-secret")

	var gotTarget string
	var gotTask string
	server.BuildTeam = func(task string, opts autoteam.Options) (*spec.TeamSpec, autoteam.Profile, error) {
		gotTask = task
		return &spec.TeamSpec{Name: "auto-incident-team"}, autoteam.ProfileIncident, nil
	}
	server.RunTask = func(_ context.Context, team *spec.TeamSpec, task string) (*runtime.RunResult, error) {
		gotTask = task
		for _, cfg := range team.Channels {
			if cfg.Kind == "feishu" && len(cfg.AllowFrom) > 0 {
				gotTarget = cfg.AllowFrom[0]
			}
		}
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
	if gotTarget != "oc_abc" {
		t.Fatalf("unexpected target %q", gotTarget)
	}
}
