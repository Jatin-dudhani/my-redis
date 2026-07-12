package store

import (
	"sync"
	"time"
)

type Store struct {
	mu      sync.RWMutex
	data    map[string]string
	expires map[string]time.Time
	stopCh  chan struct{}
}

func New() *Store {
	return &Store{
		data:    make(map[string]string),
		expires: make(map[string]time.Time),
	}
}

func (s *Store) StartCleanup(interval time.Duration) {
	s.stopCh = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.deleteExpired()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *Store) StopCleanup() {
	if s.stopCh != nil {
		close(s.stopCh)
	}
}

func (s *Store) deleteExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for key, exp := range s.expires {
		if now.After(exp) {
			delete(s.data, key)
			delete(s.expires, key)
		}
	}
}

func (s *Store) isExpired(key string, now time.Time) bool {
	exp, ok := s.expires[key]
	return ok && now.After(exp)
}

func (s *Store) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	delete(s.expires, key)
}

func (s *Store) SetWithTTL(key, value string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	s.expires[key] = time.Now().Add(ttl)
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.isExpired(key, time.Now()) {
		return "", false
	}
	val, ok := s.data[key]
	return val, ok
}

func (s *Store) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	delete(s.expires, key)
}

func (s *Store) Exists(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.isExpired(key, time.Now()) {
		return false
	}
	_, ok := s.data[key]
	return ok
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	now := time.Now()
	for key := range s.data {
		if !s.isExpired(key, now) {
			count++
		}
	}
	return count
}

func (s *Store) All() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	copy := make(map[string]string)
	for k, v := range s.data {
		if !s.isExpired(k, now) {
			copy[k] = v
		}
	}
	return copy
}

func (s *Store) Load(data map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = data
}

func (s *Store) Expire(key string, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[key]; !ok {
		return false
	}
	s.expires[key] = time.Now().Add(ttl)
	return true
}

func (s *Store) TTL(key string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.data[key]; !ok {
		return -2
	}
	exp, ok := s.expires[key]
	if !ok {
		return -1
	}
	rem := time.Until(exp)
	if rem <= 0 {
		return -2
	}
	return int64(rem.Seconds())
}
