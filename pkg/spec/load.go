package spec

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func LoadTeam(path string) (*TeamSpec, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var team TeamSpec
	if err := yaml.Unmarshal(content, &team); err != nil {
		return nil, fmt.Errorf("parse team spec: %w", err)
	}

	team.SourcePath = filepath.Clean(path)
	team.BaseDir = filepath.Dir(team.SourcePath)

	if err := team.Validate(); err != nil {
		return nil, err
	}

	return &team, nil
}
