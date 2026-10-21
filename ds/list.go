package ds

import "sync"

type listNode struct {
	val  string
	prev *listNode
	next *listNode
}

type List struct {
	mu     sync.RWMutex
	head   *listNode
	tail   *listNode
	length int
}

func NewList() *List {
	return &List{}
}

func (l *List) LPush(vals ...string) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, v := range vals {
		node := &listNode{val: v, next: l.head}
		if l.head != nil {
			l.head.prev = node
		}
		l.head = node
		if l.tail == nil {
			l.tail = node
		}
		l.length++
	}
	return l.length
}

func (l *List) RPush(vals ...string) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, v := range vals {
		node := &listNode{val: v, prev: l.tail}
		if l.tail != nil {
			l.tail.next = node
		}
		l.tail = node
		if l.head == nil {
			l.head = node
		}
		l.length++
	}
	return l.length
}

func (l *List) LPop() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.head == nil {
		return "", false
	}
	node := l.head
	l.head = node.next
	if l.head != nil {
		l.head.prev = nil
	} else {
		l.tail = nil
	}
	l.length--
	return node.val, true
}

func (l *List) RPop() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.tail == nil {
		return "", false
	}
	node := l.tail
	l.tail = node.prev
	if l.tail != nil {
		l.tail.next = nil
	} else {
		l.head = nil
	}
	l.length--
	return node.val, true
}

func (l *List) LRange(start, stop int) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.length == 0 {
		return nil
	}

	if start < 0 {
		start = l.length + start
	}
	if stop < 0 {
		stop = l.length + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= l.length {
		stop = l.length - 1
	}
	if start > stop || start >= l.length {
		return nil
	}

	count := stop - start + 1
	result := make([]string, 0, count)

	// Bi-directional optimization: start from head or tail depending on distance
	if start <= l.length/2 {
		curr := l.head
		for i := 0; i < start; i++ {
			curr = curr.next
		}
		for i := 0; i < count && curr != nil; i++ {
			result = append(result, curr.val)
			curr = curr.next
		}
	} else {
		curr := l.tail
		for i := l.length - 1; i > stop; i-- {
			curr = curr.prev
		}
		// now curr is at index 'stop'. We want to collect elements from stop down to start, then reverse
		temp := make([]string, count)
		for i := count - 1; i >= 0 && curr != nil; i-- {
			temp[i] = curr.val
			curr = curr.prev
		}
		result = temp
	}

	return result
}

func (l *List) LLen() int {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.length
}

func (l *List) Items() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	items := make([]string, 0, l.length)
	curr := l.head
	for curr != nil {
		items = append(items, curr.val)
		curr = curr.next
	}
	return items
}
