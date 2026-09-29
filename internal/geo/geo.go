// Package geo answers one question for v0.1: "how long would it take to walk there?"
//
// It deliberately knows nothing about events, schedules or users. Keeping it
// small and separate means we can later swap the walking estimate for a real
// routing or transit service without touching anything else.
package geo

import (
	"math"
	"time"
)

// Point is a location on Earth in decimal degrees.
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

const (
	earthRadiusMeters = 6_371_000.0

	// walkMetersPerMinute is about 4.8 km/h: a relaxed pace, not a brisk one.
	// We would rather say "12 minutes" and have it take 10 than the reverse.
	walkMetersPerMinute = 80.0

	// detourFactor turns straight-line distance into rough street distance.
	// Streets don't go in straight lines; 1.3 is a common rule of thumb for
	// a grid-ish downtown like Halifax's peninsula.
	detourFactor = 1.3
)

// DistanceMeters is the straight-line ("as the crow flies") distance between
// two points, using the haversine formula for distance on a sphere.
func DistanceMeters(a, b Point) float64 {
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusMeters * math.Asin(math.Sqrt(h))
}

// WalkTime estimates a one-way walk, rounded UP to a whole minute.
// Rounding up is on purpose: underestimating travel is how people end up late.
func WalkTime(a, b Point) time.Duration {
	meters := DistanceMeters(a, b) * detourFactor
	minutes := math.Ceil(meters / walkMetersPerMinute)
	return time.Duration(minutes) * time.Minute
}
