package model

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOpenAICompatibleProviderGenerate(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("Authorization"); got == "" {
				t.Fatalf("expected authorization header")
			}
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST request, got %s", r.Method)
			}
			if r.URL.String() != "https://example.test/v1/chat/completions" {
				t.Fatalf("unexpected request url %s", r.URL.String())
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !strings.Contains(string(body), `"model":"openai/gpt-4.1-mini"`) {
				t.Fatalf("expected model in request body, got %s", string(body))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"hello from provider"}}]}`)),
			}, nil
		}),
	}

	provider := NewOpenAICompatibleProvider(client, spec.ProviderSpec{
		Kind:    "openai-compatible",
		BaseURL: "https://example.test/v1",
		APIKey:  "secret",
	})

	out, err := provider.Generate(context.Background(), Prompt{
		ModelRef: "openai/gpt-4.1-mini",
		System:   "You are helpful.",
		Input:    "Say hello.",
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if out != "hello from provider" {
		t.Fatalf("unexpected output %q", out)
	}
}

func TestOpenAICompatibleProviderGenerateFailsWithoutAPIKey(t *testing.T) {
	provider := NewOpenAICompatibleProvider(http.DefaultClient, spec.ProviderSpec{
		Kind:    "openai-compatible",
		BaseURL: "https://example.test/v1",
	})

	_, err := provider.Generate(context.Background(), Prompt{
		ModelRef: "openai/gpt-4.1-mini",
		System:   "You are helpful.",
		Input:    "Say hello.",
	})
	if err == nil {
		t.Fatalf("expected missing api key error")
	}
}
