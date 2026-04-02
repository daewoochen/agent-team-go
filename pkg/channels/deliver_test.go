package channels

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestDeliverPreparedTelegram(t *testing.T) {
	var gotAuth string
	var gotPath string
	var gotPayload map[string]any

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		gotAuth = r.Header.Get("Authorization")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":42}}`)),
		}, nil
	})}

	team := &spec.TeamSpec{
		Name: "demo",
		Channels: []spec.ChannelConfig{
			{Kind: "telegram", Enabled: true, BaseURL: "https://telegram.example.test", Token: "env:TELEGRAM_BOT_TOKEN", AllowFrom: []string{"env:TELEGRAM_CHAT_ID"}},
		},
	}
	t.Setenv("TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat-1")

	reports, err := NewDeliverer(client).Deliver(context.Background(), team, []Delivery{{
		Channel: "telegram",
		Title:   "[TG] Demo",
		Body:    "hello",
	}})
	if err != nil {
		t.Fatalf("Deliver returned error: %v", err)
	}
	if len(reports) != 1 || reports[0].MessageID != "42" {
		t.Fatalf("unexpected reports: %+v", reports)
	}
	if gotAuth != "" {
		t.Fatalf("telegram requests should not set auth header, got %q", gotAuth)
	}
	if gotPath != "/botbot-token/sendMessage" {
		t.Fatalf("unexpected telegram path %q", gotPath)
	}
	if gotPayload["chat_id"] != "chat-1" {
		t.Fatalf("unexpected chat id payload: %+v", gotPayload)
	}
}

func TestDeliverPreparedFeishu(t *testing.T) {
	var authCalls int
	var messageCalls int

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(r.URL.Path, "/auth/v3/tenant_access_token/internal"):
			authCalls++
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"code":0,"tenant_access_token":"tenant-token"}`)),
			}, nil
		case strings.Contains(r.URL.Path, "/im/v1/messages"):
			messageCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer tenant-token" {
				t.Fatalf("unexpected feishu auth header %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"code":0,"data":{"message_id":"om_123"}}`)),
			}, nil
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"code":404}`)),
			}, nil
		}
	})}

	team := &spec.TeamSpec{
		Name: "demo",
		Channels: []spec.ChannelConfig{
			{Kind: "feishu", Enabled: true, BaseURL: "https://feishu.example.test", AppID: "env:FEISHU_APP_ID", AppSecret: "env:FEISHU_APP_SECRET", AllowFrom: []string{"env:FEISHU_CHAT_ID"}},
		},
	}
	t.Setenv("FEISHU_APP_ID", "app-id")
	t.Setenv("FEISHU_APP_SECRET", "app-secret")
	t.Setenv("FEISHU_CHAT_ID", "oc_123")

	reports, err := NewDeliverer(client).Deliver(context.Background(), team, []Delivery{{
		Channel: "feishu",
		Title:   "[FS] Demo",
		Body:    "hello",
	}})
	if err != nil {
		t.Fatalf("Deliver returned error: %v", err)
	}
	if len(reports) != 1 || reports[0].MessageID != "om_123" {
		t.Fatalf("unexpected reports: %+v", reports)
	}
	if authCalls != 1 || messageCalls != 1 {
		t.Fatalf("unexpected feishu call counts auth=%d message=%d", authCalls, messageCalls)
	}
}

func TestDeliverPreparedCLI(t *testing.T) {
	team := &spec.TeamSpec{
		Name: "demo",
		Channels: []spec.ChannelConfig{
			{Kind: "cli", Enabled: true},
		},
	}

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe returned error: %v", err)
	}
	defer readEnd.Close()
	defer writeEnd.Close()

	deliverer := NewDeliverer(nil)
	deliverer.stdout = writeEnd
	reports, err := deliverer.Deliver(context.Background(), team, []Delivery{{
		Channel: "cli",
		Title:   "[CLI] Demo",
		Body:    "hello",
	}})
	if err != nil {
		t.Fatalf("Deliver returned error: %v", err)
	}
	_ = writeEnd.Close()
	content, _ := io.ReadAll(readEnd)
	if len(reports) != 1 || reports[0].Status != "delivered" {
		t.Fatalf("unexpected reports: %+v", reports)
	}
	if !strings.Contains(string(content), "[CLI] Demo") {
		t.Fatalf("expected CLI delivery to write message, got %q", string(content))
	}
}
