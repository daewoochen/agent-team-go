package agents

import "github.com/daewoochen/agent-team-go/pkg/spec"

func FindByRole(team *spec.TeamSpec, role string) (spec.AgentSpec, bool) {
	for _, agent := range team.Agents {
		if agent.Role == role {
			return agent, true
		}
	}
	return spec.AgentSpec{}, false
}
