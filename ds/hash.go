package ds

import "sync"

type Hash struct {
	mu    sync.RWMutex
	items map[string]string
}

func NewHash() *Hash {
	return &Hash{items: make(map[string]string)}
}

func (h *Hash) HSet(field, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items[field] = value
}

func (h *Hash) HGet(field string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	val, ok := h.items[field]
	return val, ok
}

func (h *Hash) HDel(fields ...string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	count := 0
	for _, f := range fields {
		if _, ok := h.items[f]; ok {
			delete(h.items, f)
			count++
		}
	}
	return count
}

func (h *Hash) HExists(field string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.items[field]
	return ok
}

func (h *Hash) HGetAll() map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	copy := make(map[string]string, len(h.items))
	for k, v := range h.items {
		copy[k] = v
	}
	return copy
}

func (h *Hash) HLen() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.items)
}
