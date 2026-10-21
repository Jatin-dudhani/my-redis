package cluster

import (
	"fmt"
	"sync"
)

const TotalSlots = 16384

type Node struct {
	ID        string
	Addr      string
	IsMaster  bool
	StartSlot uint16
	EndSlot   uint16
}

type Cluster struct {
	mu         sync.RWMutex
	nodeID     string
	myAddr     string
	isMaster   bool
	startSlot  uint16
	endSlot    uint16
	slotToNode [TotalSlots]*Node
	nodes      map[string]*Node
}

func NewCluster(nodeID, myAddr string, startSlot, endSlot uint16) *Cluster {
	c := &Cluster{
		nodeID:    nodeID,
		myAddr:    myAddr,
		isMaster:  true,
		startSlot: startSlot,
		endSlot:   endSlot,
		nodes:     make(map[string]*Node),
	}

	me := &Node{
		ID:        nodeID,
		Addr:      myAddr,
		IsMaster:  true,
		StartSlot: startSlot,
		EndSlot:   endSlot,
	}
	c.nodes[nodeID] = me
	for s := startSlot; s <= endSlot && s < TotalSlots; s++ {
		c.slotToNode[s] = me
	}
	return c
}

func (c *Cluster) AddRemoteNode(node *Node) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nodes[node.ID] = node
	for s := node.StartSlot; s <= node.EndSlot && s < TotalSlots; s++ {
		c.slotToNode[s] = node
	}
}

// CheckSlot returns (isLocal, movedAddr, slot).
// If the current node owns the slot, isLocal is true.
// If another node owns the slot, isLocal is false and movedAddr has the target address for -MOVED redirection.
func (c *Cluster) CheckSlot(key string) (bool, string, uint16) {
	slot := KeySlot(key)

	c.mu.RLock()
	defer c.mu.RUnlock()

	if slot >= c.startSlot && slot <= c.endSlot {
		return true, "", slot
	}

	targetNode := c.slotToNode[slot]
	if targetNode != nil && targetNode.Addr != "" {
		return false, targetNode.Addr, slot
	}

	return false, c.myAddr, slot
}

func (c *Cluster) NodeID() string {
	return c.nodeID
}

func (c *Cluster) NodesInfo() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var lines string
	for _, n := range c.nodes {
		role := "master"
		if !n.IsMaster {
			role = "slave"
		}
		lines += fmt.Sprintf("%s %s %s - 0 0 1 connected %d-%d\n",
			n.ID, n.Addr, role, n.StartSlot, n.EndSlot)
	}
	return lines
}
