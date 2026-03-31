package channels

import (
	"testing"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

func TestValidateTeam(t *testing.T) {
	team := &spec.TeamSpec{
		Name: "demo",
		Agents: []spec.AgentSpec{
			{Name: "captain", Role: "captain", Goal: "Lead"},
		},
		Channels: []spec.ChannelConfig{
			{Kind: "cli", Enabled: true},
			{Kind: "telegram", Enabled: true, Token: "token"},
			{Kind: "feishu", Enabled: true, AppID: "app", AppSecret: "secret"},
		},
	}
	if err := ValidateTeam(team); err != nil {
		t.Fatalf("ValidateTeam returned error: %v", err)
	}
}

func TestValidateTeamErrors(t *testing.T) {
	team := &spec.TeamSpec{
		Name: "demo",
		Agents: []spec.AgentSpec{
			{Name: "captain", Role: "captain", Goal: "Lead"},
		},
		Channels: []spec.ChannelConfig{
			{Kind: "telegram", Enabled: true},
		},
	}
	if err := ValidateTeam(team); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestBuildTeamDeliveries(t *testing.T) {
	team := &spec.TeamSpec{
		Name: "ops-team",
		Agents: []spec.AgentSpec{
			{Name: "captain", Role: "captain", Goal: "Lead"},
		},
		Channels: []spec.ChannelConfig{
			{Kind: "cli", Enabled: true},
			{Kind: "telegram", Enabled: true, Token: "token", AllowFrom: []string{"chat-1"}},
		},
	}

	deliveries, err := BuildTeamDeliveries(team, DeliveryContext{
		TeamName: "ops-team",
		RunID:    "run-1",
		Task:     "Handle launch",
		Summary:  "All tasks completed.",
	})
	if err != nil {
		t.Fatalf("BuildTeamDeliveries returned error: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("expected 2 deliveries, got %d", len(deliveries))
	}
	if deliveries[1].Target != "chat-1" {
		t.Fatalf("unexpected telegram target %q", deliveries[1].Target)
	}
}
