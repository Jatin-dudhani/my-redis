package cluster

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// NodeState describes cluster node health.
type NodeState string

const (
	NodeStateOnline NodeState = "online"
	NodeStatePFail  NodeState = "pfail"
	NodeStateFail   NodeState = "fail"
)

// GossipNodeSummary contains summarized node state disseminated across the cluster.
type GossipNodeSummary struct {
	ID        string    `json:"id"`
	Addr      string    `json:"addr"`
	BusAddr   string    `json:"bus_addr"`
	StartSlot uint16    `json:"start_slot"`
	EndSlot   uint16    `json:"end_slot"`
	State     NodeState `json:"state"`
	Epoch     uint64    `json:"epoch"`
	LastSeen  time.Time `json:"last_seen"`
}

// GossipPacket is transmitted over the cluster bus.
type GossipPacket struct {
	Type        string              `json:"type"` // "MEET", "PING", "PONG", "FAIL"
	SenderID    string              `json:"sender_id"`
	SenderAddr  string              `json:"sender_addr"`
	BusAddr     string              `json:"bus_addr"`
	StartSlot   uint16              `json:"start_slot"`
	EndSlot     uint16              `json:"end_slot"`
	Epoch       uint64              `json:"epoch"`
	GossipNodes []GossipNodeSummary `json:"gossip_nodes"`
}

// GossipBus manages P2P node communication, heartbeat ping/pongs, and failure detection.
type GossipBus struct {
	mu          sync.RWMutex
	cluster     *Cluster
	nodeID      string
	addr        string
	busAddr     string
	ln          net.Listener
	peers       map[string]*GossipNodeSummary // nodeID -> summary
	stopCh      chan struct{}
	nodeTimeout time.Duration
	epoch       uint64
	ready       chan struct{}
}

// NewGossipBus creates a gossip bus instance for a cluster node.
func NewGossipBus(c *Cluster, nodeID, addr, busAddr string) *GossipBus {
	return &GossipBus{
		cluster:     c,
		nodeID:      nodeID,
		addr:        addr,
		busAddr:     busAddr,
		peers:       make(map[string]*GossipNodeSummary),
		stopCh:      make(chan struct{}),
		nodeTimeout: 3 * time.Second,
		ready:       make(chan struct{}),
		epoch:       1,
	}
}

// SetNodeTimeout sets failure detection timeout.
func (g *GossipBus) SetNodeTimeout(d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodeTimeout = d
}

// BusAddr returns bound network address.
func (g *GossipBus) BusAddr() string {
	<-g.ready
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.ln != nil {
		return g.ln.Addr().String()
	}
	return g.busAddr
}

// Start begins listening on busAddr and launches gossip heartbeats.
func (g *GossipBus) Start() error {
	ln, err := net.Listen("tcp", g.busAddr)
	if err != nil {
		return fmt.Errorf("listen on bus %s: %w", g.busAddr, err)
	}

	g.mu.Lock()
	g.ln = ln
	g.mu.Unlock()

	close(g.ready)

	go g.acceptLoop()
	go g.heartbeatLoop()

	return nil
}

// Stop shuts down the gossip listener and heartbeats.
func (g *GossipBus) Stop() {
	g.mu.Lock()
	select {
	case <-g.stopCh:
	default:
		close(g.stopCh)
	}
	if g.ln != nil {
		_ = g.ln.Close()
	}
	g.mu.Unlock()
}

func (g *GossipBus) acceptLoop() {
	for {
		conn, err := g.ln.Accept()
		if err != nil {
			select {
			case <-g.stopCh:
				return
			default:
				continue
			}
		}
		go g.handleConnection(conn)
	}
}

func (g *GossipBus) handleConnection(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	var packet GossipPacket
	if err := json.NewDecoder(conn).Decode(&packet); err != nil {
		return
	}

	g.processPacket(&packet)

	// Respond with PONG
	if packet.Type == "PING" || packet.Type == "MEET" {
		pong := g.createPacket("PONG")
		_ = json.NewEncoder(conn).Encode(pong)
	}
}

