package store

import (
	"container/list"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

const numShards = 256

type shard struct {
	mu       sync.RWMutex
	data     map[string]interface{}
	expires  map[string]time.Time
	lruList  *list.List
	lruIndex map[string]*list.Element
}

type lruEntry struct {
	key string
}

type Store struct {
	shards    [numShards]*shard
	maxKeys   int64
	totalKeys int64
	cleanupMu sync.Mutex
	stopCh    chan struct{}
	rnd       *rand.Rand
	rndMu     sync.Mutex
}

func fnv32(key string) uint32 {
	var hash uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= 16777619
	}
	return hash
}

func (s *Store) getShard(key string) *shard {
	idx := fnv32(key) & (numShards - 1)
	return s.shards[idx]
}

func (s *Store) getShardIndex(key string) int {
	return int(fnv32(key) & (numShards - 1))
}

func newShard() *shard {
	return &shard{
		data:     make(map[string]interface{}),
		expires:  make(map[string]time.Time),
		lruList:  list.New(),
		lruIndex: make(map[string]*list.Element),
	}
}

func New() *Store {
	s := &Store{
		stopCh: make(chan struct{}),
		rnd:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for i := 0; i < numShards; i++ {
		s.shards[i] = newShard()
	}
	return s
}

func (s *Store) SetMaxKeys(n int) {
	atomic.StoreInt64(&s.maxKeys, int64(n))
	if n > 0 {
		s.evictOverLimit()
	}
}

func (s *Store) MaxKeys() int {
	return int(atomic.LoadInt64(&s.maxKeys))
}

func (sh *shard) evictOneLocked() (string, bool) {
	elem := sh.lruList.Back()
	if elem == nil {
		return "", false
	}
	entry := elem.Value.(lruEntry)
	delete(sh.data, entry.key)
	delete(sh.expires, entry.key)
	delete(sh.lruIndex, entry.key)
	sh.lruList.Remove(elem)
	return entry.key, true
}

func (s *Store) evictOverLimit() {
	max := atomic.LoadInt64(&s.maxKeys)
	if max <= 0 {
		return
	}
	for atomic.LoadInt64(&s.totalKeys) > max {
		// Pick random shards to evict from
		evicted := false
		s.rndMu.Lock()
		startIdx := s.rnd.Intn(numShards)
		s.rndMu.Unlock()

		for i := 0; i < numShards; i++ {
			sh := s.shards[(startIdx+i)%numShards]
			sh.mu.Lock()
			if _, ok := sh.evictOneLocked(); ok {
				atomic.AddInt64(&s.totalKeys, -1)
				evicted = true
				sh.mu.Unlock()
				break
			}
			sh.mu.Unlock()
		}
		if !evicted {
			break
		}
	}
}

func (sh *shard) touchLocked(key string) {
	if elem, ok := sh.lruIndex[key]; ok {
		sh.lruList.MoveToFront(elem)
	}
}

func (s *Store) StartCleanup(interval time.Duration) {
	s.cleanupMu.Lock()
	if s.stopCh != nil {
		select {
		case <-s.stopCh:
		default:
			close(s.stopCh)
		}
	}
	stopCh := make(chan struct{})
	s.stopCh = stopCh
	s.cleanupMu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.activeExpireCycle()
			case <-stopCh:
				return
			}
		}
	}()
}

func (s *Store) StopCleanup() {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	if s.stopCh != nil {
		select {
		case <-s.stopCh:
		default:
			close(s.stopCh)
		}
	}
}

// activeExpireCycle implements Redis active probabilistic key expiration.
// Samples 20 keys with TTL across shards. If > 25% expired, repeats (capped at 25ms).
func (s *Store) activeExpireCycle() {
	deadline := time.Now().Add(25 * time.Millisecond)

	for {
		if time.Now().After(deadline) {
			break
		}

		tested := 0
		expired := 0
		now := time.Now()

		s.rndMu.Lock()
		shardIdx := s.rnd.Intn(numShards)
		s.rndMu.Unlock()

		sh := s.shards[shardIdx]
		sh.mu.Lock()

		for k, exp := range sh.expires {
			tested++
			if now.After(exp) {
				delete(sh.data, k)
				delete(sh.expires, k)
				if elem, ok := sh.lruIndex[k]; ok {
					sh.lruList.Remove(elem)
					delete(sh.lruIndex, k)
				}
				atomic.AddInt64(&s.totalKeys, -1)
				expired++
			}
			if tested >= 20 {
				break
			}
		}
		sh.mu.Unlock()

		if tested == 0 || float64(expired)/float64(tested) <= 0.25 {
			break
		}
	}
}

func (sh *shard) isExpiredLocked(key string, now time.Time) bool {
	exp, ok := sh.expires[key]
	return ok && now.After(exp)
}

func (s *Store) Set(key string, value interface{}) {
	sh := s.getShard(key)
	sh.mu.Lock()

	_, exists := sh.data[key]
	if !exists {
		elem := sh.lruList.PushFront(lruEntry{key: key})
		sh.lruIndex[key] = elem
		atomic.AddInt64(&s.totalKeys, 1)
	} else {
		sh.touchLocked(key)
	}
	sh.data[key] = value
	delete(sh.expires, key)
	sh.mu.Unlock()

	max := atomic.LoadInt64(&s.maxKeys)
	if max > 0 && atomic.LoadInt64(&s.totalKeys) > max {
		s.evictOverLimit()
	}
}

