package memory

import (
	"os"
	"time"

	"github.com/daewoochen/agent-team-go/pkg/observe"
)

type Entry struct {
	RunID         string    `json:"run_id"`
	Task          string    `json:"task"`
	Summary       string    `json:"summary"`
	Status        string    `json:"status"`
	Timestamp     time.Time `json:"timestamp"`
	ArtifactNames []string  `json:"artifact_names"`
}

type Snapshot struct {
	TeamName string  `json:"team_name"`
	Entries  []Entry `json:"entries"`
}

type Store struct {
	Path       string
	TeamName   string
	MaxEntries int
}

func NewFileStore(teamName, path string, maxEntries int) *Store {
	return &Store{
		Path:       path,
		TeamName:   teamName,
		MaxEntries: maxEntries,
	}
}

func (s *Store) Load() (*Snapshot, error) {
	var snapshot Snapshot
	if err := observe.ReadJSON(s.Path, &snapshot); err != nil {
		if os.IsNotExist(err) {
			return &Snapshot{TeamName: s.TeamName}, nil
		}
		return nil, err
	}
	if snapshot.TeamName == "" {
		snapshot.TeamName = s.TeamName
	}
	return &snapshot, nil
}

func (s *Store) Append(entry Entry) error {
	snapshot, err := s.Load()
	if err != nil {
		return err
	}
	snapshot.TeamName = s.TeamName
	snapshot.Entries = append(snapshot.Entries, entry)
	if s.MaxEntries > 0 && len(snapshot.Entries) > s.MaxEntries {
		snapshot.Entries = snapshot.Entries[len(snapshot.Entries)-s.MaxEntries:]
	}
	return observe.WriteJSON(s.Path, snapshot)
}
