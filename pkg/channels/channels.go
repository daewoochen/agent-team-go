package channels

import (
	"fmt"
	"strings"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type DeliveryContext struct {
	TeamName string
	RunID    string
	Task     string
	Summary  string
}

type Delivery struct {
	Channel string `json:"channel"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Target  string `json:"target,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

type Adapter interface {
	Kind() string
	Validate(spec.ChannelConfig) error
	BuildDelivery(spec.ChannelConfig, DeliveryContext) (Delivery, bool)
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

func (a adapter) BuildDelivery(cfg spec.ChannelConfig, ctx DeliveryContext) (Delivery, bool) {
	if !cfg.Enabled {
		return Delivery{}, false
	}

	body := strings.TrimSpace(ctx.Summary)
	if body == "" {
		body = fmt.Sprintf("Run %s completed for task: %s", ctx.RunID, ctx.Task)
	}
	delivery := Delivery{
		Channel: a.kind,
		Title:   fmt.Sprintf("[%s] %s", strings.ToUpper(a.kind), ctx.TeamName),
		Body:    body,
	}

	switch a.kind {
	case "cli":
		delivery.Target = "stdout"
		delivery.Mode = "local"
	case "telegram":
		delivery.Target = joinTargets(cfg.AllowFrom, "configured-chat")
		delivery.Mode = "bot"
	case "feishu":
		delivery.Target = joinTargets(cfg.AllowFrom, "configured-chat")
		if cfg.Mode != "" {
			delivery.Mode = cfg.Mode
		} else {
			delivery.Mode = "bot"
		}
	default:
		return Delivery{}, false
	}

	return delivery, true
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

func BuildTeamDeliveries(team *spec.TeamSpec, ctx DeliveryContext) ([]Delivery, error) {
	if err := ValidateTeam(team); err != nil {
		return nil, err
	}

	adapters := DefaultAdapters()
	deliveries := make([]Delivery, 0, len(team.Channels))
	for _, cfg := range team.Channels {
		adapter, ok := adapters[cfg.Kind]
		if !ok {
			return nil, fmt.Errorf("unsupported channel kind %q", cfg.Kind)
		}
		delivery, include := adapter.BuildDelivery(cfg, ctx)
		if include {
			deliveries = append(deliveries, delivery)
		}
	}
	return deliveries, nil
}

func joinTargets(targets []string, fallback string) string {
	if len(targets) == 0 {
		return fallback
	}
	return strings.Join(targets, ",")
}
