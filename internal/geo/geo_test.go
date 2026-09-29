package geo

import (
	"math"
	"testing"
	"time"
)

func TestDistanceOneDegreeOfLatitude(t *testing.T) {
	// One degree of latitude is about 111.2 km everywhere on Earth.
	d := DistanceMeters(Point{44, -63}, Point{45, -63})
	if math.Abs(d-111_195) > 100 {
		t.Fatalf("got %.0f m, want about 111195 m", d)
	}
}

func TestDistanceToSelfIsZero(t *testing.T) {
	p := Point{44.6425, -63.5751}
	if d := DistanceMeters(p, p); d != 0 {
		t.Fatalf("got %v, want 0", d)
	}
}

func TestWalkTimeRoundsUp(t *testing.T) {
	a := Point{44.6400, -63.5800}
	// ~100 m north: 100 * 1.3 detour = 130 m, / 80 m per min = 1.6 min -> 2 min
	b := Point{44.6400 + 100.0/111_195, -63.5800}
	if got := WalkTime(a, b); got != 2*time.Minute {
		t.Fatalf("got %v, want 2m", got)
	}
}
