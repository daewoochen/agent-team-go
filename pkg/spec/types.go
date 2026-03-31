package spec

import (
	"fmt"
	"path/filepath"
	"strings"
)

type TeamSpec struct {
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Models      ModelConfig        `yaml:"models"`
	Agents      []AgentSpec        `yaml:"agents"`
	Skills      []SkillRequirement `yaml:"skills"`
	Channels    []ChannelConfig    `yaml:"channels"`
	Policies    PolicySpec         `yaml:"policies"`
	Memory      MemoryConfig       `yaml:"memory"`
	Budget      BudgetConfig       `yaml:"budget"`
	BaseDir     string             `yaml:"-"`
	SourcePath  string             `yaml:"-"`
}

type AgentSpec struct {
	Name             string            `yaml:"name"`
	Role             string            `yaml:"role"`
	Goal             string            `yaml:"goal"`
	Model            string            `yaml:"model"`
	MaxAttempts      int               `yaml:"max_attempts"`
	AllowedTools     []string          `yaml:"allowed_tools"`
	RequiredSkills   []string          `yaml:"required_skills"`
	DelegationPolicy DelegationPolicy  `yaml:"delegation_policy"`
	Metadata         map[string]string `yaml:"metadata"`
}

type DelegationPolicy struct {
	MaxDepth            int  `yaml:"max_depth"`
	AllowPeerDelegation bool `yaml:"allow_peer_delegation"`
	RequireArtifacts    bool `yaml:"require_artifacts"`
}

type SkillRequirement struct {
	Name    string      `yaml:"name"`
	Version string      `yaml:"version"`
	Source  SkillSource `yaml:"source"`
}

type SkillSource struct {
	Type     string `yaml:"type"`
	Path     string `yaml:"path,omitempty"`
	URL      string `yaml:"url,omitempty"`
	Ref      string `yaml:"ref,omitempty"`
	Registry string `yaml:"registry,omitempty"`
}

type ModelConfig struct {
	DefaultModel string                  `yaml:"default_model"`
	Providers    map[string]ProviderSpec `yaml:"providers"`
}

type ProviderSpec struct {
	Kind       string `yaml:"kind"`
	BaseURL    string `yaml:"base_url,omitempty"`
	APIKeyEnv  string `yaml:"api_key_env,omitempty"`
	APIKey     string `yaml:"api_key,omitempty"`
	DefaultRef string `yaml:"default_ref,omitempty"`
}

type ChannelConfig struct {
	Kind              string   `yaml:"kind"`
	Enabled           bool     `yaml:"enabled"`
	Token             string   `yaml:"token,omitempty"`
	AllowFrom         []string `yaml:"allow_from,omitempty"`
	AppID             string   `yaml:"app_id,omitempty"`
	AppSecret         string   `yaml:"app_secret,omitempty"`
	EncryptKey        string   `yaml:"encrypt_key,omitempty"`
	VerificationToken string   `yaml:"verification_token,omitempty"`
	Mode              string   `yaml:"mode,omitempty"`
}

type PolicySpec struct {
	AllowExternalSkillInstall   bool   `yaml:"allow_external_skill_install"`
	RequireApprovalForExtSkills bool   `yaml:"require_approval_for_external_skills"`
	RequireApprovalForMessages  bool   `yaml:"require_approval_for_messages"`
	RequireApprovalForGitWrite  bool   `yaml:"require_approval_for_git_write"`
	ApprovalMode                string `yaml:"approval_mode"`
}

type MemoryConfig struct {
	Backend    string `yaml:"backend"`
	Path       string `yaml:"path"`
	MaxEntries int    `yaml:"max_entries"`
}

type BudgetConfig struct {
	MaxDelegations int `yaml:"max_delegations"`
	MaxTokens      int `yaml:"max_tokens"`
}

func (p PolicySpec) Validate() error {
	switch strings.TrimSpace(p.ApprovalMode) {
	case "", "auto", "manual":
		return nil
	default:
		return fmt.Errorf("unsupported approval_mode %q", p.ApprovalMode)
	}
}

