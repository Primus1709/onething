package schedule

import (
	"testing"
	"time"
)

// halifax is loaded once for all tests in this package.
var halifax = mustLoad("America/Halifax")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func clock(s string) Clock {
	c, err := ParseClock(s)
	if err != nil {
		panic(err)
	}
	return c
}

// at builds a Halifax time on a given date, e.g. at(2026, 9, 29, "16:00").
func at(y int, m time.Month, d int, hhmm string) time.Time {
	return clock(hhmm).On(time.Date(y, m, d, 12, 0, 0, 0, halifax), halifax)
}

func TestFreeBlocks(t *testing.T) {
	week := Weekly{
		DayStart: clock("09:00"),
		DayEnd:   clock("22:00"),
		Commitments: []Commitment{
			{Title: "Class", Days: []Weekday{Weekday(time.Tuesday)}, Start: clock("14:30"), End: clock("16:00")},
			{Title: "Lab", Days: []Weekday{Weekday(time.Tuesday)}, Start: clock("20:00"), End: clock("21:30")},
			{Title: "Monday only", Days: []Weekday{Weekday(time.Monday)}, Start: clock("10:00"), End: clock("11:00")},
		},
	}
	tue := time.Date(2026, 9, 29, 0, 0, 0, 0, halifax) // a Tuesday

	got := week.FreeBlocks(tue, halifax)
	want := []Block{
		{at(2026, 9, 29, "09:00"), at(2026, 9, 29, "14:30")},
		{at(2026, 9, 29, "16:00"), at(2026, 9, 29, "20:00")}, // the brief's example
		{at(2026, 9, 29, "21:30"), at(2026, 9, 29, "22:00")},
	}
	assertBlocks(t, got, want)
}

func TestOverlappingCommitmentsMerge(t *testing.T) {
	tue := Weekday(time.Tuesday)
	week := Weekly{
		DayStart: clock("09:00"),
		DayEnd:   clock("18:00"),
		Commitments: []Commitment{
			{Title: "A", Days: []Weekday{tue}, Start: clock("10:00"), End: clock("12:00")},
			{Title: "B", Days: []Weekday{tue}, Start: clock("11:00"), End: clock("13:00")}, // overlaps A
			{Title: "C", Days: []Weekday{tue}, Start: clock("11:30"), End: clock("11:45")}, // inside A and B
			{Title: "D", Days: []Weekday{tue}, Start: clock("07:00"), End: clock("09:30")}, // before the day starts
			{Title: "E", Days: []Weekday{tue}, Start: clock("17:30"), End: clock("19:00")}, // past the day end
		},
	}
	got := week.FreeBlocks(time.Date(2026, 9, 29, 0, 0, 0, 0, halifax), halifax)
	want := []Block{
		{at(2026, 9, 29, "09:30"), at(2026, 9, 29, "10:00")},
		{at(2026, 9, 29, "13:00"), at(2026, 9, 29, "17:30")},
	}
	assertBlocks(t, got, want)
}

func TestNoCommitmentsMeansWholeDayFree(t *testing.T) {
	week := Weekly{DayStart: clock("10:00"), DayEnd: clock("20:00")}
	got := week.FreeBlocks(time.Date(2026, 10, 3, 0, 0, 0, 0, halifax), halifax)
	assertBlocks(t, got, []Block{{at(2026, 10, 3, "10:00"), at(2026, 10, 3, "20:00")}})
}

func TestFromNow(t *testing.T) {
	blocks := []Block{
		{at(2026, 9, 29, "09:00"), at(2026, 9, 29, "14:30")},
		{at(2026, 9, 29, "16:00"), at(2026, 9, 29, "20:00")},
	}
	got := FromNow(blocks, at(2026, 9, 29, "16:05"))
	assertBlocks(t, got, []Block{{at(2026, 9, 29, "16:05"), at(2026, 9, 29, "20:00")}})
}

// Daylight saving time ends in Halifax at 2:00 AM on Sunday, Nov 1 2026:
// the clock goes from 1:59 back to 1:00. So midnight-to-4-AM on the clock
// is actually FIVE hours long. Because Block holds real instants, Duration
// gets this right automatically. If we stored bare "hours and minutes" and
// subtracted them, we'd be off by an hour twice a year.
func TestFreeTimeAcrossDaylightSavingChange(t *testing.T) {
	week := Weekly{DayStart: clock("00:00"), DayEnd: clock("04:00")}
	got := week.FreeBlocks(time.Date(2026, 11, 1, 12, 0, 0, 0, halifax), halifax)
	if len(got) != 1 {
		t.Fatalf("want 1 block, got %d", len(got))
	}
	if d := got[0].Duration(); d != 5*time.Hour {
		t.Fatalf("00:00 to 04:00 on Nov 1 should last 5 real hours, got %v", d)
	}
}

func TestValidate(t *testing.T) {
	bad := Weekly{DayStart: clock("22:00"), DayEnd: clock("09:00")}
	if bad.Validate() == nil {
		t.Error("day_end before day_start should be rejected")
	}
	backwards := Weekly{DayStart: clock("09:00"), DayEnd: clock("22:00"), Commitments: []Commitment{
		{Title: "Oops", Days: []Weekday{1}, Start: clock("16:00"), End: clock("14:00")},
	}}
	if backwards.Validate() == nil {
		t.Error("a commitment ending before it starts should be rejected")
	}
}

func assertBlocks(t *testing.T, got, want []Block) {
	t.Helper() // makes failures point at the calling test line, not here
	if len(got) != len(want) {
		t.Fatalf("got %d blocks %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if !got[i].Start.Equal(want[i].Start) || !got[i].End.Equal(want[i].End) {
			t.Errorf("block %d: got %s–%s, want %s–%s", i,
				got[i].Start.Format("15:04"), got[i].End.Format("15:04"),
				want[i].Start.Format("15:04"), want[i].End.Format("15:04"))
		}
	}
}
