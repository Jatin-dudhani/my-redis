package pubsub

import "sync"

type Message struct {
	Channel string
	Payload string
}

type Subscriber struct {
	Messages chan Message
	done     chan struct{}
}

type Hub struct {
	mu       sync.RWMutex
	channels map[string]map[*Subscriber]struct{}
}

func NewHub() *Hub {
	return &Hub{
		channels: make(map[string]map[*Subscriber]struct{}),
	}
}

func (h *Hub) Subscribe(channel string, sub *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.channels[channel] == nil {
		h.channels[channel] = make(map[*Subscriber]struct{})
	}
	h.channels[channel][sub] = struct{}{}
}

func (h *Hub) Unsubscribe(channel string, sub *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if subs, ok := h.channels[channel]; ok {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(h.channels, channel)
		}
	}
}

func (h *Hub) UnsubscribeAll(sub *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for channel, subs := range h.channels {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(h.channels, channel)
		}
	}
}

func (h *Hub) Publish(channel, message string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	subs := h.channels[channel]
	count := 0
	for sub := range subs {
		select {
		case sub.Messages <- Message{Channel: channel, Payload: message}:
			count++
		default:
		}
	}
	return count
}

func NewSubscriber() *Subscriber {
	return &Subscriber{
		Messages: make(chan Message, 256),
		done:     make(chan struct{}),
	}
}

func (s *Subscriber) Close() {
	close(s.done)
}
