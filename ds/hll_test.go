package ds

import (
	"fmt"
	"math"
	"testing"
)

func TestHLLEmpty(t *testing.T) {
	h := NewHLL()
	if c := h.Count(); c != 0 {
		t.Fatalf("expected 0 for empty HLL, got %d", c)
	}
}

func TestHLLSmallCardinality(t *testing.T) {
	h := NewHLL()
	// Insert 100 unique items
	for i := 0; i < 100; i++ {
		h.Add([]byte(fmt.Sprintf("user:%d", i)))
	}

	count := h.Count()
	// With linear counting, 100 items should be within 5% error margin
	errMargin := math.Abs(float64(count)-100.0) / 100.0
	if errMargin > 0.05 {
		t.Fatalf("expected around 100, got %d (error %.2f%%)", count, errMargin*100)
	}
}

func TestHLLLargeCardinality(t *testing.T) {
	h := NewHLL()
	const total = 50000
	for i := 0; i < total; i++ {
		h.Add([]byte(fmt.Sprintf("item:%d", i)))
	}

	count := h.Count()
	// Theoretical standard error for 16384 registers is 1.04 / sqrt(16384) = 0.81%
	errMargin := math.Abs(float64(count)-float64(total)) / float64(total)
	if errMargin > 0.03 { // within 3% tolerance
		t.Fatalf("expected around %d, got %d (error %.2f%%)", total, count, errMargin*100)
	}
}

func TestHLLMerge(t *testing.T) {
	h1 := NewHLL()
	h2 := NewHLL()

	for i := 0; i < 5000; i++ {
		h1.Add([]byte(fmt.Sprintf("val:%d", i)))
	}
	for i := 2500; i < 7500; i++ {
		h2.Add([]byte(fmt.Sprintf("val:%d", i)))
	}

	h1.Merge(h2)
	count := h1.Count()
	// Total unique items: 7500
	errMargin := math.Abs(float64(count)-7500.0) / 7500.0
	if errMargin > 0.03 {
		t.Fatalf("expected around 7500, got %d (error %.2f%%)", count, errMargin*100)
	}
}