func (t *TeamSpec) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("team name is required")
	}
	if len(t.Agents) == 0 {
		return fmt.Errorf("at least one agent is required")
	}

	names := map[string]struct{}{}
	hasCaptain := false
	for _, agent := range t.Agents {
		if strings.TrimSpace(agent.Name) == "" {
			return fmt.Errorf("agent name is required")
		}
		if strings.TrimSpace(agent.Role) == "" {
			return fmt.Errorf("role is required for agent %q", agent.Name)
		}
		if _, ok := names[agent.Name]; ok {
			return fmt.Errorf("duplicate agent name %q", agent.Name)
		}
		names[agent.Name] = struct{}{}
		if agent.MaxAttempts < 0 {
			return fmt.Errorf("agent %q has invalid max_attempts %d", agent.Name, agent.MaxAttempts)
		}
		if strings.TrimSpace(agent.Model) == "" {
			agent.Model = t.Models.DefaultModel
		}
		if agent.Role == "captain" {
			hasCaptain = true
		}
	}
	if !hasCaptain {
		return fmt.Errorf("at least one captain agent is required")
	}

	for _, skill := range t.Skills {
		if strings.TrimSpace(skill.Name) == "" {
			return fmt.Errorf("skill name is required")
		}
		if err := skill.Source.Validate(); err != nil {
			return fmt.Errorf("skill %q: %w", skill.Name, err)
		}
	}

	if err := t.Models.Validate(); err != nil {
		return err
	}
	if err := t.Policies.Validate(); err != nil {
		return err
	}
	if err := t.Memory.Validate(); err != nil {
		return err
	}

	return nil
}

func (m MemoryConfig) Validate() error {
	switch strings.TrimSpace(m.Backend) {
	case "", "file":
	default:
		return fmt.Errorf("unsupported memory backend %q", m.Backend)
	}
	if m.MaxEntries < 0 {
		return fmt.Errorf("memory max_entries must be >= 0")
	}
	return nil
}

func (s SkillSource) Validate() error {
	if strings.TrimSpace(s.Type) == "" {
		return fmt.Errorf("skill source type is required")
	}
	switch s.Type {
	case "local":
		if strings.TrimSpace(s.Path) == "" {
			return fmt.Errorf("local skill source requires path")
		}
	case "git":
		if strings.TrimSpace(s.URL) == "" {
			return fmt.Errorf("git skill source requires url")
		}
	case "registry":
	default:
		return fmt.Errorf("unsupported skill source type %q", s.Type)
	}
	return nil
}

func (t *TeamSpec) SkillMap() map[string]SkillRequirement {
	out := make(map[string]SkillRequirement, len(t.Skills))
	for _, skill := range t.Skills {
		out[skill.Name] = skill
	}
	return out
}

func (t *TeamSpec) RequiredSkillRequirements() []SkillRequirement {
	byName := t.SkillMap()
	merged := map[string]SkillRequirement{}

	for name, skill := range byName {
		merged[name] = skill
	}

	for _, agent := range t.Agents {
		for _, skillName := range agent.RequiredSkills {
			if skill, ok := byName[skillName]; ok {
				merged[skillName] = skill
				continue
			}
			merged[skillName] = SkillRequirement{
				Name:    skillName,
				Version: "latest",
				Source: SkillSource{
					Type:     "registry",
					Registry: "builtin",
				},
			}
		}
	}

	out := make([]SkillRequirement, 0, len(merged))
	for _, skill := range merged {
		out = append(out, skill)
	}
	return out
}

func (m ModelConfig) Validate() error {
	for name, provider := range m.Providers {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("model provider name is required")
		}
		if strings.TrimSpace(provider.Kind) == "" {
			return fmt.Errorf("model provider %q must declare kind", name)
		}
	}
	return nil
}

func (t *TeamSpec) ResolveModel(agent AgentSpec) string {
	if strings.TrimSpace(agent.Model) != "" {
		return agent.Model
	}
	return t.Models.DefaultModel
}

func (t *TeamSpec) ResolveMaxAttempts(agent AgentSpec) int {
	if agent.MaxAttempts > 0 {
		return agent.MaxAttempts
	}
	return 1
}

func (t *TeamSpec) ResolveApprovalMode() string {
	if strings.TrimSpace(t.Policies.ApprovalMode) == "" {
		return "auto"
	}
	return t.Policies.ApprovalMode
}

func (t *TeamSpec) ResolveMemoryBackend() string {
	if strings.TrimSpace(t.Memory.Backend) != "" {
		return t.Memory.Backend
	}
	if strings.TrimSpace(t.Memory.Path) != "" {
		return "file"
	}
	return ""
}

func (t *TeamSpec) ResolveMemoryPath(workDir string) string {
	if t.ResolveMemoryBackend() == "" {
		return ""
	}
	path := strings.TrimSpace(t.Memory.Path)
	if path == "" {
		path = filepath.Join(".agentteam", "memory", t.Name+".json")
	}
	if filepath.IsAbs(path) {
		return path
	}
	baseDir := t.BaseDir
	if baseDir == "" {
		baseDir = workDir
	}
	return filepath.Join(baseDir, path)
}

func (t *TeamSpec) ResolveMemoryMaxEntries() int {
	if t.Memory.MaxEntries > 0 {
		return t.Memory.MaxEntries
	}
	return 20
}

func (t *TeamSpec) ModelProvider(name string) (ProviderSpec, bool) {
	provider, ok := t.Models.Providers[name]
	return provider, ok
}
