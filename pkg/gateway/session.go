package gateway

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/daewoochen/agent-team-go/pkg/observe"
)

type SessionTurn struct {
	Role      string    `json:"role"`
	Text      string    `json:"text"`
	RunID     string    `json:"run_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type Session struct {
	ID               string        `json:"id"`
	Channel          string        `json:"channel"`
	Target           string        `json:"target"`
	UserID           string        `json:"user_id,omitempty"`
	PreferredProfile string        `json:"preferred_profile,omitempty"`
	TeamName         string        `json:"team_name,omitempty"`
	LastRunID        string        `json:"last_run_id,omitempty"`
	LastSummary      string        `json:"last_summary,omitempty"`
	UpdatedAt        time.Time     `json:"updated_at"`
	Turns            []SessionTurn `json:"turns"`
}

type SessionStore struct {
	BaseDir  string
	MaxTurns int
}

func NewSessionStore(workDir string, maxTurns int) *SessionStore {
	if maxTurns <= 0 {
		maxTurns = 12
	}
	return &SessionStore{
		BaseDir:  filepath.Join(filepath.Clean(workDir), ".agentteam", "sessions"),
		MaxTurns: maxTurns,
	}
}

func (s *SessionStore) PathFor(channel, target string) string {
	return filepath.Join(s.BaseDir, sanitize(channel), sanitize(target)+".json")
}

func (s *SessionStore) Load(msg InboundMessage) (*Session, error) {
	path := s.PathFor(msg.Channel, msg.Target)
	var session Session
	if err := observe.ReadJSON(path, &session); err != nil {
		if os.IsNotExist(err) {
			return &Session{
				ID:      sessionID(msg.Channel, msg.Target),
				Channel: msg.Channel,
				Target:  msg.Target,
				UserID:  msg.UserID,
			}, nil
		}
		return nil, err
	}
	if session.ID == "" {
		session.ID = sessionID(msg.Channel, msg.Target)
	}
	if session.Channel == "" {
		session.Channel = msg.Channel
	}
	if session.Target == "" {
		session.Target = msg.Target
	}
	if session.UserID == "" {
		session.UserID = msg.UserID
	}
	return &session, nil
}

func (s *SessionStore) Save(session *Session) error {
	if session == nil {
		return fmt.Errorf("session is required")
	}
	if strings.TrimSpace(session.ID) == "" {
		session.ID = sessionID(session.Channel, session.Target)
	}
	session.UpdatedAt = time.Now().UTC()
	if s.MaxTurns > 0 && len(session.Turns) > s.MaxTurns {
		session.Turns = session.Turns[len(session.Turns)-s.MaxTurns:]
	}
	return observe.WriteJSON(s.PathFor(session.Channel, session.Target), session)
}

func (s *SessionStore) Delete(msg InboundMessage) error {
	path := s.PathFor(msg.Channel, msg.Target)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *SessionStore) List() ([]Session, error) {
	sessions := []Session{}
	if _, err := os.Stat(s.BaseDir); err != nil {
		if os.IsNotExist(err) {
			return sessions, nil
		}
		return nil, err
	}

	if err := filepath.WalkDir(s.BaseDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		var session Session
		if err := observe.ReadJSON(path, &session); err != nil {
			return err
		}
		if strings.TrimSpace(session.ID) == "" {
			session.ID = sessionID(session.Channel, session.Target)
		}
		sessions = append(sessions, session)
		return nil
	}); err != nil {
		return nil, err
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

func (s *SessionStore) AppendTurn(session *Session, role, text, runID string) {
	if session == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	session.Turns = append(session.Turns, SessionTurn{
		Role:      role,
		Text:      text,
		RunID:     strings.TrimSpace(runID),
		Timestamp: time.Now().UTC(),
	})
	if s.MaxTurns > 0 && len(session.Turns) > s.MaxTurns {
		session.Turns = session.Turns[len(session.Turns)-s.MaxTurns:]
	}
}

func BuildSessionContext(session *Session, limit int) string {
	if session == nil || len(session.Turns) == 0 || limit == 0 {
		return ""
	}
	if limit < 0 || limit > len(session.Turns) {
		limit = len(session.Turns)
	}
	turns := session.Turns[len(session.Turns)-limit:]

	lines := []string{"Conversation context:"}
	for _, turn := range turns {
		lines = append(lines, fmt.Sprintf("- %s: %s", turn.Role, firstLine(turn.Text)))
	}
	return strings.Join(lines, "\n")
}

func FormatSession(session *Session, limit int) string {
	if session == nil {
		return "No session found."
	}
	lines := []string{
		fmt.Sprintf("Session: %s", session.ID),
		fmt.Sprintf("Channel: %s", session.Channel),
		fmt.Sprintf("Target: %s", session.Target),
	}
	if session.PreferredProfile != "" {
		lines = append(lines, fmt.Sprintf("Preferred profile: %s", session.PreferredProfile))
	}
	if session.LastRunID != "" {
		lines = append(lines, fmt.Sprintf("Last run: %s", session.LastRunID))
	}
	if session.LastSummary != "" {
		lines = append(lines, fmt.Sprintf("Last summary: %s", firstLine(session.LastSummary)))
	}
	if session.UpdatedAt.IsZero() {
		lines = append(lines, "Updated: <never>")
	} else {
		lines = append(lines, fmt.Sprintf("Updated: %s", session.UpdatedAt.Format(time.RFC3339)))
	}

	if len(session.Turns) == 0 {
		lines = append(lines, "Turns: none")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "Recent turns:")
	if limit <= 0 || limit > len(session.Turns) {
		limit = len(session.Turns)
	}
	start := len(session.Turns) - limit
	for _, turn := range session.Turns[start:] {
		lines = append(lines, fmt.Sprintf("- %s %s: %s", turn.Timestamp.Format(time.RFC3339), turn.Role, firstLine(turn.Text)))
	}
	return strings.Join(lines, "\n")
}

func sessionID(channel, target string) string {
	return sanitize(channel) + ":" + sanitize(target)
}

func sanitize(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_", ".", "_", "@", "_", "?", "_", "&", "_", "=", "_")
	return replacer.Replace(value)
}

func firstLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}
