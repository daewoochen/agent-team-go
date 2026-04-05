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
	Sessions   *SessionStore
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
	SessionID      string                    `json:"session_id,omitempty"`
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
		Sessions:   NewSessionStore(cleanWorkDir, 12),
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
	session, err := s.Sessions.Load(msg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	command, isCommand := ParseCommand(msg.Text)
	if isCommand {
		if handled := s.handleCommand(w, r, msg, session, command); handled {
			return
		}
	}

	selectedProfile := s.resolveProfile(session)
	userTask := strings.TrimSpace(msg.Text)
	teamName := session.TeamName
	if teamName == "" {
		teamName = sessionTeamName(msg.Channel, msg.Target, selectedProfile)
	}

	team, profile, err := s.BuildTeam(userTask, autoteam.Options{
		Profile: selectedProfile,
		WorkDir: s.WorkDir,
		Name:    teamName,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := ensureInboundChannel(team, msg, s.Deliver); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	executionTask := buildExecutionTask(session, userTask)
	result, err := s.RunTask(r.Context(), team, executionTask)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	session.Channel = msg.Channel
	session.Target = msg.Target
	session.UserID = msg.UserID
	session.TeamName = team.Name
	session.PreferredProfile = string(profile)
	session.LastRunID = result.RunID
	session.LastSummary = result.Summary
	s.Sessions.AppendTurn(session, "user", userTask, "")
	s.Sessions.AppendTurn(session, "assistant", result.Summary, result.RunID)
	if err := s.Sessions.Save(session); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := WebhookResponse{
		OK:             true,
		Channel:        msg.Channel,
		SessionID:      session.ID,
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

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request, msg InboundMessage, session *Session, command Command) bool {
	summary, continueTask, continueProfile, reset, err := HandleCommand(command, session)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return true
	}

	if continueProfile != "" {
		session.PreferredProfile = continueProfile
	}
	if reset {
		if err := s.Sessions.Delete(msg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return true
		}
		fresh, err := s.Sessions.Load(msg)
		if err == nil {
			session = fresh
		}
	}
	if continueTask != "" {
		if err := s.Sessions.Save(session); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return true
		}
		msg.Text = continueTask
		s.runInbound(w, r, msg)
		return true
	}

	if err := s.Sessions.Save(session); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return true
	}
	response := WebhookResponse{
		OK:        true,
		Channel:   msg.Channel,
		SessionID: session.ID,
		Profile:   session.PreferredProfile,
		Summary:   summary,
	}
	if s.Deliver && strings.TrimSpace(summary) != "" {
		reports, deliverErr := s.deliverControlMessage(r.Context(), msg, summary)
		response.Reports = reports
		if deliverErr != nil {
			response.Error = deliverErr.Error()
			writeJSON(w, http.StatusBadGateway, response)
			return true
		}
	}
	writeJSON(w, http.StatusOK, response)
	return true
}

func (s *Server) resolveProfile(session *Session) string {
	explicit := strings.ToLower(strings.TrimSpace(s.Profile))
	switch {
	case explicit != "" && explicit != "auto":
		return explicit
	case session != nil && strings.TrimSpace(session.PreferredProfile) != "":
		return session.PreferredProfile
	default:
		return "auto"
	}
}

func (s *Server) deliverControlMessage(ctx context.Context, msg InboundMessage, summary string) ([]channels.DeliveryReport, error) {
	team := &spec.TeamSpec{}
	if err := ensureInboundChannel(team, msg, true); err != nil {
		return nil, err
	}
	delivery := channels.Delivery{
		Channel: msg.Channel,
		Title:   fmt.Sprintf("[%s] agent-team-go", strings.ToUpper(msg.Channel)),
		Body:    summary,
		Target:  msg.Target,
		Mode:    "bot",
	}
	return s.Deliveries(ctx, team, []channels.Delivery{delivery})
}

func ensureInboundChannel(team *spec.TeamSpec, msg InboundMessage, enabled bool) error {
	switch msg.Channel {
	case "telegram":
		cfg := spec.ChannelConfig{
			Kind:      "telegram",
			Enabled:   enabled,
			AllowFrom: []string{msg.Target},
		}
		if enabled {
			if strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")) == "" {
				return fmt.Errorf("TELEGRAM_BOT_TOKEN is required for telegram webhook replies")
			}
			cfg.Token = "env:TELEGRAM_BOT_TOKEN"
		}
		upsertChannel(&team.Channels, cfg)
	case "feishu":
		cfg := spec.ChannelConfig{
			Kind:      "feishu",
			Enabled:   enabled,
			AllowFrom: []string{msg.Target},
		}
		if enabled {
			if strings.TrimSpace(os.Getenv("FEISHU_APP_ID")) == "" || strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET")) == "" {
				return fmt.Errorf("FEISHU_APP_ID and FEISHU_APP_SECRET are required for feishu webhook replies")
			}
			cfg.AppID = "env:FEISHU_APP_ID"
			cfg.AppSecret = "env:FEISHU_APP_SECRET"
		}
		upsertChannel(&team.Channels, cfg)
	default:
		return fmt.Errorf("unsupported inbound channel %q", msg.Channel)
	}
	return nil
}

func buildExecutionTask(session *Session, task string) string {
	context := BuildSessionContext(session, 6)
	if strings.TrimSpace(context) == "" {
		return task
	}
	return strings.TrimSpace(context + "\n\nCurrent user request:\n" + strings.TrimSpace(task) + "\n\nKeep continuity with the earlier conversation when it helps.")
}

func sessionTeamName(channel, target, profile string) string {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = "auto"
	}
	return fmt.Sprintf("chat-%s-%s-%s", sanitize(channel), sanitize(target), sanitize(profile))
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
