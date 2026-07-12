package ds

import "sync"

type List struct {
	mu   sync.RWMutex
	items []string
}

func NewList() *List {
	return &List{}
}

func (l *List) LPush(vals ...string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	// reverse to match Redis: LPUSH a b -> [b, a]
	for i := 0; i < len(vals)/2; i++ {
		j := len(vals) - 1 - i
		vals[i], vals[j] = vals[j], vals[i]
	}
	l.items = append(vals, l.items...)
	return len(l.items)
}

func (l *List) RPush(vals ...string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = append(l.items, vals...)
	return len(l.items)
}

func (l *List) LPop() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.items) == 0 {
		return "", false
	}
	val := l.items[0]
	l.items = l.items[1:]
	return val, true
}

func (l *List) RPop() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.items) == 0 {
		return "", false
	}
	val := l.items[len(l.items)-1]
	l.items = l.items[:len(l.items)-1]
	return val, true
}

func (l *List) LRange(start, stop int) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.items) == 0 {
		return nil
	}
	if start < 0 {
		start = len(l.items) + start
	}
	if stop < 0 {
		stop = len(l.items) + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= len(l.items) {
		stop = len(l.items) - 1
	}
	if start > stop || start >= len(l.items) {
		return nil
	}
	result := make([]string, stop-start+1)
	copy(result, l.items[start:stop+1])
	return result
}

func (l *List) LLen() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.items)
}
