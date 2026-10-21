package ds

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	GeoMinLat = -85.05112878
	GeoMaxLat = 85.05112878
	GeoMinLon = -180.0
	GeoMaxLon = 180.0

	// EarthRadiusMeters is Redis's standard Earth radius in meters
	EarthRadiusMeters = 6372797.560856
	GeoBase32Charset  = "0123456789bcdefghjkmnpqrstuvwxyz"
)

var (
	ErrInvalidCoordinates = errors.New("ERR invalid longitude/latitude")
)

// ValidateCoordinates checks if lon and lat are within legal geospatial ranges.
func ValidateCoordinates(lon, lat float64) error {
	if lon < GeoMinLon || lon > GeoMaxLon || lat < GeoMinLat || lat > GeoMaxLat {
		return ErrInvalidCoordinates
	}
	return nil
}

// EncodeGeohash52 encodes (lon, lat) into a 52-bit integer geohash.
func EncodeGeohash52(lon, lat float64) (uint64, error) {
	if err := ValidateCoordinates(lon, lat); err != nil {
		return 0, err
	}

	normalizedLon := (lon - GeoMinLon) / (GeoMaxLon - GeoMinLon)
	normalizedLat := (lat - GeoMinLat) / (GeoMaxLat - GeoMinLat)

	// 26 bits each
	lonInt := uint64(normalizedLon * (1 << 26))
	latInt := uint64(normalizedLat * (1 << 26))

	if lonInt >= (1 << 26) {
		lonInt = (1 << 26) - 1
	}
	if latInt >= (1 << 26) {
		latInt = (1 << 26) - 1
	}

	return interleave52(lonInt, latInt), nil
}

// DecodeGeohash52 decodes a 52-bit integer geohash back to (lon, lat).
func DecodeGeohash52(hash uint64) (lon, lat float64) {
	lonInt, latInt := deinterleave52(hash)

	// Add 0.5 to center coordinate in the bounding box
	normalizedLon := (float64(lonInt) + 0.5) / float64(uint64(1)<<26)
	normalizedLat := (float64(latInt) + 0.5) / float64(uint64(1)<<26)

	lon = normalizedLon*(GeoMaxLon-GeoMinLon) + GeoMinLon
	lat = normalizedLat*(GeoMaxLat-GeoMinLat) + GeoMinLat
	return lon, lat
}

func interleave52(x, y uint64) uint64 {
	var result uint64
	for i := 0; i < 26; i++ {
		bitX := (x >> i) & 1
		bitY := (y >> i) & 1
		result |= (bitX << (2 * i)) | (bitY << (2*i + 1))
	}
	return result
}

func deinterleave52(hash uint64) (x, y uint64) {
	for i := 0; i < 26; i++ {
		bitX := (hash >> (2 * i)) & 1
		bitY := (hash >> (2*i + 1)) & 1
		x |= bitX << i
		y |= bitY << i
	}
	return x, y
}

// GeohashString converts 52-bit geohash into an 11-char Redis base32 geohash string.
func GeohashString(hash uint64) string {
	// Shift left by 8 to align 52 bits to 60 bits (12 x 5-bit characters), but Redis uses 11 chars
	val := hash << 8
	var buf [11]byte
	for i := 0; i < 11; i++ {
		idx := (val >> (55 - i*5)) & 0x1f
		buf[i] = GeoBase32Charset[idx]
	}
	return string(buf[:])
}

// DistanceUnits converts distance string to multiplier in meters.
func DistanceUnitMultiplier(unit string) (float64, error) {
	switch strings.ToLower(unit) {
	case "m", "":
		return 1.0, nil
	case "km":
		return 1000.0, nil
	case "ft":
		return 0.3048, nil
	case "mi":
		return 1609.344, nil
	default:
		return 0, fmt.Errorf("ERR unsupported unit_type '%s'", unit)
	}
}

// HaversineDistance computes great-circle distance between two coordinates in meters.
func HaversineDistance(lon1, lat1, lon2, lat2 float64) float64 {
	phi1 := lat1 * math.Pi / 180.0
	phi2 := lat2 * math.Pi / 180.0
	deltaPhi := (lat2 - lat1) * math.Pi / 180.0
	deltaLambda := (lon2 - lon1) * math.Pi / 180.0

	sinDeltaPhi := math.Sin(deltaPhi / 2.0)
	sinDeltaLambda := math.Sin(deltaLambda / 2.0)

	a := sinDeltaPhi*sinDeltaPhi + math.Cos(phi1)*math.Cos(phi2)*sinDeltaLambda*sinDeltaLambda
	if a > 1.0 {
		a = 1.0
	}
	c := 2.0 * math.Atan2(math.Sqrt(a), math.Sqrt(1.0-a))

	return EarthRadiusMeters * c
}

// GeoSearchResult holds member search result with distance and coords.
type GeoSearchResult struct {
	Member   string
	Distance float64
	Lon      float64
	Lat      float64
	Hash     uint64
}

// SearchRadius finds members in zset within radiusMeters from center.
func SearchRadius(zset *SSet, centerLon, centerLat float64, radiusMeters float64, asc bool, count int) []GeoSearchResult {
	zset.mu.RLock()
	defer zset.mu.RUnlock()

	var results []GeoSearchResult

	for member, score := range zset.dict {
		hash := uint64(score)
		lon, lat := DecodeGeohash52(hash)
		dist := HaversineDistance(centerLon, centerLat, lon, lat)
		if dist <= radiusMeters {
			results = append(results, GeoSearchResult{
				Member:   member,
				Distance: dist,
				Lon:      lon,
				Lat:      lat,
				Hash:     hash,
			})
		}
	}

	if asc {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Distance < results[j].Distance
		})
	} else {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Distance > results[j].Distance
		})
	}

	if count > 0 && len(results) > count {
		results = results[:count]
	}

	return results
}
