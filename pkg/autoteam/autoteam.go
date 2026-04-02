package autoteam

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type Profile string

const (
	ProfileAuto      Profile = "auto"
	ProfileSoftware  Profile = "software"
	ProfileResearch  Profile = "research"
	ProfileIncident  Profile = "incident"
	ProfileContent   Profile = "content"
	ProfileAssistant Profile = "assistant"
)

type Options struct {
	Profile string
	WorkDir string
	Name    string
}

func Build(task string, opts Options) (*spec.TeamSpec, Profile, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return nil, "", fmt.Errorf("task is required")
	}

	profile := normalizeProfile(opts.Profile)
	if profile == ProfileAuto {
		profile = Guess(task)
	}

	workDir := filepath.Clean(opts.WorkDir)
	if workDir == "." || workDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			workDir = cwd
		}
	}

	models := defaultModels()
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = fmt.Sprintf("auto-%s-team", profile)
	}

	team := &spec.TeamSpec{
		Name:        name,
		Description: descriptionFor(profile),
		BaseDir:     workDir,
		Models:      models,
		Skills:      skillsFor(profile),
		Agents:      agentsFor(profile, models.DefaultModel),
		Channels:    defaultChannels(),
		Policies: spec.PolicySpec{
			AllowExternalSkillInstall:   true,
			RequireApprovalForExtSkills: false,
			RequireApprovalForMessages:  false,
			RequireApprovalForGitWrite:  false,
			ApprovalMode:                "auto",
		},
		Memory: spec.MemoryConfig{
			Backend:    "file",
			Path:       filepath.Join(".agentteam", "memory", name+".json"),
			MaxEntries: 12,
		},
		Budget: spec.BudgetConfig{
			MaxDelegations: 10,
			MaxTokens:      120000,
		},
	}

	return team, profile, nil
}

func Guess(task string) Profile {
	text := strings.ToLower(task)
	score := map[Profile]int{
		ProfileSoftware:  0,
		ProfileResearch:  0,
		ProfileIncident:  0,
		ProfileContent:   0,
		ProfileAssistant: 1,
	}

	addScore := func(profile Profile, keywords ...string) {
		for _, keyword := range keywords {
			if strings.Contains(text, keyword) {
				score[profile]++
			}
		}
	}

	addScore(ProfileSoftware, "ship", "release", "launch", "mvp", "feature", "build", "product", "roadmap")
	addScore(ProfileResearch, "research", "compare", "investigate", "analyze", "analysis", "report", "benchmark")
	addScore(ProfileIncident, "incident", "outage", "alert", "sev", "rollback", "mitigate", "postmortem")
	addScore(ProfileContent, "content", "blog", "article", "copy", "tweet", "campaign", "script", "video", "newsletter")
	addScore(ProfileAssistant, "coordinate", "follow up", "assistant", "ops", "schedule", "reply", "message")

	best := ProfileSoftware
	bestScore := -1
	for _, profile := range []Profile{ProfileIncident, ProfileResearch, ProfileContent, ProfileSoftware, ProfileAssistant} {
		if score[profile] > bestScore {
			best = profile
			bestScore = score[profile]
		}
	}
	return best
}

func normalizeProfile(profile string) Profile {
	switch Profile(strings.ToLower(strings.TrimSpace(profile))) {
	case "", ProfileAuto:
		return ProfileAuto
	case ProfileSoftware, ProfileResearch, ProfileIncident, ProfileContent, ProfileAssistant:
		return Profile(strings.ToLower(strings.TrimSpace(profile)))
	default:
		return ProfileAuto
	}
}

func defaultModels() spec.ModelConfig {
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
		baseURL := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return spec.ModelConfig{
			DefaultModel: "openai/gpt-4.1-mini",
			Providers: map[string]spec.ProviderSpec{
				"openai": {
					Kind:      "openai-compatible",
					BaseURL:   baseURL,
					APIKeyEnv: "OPENAI_API_KEY",
				},
			},
		}
	}
	return spec.ModelConfig{
		DefaultModel: "mock/generalist",
		Providers: map[string]spec.ProviderSpec{
			"mock": {
				Kind: "mock",
			},
		},
	}
}

func defaultChannels() []spec.ChannelConfig {
	channels := []spec.ChannelConfig{
		{Kind: "cli", Enabled: true},
	}
	if strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")) != "" && strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")) != "" {
		channels = append(channels, spec.ChannelConfig{
			Kind:      "telegram",
			Enabled:   true,
			Token:     "env:TELEGRAM_BOT_TOKEN",
			AllowFrom: []string{"env:TELEGRAM_CHAT_ID"},
		})
	}
	if strings.TrimSpace(os.Getenv("FEISHU_APP_ID")) != "" && strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET")) != "" && strings.TrimSpace(os.Getenv("FEISHU_CHAT_ID")) != "" {
		channels = append(channels, spec.ChannelConfig{
			Kind:      "feishu",
			Enabled:   true,
			AppID:     "env:FEISHU_APP_ID",
			AppSecret: "env:FEISHU_APP_SECRET",
			AllowFrom: []string{"env:FEISHU_CHAT_ID"},
		})
	}
	return channels
}

