package ds

import (
	"fmt"
	"sort"
	"sync"
)

type SSet struct {
	mu      sync.RWMutex
	items   map[string]float64 // member -> score
}

type zsetEntry struct {
	Member string
	Score  float64
}

func NewSSet() *SSet {
	return &SSet{items: make(map[string]float64)}
}

func (z *SSet) ZAdd(score float64, member string) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.items[member] = score
}

func (z *SSet) ZRem(members ...string) int {
	z.mu.Lock()
	defer z.mu.Unlock()
	count := 0
	for _, m := range members {
		if _, ok := z.items[m]; ok {
			delete(z.items, m)
			count++
		}
	}
	return count
}

func (z *SSet) ZScore(member string) (float64, bool) {
	z.mu.RLock()
	defer z.mu.RUnlock()
	score, ok := z.items[member]
	return score, ok
}

func (z *SSet) ZCard() int {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return len(z.items)
}

func (z *SSet) ZRange(start, stop int, withScores bool) []string {
	z.mu.RLock()
	defer z.mu.RUnlock()
	if len(z.items) == 0 {
		return nil
	}
	entries := make([]zsetEntry, 0, len(z.items))
	for member, score := range z.items {
		entries = append(entries, zsetEntry{member, score})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Score != entries[j].Score {
			return entries[i].Score < entries[j].Score
		}
		return entries[i].Member < entries[j].Member
	})
	if start < 0 {
		start = len(entries) + start
	}
	if stop < 0 {
		stop = len(entries) + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= len(entries) {
		stop = len(entries) - 1
	}
	if start > stop || start >= len(entries) {
		return nil
	}
	slice := entries[start : stop+1]
	if withScores {
		result := make([]string, 0, len(slice)*2)
		for _, e := range slice {
			result = append(result, e.Member, floatToStr(e.Score))
		}
		return result
	}
	result := make([]string, len(slice))
	for i, e := range slice {
		result[i] = e.Member
	}
	return result
}

func floatToStr(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}
