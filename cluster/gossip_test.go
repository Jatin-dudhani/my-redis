package cluster

import (
	"testing"
	"time"
)

func TestGossipMeetAndTopologySync(t *testing.T) {
	// Create Node 1 (slots 0-8191)
	c1 := NewCluster("node-1", "127.0.0.1:6379", 0, 8191)
	bus1 := NewGossipBus(c1, "node-1", "127.0.0.1:6379", "127.0.0.1:0")
	if err := bus1.Start(); err != nil {
		t.Fatalf("bus1 start failed: %v", err)
	}
	defer bus1.Stop()

	// Create Node 2 (slots 8192-16383)
	c2 := NewCluster("node-2", "127.0.0.1:6380", 8192, 16383)
	bus2 := NewGossipBus(c2, "node-2", "127.0.0.1:6380", "127.0.0.1:0")
	if err := bus2.Start(); err != nil {
		t.Fatalf("bus2 start failed: %v", err)
	}
	defer bus2.Stop()

	// Node 1 meets Node 2
	bus2Addr := bus2.BusAddr()
	if err := bus1.Meet(bus2Addr); err != nil {
		t.Fatalf("bus1 meet bus2 failed: %v", err)
	}

	// Verify Node 1 discovered Node 2
	if bus1.PeerCount() != 1 {
		t.Fatalf("expected 1 peer on bus1, got %d", bus1.PeerCount())
	}

	// Wait briefly for reciprocal sync or heartbeats
	time.Sleep(100 * time.Millisecond)

	// Check if cluster slot routing table in c1 now knows about node-2
	// Slot 10000 belongs to Node 2
	isLocal, targetAddr, slot := c1.CheckSlot("test_key_in_slot_10000")
	_ = slot
	if isLocal {
		t.Fatalf("expected slot to not be local on node-1")
	}
	if targetAddr != "127.0.0.1:6380" {
		t.Fatalf("expected redirection to 127.0.0.1:6380, got %s", targetAddr)
	}
}

func TestGossipFailureDetection(t *testing.T) {
	c1 := NewCluster("node-1", "127.0.0.1:6379", 0, 8191)
	bus1 := NewGossipBus(c1, "node-1", "127.0.0.1:6379", "127.0.0.1:0")
	bus1.SetNodeTimeout(50 * time.Millisecond)
	if err := bus1.Start(); err != nil {
		t.Fatalf("bus1 start failed: %v", err)
	}
	defer bus1.Stop()

	c2 := NewCluster("node-2", "127.0.0.1:6380", 8192, 16383)
	bus2 := NewGossipBus(c2, "node-2", "127.0.0.1:6380", "127.0.0.1:0")
	if err := bus2.Start(); err != nil {
		t.Fatalf("bus2 start failed: %v", err)
	}

	// Meet
	if err := bus1.Meet(bus2.BusAddr()); err != nil {
		t.Fatalf("meet failed: %v", err)
	}

	// Stop node 2 immediately to simulate crash
	bus2.Stop()

	// Wait for failure detection timeout to trip
	time.Sleep(120 * time.Millisecond)

	state, ok := bus1.GetPeerState("node-2")
	if !ok || state != NodeStatePFail {
		t.Fatalf("expected node-2 to be marked pfail, got %s (ok=%v)", state, ok)
	}
}
