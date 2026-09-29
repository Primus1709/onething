// Package fit answers: "if I go to this event, can I get there, be there,
// and get back before my next commitment?"
//
// It is pure logic — no database, no network, no clock. Every input is passed
// in, which is why it can be tested exhaustively. This is the heart of the
// product, so it is the part that most deserves to be boring and correct.
package fit

import (
	"time"

	"onething/internal/event"
	"onething/internal/schedule"
)

// Rules are the person's own tolerances. They come from their settings.
type Rules struct {
	// Buffer is breathing room at both ends of the trip: time to get ready
	// after a class, and time to spare before the next thing.
	Buffer time.Duration

	// MaxLate is how late you'd be willing to walk in. It only ever applies
	// when the event says late entry is fine (LateEntryOK == Yes).
	MaxLate time.Duration

	// MaxLeaveEarly is how early you'd be willing to slip out. It only
	// applies when the event says leaving early is fine.
	MaxLeaveEarly time.Duration

	// MinAttend is the smallest fraction of the event worth going for.
	// 0.5 means "if I'd miss more than half, don't suggest it".
	MinAttend float64
}

// Result describes one feasible (or infeasible) plan.
type Result struct {
	Fits   bool
	Reason string // why it doesn't fit; empty when it does

	LeaveHomeAt  time.Time
	Arrive       time.Time
	LeaveEventAt time.Time
	HomeBy       time.Time

	ArriveLate time.Duration // > 0 if the plan involves walking in late
	LeaveEarly time.Duration // > 0 if the plan involves slipping out early
}

func noFit(reason string) Result { return Result{Reason: reason} }

// Reasons an event doesn't fit. They're constants so tests (and later, the
// API) can compare them without depending on exact wording elsewhere.
const (
	ReasonBadEvent  = "the event has no valid duration"
	ReasonOutside   = "it's outside your free time"
	ReasonTooSoon   = "you couldn't get there before it starts"
	ReasonTooLate   = "it ends too close to your next commitment"
	ReasonTooLittle = "you'd only catch a small part of it"
)

// Check plans one trip: leave from home base during free block b, walk to the
// event (travel one way), attend, walk back, and be home — with Buffer to
// spare — before the block ends.
//
// v0.1 assumes every trip starts and ends at the same home base. That's
// conservative: if your next commitment is actually on the way, we just leave
// you a little more time than you need.
func Check(ev event.Event, b schedule.Block, travel time.Duration, r Rules) Result {
	if !ev.End.After(ev.Start) {
		return noFit(ReasonBadEvent)
	}
	if !ev.Start.Before(b.End) || !ev.End.After(b.Start) {
		return noFit(ReasonOutside)
	}

	earliestLeave := b.Start.Add(r.Buffer) // soonest you can head out
	latestHome := b.End.Add(-r.Buffer)     // when you need to be back

	var res Result

	// --- Getting there -------------------------------------------------
	earliestArrive := earliestLeave.Add(travel)
	if earliestArrive.After(ev.Start) {
		late := earliestArrive.Sub(ev.Start)
		if ev.LateEntryOK != event.Yes || late > r.MaxLate {
			return noFit(ReasonTooSoon)
		}
		res.Arrive = earliestArrive
		res.ArriveLate = late
	} else {
		res.Arrive = ev.Start
	}
	res.LeaveHomeAt = res.Arrive.Add(-travel)

	// --- Getting back --------------------------------------------------
	res.LeaveEventAt = ev.End
	if ev.End.Add(travel).After(latestHome) {
		leaveAt := latestHome.Add(-travel)
		early := ev.End.Sub(leaveAt)
		if ev.LeaveEarlyOK != event.Yes || early > r.MaxLeaveEarly {
			return noFit(ReasonTooLate)
		}
		res.LeaveEventAt = leaveAt
		res.LeaveEarly = early
	}
	res.HomeBy = res.LeaveEventAt.Add(travel)

	// --- Is it still worth it? ------------------------------------------
	attended := res.LeaveEventAt.Sub(res.Arrive)
	if attended <= 0 || float64(attended) < r.MinAttend*float64(ev.Duration()) {
		return noFit(ReasonTooLittle)
	}

	res.Fits = true
	return res
}

// FirstFit tries each free block in order and returns the first plan that
// works, along with the block it fits in.
func FirstFit(ev event.Event, blocks []schedule.Block, travel time.Duration, r Rules) (Result, schedule.Block, bool) {
	best := noFit(ReasonOutside)
	for _, b := range blocks {
		res := Check(ev, b, travel, r)
		if res.Fits {
			return res, b, true
		}
		if res.Reason != ReasonOutside {
			best = res // "too tight" is more informative than "outside"
		}
	}
	return best, schedule.Block{}, false
}
