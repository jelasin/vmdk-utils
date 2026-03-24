package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Session struct {
	ImagePath       string    `json:"imagePath"`
	Device          string    `json:"device"`
	ReadOnly        bool      `json:"readOnly"`
	Partition       int       `json:"partition,omitempty"`
	PartitionDevice string    `json:"partitionDevice,omitempty"`
	Mountpoint      string    `json:"mountpoint,omitempty"`
	LVMVGs          []string  `json:"lvmVgs,omitempty"`
	AutoDetected    bool      `json:"autoDetected,omitempty"`
	Status          string    `json:"status"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Store struct {
	path     string
	sessions []Session
}

func Open() (*Store, error) {
	baseDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate home dir: %w", err)
	}
	path := filepath.Join(baseDir, ".local", "state", "vmdkctl", "sessions.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	store := &Store{path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Sessions() []Session {
	copyOf := make([]Session, len(s.sessions))
	copy(copyOf, s.sessions)
	return copyOf
}

func (s *Store) FindByImage(image string) (Session, bool) {
	for _, session := range s.sessions {
		if session.ImagePath == image {
			return session, true
		}
	}
	return Session{}, false
}

func (s *Store) FindByMountpoint(mountpoint string) (Session, bool) {
	for _, session := range s.sessions {
		if session.Mountpoint == mountpoint {
			return session, true
		}
	}
	return Session{}, false
}

func (s *Store) Upsert(session Session) error {
	session.UpdatedAt = time.Now().UTC()
	for i := range s.sessions {
		if s.sessions[i].Device == session.Device || s.sessions[i].ImagePath == session.ImagePath {
			s.sessions[i] = session
			return s.save()
		}
	}
	s.sessions = append(s.sessions, session)
	return s.save()
}

func (s *Store) RemoveByDevice(device string) error {
	filtered := s.sessions[:0]
	for _, session := range s.sessions {
		if session.Device != device {
			filtered = append(filtered, session)
		}
	}
	s.sessions = filtered
	return s.save()
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.sessions = []Session{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read state file: %w", err)
	}
	if len(data) == 0 {
		s.sessions = []Session{}
		return nil
	}
	if err := json.Unmarshal(data, &s.sessions); err != nil {
		return fmt.Errorf("parse state file: %w", err)
	}
	return nil
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	return nil
}
