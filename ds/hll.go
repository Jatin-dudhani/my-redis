package ds

import (
	"math"
	"math/bits"
	"sync"
)

const (
	HLLRegisters = 16384 // 2^14 registers
	HLLAlpha     = 0.72134752044448170368 // 0.7213 / (1 + 1.079 / 16384)
)

// HLL represents a HyperLogLog probabilistic cardinality estimator.
type HLL struct {
	mu        sync.RWMutex
	registers [HLLRegisters]uint8
}

// NewHLL creates an empty HyperLogLog instance.
func NewHLL() *HLL {
	return &HLL{}
}

// Murmur3-like 64-bit mixer for robust, uniform distribution.
func hllHash(data []byte) uint64 {
	var h uint64 = 0xc6a4a7935bd1e995 ^ (uint64(len(data)) * 0x5bd1e995)
	for len(data) >= 8 {
		k := uint64(data[0]) | uint64(data[1])<<8 | uint64(data[2])<<16 | uint64(data[3])<<24 |
			uint64(data[4])<<32 | uint64(data[5])<<40 | uint64(data[6])<<48 | uint64(data[7])<<56
		k *= 0xc6a4a7935bd1e995
		k ^= k >> 47
		k *= 0xc6a4a7935bd1e995
		h ^= k
		h *= 0xc6a4a7935bd1e995
		data = data[8:]
	}
	if len(data) > 0 {
		var k uint64
		for i, b := range data {
			k |= uint64(b) << (8 * i)
		}
		k *= 0xc6a4a7935bd1e995
		k ^= k >> 47
		k *= 0xc6a4a7935bd1e995
		h ^= k
		h *= 0xc6a4a7935bd1e995
	}
	h ^= h >> 47
	h *= 0xc6a4a7935bd1e995
	h ^= h >> 47
	return h
}

// Add adds an element to the HyperLogLog.
// Returns true if at least one register was updated, false otherwise.
func (h *HLL) Add(element []byte) bool {
	hash := hllHash(element)
	// Register index: lower 14 bits (0..16383)
	idx := hash & (HLLRegisters - 1)
	// Remaining 50 bits
	val := hash >> 14

	// Count leading zeros in 50-bit word + 1
	// val is shifted right by 14 bits, so 64 - 14 = 50 bits.
	lz := bits.LeadingZeros64(val) - 14 + 1
	if lz < 1 {
		lz = 1
	} else if lz > 51 {
		lz = 51
	}
	rank := uint8(lz)

	h.mu.Lock()
	defer h.mu.Unlock()

	if rank > h.registers[idx] {
		h.registers[idx] = rank
		return true
	}
	return false
}

// Count returns the estimated unique elements cardinality.
func (h *HLL) Count() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return estimateRegisters(h.registers[:])
}

// Merge merges another HLL's registers into this one.
func (h *HLL) Merge(other *HLL) {
	other.mu.RLock()
	defer other.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()

	for i := 0; i < HLLRegisters; i++ {
		if other.registers[i] > h.registers[i] {
			h.registers[i] = other.registers[i]
		}
	}
}

// EstimateRegisters calculates the estimate across multiple registers.
func EstimateRegisters(registers []uint8) uint64 {
	return estimateRegisters(registers)
}

func estimateRegisters(registers []uint8) uint64 {
	var sum float64
	var zeroRegisters uint64

	for _, reg := range registers {
		sum += math.Pow(2.0, -float64(reg))
		if reg == 0 {
			zeroRegisters++
		}
	}

	m := float64(HLLRegisters)
	// Raw Flajolet-Martin harmonic mean estimate
	e := HLLAlpha * m * m / sum

	// Linear Counting correction for small cardinalities
	if e <= 2.5*m {
		if zeroRegisters > 0 {
			e = m * math.Log(m/float64(zeroRegisters))
		}
	}

	if e < 0 {
		return 0
	}
	return uint64(math.Round(e))
}

// Bytes returns a copy of raw register data (16384 bytes).
func (h *HLL) Bytes() []byte {
	h.mu.RLock()
	defer h.mu.RUnlock()

	cp := make([]byte, HLLRegisters)
	copy(cp, h.registers[:])
	return cp
}

// LoadBytes restores registers from a byte slice.
func (h *HLL) LoadBytes(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	copy(h.registers[:], data)
}
