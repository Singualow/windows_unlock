package secret

import "sync"

// MemoryStore is used by protocol tests and the service's explicit --console
// development mode. Production Windows builds use LSAStore.
type MemoryStore struct {
	mu     sync.Mutex
	values map[string][]byte
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{values: make(map[string][]byte)} }

func (s *MemoryStore) Put(name string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[name] = append([]byte(nil), value...)
	return nil
}

func (s *MemoryStore) Get(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[name]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *MemoryStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value := s.values[name]; value != nil {
		for i := range value {
			value[i] = 0
		}
	}
	delete(s.values, name)
	return nil
}
