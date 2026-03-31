package model

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type Builder func(*http.Client, spec.ProviderSpec) (Provider, error)

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
	builders   map[string]Builder
}

func NewFactory() *Factory {
	return &Factory{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		builders: map[string]Builder{
			"mock": func(_ *http.Client, _ spec.ProviderSpec) (Provider, error) {
				return NewMockProvider(), nil
			},
			"openai-compatible": func(client *http.Client, provider spec.ProviderSpec) (Provider, error) {
				return NewOpenAICompatibleProvider(client, provider), nil
			},
		},
	}
}

func (f *Factory) Register(kind string, builder Builder) {
	if f.builders == nil {
		f.builders = map[string]Builder{}
	}
	f.builders[kind] = builder
}

func (f *Factory) Build(provider spec.ProviderSpec) (Provider, error) {
	builder, ok := f.builders[provider.Kind]
	if !ok {
		return nil, fmt.Errorf("unsupported provider kind %q", provider.Kind)
	}
	return builder(f.HTTPClient, provider)
}
