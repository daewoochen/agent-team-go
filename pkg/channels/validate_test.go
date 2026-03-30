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
