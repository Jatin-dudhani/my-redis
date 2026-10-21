package ds

import (
	"math/bits"
	"strings"
)

// GetBit returns the bit value (0 or 1) at offset in the byte slice b.
// Bit 0 is the most significant bit (MSB) of the first byte (0x80).
func GetBit(b []byte, offset int) int {
	if offset < 0 {
		return 0
	}
	byteIndex := offset / 8
	if byteIndex >= len(b) {
		return 0
	}
	bitOffset := 7 - (offset % 8)
	return int((b[byteIndex] >> bitOffset) & 1)
}

// SetBit sets or clears the bit at offset in the byte slice b.
// Expands the slice with zeroes if offset exceeds the current length.
// Returns the updated slice and the original bit value (0 or 1).
func SetBit(b []byte, offset int, val int) ([]byte, int) {
	if offset < 0 {
		return b, 0
	}
	byteIndex := offset / 8
	bitOffset := 7 - (offset % 8)

	if byteIndex >= len(b) {
		newB := make([]byte, byteIndex+1)
		copy(newB, b)
		b = newB
	}

	oldBit := int((b[byteIndex] >> bitOffset) & 1)
	if val != 0 {
		b[byteIndex] |= (1 << bitOffset)
	} else {
		b[byteIndex] &= ^(1 << bitOffset)
	}

	return b, oldBit
}

// BitCount counts the number of set bits (1s) in b within [start, end].
// By default, start and end are byte indices (with negative indices allowed).
// If isBit is true, start and end are bit offsets.
func BitCount(b []byte, start, end int, hasRange bool, isBit bool) int64 {
	if len(b) == 0 {
		return 0
	}

	if !hasRange {
		var total int64
		for _, byt := range b {
			total += int64(bits.OnesCount8(byt))
		}
		return total
	}

	if isBit {
		totalBits := len(b) * 8
		if start < 0 {
			start += totalBits
		}
		if end < 0 {
			end += totalBits
		}
		if start < 0 {
			start = 0
		}
		if end >= totalBits {
			end = totalBits - 1
		}
		if start > end || start >= totalBits {
			return 0
		}

		var count int64
		for bit := start; bit <= end; bit++ {
			if GetBit(b, bit) == 1 {
				count++
			}
		}
		return count
	}

	// Byte range
	n := len(b)
	if start < 0 {
		start += n
	}
	if end < 0 {
		end += n
	}
	if start < 0 {
		start = 0
	}
	if end >= n {
		end = n - 1
	}
	if start > end || start >= n {
		return 0
	}

	var total int64
	for i := start; i <= end; i++ {
		total += int64(bits.OnesCount8(b[i]))
	}
	return total
}

// BitPos finds the first bit set to bitVal (0 or 1) in b.
// Returns the bit offset, or -1 if not found.
func BitPos(b []byte, bitVal int, start, end int, hasRange bool, hasEnd bool, isBit bool) int64 {
	if len(b) == 0 {
		if bitVal == 0 {
			return 0
		}
		return -1
	}

	totalBits := len(b) * 8
	bitStart := 0
	bitEnd := totalBits - 1

	if hasRange {
		if isBit {
			bitStart = start
			if bitStart < 0 {
				bitStart += totalBits
			}
			if hasEnd {
				bitEnd = end
				if bitEnd < 0 {
					bitEnd += totalBits
				}
			}
		} else {
			byteStart := start
			if byteStart < 0 {
				byteStart += len(b)
			}
			bitStart = byteStart * 8
			if hasEnd {
				byteEnd := end
				if byteEnd < 0 {
					byteEnd += len(b)
				}
				bitEnd = (byteEnd+1)*8 - 1
			}
		}
	}

	if bitStart < 0 {
		bitStart = 0
	}
	if bitEnd >= totalBits {
		bitEnd = totalBits - 1
	}
	if bitStart > bitEnd || bitStart >= totalBits {
		if bitVal == 0 {
			return int64(bitStart)
		}
		return -1
	}

	for i := bitStart; i <= bitEnd; i++ {
		if GetBit(b, i) == bitVal {
			return int64(i)
		}
	}

	// In Redis, if looking for 0 and no end is specified, any position past len is 0
	if bitVal == 0 && !hasEnd {
		return int64(totalBits)
	}

	return -1
}

// BitOp executes bitwise operations (AND, OR, XOR, NOT) on multiple byte slices.
func BitOp(op string, sources [][]byte) []byte {
	op = strings.ToUpper(op)
	if len(sources) == 0 {
		return nil
	}

	if op == "NOT" {
		src := sources[0]
		res := make([]byte, len(src))
		for i := 0; i < len(src); i++ {
			res[i] = ^src[i]
		}
		return res
	}

	// Find max length
	maxLen := 0
	for _, src := range sources {
		if len(src) > maxLen {
			maxLen = len(src)
		}
	}

	res := make([]byte, maxLen)
	for i := 0; i < maxLen; i++ {
		switch op {
		case "AND":
			val := byte(0xFF)
			for _, src := range sources {
				if i < len(src) {
					val &= src[i]
				} else {
					val = 0
				}
			}
			res[i] = val
		case "OR":
			val := byte(0)
			for _, src := range sources {
				if i < len(src) {
					val |= src[i]
				}
			}
			res[i] = val
		case "XOR":
			val := byte(0)
			if len(sources) > 0 && i < len(sources[0]) {
				val = sources[0][i]
			}
			for s := 1; s < len(sources); s++ {
				if i < len(sources[s]) {
					val ^= sources[s][i]
				}
			}
			res[i] = val
		}
	}

	return res
}
