package ds

import (
	"math/rand"
	"time"
)

const (
	skiplistMaxLevel = 32
	skiplistP        = 0.25
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

type skipListLevel struct {
	forward *skipListNode
	span    uint32
}

type skipListNode struct {
	member   string
	score    float64
	backward *skipListNode
	level    []skipListLevel
}

type skipList struct {
	header *skipListNode
	tail   *skipListNode
	length int
	level  int
}

func newSkipListNode(level int, score float64, member string) *skipListNode {
	return &skipListNode{
		member: member,
		score:  score,
		level:  make([]skipListLevel, level),
	}
}

func newSkipList() *skipList {
	sl := &skipList{
		level:  1,
		length: 0,
		header: newSkipListNode(skiplistMaxLevel, 0, ""),
	}
	return sl
}

func (sl *skipList) randomLevel() int {
	lvl := 1
	for float64(rnd.Int31()&0xFFFF) < float64(0xFFFF)*skiplistP {
		lvl++
		if lvl >= skiplistMaxLevel {
			break
		}
	}
	return lvl
}

func (sl *skipList) insert(score float64, member string) *skipListNode {
	update := make([]*skipListNode, skiplistMaxLevel)
	rank := make([]uint32, skiplistMaxLevel)
	x := sl.header

	for i := sl.level - 1; i >= 0; i-- {
		if i == sl.level-1 {
			rank[i] = 0
		} else {
			rank[i] = rank[i+1]
		}
		for x.level[i].forward != nil &&
			(x.level[i].forward.score < score ||
				(x.level[i].forward.score == score && x.level[i].forward.member < member)) {
			rank[i] += x.level[i].span
			x = x.level[i].forward
		}
		update[i] = x
	}

	lvl := sl.randomLevel()
	if lvl > sl.level {
		for i := sl.level; i < lvl; i++ {
			rank[i] = 0
			update[i] = sl.header
			update[i].level[i].span = uint32(sl.length)
		}
		sl.level = lvl
	}

	x = newSkipListNode(lvl, score, member)
	for i := 0; i < lvl; i++ {
		x.level[i].forward = update[i].level[i].forward
		update[i].level[i].forward = x

		x.level[i].span = update[i].level[i].span - (rank[0] - rank[i])
		update[i].level[i].span = (rank[0] - rank[i]) + 1
	}

	for i := lvl; i < sl.level; i++ {
		update[i].level[i].span++
	}

	if update[0] == sl.header {
		x.backward = nil
	} else {
		x.backward = update[0]
	}
	if x.level[0].forward != nil {
		x.level[0].forward.backward = x
	} else {
		sl.tail = x
	}
	sl.length++
	return x
}

func (sl *skipList) deleteNode(x *skipListNode, update []*skipListNode) {
	for i := 0; i < sl.level; i++ {
		if update[i].level[i].forward == x {
			update[i].level[i].span += x.level[i].span - 1
			update[i].level[i].forward = x.level[i].forward
		} else {
			update[i].level[i].span--
		}
	}
	if x.level[0].forward != nil {
		x.level[0].forward.backward = x.backward
	} else {
		sl.tail = x.backward
	}
	for sl.level > 1 && sl.header.level[sl.level-1].forward == nil {
		sl.level--
	}
	sl.length--
}

func (sl *skipList) delete(score float64, member string) bool {
	update := make([]*skipListNode, skiplistMaxLevel)
	x := sl.header

	for i := sl.level - 1; i >= 0; i-- {
		for x.level[i].forward != nil &&
			(x.level[i].forward.score < score ||
				(x.level[i].forward.score == score && x.level[i].forward.member < member)) {
			x = x.level[i].forward
		}
		update[i] = x
	}

	x = x.level[0].forward
	if x != nil && x.score == score && x.member == member {
		sl.deleteNode(x, update)
		return true
	}
	return false
}

// getElementByRank returns the 1-based ranked node.
func (sl *skipList) getElementByRank(rank int) *skipListNode {
	if rank <= 0 || rank > sl.length {
		return nil
	}
	traversed := 0
	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		for x.level[i].forward != nil && traversed+int(x.level[i].span) <= rank {
			traversed += int(x.level[i].span)
			x = x.level[i].forward
		}
		if traversed == rank {
			return x
		}
	}
	return nil
}
