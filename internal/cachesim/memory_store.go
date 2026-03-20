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
		// Prefix matching depends on the earliest contiguous checkpoints remaining available.
		// Keeping only the newest entries would drop the root prefixes and collapse reads to zero.
		state.Checkpoints = append([]Checkpoint(nil), state.Checkpoints[:s.maxCheckpoints]...)
	}
	s.states[scope] = copyState(state)
	return nil
}

func (s *MemoryStore) UpdateLimits(maxScopes int, maxCheckpoints int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxScopes > 0 {
		s.maxScopes = maxScopes
		// Evict excess scopes when the limit is reduced.
		for s.maxScopes > 0 && len(s.states) > s.maxScopes {
			s.evictOldest()
		}
	}
	if maxCheckpoints > 0 {
		s.maxCheckpoints = maxCheckpoints
		// Trim existing scopes' checkpoint lists when the limit is reduced.
		for scope, state := range s.states {
			if len(state.Checkpoints) > s.maxCheckpoints {
				state.Checkpoints = append([]Checkpoint(nil), state.Checkpoints[:s.maxCheckpoints]...)
				s.states[scope] = state
			}
		}
	}
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
