package cluster

import (
	"testing"
)

func TestCRC16KnownVector(t *testing.T) {
	// Standard CRC16-CCITT XMODEM vector for "123456789" is 0x31C3
	res := CRC16([]byte("123456789"))
	if res != 0x31C3 {
		t.Fatalf("expected 0x31C3, got 0x%X", res)
	}
}

func TestHashTag(t *testing.T) {
	tag1 := ExtractHashTag("{user100}:profile")
	tag2 := ExtractHashTag("{user100}:orders")
	if tag1 != "user100" || tag2 != "user100" {
		t.Fatalf("expected user100, got %s and %s", tag1, tag2)
	}

	slot1 := KeySlot("{user100}:profile")
	slot2 := KeySlot("{user100}:orders")
	if slot1 != slot2 {
		t.Fatalf("expected identical slots with same tag, got %d and %d", slot1, slot2)
	}
}

func TestClusterRoutingAndMoved(t *testing.T) {
	// Node 1 owns slots 0 to 8191
	c := NewCluster("node-1", "127.0.0.1:7000", 0, 8191)

	// Node 2 owns slots 8192 to 16383
	c.AddRemoteNode(&Node{
		ID:        "node-2",
		Addr:      "127.0.0.1:7001",
		IsMaster:  true,
		StartSlot: 8192,
		EndSlot:   16383,
	})

	// find a key that maps to slot < 8192 and another key that maps to slot >= 8192
	var localKey, remoteKey string
	for i := 0; i < 1000; i++ {
		k := string(rune('a' + (i % 26))) + string(rune('0' + (i / 26)))
		s := KeySlot(k)
		if s < 8192 && localKey == "" {
			localKey = k
		}
		if s >= 8192 && remoteKey == "" {
			remoteKey = k
		}
		if localKey != "" && remoteKey != "" {
			break
		}
	}

	isLocal, _, _ := c.CheckSlot(localKey)
	if !isLocal {
		t.Fatalf("expected %s to be local", localKey)
	}

	isLocal, targetAddr, slot := c.CheckSlot(remoteKey)
	if isLocal {
		t.Fatalf("expected %s to be remote", remoteKey)
	}
	if targetAddr != "127.0.0.1:7001" {
		t.Fatalf("expected target addr 127.0.0.1:7001, got %s (slot %d)", targetAddr, slot)
	}
}
