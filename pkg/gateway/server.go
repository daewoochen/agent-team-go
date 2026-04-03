package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/daewoochen/agent-team-go/pkg/autoteam"
	"github.com/daewoochen/agent-team-go/pkg/channels"
	"github.com/daewoochen/agent-team-go/pkg/runtime"
	"github.com/daewoochen/agent-team-go/pkg/spec"
)

type BuildFunc func(string, autoteam.Options) (*spec.TeamSpec, autoteam.Profile, error)
type RunFunc func(context.Context, *spec.TeamSpec, string) (*runtime.RunResult, error)
type DeliverFunc func(context.Context, *spec.TeamSpec, []channels.Delivery) ([]channels.DeliveryReport, error)

type Server struct {
	WorkDir    string
	Profile    string
	Deliver    bool
	BuildTeam  BuildFunc
	RunTask    RunFunc
	Deliveries DeliverFunc
}

type InboundMessage struct {
	Channel string `json:"channel"`
	Target  string `json:"target"`
	UserID  string `json:"user_id,omitempty"`
	Text    string `json:"text"`
}

type WebhookResponse struct {
	OK             bool                      `json:"ok"`
	Channel        string                    `json:"channel,omitempty"`
	Profile        string                    `json:"profile,omitempty"`
	RunID          string                    `json:"run_id,omitempty"`
	Status         string                    `json:"status,omitempty"`
	Summary        string                    `json:"summary,omitempty"`
	ReplayPath     string                    `json:"replay_path,omitempty"`
	CheckpointPath string                    `json:"checkpoint_path,omitempty"`
	Deliveries     []channels.Delivery       `json:"deliveries,omitempty"`
	Reports        []channels.DeliveryReport `json:"reports,omitempty"`
	Error          string                    `json:"error,omitempty"`
}

func NewServer(workDir, profile string, deliver bool) *Server {
	cleanWorkDir := filepath.Clean(workDir)
	return &Server{
		WorkDir: cleanWorkDir,
		Profile: profile,
		Deliver: deliver,
		BuildTeam: func(task string, opts autoteam.Options) (*spec.TeamSpec, autoteam.Profile, error) {
			return autoteam.Build(task, opts)
		},
		RunTask: func(ctx context.Context, team *spec.TeamSpec, task string) (*runtime.RunResult, error) {
			return runtime.NewRunner(cleanWorkDir).Run(ctx, team, task)
		},
		Deliveries: channels.DeliverPrepared,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/webhooks/telegram", s.handleTelegram)
	mux.HandleFunc("/webhooks/feishu", s.handleFeishu)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleTelegram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	msg, ok, err := decodeTelegram(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !ok {
		writeJSON(w, http.StatusAccepted, WebhookResponse{OK: true, Channel: "telegram", Summary: "ignored non-text update"})
		return
	}
	s.runInbound(w, r, msg)
}

func (s *Server) handleFeishu(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	msg, challenge, ok, err := decodeFeishu(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if challenge != "" {
		writeJSON(w, http.StatusOK, map[string]string{"challenge": challenge})
		return
	}
	if !ok {
		writeJSON(w, http.StatusAccepted, WebhookResponse{OK: true, Channel: "feishu", Summary: "ignored non-text event"})
		return
	}
	s.runInbound(w, r, msg)
}

func (s *Server) runInbound(w http.ResponseWriter, r *http.Request, msg InboundMessage) {
	team, profile, err := s.BuildTeam(msg.Text, autoteam.Options{
		Profile: s.Profile,
		WorkDir: s.WorkDir,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := ensureReplyChannel(team, msg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result, err := s.RunTask(r.Context(), team, msg.Text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := WebhookResponse{
		OK:             true,
		Channel:        msg.Channel,
		Profile:        string(profile),
		RunID:          result.RunID,
		Status:         string(result.Status),
		Summary:        result.Summary,
		ReplayPath:     result.ReplayPath,
		CheckpointPath: result.CheckpointPath,
		Deliveries:     result.Deliveries,
	}

	if s.Deliver && len(result.Deliveries) > 0 {
		reports, err := s.Deliveries(r.Context(), team, result.Deliveries)
		response.Reports = reports
		if err != nil {
			response.Error = err.Error()
			writeJSON(w, http.StatusBadGateway, response)
			return
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func ensureReplyChannel(team *spec.TeamSpec, msg InboundMessage) error {
	switch msg.Channel {
	case "telegram":
		if strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")) == "" {
			return fmt.Errorf("TELEGRAM_BOT_TOKEN is required for telegram webhook replies")
		}
		upsertChannel(&team.Channels, spec.ChannelConfig{
			Kind:      "telegram",
			Enabled:   true,
			Token:     "env:TELEGRAM_BOT_TOKEN",
			AllowFrom: []string{msg.Target},
		})
	case "feishu":
		if strings.TrimSpace(os.Getenv("FEISHU_APP_ID")) == "" || strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET")) == "" {
			return fmt.Errorf("FEISHU_APP_ID and FEISHU_APP_SECRET are required for feishu webhook replies")
		}
		upsertChannel(&team.Channels, spec.ChannelConfig{
			Kind:      "feishu",
			Enabled:   true,
			AppID:     "env:FEISHU_APP_ID",
			AppSecret: "env:FEISHU_APP_SECRET",
			AllowFrom: []string{msg.Target},
		})
	default:
		return fmt.Errorf("unsupported inbound channel %q", msg.Channel)
	}
	return nil
}

func upsertChannel(channelsList *[]spec.ChannelConfig, next spec.ChannelConfig) {
	for i := range *channelsList {
		if (*channelsList)[i].Kind == next.Kind {
			(*channelsList)[i] = next
			return
		}
	}
	*channelsList = append(*channelsList, next)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, WebhookResponse{
		OK:    false,
		Error: message,
	})
}

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
