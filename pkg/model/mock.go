package model

import (
	"context"
	"fmt"
	"strings"
)

type MockProvider struct{}

func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

func (m *MockProvider) Generate(_ context.Context, prompt Prompt) (string, error) {
	var lines []string
	lines = append(lines, fmt.Sprintf("[%s:%s]", prompt.AgentName, prompt.ModelRef))
	lines = append(lines, fmt.Sprintf("Goal: %s", prompt.Goal))
	if strings.TrimSpace(prompt.System) != "" {
		lines = append(lines, fmt.Sprintf("System: %s", prompt.System))
	}
	lines = append(lines, fmt.Sprintf("Input: %s", prompt.Input))
	lines = append(lines, "Output: Provide a concise, execution-focused response.")
	return strings.Join(lines, "\n"), nil
}
