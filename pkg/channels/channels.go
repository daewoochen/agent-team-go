package channels

import (
	"fmt"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type Adapter interface {
	Kind() string
	Validate(spec.ChannelConfig) error
}

type adapter struct {
	kind string
}

func (a adapter) Kind() string {
	return a.kind
}

func (a adapter) Validate(cfg spec.ChannelConfig) error {
	switch a.kind {
	case "cli":
		return nil
	case "telegram":
		if cfg.Enabled && cfg.Token == "" {
			return fmt.Errorf("telegram channel requires token when enabled")
		}
	case "feishu":
		if cfg.Enabled && (cfg.AppID == "" || cfg.AppSecret == "") {
			return fmt.Errorf("feishu channel requires app_id and app_secret when enabled")
		}
	}
	return nil
}

func DefaultAdapters() map[string]Adapter {
	return map[string]Adapter{
		"cli":      adapter{kind: "cli"},
		"telegram": adapter{kind: "telegram"},
		"feishu":   adapter{kind: "feishu"},
	}
}

func ValidateTeam(team *spec.TeamSpec) error {
	adapters := DefaultAdapters()
	for _, cfg := range team.Channels {
		adapter, ok := adapters[cfg.Kind]
		if !ok {
			return fmt.Errorf("unsupported channel kind %q", cfg.Kind)
		}
		if err := adapter.Validate(cfg); err != nil {
			return err
		}
	}
	return nil
}
