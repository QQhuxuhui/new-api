package cachesim

import "sync"

type MemoryStore struct {
	mu             sync.Mutex
	states         map[ScopeKey]State
	maxScopes      int
	maxCheckpoints int
}

func NewMemoryStore(maxScopes int, maxCheckpoints int) *MemoryStore {
	return &MemoryStore{
		states:         make(map[ScopeKey]State),
		maxScopes:      maxScopes,
		maxCheckpoints: maxCheckpoints,
	}
}

func (s *MemoryStore) Load(scope ScopeKey) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[scope]
	if !ok {
		return State{}, nil
	}
	return copyState(state), nil
}

func (s *MemoryStore) Save(scope ScopeKey, state State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.states[scope]; !ok && s.maxScopes > 0 && len(s.states) >= s.maxScopes {
		s.evictOldest()
	}
	if s.maxCheckpoints > 0 && len(state.Checkpoints) > s.maxCheckpoints {
		state.Checkpoints = append([]Checkpoint(nil), state.Checkpoints[len(state.Checkpoints)-s.maxCheckpoints:]...)
	}
	s.states[scope] = copyState(state)
	return nil
}

func (s *MemoryStore) evictOldest() {
	var oldestScope ScopeKey
	var oldestFound bool
	for scope, state := range s.states {
		if !oldestFound || state.LastSeenAt.Before(s.states[oldestScope].LastSeenAt) {
			oldestScope = scope
			oldestFound = true
		}
	}
	if oldestFound {
		delete(s.states, oldestScope)
	}
}

func copyState(state State) State {
	out := state
	out.Checkpoints = append([]Checkpoint(nil), state.Checkpoints...)
	return out
}
