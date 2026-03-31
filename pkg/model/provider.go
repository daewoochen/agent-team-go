package model

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type Prompt struct {
	AgentName string
	Role      string
	Goal      string
	System    string
	Input     string
	ModelRef  string
}

type Provider interface {
	Generate(context.Context, Prompt) (string, error)
}

type Factory struct {
	HTTPClient *http.Client
}

func NewFactory() *Factory {
	return &Factory{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (f *Factory) Build(provider spec.ProviderSpec) (Provider, error) {
	switch provider.Kind {
	case "mock":
		return NewMockProvider(), nil
	case "openai-compatible":
		return NewOpenAICompatibleProvider(f.HTTPClient, provider), nil
	default:
		return nil, fmt.Errorf("unsupported provider kind %q", provider.Kind)
	}
}