func descriptionFor(profile Profile) string {
	switch profile {
	case ProfileResearch:
		return "Auto-generated research team for comparison, analysis, and recommendation tasks."
	case ProfileIncident:
		return "Auto-generated incident response team for triage, risk review, and stakeholder updates."
	case ProfileContent:
		return "Auto-generated content team for planning, drafting, and reviewing launch assets."
	case ProfileAssistant:
		return "Auto-generated assistant team for coordination, response drafting, and execution support."
	default:
		return "Auto-generated software delivery team for planning, implementation, and review."
	}
}

func skillsFor(profile Profile) []spec.SkillRequirement {
	skills := []spec.SkillRequirement{
		{
			Name:    "research-kit",
			Version: ">=0.1.0",
			Source: spec.SkillSource{
				Type:     "registry",
				Registry: "builtin",
			},
		},
	}

	switch profile {
	case ProfileSoftware:
		skills = append(skills, spec.SkillRequirement{
			Name:    "github",
			Version: ">=0.1.0",
			Source: spec.SkillSource{
				Type:     "registry",
				Registry: "builtin",
			},
		})
	case ProfileIncident, ProfileAssistant:
		skills = append(skills, spec.SkillRequirement{
			Name:    "telegram-messenger",
			Version: ">=0.1.0",
			Source: spec.SkillSource{
				Type:     "registry",
				Registry: "builtin",
			},
		})
	case ProfileContent:
		skills = append(skills, spec.SkillRequirement{
			Name:    "github",
			Version: ">=0.1.0",
			Source: spec.SkillSource{
				Type:     "registry",
				Registry: "builtin",
			},
		})
	}
	return skills
}

func agentsFor(profile Profile, defaultModel string) []spec.AgentSpec {
	modelPrefix := strings.SplitN(defaultModel, "/", 2)[0]
	modelRef := func(name string) string {
		switch modelPrefix {
		case "openai":
			switch name {
			case "captain":
				return "openai/gpt-4.1"
			default:
				return "openai/gpt-4.1-mini"
			}
		default:
			return fmt.Sprintf("mock/%s", name)
		}
	}

	base := []spec.AgentSpec{
		{Name: "captain", Role: "captain", Goal: "Own coordination, sequencing, and final synthesis.", Model: modelRef("captain")},
		{Name: "planner", Role: "planner", Goal: "Break the request into clear work items.", Model: modelRef("planner")},
	}

	switch profile {
	case ProfileResearch:
		return append(base,
			spec.AgentSpec{Name: "researcher", Role: "researcher", Goal: "Collect facts, comparisons, and tradeoffs.", Model: modelRef("researcher"), RequiredSkills: []string{"research-kit"}},
			spec.AgentSpec{Name: "reviewer", Role: "reviewer", Goal: "Pressure test the recommendation and quality bar.", Model: modelRef("reviewer")},
		)
	case ProfileIncident:
		return append(base,
			spec.AgentSpec{Name: "researcher", Role: "researcher", Goal: "Gather evidence, timeline, and impact details.", Model: modelRef("researcher"), RequiredSkills: []string{"research-kit"}},
			spec.AgentSpec{Name: "reviewer", Role: "reviewer", Goal: "Review mitigations and stakeholder messaging.", Model: modelRef("reviewer")},
		)
	case ProfileContent:
		return append(base,
			spec.AgentSpec{Name: "researcher", Role: "researcher", Goal: "Surface audience insights and launch constraints.", Model: modelRef("researcher"), RequiredSkills: []string{"research-kit"}},
			spec.AgentSpec{Name: "writer", Role: "writer", Goal: "Draft content assets and launch copy.", Model: modelRef("writer")},
			spec.AgentSpec{Name: "reviewer", Role: "reviewer", Goal: "Review voice, clarity, and release quality.", Model: modelRef("reviewer")},
		)
	case ProfileAssistant:
		return append(base,
			spec.AgentSpec{Name: "researcher", Role: "researcher", Goal: "Collect the latest context and unresolved details.", Model: modelRef("researcher"), RequiredSkills: []string{"research-kit"}},
			spec.AgentSpec{Name: "reviewer", Role: "reviewer", Goal: "Check quality, completeness, and response safety.", Model: modelRef("reviewer")},
		)
	default:
		return append(base,
			spec.AgentSpec{Name: "researcher", Role: "researcher", Goal: "Surface risks, dependencies, and launch assumptions.", Model: modelRef("researcher"), RequiredSkills: []string{"research-kit"}},
			spec.AgentSpec{Name: "coder", Role: "coder", Goal: "Shape the implementation path and delivery plan.", Model: modelRef("coder"), RequiredSkills: []string{"github"}},
			spec.AgentSpec{Name: "reviewer", Role: "reviewer", Goal: "Review quality, readiness, and rollback posture.", Model: modelRef("reviewer")},
		)
	}
}