func (g *GossipBus) processPacket(p *GossipPacket) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Update or discover sender
	if p.SenderID != g.nodeID {
		peer, exists := g.peers[p.SenderID]
		if !exists {
			peer = &GossipNodeSummary{
				ID:        p.SenderID,
				Addr:      p.SenderAddr,
				BusAddr:   p.BusAddr,
				StartSlot: p.StartSlot,
				EndSlot:   p.EndSlot,
				State:     NodeStateOnline,
				Epoch:     p.Epoch,
				LastSeen:  time.Now(),
			}
			g.peers[p.SenderID] = peer
			// Add to cluster routing table
			if g.cluster != nil {
				g.cluster.AddRemoteNode(&Node{
					ID:        p.SenderID,
					Addr:      p.SenderAddr,
					IsMaster:  true,
					StartSlot: p.StartSlot,
					EndSlot:   p.EndSlot,
				})
			}
		} else {
			peer.LastSeen = time.Now()
			peer.State = NodeStateOnline
			peer.Epoch = p.Epoch
		}
	}

	// Ingest transitive gossip node summaries
	for _, n := range p.GossipNodes {
		if n.ID == g.nodeID {
			continue
		}
		peer, exists := g.peers[n.ID]
		if !exists {
			g.peers[n.ID] = &GossipNodeSummary{
				ID:        n.ID,
				Addr:      n.Addr,
				BusAddr:   n.BusAddr,
				StartSlot: n.StartSlot,
				EndSlot:   n.EndSlot,
				State:     n.State,
				Epoch:     n.Epoch,
				LastSeen:  time.Now(),
			}
			if g.cluster != nil {
				g.cluster.AddRemoteNode(&Node{
					ID:        n.ID,
					Addr:      n.Addr,
					IsMaster:  true,
					StartSlot: n.StartSlot,
					EndSlot:   n.EndSlot,
				})
			}
		} else if n.Epoch > peer.Epoch {
			peer.Epoch = n.Epoch
			peer.State = n.State
		}
	}
}

func (g *GossipBus) createPacket(packetType string) GossipPacket {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var nodes []GossipNodeSummary
	for _, p := range g.peers {
		nodes = append(nodes, *p)
	}

	start := uint16(0)
	end := uint16(0)
	if g.cluster != nil {
		start = g.cluster.startSlot
		end = g.cluster.endSlot
	}

	return GossipPacket{
		Type:        packetType,
		SenderID:    g.nodeID,
		SenderAddr:  g.addr,
		BusAddr:     g.busAddr,
		StartSlot:   start,
		EndSlot:     end,
		Epoch:       g.epoch,
		GossipNodes: nodes,
	}
}

// Meet contacts a remote node's gossip bus to introduce this node into the cluster.
func (g *GossipBus) Meet(remoteBusAddr string) error {
	conn, err := net.DialTimeout("tcp", remoteBusAddr, 2*time.Second)
	if err != nil {
		return fmt.Errorf("meet dial: %w", err)
	}
	defer conn.Close()

	packet := g.createPacket("MEET")
	if err := json.NewEncoder(conn).Encode(packet); err != nil {
		return fmt.Errorf("meet encode: %w", err)
	}

	var pong GossipPacket
	if err := json.NewDecoder(conn).Decode(&pong); err != nil {
		return fmt.Errorf("meet response: %w", err)
	}

	g.processPacket(&pong)
	return nil
}

func (g *GossipBus) heartbeatLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			g.sendHeartbeats()
			g.checkFailures()
		case <-g.stopCh:
			return
		}
	}
}

func (g *GossipBus) sendHeartbeats() {
	g.mu.RLock()
	targets := make([]string, 0, len(g.peers))
	for _, p := range g.peers {
		if p.BusAddr != "" {
			targets = append(targets, p.BusAddr)
		}
	}
	g.mu.RUnlock()

	for _, target := range targets {
		go func(addr string) {
			conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
			if err != nil {
				return
			}
			defer conn.Close()

			ping := g.createPacket("PING")
			if err := json.NewEncoder(conn).Encode(ping); err != nil {
				return
			}

			var pong GossipPacket
			if err := json.NewDecoder(conn).Decode(&pong); err == nil {
				g.processPacket(&pong)
			}
		}(target)
	}
}

func (g *GossipBus) checkFailures() {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	for _, peer := range g.peers {
		if peer.State == NodeStateOnline && now.Sub(peer.LastSeen) > g.nodeTimeout {
			peer.State = NodeStatePFail
		}
	}
}

// PeerCount returns number of known remote cluster nodes.
func (g *GossipBus) PeerCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.peers)
}

// GetPeerState returns state of peer.
func (g *GossipBus) GetPeerState(id string) (NodeState, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	p, ok := g.peers[id]
	if !ok {
		return "", false
	}
	return p.State, true
}
