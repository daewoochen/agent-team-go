package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type DeliveryReport struct {
	Channel   string `json:"channel"`
	Target    string `json:"target"`
	Status    string `json:"status"`
	MessageID string `json:"message_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type Deliverer struct {
	client *http.Client
	stdout io.Writer
}

func NewDeliverer(client *http.Client) *Deliverer {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Deliverer{
		client: client,
		stdout: os.Stdout,
	}
}

func DeliverPrepared(ctx context.Context, team *spec.TeamSpec, deliveries []Delivery) ([]DeliveryReport, error) {
	return NewDeliverer(nil).Deliver(ctx, team, deliveries)
}

func (d *Deliverer) Deliver(ctx context.Context, team *spec.TeamSpec, deliveries []Delivery) ([]DeliveryReport, error) {
	if err := ValidateTeam(team); err != nil {
		return nil, err
	}

	reports := make([]DeliveryReport, 0, len(deliveries))
	errs := make([]string, 0)
	for _, delivery := range deliveries {
		cfg, ok := findChannelConfig(team, delivery.Channel)
		if !ok {
			errs = append(errs, fmt.Sprintf("missing channel config for %s", delivery.Channel))
			continue
		}
		nextReports, err := d.deliverOne(ctx, cfg, delivery)
		reports = append(reports, nextReports...)
		if err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return reports, fmt.Errorf(strings.Join(errs, "; "))
	}
	return reports, nil
}

func (d *Deliverer) deliverOne(ctx context.Context, cfg spec.ChannelConfig, delivery Delivery) ([]DeliveryReport, error) {
	switch cfg.Kind {
	case "cli":
		return d.deliverCLI(delivery), nil
	case "telegram":
		return d.deliverTelegram(ctx, cfg, delivery)
	case "feishu":
		return d.deliverFeishu(ctx, cfg, delivery)
	default:
		return nil, fmt.Errorf("unsupported delivery channel %q", cfg.Kind)
	}
}

func (d *Deliverer) deliverCLI(delivery Delivery) []DeliveryReport {
	if d.stdout != nil {
		fmt.Fprintf(d.stdout, "%s\n\n%s\n", delivery.Title, delivery.Body)
	}
	return []DeliveryReport{{
		Channel: delivery.Channel,
		Target:  "stdout",
		Status:  "delivered",
	}}
}

func (d *Deliverer) deliverTelegram(ctx context.Context, cfg spec.ChannelConfig, delivery Delivery) ([]DeliveryReport, error) {
	token := resolveSecret(cfg.Token)
	if token == "" {
		return nil, fmt.Errorf("telegram token is not configured")
	}

	targets := deliveryTargets(cfg, delivery)
	if len(targets) == 0 {
		return nil, fmt.Errorf("telegram delivery requires at least one chat id in allow_from")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}

	message := renderMessage(delivery)
	reports := make([]DeliveryReport, 0, len(targets))
	errs := make([]string, 0)
	for _, target := range targets {
		payload := map[string]any{
			"chat_id": target,
			"text":    message,
		}
		var response struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
			Result      struct {
				MessageID int `json:"message_id"`
			} `json:"result"`
		}
		if err := d.postJSON(ctx, fmt.Sprintf("%s/bot%s/sendMessage", baseURL, token), "", payload, &response); err != nil {
			reports = append(reports, DeliveryReport{Channel: "telegram", Target: target, Status: "failed", Detail: err.Error()})
			errs = append(errs, fmt.Sprintf("telegram target %s: %v", target, err))
			continue
		}
		if !response.OK {
			detail := response.Description
			if detail == "" {
				detail = "telegram returned ok=false"
			}
			reports = append(reports, DeliveryReport{Channel: "telegram", Target: target, Status: "failed", Detail: detail})
			errs = append(errs, fmt.Sprintf("telegram target %s: %s", target, detail))
			continue
		}
		reports = append(reports, DeliveryReport{
			Channel:   "telegram",
			Target:    target,
			Status:    "delivered",
			MessageID: fmt.Sprintf("%d", response.Result.MessageID),
		})
	}

	if len(errs) > 0 {
		return reports, fmt.Errorf(strings.Join(errs, "; "))
	}
	return reports, nil
}

func (d *Deliverer) deliverFeishu(ctx context.Context, cfg spec.ChannelConfig, delivery Delivery) ([]DeliveryReport, error) {
	appID := resolveSecret(cfg.AppID)
	appSecret := resolveSecret(cfg.AppSecret)
	if appID == "" || appSecret == "" {
		return nil, fmt.Errorf("feishu app credentials are not configured")
	}

	targets := deliveryTargets(cfg, delivery)
	if len(targets) == 0 {
		return nil, fmt.Errorf("feishu delivery requires at least one chat id in allow_from")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}

	var authResponse struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := d.postJSON(ctx, baseURL+"/open-apis/auth/v3/tenant_access_token/internal", "", map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	}, &authResponse); err != nil {
		return nil, err
	}
	if authResponse.Code != 0 || authResponse.TenantAccessToken == "" {
		return nil, fmt.Errorf("feishu auth failed: %s", strings.TrimSpace(authResponse.Msg))
	}

	contentBytes, err := json.Marshal(map[string]string{"text": renderMessage(delivery)})
	if err != nil {
		return nil, err
	}

	reports := make([]DeliveryReport, 0, len(targets))
	errs := make([]string, 0)
	for _, target := range targets {
		payload := map[string]any{
			"receive_id": target,
			"msg_type":   "text",
			"content":    string(contentBytes),
		}
		var messageResponse struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				MessageID string `json:"message_id"`
			} `json:"data"`
		}
		if err := d.postJSON(ctx, baseURL+"/open-apis/im/v1/messages?receive_id_type=chat_id", "Bearer "+authResponse.TenantAccessToken, payload, &messageResponse); err != nil {
			reports = append(reports, DeliveryReport{Channel: "feishu", Target: target, Status: "failed", Detail: err.Error()})
			errs = append(errs, fmt.Sprintf("feishu target %s: %v", target, err))
			continue
		}
		if messageResponse.Code != 0 {
			detail := strings.TrimSpace(messageResponse.Msg)
			if detail == "" {
				detail = "feishu returned non-zero code"
			}
			reports = append(reports, DeliveryReport{Channel: "feishu", Target: target, Status: "failed", Detail: detail})
			errs = append(errs, fmt.Sprintf("feishu target %s: %s", target, detail))
			continue
		}
		reports = append(reports, DeliveryReport{
			Channel:   "feishu",
			Target:    target,
			Status:    "delivered",
			MessageID: messageResponse.Data.MessageID,
		})
	}

	if len(errs) > 0 {
		return reports, fmt.Errorf(strings.Join(errs, "; "))
	}
	return reports, nil
}

func (d *Deliverer) postJSON(ctx context.Context, endpoint, authHeader string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if out == nil {
		return nil
	}
	if len(responseBody) == 0 {
		return nil
	}
	return json.Unmarshal(responseBody, out)
}

func findChannelConfig(team *spec.TeamSpec, kind string) (spec.ChannelConfig, bool) {
	for _, cfg := range team.Channels {
		if cfg.Kind == kind && cfg.Enabled {
			return cfg, true
		}
	}
	return spec.ChannelConfig{}, false
}

func deliveryTargets(cfg spec.ChannelConfig, delivery Delivery) []string {
	targets := resolveTargets(cfg.AllowFrom)
	if len(targets) > 0 {
		return targets
	}
	if strings.TrimSpace(delivery.Target) == "" || strings.Contains(delivery.Target, "configured-chat") {
		return nil
	}
	return splitTargets(delivery.Target)
}

func renderMessage(delivery Delivery) string {
	body := strings.TrimSpace(delivery.Body)
	title := strings.TrimSpace(delivery.Title)
	switch {
	case title == "":
		return body
	case body == "":
		return title
	default:
		return title + "\n\n" + body
	}
}

func resolveSecret(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "env:") {
		return strings.TrimSpace(os.Getenv(strings.TrimSpace(strings.TrimPrefix(value, "env:"))))
	}
	return value
}

func resolveTargets(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		resolved := resolveSecret(value)
		if resolved == "" {
			continue
		}
		out = append(out, splitTargets(resolved)...)
	}
	return out
}

func splitTargets(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}
