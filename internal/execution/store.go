package execution

import "sync"

// Store는 한 번의 실행에서 공유하고 완료 후 참조를 해제하는 사용자 저장소입니다.
// 영 값으로 사용할 수 있으며 완료된 저장소는 다시 열리지 않습니다.
type Store struct {
	mu     sync.RWMutex
	values map[string]any
	closed bool
}

func (s *Store) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.values == nil {
		s.values = make(map[string]any)
	}
	s.values[key] = value
}

func (s *Store) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[key]
	return value, ok
}

// ReleaseStore는 완료된 실행의 저장값 참조를 해제합니다.
// 이후 Set은 값을 보관하지 않고 Get은 nil, false를 반환합니다.
func (s *Store) ReleaseStore() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.values = nil
}
