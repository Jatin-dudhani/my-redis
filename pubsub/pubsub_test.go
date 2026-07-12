package pubsub

import (
	"testing"
	"time"
)

func TestSubscribeAndPublish(t *testing.T) {
	h := NewHub()
	sub := NewSubscriber()
	defer sub.Close()

	h.Subscribe("news", sub)
	n := h.Publish("news", "hello")
	if n != 1 {
		t.Fatalf("expected 1 subscriber, got %d", n)
	}

	select {
	case msg := <-sub.Messages:
		if msg.Channel != "news" || msg.Payload != "hello" {
			t.Fatalf("expected {news hello}, got %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestPublishNoSubscribers(t *testing.T) {
	h := NewHub()
	n := h.Publish("news", "hello")
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestMultipleSubscribers(t *testing.T) {
	h := NewHub()
	sub1 := NewSubscriber()
	sub2 := NewSubscriber()
	defer sub1.Close()
	defer sub2.Close()

	h.Subscribe("news", sub1)
	h.Subscribe("news", sub2)

	n := h.Publish("news", "hello")
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
}

func TestUnsubscribe(t *testing.T) {
	h := NewHub()
	sub := NewSubscriber()
	defer sub.Close()

	h.Subscribe("news", sub)
	h.Unsubscribe("news", sub)

	n := h.Publish("news", "hello")
	if n != 0 {
		t.Fatalf("expected 0 after unsubscribe, got %d", n)
	}
}

func TestUnsubscribeAll(t *testing.T) {
	h := NewHub()
	sub := NewSubscriber()
	defer sub.Close()

	h.Subscribe("news", sub)
	h.Subscribe("sports", sub)
	h.UnsubscribeAll(sub)

	if n := h.Publish("news", "x"); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
	if n := h.Publish("sports", "y"); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestMultipleChannels(t *testing.T) {
	h := NewHub()
	sub := NewSubscriber()
	defer sub.Close()

	h.Subscribe("a", sub)
	h.Subscribe("b", sub)

	n := h.Publish("a", "msg")
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
}

func TestChannelCleanupOnUnsubscribe(t *testing.T) {
	h := NewHub()
	sub := NewSubscriber()
	h.Subscribe("temp", sub)
	h.Unsubscribe("temp", sub)

	// internal: channel map should be empty after last sub unsubscribes
	n := h.Publish("temp", "x")
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestSubscriberBuffer(t *testing.T) {
	h := NewHub()
	sub := NewSubscriber()
	defer sub.Close()

	h.Subscribe("ch", sub)

	// send more messages than buffer can hold — should not block
	for i := 0; i < 300; i++ {
		h.Publish("ch", "msg")
	}
}
