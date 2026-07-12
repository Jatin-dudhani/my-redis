package ds

import "sync"

type Set struct {
	mu    sync.RWMutex
	items map[string]struct{}
}

func NewSet() *Set {
	return &Set{items: make(map[string]struct{})}
}

func (s *Set) SAdd(vals ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, v := range vals {
		if _, ok := s.items[v]; !ok {
			s.items[v] = struct{}{}
			count++
		}
	}
	return count
}

func (s *Set) SMembers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, 0, len(s.items))
	for k := range s.items {
		result = append(result, k)
	}
	return result
}

func (s *Set) SRem(vals ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, v := range vals {
		if _, ok := s.items[v]; ok {
			delete(s.items, v)
			count++
		}
	}
	return count
}

func (s *Set) SIsMember(val string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.items[val]
	return ok
}

func (s *Set) SCard() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}
