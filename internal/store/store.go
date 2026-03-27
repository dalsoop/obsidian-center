package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dalsoop/obsidian-center/internal/note"
)

type Store struct {
	mu    sync.RWMutex
	path  string
	notes map[string]*note.Note
}

func New(dataDir string) *Store {
	os.MkdirAll(dataDir, 0755)
	s := &Store{
		path:  filepath.Join(dataDir, "notes.json"),
		notes: make(map[string]*note.Note),
	}
	s.load()
	return s
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var notes []*note.Note
	json.Unmarshal(data, &notes)
	for _, n := range notes {
		s.notes[n.ID] = n
	}
}

// save must be called with mu already held
func (s *Store) save() error {
	notes := make([]*note.Note, 0, len(s.notes))
	for _, n := range s.notes {
		notes = append(notes, n)
	}
	data, err := json.MarshalIndent(notes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) Submit(n *note.Note) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n.Status = note.StatusDraft
	n.Verification = "unverified"
	s.notes[n.ID] = n
	return s.save()
}

func (s *Store) Get(id string) (*note.Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n, ok := s.notes[id]
	if !ok {
		return nil, fmt.Errorf("note not found: %s", id)
	}
	return n, nil
}

func (s *Store) List(status note.Status) []*note.Note {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*note.Note
	for _, n := range s.notes {
		if status == "" || n.Status == status {
			result = append(result, n)
		}
	}
	return result
}

func (s *Store) RequestReview(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, ok := s.notes[id]
	if !ok {
		return fmt.Errorf("note not found: %s", id)
	}
	if n.Status != note.StatusDraft {
		return fmt.Errorf("note is not draft: %s", n.Status)
	}
	n.Status = note.StatusReview
	return s.save()
}

func (s *Store) AddReview(id string, r note.Review) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, ok := s.notes[id]
	if !ok {
		return fmt.Errorf("note not found: %s", id)
	}

	n.Reviews = append(n.Reviews, r)

	switch r.Status {
	case "approved":
		n.Status = note.StatusApproved
		n.Verification = "verified"
	case "rejected":
		n.Status = note.StatusRejected
	case "changes_requested":
		n.Status = note.StatusDraft
	}

	return s.save()
}

func (s *Store) Merge(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, ok := s.notes[id]
	if !ok {
		return fmt.Errorf("note not found: %s", id)
	}
	if n.Status != note.StatusApproved {
		return fmt.Errorf("note is not approved: %s", n.Status)
	}
	n.Status = note.StatusMerged
	return s.save()
}
