package ds

import (
	"fmt"
	"sync"
)

type SSet struct {
	mu   sync.RWMutex
	dict map[string]float64
	sl   *skipList
}

type ZSetEntry struct {
	Member string
	Score  float64
}

func NewSSet() *SSet {
	return &SSet{
		dict: make(map[string]float64),
		sl:   newSkipList(),
	}
}

func (z *SSet) ZAdd(score float64, member string) {
	z.mu.Lock()
	defer z.mu.Unlock()

	if oldScore, exists := z.dict[member]; exists {
		if oldScore == score {
			return
		}
		z.sl.delete(oldScore, member)
	}
	z.dict[member] = score
	z.sl.insert(score, member)
}

func (z *SSet) ZRem(members ...string) int {
	z.mu.Lock()
	defer z.mu.Unlock()

	count := 0
	for _, m := range members {
		if score, ok := z.dict[m]; ok {
			z.sl.delete(score, m)
			delete(z.dict, m)
			count++
		}
	}
	return count
}

func (z *SSet) ZScore(member string) (float64, bool) {
	z.mu.RLock()
	defer z.mu.RUnlock()

	score, ok := z.dict[member]
	return score, ok
}

func (z *SSet) ZCard() int {
	z.mu.RLock()
	defer z.mu.RUnlock()

	return z.sl.length
}

func (z *SSet) ZRange(start, stop int, withScores bool) []string {
	z.mu.RLock()
	defer z.mu.RUnlock()

	length := z.sl.length
	if length == 0 {
		return nil
	}

	if start < 0 {
		start = length + start
	}
	if stop < 0 {
		stop = length + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= length {
		stop = length - 1
	}
	if start > stop || start >= length {
		return nil
	}

	// 1-based rank
	rank := start + 1
	node := z.sl.getElementByRank(rank)
	count := stop - start + 1

	if withScores {
		result := make([]string, 0, count*2)
		for i := 0; i < count && node != nil; i++ {
			result = append(result, node.member, floatToStr(node.score))
			node = node.level[0].forward
		}
		return result
	}

	result := make([]string, 0, count)
	for i := 0; i < count && node != nil; i++ {
		result = append(result, node.member)
		node = node.level[0].forward
	}
	return result
}

func (z *SSet) Entries() []ZSetEntry {
	z.mu.RLock()
	defer z.mu.RUnlock()

	entries := make([]ZSetEntry, 0, z.sl.length)
	node := z.sl.header.level[0].forward
	for node != nil {
		entries = append(entries, ZSetEntry{Member: node.member, Score: node.score})
		node = node.level[0].forward
	}
	return entries
}

func floatToStr(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}
