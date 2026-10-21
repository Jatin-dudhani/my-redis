package ds

import (
	"math"
	"testing"
)

func TestGeohashEncodeDecode(t *testing.T) {
	// Palermo: 13.361389, 38.115556
	lon := 13.361389
	lat := 38.115556

	hash, err := EncodeGeohash52(lon, lat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dLon, dLat := DecodeGeohash52(hash)

	// Accuracy for 26-bit grid is ~0.6 meters (< 0.0001 degrees)
	if math.Abs(lon-dLon) > 0.0001 {
		t.Fatalf("longitude drift too high: expected %f, got %f", lon, dLon)
	}
	if math.Abs(lat-dLat) > 0.0001 {
		t.Fatalf("latitude drift too high: expected %f, got %f", lat, dLat)
	}
}

func TestHaversineDistance(t *testing.T) {
	// Palermo: 13.361389, 38.115556
	// Catania: 15.087269, 37.502669
	// Actual distance ~ 166.27 km (166274 m)
	d := HaversineDistance(13.361389, 38.115556, 15.087269, 37.502669)

	if math.Abs(d-166274) > 1000 {
		t.Fatalf("expected distance around 166,274 meters, got %f", d)
	}
}

func TestSearchRadius(t *testing.T) {
	zset := NewSSet()

	// Add Palermo and Catania
	hPalermo, _ := EncodeGeohash52(13.361389, 38.115556)
	zset.ZAdd(float64(hPalermo), "Palermo")

	hCatania, _ := EncodeGeohash52(15.087269, 37.502669)
	zset.ZAdd(float64(hCatania), "Catania")

	// Search 100km around Palermo -> should only find Palermo
	res100 := SearchRadius(zset, 13.361389, 38.115556, 100000, true, 0)
	if len(res100) != 1 || res100[0].Member != "Palermo" {
		t.Fatalf("expected 1 result (Palermo), got %v", res100)
	}

	// Search 200km around Palermo -> should find Palermo and Catania
	res200 := SearchRadius(zset, 13.361389, 38.115556, 200000, true, 0)
	if len(res200) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res200))
	}
	if res200[0].Member != "Palermo" || res200[1].Member != "Catania" {
		t.Fatalf("expected ordered results [Palermo, Catania], got %v", res200)
	}
}
