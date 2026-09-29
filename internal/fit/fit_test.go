package fit

import (
	"testing"
	"time"

	"onething/internal/event"
	"onething/internal/schedule"
)

var halifax, _ = time.LoadLocation("America/Halifax")

// tue returns 2026-09-29 (a Tuesday) at the given wall-clock time in Halifax.
func tue(hh, mm int) time.Time { return time.Date(2026, 9, 29, hh, mm, 0, 0, halifax) }

var rules = Rules{
	Buffer:        10 * time.Minute,
	MaxLate:       10 * time.Minute,
	MaxLeaveEarly: 15 * time.Minute,
	MinAttend:     0.5,
}

// The exact example from the project brief:
// class until 4:00, free until 8:00, event 5:30–7:00, 15 minutes each way.
func TestBriefExampleFits(t *testing.T) {
	ev := event.Event{Start: tue(17, 30), End: tue(19, 0)}
	block := schedule.Block{Start: tue(16, 0), End: tue(20, 0)}

	res := Check(ev, block, 15*time.Minute, rules)
	if !res.Fits {
		t.Fatalf("should fit, got: %s", res.Reason)
	}
	if !res.LeaveHomeAt.Equal(tue(17, 15)) {
		t.Errorf("leave at %s, want 17:15", res.LeaveHomeAt.Format("15:04"))
	}
	if !res.HomeBy.Equal(tue(19, 15)) {
		t.Errorf("home by %s, want 19:15", res.HomeBy.Format("15:04"))
	}
}

// Table-driven test: each row is one scenario. Adding a new edge case is one
// more line, which makes it cheap to pin down every rule we care about.
func TestCheck(t *testing.T) {
	block := schedule.Block{Start: tue(16, 0), End: tue(20, 0)}
	travel := 15 * time.Minute

	tests := []struct {
		name       string
		ev         event.Event
		block      schedule.Block
		wantFit    bool
		wantReason string
		wantLate   time.Duration
		wantEarly  time.Duration
		rules      *Rules // nil = use the default rules above
	}{
		{
			name:    "comfortably inside the block",
			ev:      event.Event{Start: tue(17, 0), End: tue(18, 0)},
			block:   block,
			wantFit: true,
		},
		{
			name: "starts too soon, late entry unknown -> no",
			// earliest leave 16:10, arrive 16:25; event started 16:20
			ev:         event.Event{Start: tue(16, 20), End: tue(18, 0)},
			block:      block,
			wantReason: ReasonTooSoon,
		},
		{
			name:     "starts too soon, but walking in late is fine and within tolerance",
			ev:       event.Event{Start: tue(16, 20), End: tue(18, 0), LateEntryOK: event.Yes},
			block:    block,
			wantFit:  true,
			wantLate: 5 * time.Minute,
		},
		{
			name:       "late entry is fine, but we'd be later than the person tolerates",
			ev:         event.Event{Start: tue(16, 10), End: tue(18, 0), LateEntryOK: event.Yes},
			block:      block,
			wantReason: ReasonTooSoon, // would be 15 min late; MaxLate is 10
		},
		{
			name: "ends too close to next commitment",
			// must be home by 19:50, so must leave by 19:35; event ends 19:45
			ev:         event.Event{Start: tue(18, 0), End: tue(19, 45)},
			block:      block,
			wantReason: ReasonTooLate,
		},
		{
			name:      "ends too close, but leaving early is fine and within tolerance",
			ev:        event.Event{Start: tue(18, 0), End: tue(19, 45), LeaveEarlyOK: event.Yes},
			block:     block,
			wantFit:   true,
			wantEarly: 10 * time.Minute,
		},
		{
			name: "would only catch a small part of it",
			// with a very forgiving lateness tolerance, you'd arrive 16:25
			// to a 16:00-16:40 event: 15 of 40 minutes is under half.
			ev:         event.Event{Start: tue(16, 0), End: tue(16, 40), LateEntryOK: event.Yes},
			block:      block,
			rules:      &Rules{Buffer: 10 * time.Minute, MaxLate: time.Hour, MinAttend: 0.5},
			wantReason: ReasonTooLittle,
		},
		{
			name:       "entirely after the free block",
			ev:         event.Event{Start: tue(20, 30), End: tue(21, 0)},
			block:      block,
			wantReason: ReasonOutside,
		},
		{
			name:       "entirely before the free block",
			ev:         event.Event{Start: tue(12, 0), End: tue(13, 0)},
			block:      block,
			wantReason: ReasonOutside,
		},
		{
			name:       "broken event with end before start",
			ev:         event.Event{Start: tue(18, 0), End: tue(17, 0)},
			block:      block,
			wantReason: ReasonBadEvent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := rules
			if tc.rules != nil {
				r = *tc.rules
			}
			res := Check(tc.ev, tc.block, travel, r)
			if res.Fits != tc.wantFit {
				t.Fatalf("Fits = %v (%q), want %v", res.Fits, res.Reason, tc.wantFit)
			}
			if !tc.wantFit && res.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", res.Reason, tc.wantReason)
			}
			if res.ArriveLate != tc.wantLate {
				t.Errorf("ArriveLate = %v, want %v", res.ArriveLate, tc.wantLate)
			}
			if res.LeaveEarly != tc.wantEarly {
				t.Errorf("LeaveEarly = %v, want %v", res.LeaveEarly, tc.wantEarly)
			}
		})
	}
}

func TestFirstFitPicksTheBlockThatWorks(t *testing.T) {
	blocks := []schedule.Block{
		{Start: tue(9, 0), End: tue(14, 30)},
		{Start: tue(16, 0), End: tue(20, 0)},
	}
	ev := event.Event{Start: tue(17, 30), End: tue(18, 30)}
	res, b, ok := FirstFit(ev, blocks, 10*time.Minute, rules)
	if !ok {
		t.Fatalf("should fit the afternoon block, got %q", res.Reason)
	}
	if !b.Start.Equal(tue(16, 0)) {
		t.Errorf("fit the wrong block: %v", b)
	}
}

func TestFirstFitKeepsTheMostUsefulReason(t *testing.T) {
	blocks := []schedule.Block{
		{Start: tue(9, 0), End: tue(12, 0)},  // event is outside this one
		{Start: tue(16, 0), End: tue(20, 0)}, // event ends too close to 20:00
	}
	ev := event.Event{Start: tue(18, 0), End: tue(19, 45)}
	res, _, ok := FirstFit(ev, blocks, 15*time.Minute, rules)
	if ok || res.Reason != ReasonTooLate {
		t.Fatalf("want %q, got ok=%v reason=%q", ReasonTooLate, ok, res.Reason)
	}
}
