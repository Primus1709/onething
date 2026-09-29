// Package schedule turns "what I do every week" into "when am I free today".
//
// v0.1 supports a manually entered weekly schedule (step 1 of the roadmap in
// the brief). ICS import and calendar integrations will later produce the same
// []Block output, so nothing downstream will need to change.
package schedule

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Clock is a wall-clock time of day, like "14:30", with no date attached.
// A weekly schedule is written in wall-clock terms ("class at 2:30 every
// Tuesday"), so we keep it that way until we know which actual day we mean.
type Clock struct {
	Hour, Minute int
}

func ParseClock(s string) (Clock, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return Clock{}, fmt.Errorf("time of day %q must look like 14:30", s)
	}
	return Clock{Hour: t.Hour(), Minute: t.Minute()}, nil
}

func (c *Clock) UnmarshalText(b []byte) error {
	parsed, err := ParseClock(string(b))
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}

func (c Clock) String() string { return fmt.Sprintf("%02d:%02d", c.Hour, c.Minute) }

func (c Clock) minutes() int { return c.Hour*60 + c.Minute }

// On pins this clock time to a specific calendar day in a specific zone.
// time.Date works in wall-clock terms, so 14:30 on a daylight-saving day is
// still 14:30 on the clock — which is what a class schedule means.
func (c Clock) On(day time.Time, loc *time.Location) time.Time {
	y, m, d := day.In(loc).Date()
	return time.Date(y, m, d, c.Hour, c.Minute, 0, 0, loc)
}

// Weekday wraps time.Weekday so config files can say "tue" instead of 2.
type Weekday time.Weekday

var weekdayNames = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

func (w *Weekday) UnmarshalText(b []byte) error {
	key := strings.ToLower(string(b))
	if len(key) > 3 {
		key = key[:3] // accept "tuesday" as well as "tue"
	}
	wd, ok := weekdayNames[key]
	if !ok {
		return fmt.Errorf("unknown weekday %q", b)
	}
	*w = Weekday(wd)
	return nil
}

// Commitment is something that repeats every week: a class, a shift, a lab.
type Commitment struct {
	Title string    `json:"title"`
	Days  []Weekday `json:"days"`
	Start Clock     `json:"start"`
	End   Clock     `json:"end"`
}

func (c Commitment) happensOn(wd time.Weekday) bool {
	for _, d := range c.Days {
		if time.Weekday(d) == wd {
			return true
		}
	}
	return false
}

// Weekly is a person's recurring week.
type Weekly struct {
	// DayStart and DayEnd bound when you'd consider going out at all.
	// Without them, 3 AM would count as "free time".
	DayStart    Clock        `json:"day_start"`
	DayEnd      Clock        `json:"day_end"`
	Commitments []Commitment `json:"commitments"`
}

// Validate catches mistakes in hand-written schedules before they turn into
// confusing recommendations.
func (w Weekly) Validate() error {
	if w.DayEnd.minutes() <= w.DayStart.minutes() {
		return errors.New("day_end must be after day_start")
	}
	for _, c := range w.Commitments {
		if c.End.minutes() <= c.Start.minutes() {
			return fmt.Errorf("%q: end must be after start (overnight commitments aren't supported yet)", c.Title)
		}
		if len(c.Days) == 0 {
			return fmt.Errorf("%q: needs at least one day", c.Title)
		}
	}
	return nil
}

// Block is a stretch of free time between two real instants.
type Block struct {
	Start, End time.Time
}

func (b Block) Duration() time.Duration { return b.End.Sub(b.Start) }

// FreeBlocks returns the free stretches on the given day, in order.
//
// The algorithm: collect that day's busy intervals, sort them by start time,
// then walk a "cursor" from DayStart to DayEnd. Every gap between the cursor
// and the next busy interval is free time. Overlapping commitments merge on
// their own, because the cursor only ever moves forward.
func (w Weekly) FreeBlocks(day time.Time, loc *time.Location) []Block {
	dayStart := w.DayStart.On(day, loc)
	dayEnd := w.DayEnd.On(day, loc)
	weekday := day.In(loc).Weekday()

	var busy []Block
	for _, c := range w.Commitments {
		if c.happensOn(weekday) {
			busy = append(busy, Block{Start: c.Start.On(day, loc), End: c.End.On(day, loc)})
		}
	}
	sort.Slice(busy, func(i, j int) bool { return busy[i].Start.Before(busy[j].Start) })

	var free []Block
	cursor := dayStart
	for _, b := range busy {
		if !b.Start.After(cursor) { // this commitment starts before the cursor: overlap
			if b.End.After(cursor) {
				cursor = b.End
			}
			continue
		}
		gapEnd := minTime(b.Start, dayEnd)
		if gapEnd.After(cursor) {
			free = append(free, Block{Start: cursor, End: gapEnd})
		}
		if b.End.After(cursor) {
			cursor = b.End
		}
	}
	if dayEnd.After(cursor) {
		free = append(free, Block{Start: cursor, End: dayEnd})
	}
	return free
}

// FromNow drops free time that is already in the past and shortens the block
// you're currently in so it starts now.
func FromNow(blocks []Block, now time.Time) []Block {
	var out []Block
	for _, b := range blocks {
		if !b.End.After(now) {
			continue
		}
		if b.Start.Before(now) {
			b.Start = now
		}
		out = append(out, b)
	}
	return out
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
