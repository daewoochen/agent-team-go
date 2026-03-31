package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type OpenAICompatibleProvider struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func NewOpenAICompatibleProvider(client *http.Client, cfg spec.ProviderSpec) *OpenAICompatibleProvider {
	baseURL := strings.TrimSuffix(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAICompatibleProvider{
		client:  client,
		baseURL: baseURL,
		apiKey:  cfg.APIKeyValue(),
	}
}

func (p *OpenAICompatibleProvider) Generate(ctx context.Context, prompt Prompt) (string, error) {
	if strings.TrimSpace(p.apiKey) == "" {
		return "", fmt.Errorf("missing api key for openai-compatible provider")
	}

	reqBody := chatCompletionRequest{
		Model: prompt.ModelRef,
		Messages: []chatMessage{
			{Role: "system", Content: prompt.System},
			{Role: "user", Content: prompt.Input},
		},
		Temperature: 0.2,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("chat completion request failed with status %s", resp.Status)
	}

	var decoded chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("chat completion returned no choices")
	}
	return strings.TrimSpace(decoded.Choices[0].Message.Content), nil
}
