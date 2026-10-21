package ds

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type StreamID struct {
	Millis uint64
	Seq    uint64
}

func (id StreamID) String() string {
	return fmt.Sprintf("%d-%d", id.Millis, id.Seq)
}

func ParseStreamID(s string) (StreamID, error) {
	if s == "*" {
		return StreamID{Millis: uint64(time.Now().UnixMilli()), Seq: 0}, nil
	}
	parts := strings.Split(s, "-")
	if len(parts) == 1 {
		m, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return StreamID{}, err
		}
		return StreamID{Millis: m, Seq: 0}, nil
	}
	if len(parts) != 2 {
		return StreamID{}, fmt.Errorf("invalid stream ID: %s", s)
	}
	m, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return StreamID{}, err
	}
	seq, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return StreamID{}, err
	}
	return StreamID{Millis: m, Seq: seq}, nil
}

func (id StreamID) Less(other StreamID) bool {
	if id.Millis != other.Millis {
		return id.Millis < other.Millis
	}
	return id.Seq < other.Seq
}

type StreamMessage struct {
	ID     StreamID
	Values []string // field, val, field, val...
}

type Stream struct {
	mu       sync.RWMutex
	messages []StreamMessage
	lastID   StreamID
}

func NewStream() *Stream {
	return &Stream{}
}

func (st *Stream) XAdd(idStr string, values ...string) (StreamID, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	var id StreamID
	if idStr == "*" {
		nowMillis := uint64(time.Now().UnixMilli())
		if nowMillis <= st.lastID.Millis {
			id = StreamID{Millis: st.lastID.Millis, Seq: st.lastID.Seq + 1}
		} else {
			id = StreamID{Millis: nowMillis, Seq: 0}
		}
	} else {
		parsed, err := ParseStreamID(idStr)
		if err != nil {
			return StreamID{}, err
		}
		if len(st.messages) > 0 && !st.lastID.Less(parsed) {
			return StreamID{}, fmt.Errorf("ERR The ID specified in XADD is equal or smaller than the target stream top item")
		}
		id = parsed
	}

	st.lastID = id
	st.messages = append(st.messages, StreamMessage{
		ID:     id,
		Values: values,
	})
	return id, nil
}

func (st *Stream) XLen() int {
	st.mu.RLock()
	defer st.mu.RUnlock()

	return len(st.messages)
}

func (st *Stream) XRange(startStr, endStr string) []StreamMessage {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if len(st.messages) == 0 {
		return nil
	}

	var startID StreamID
	if startStr != "-" {
		sid, err := ParseStreamID(startStr)
		if err == nil {
			startID = sid
		}
	}

	hasEnd := false
	var endID StreamID
	if endStr != "+" {
		eid, err := ParseStreamID(endStr)
		if err == nil {
			hasEnd = true
			endID = eid
		}
	}

	var results []StreamMessage
	for _, msg := range st.messages {
		if msg.ID.Less(startID) {
			continue
		}
		if hasEnd && endID.Less(msg.ID) {
			break
		}
		results = append(results, msg)
	}
	return results
}

func (st *Stream) XReadAfter(lastIDStr string, count int) []StreamMessage {
	st.mu.RLock()
	defer st.mu.RUnlock()

	afterID, err := ParseStreamID(lastIDStr)
	if err != nil {
		return nil
	}

	var results []StreamMessage
	for _, msg := range st.messages {
		if afterID.Less(msg.ID) {
			results = append(results, msg)
			if count > 0 && len(results) >= count {
				break
			}
		}
	}
	return results
}