func (s *Store) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	sh := s.getShard(key)
	sh.mu.Lock()

	_, exists := sh.data[key]
	if !exists {
		elem := sh.lruList.PushFront(lruEntry{key: key})
		sh.lruIndex[key] = elem
		atomic.AddInt64(&s.totalKeys, 1)
	} else {
		sh.touchLocked(key)
	}
	sh.data[key] = value
	sh.expires[key] = time.Now().Add(ttl)
	sh.mu.Unlock()

	max := atomic.LoadInt64(&s.maxKeys)
	if max > 0 && atomic.LoadInt64(&s.totalKeys) > max {
		s.evictOverLimit()
	}
}

func (s *Store) Get(key string) (interface{}, bool) {
	sh := s.getShard(key)
	now := time.Now()

	sh.mu.Lock()
	defer sh.mu.Unlock()

	if sh.isExpiredLocked(key, now) {
		delete(sh.data, key)
		delete(sh.expires, key)
		if elem, ok := sh.lruIndex[key]; ok {
			sh.lruList.Remove(elem)
			delete(sh.lruIndex, key)
		}
		atomic.AddInt64(&s.totalKeys, -1)
		return nil, false
	}

	sh.touchLocked(key)
	val, ok := sh.data[key]
	return val, ok
}

func (s *Store) Delete(key string) {
	sh := s.getShard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if _, exists := sh.data[key]; exists {
		delete(sh.data, key)
		delete(sh.expires, key)
		if elem, ok := sh.lruIndex[key]; ok {
			sh.lruList.Remove(elem)
			delete(sh.lruIndex, key)
		}
		atomic.AddInt64(&s.totalKeys, -1)
	}
}

func (s *Store) Exists(key string) bool {
	sh := s.getShard(key)
	now := time.Now()

	sh.mu.Lock()
	defer sh.mu.Unlock()

	if sh.isExpiredLocked(key, now) {
		delete(sh.data, key)
		delete(sh.expires, key)
		if elem, ok := sh.lruIndex[key]; ok {
			sh.lruList.Remove(elem)
			delete(sh.lruIndex, key)
		}
		atomic.AddInt64(&s.totalKeys, -1)
		return false
	}

	sh.touchLocked(key)
	_, ok := sh.data[key]
	return ok
}

func (s *Store) Len() int {
	return int(atomic.LoadInt64(&s.totalKeys))
}

func (s *Store) AllStrings() map[string]string {
	now := time.Now()
	result := make(map[string]string)

	for i := 0; i < numShards; i++ {
		sh := s.shards[i]
		sh.mu.RLock()
		for k, v := range sh.data {
			if !sh.isExpiredLocked(k, now) {
				if str, ok := v.(string); ok {
					result[k] = str
				}
			}
		}
		sh.mu.RUnlock()
	}
	return result
}

func (s *Store) LoadStrings(data map[string]string) {
	for i := 0; i < numShards; i++ {
		s.shards[i] = newShard()
	}
	atomic.StoreInt64(&s.totalKeys, 0)

	for k, v := range data {
		s.Set(k, v)
	}
}

func (s *Store) Expire(key string, ttl time.Duration) bool {
	sh := s.getShard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if _, ok := sh.data[key]; !ok {
		return false
	}
	sh.touchLocked(key)
	sh.expires[key] = time.Now().Add(ttl)
	return true
}

func (s *Store) TTL(key string) int64 {
	sh := s.getShard(key)
	now := time.Now()

	sh.mu.Lock()
	defer sh.mu.Unlock()

	if _, ok := sh.data[key]; !ok {
		return -2
	}

	exp, ok := sh.expires[key]
	if !ok {
		return -1
	}

	if now.After(exp) {
		delete(sh.data, key)
		delete(sh.expires, key)
		if elem, ok := sh.lruIndex[key]; ok {
			sh.lruList.Remove(elem)
			delete(sh.lruIndex, key)
		}
		atomic.AddInt64(&s.totalKeys, -1)
		return -2
	}

	rem := time.Until(exp)
	if rem <= 0 {
		return -2
	}
	return int64(rem.Seconds())
}

type KeyDump struct {
	Key       string
	Value     interface{}
	ExpiresAt *time.Time
}

// DumpAll returns a point-in-time snapshot of all valid (non-expired) keys across all shards.
func (s *Store) DumpAll() []KeyDump {
	now := time.Now()
	var result []KeyDump

	for i := 0; i < numShards; i++ {
		sh := s.shards[i]
		sh.mu.RLock()
		for k, v := range sh.data {
			if !sh.isExpiredLocked(k, now) {
				var exp *time.Time
				if t, hasExp := sh.expires[k]; hasExp {
					expCopy := t
					exp = &expCopy
				}
				result = append(result, KeyDump{
					Key:       k,
					Value:     v,
					ExpiresAt: exp,
				})
			}
		}
		sh.mu.RUnlock()
	}
	return result
}
