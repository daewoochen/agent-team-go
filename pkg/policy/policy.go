package policy

import (
	"fmt"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

func CanInstallSkill(team *spec.TeamSpec, skill spec.SkillRequirement) error {
	if skill.Source.Type == "local" || skill.Source.Type == "registry" {
		return nil
	}
	if !team.Policies.AllowExternalSkillInstall {
		return fmt.Errorf("external skill installs are disabled by policy")
	}
	return nil
}
