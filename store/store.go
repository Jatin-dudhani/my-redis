package store

import (
	"container/list"
	"sync"
	"time"
)

type Store struct {
	mu       sync.RWMutex
	data     map[string]interface{}
	expires  map[string]time.Time
	stopCh   chan struct{}
	maxKeys  int
	lruList  *list.List
	lruIndex map[string]*list.Element
}

type lruEntry struct {
	key string
}

func New() *Store {
	return &Store{
		data:     make(map[string]interface{}),
		expires:  make(map[string]time.Time),
		lruList:  list.New(),
		lruIndex: make(map[string]*list.Element),
	}
}

func (s *Store) SetMaxKeys(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxKeys = n
	s.evictLocked()
}

func (s *Store) MaxKeys() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxKeys
}

func (s *Store) evictLocked() {
	if s.maxKeys <= 0 {
		return
	}
	for len(s.data) > s.maxKeys {
		elem := s.lruList.Back()
		if elem == nil {
			break
		}
		entry := elem.Value.(lruEntry)
		delete(s.data, entry.key)
		delete(s.expires, entry.key)
		delete(s.lruIndex, entry.key)
		s.lruList.Remove(elem)
	}
}

func (s *Store) touchLocked(key string) {
	if elem, ok := s.lruIndex[key]; ok {
		s.lruList.MoveToFront(elem)
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
			if elem, ok := s.lruIndex[key]; ok {
				s.lruList.Remove(elem)
				delete(s.lruIndex, key)
			}
		}
	}
}

func (s *Store) isExpired(key string, now time.Time) bool {
	exp, ok := s.expires[key]
	return ok && now.After(exp)
}

func (s *Store) Set(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[key]; !exists {
		elem := s.lruList.PushFront(lruEntry{key: key})
		s.lruIndex[key] = elem
	}
	s.data[key] = value
	delete(s.expires, key)
	s.evictLocked()
}

func (s *Store) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[key]; !exists {
		elem := s.lruList.PushFront(lruEntry{key: key})
		s.lruIndex[key] = elem
	}
	s.data[key] = value
	s.expires[key] = time.Now().Add(ttl)
	s.evictLocked()
}

func (s *Store) Get(key string) (interface{}, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isExpired(key, time.Now()) {
		return nil, false
	}
	s.touchLocked(key)
	val, ok := s.data[key]
	return val, ok
}

func (s *Store) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	delete(s.expires, key)
	if elem, ok := s.lruIndex[key]; ok {
		s.lruList.Remove(elem)
		delete(s.lruIndex, key)
	}
}

func (s *Store) Exists(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isExpired(key, time.Now()) {
		return false
	}
	s.touchLocked(key)
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

func (s *Store) AllStrings() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	result := make(map[string]string)
	for k, v := range s.data {
		if !s.isExpired(k, now) {
			if str, ok := v.(string); ok {
				result[k] = str
			}
		}
	}
	return result
}

func (s *Store) LoadStrings(data map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]interface{}, len(data))
	s.lruList = list.New()
	s.lruIndex = make(map[string]*list.Element)
	for k, v := range data {
		s.data[k] = v
		elem := s.lruList.PushFront(lruEntry{key: k})
		s.lruIndex[k] = elem
	}
}

func (s *Store) Expire(key string, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[key]; !ok {
		return false
	}
	s.touchLocked(key)
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
