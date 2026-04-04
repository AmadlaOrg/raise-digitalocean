package digitalocean

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type StateManager interface {
	LoadAll() ([]DropletState, error)
	Get(name string) (*DropletState, error)
	Add(state DropletState) error
	UpdateState(name string, state string) error
	Remove(name string) error
}

type stateManager struct {
	statePath string
	mu        sync.Mutex
}

func NewStateManager() StateManager {
	return &stateManager{statePath: defaultStatePath()}
}

func NewStateManagerWithPath(path string) StateManager {
	return &stateManager{statePath: path}
}

func defaultStatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "raise", "digitalocean", "state.json")
}

func (s *stateManager) LoadAll() ([]DropletState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *stateManager) Get(name string) (*DropletState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	states, err := s.load()
	if err != nil {
		return nil, err
	}
	for i := range states {
		if states[i].Name == name {
			return &states[i], nil
		}
	}
	return nil, fmt.Errorf("droplet %q not found in state", name)
}

func (s *stateManager) Add(state DropletState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	states, err := s.load()
	if err != nil {
		return err
	}
	for _, existing := range states {
		if existing.Name == state.Name {
			return fmt.Errorf("droplet %q already exists in state", state.Name)
		}
	}
	states = append(states, state)
	return s.save(states)
}

func (s *stateManager) UpdateState(name string, newState string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	states, err := s.load()
	if err != nil {
		return err
	}
	found := false
	for i := range states {
		if states[i].Name == name {
			states[i].State = newState
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("droplet %q not found in state", name)
	}
	return s.save(states)
}

func (s *stateManager) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	states, err := s.load()
	if err != nil {
		return err
	}
	filtered := make([]DropletState, 0, len(states))
	found := false
	for _, state := range states {
		if state.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, state)
	}
	if !found {
		return fmt.Errorf("droplet %q not found in state", name)
	}
	return s.save(filtered)
}

func (s *stateManager) load() ([]DropletState, error) {
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []DropletState{}, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}
	if len(data) == 0 {
		return []DropletState{}, nil
	}
	var states []DropletState
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}
	return states, nil
}

func (s *stateManager) save(states []DropletState) error {
	dir := filepath.Dir(s.statePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}
	if err := os.WriteFile(s.statePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}
	return nil
}
