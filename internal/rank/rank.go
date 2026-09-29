// Package rank is the funnel from the brief: many events in, one to three
// realistic options out, each with a plain-language reason.
//
// It works in two stages:
//
//  1. Filters remove anything that can't or shouldn't happen: cancelled,
//     too far, too expensive, wrong vibe for today, doesn't fit your time.
//  2. Scoring orders what's left. Every score component is between 0 and 1,
//     has a weight, and can produce a sentence for "Why this one".
//
// There is no machine learning here on purpose. If a recommendation looks
// wrong, you should be able to point at the exact line that caused it.
package rank

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"onething/internal/event"
	"onething/internal/fit"
	"onething/internal/geo"
	"onething/internal/schedule"
)

// Mood is the answer to "What are you up for today?". It's a temporary
// preference for this one request — never stored, never a diagnosis.
type Mood int

const (
	Surprise     Mood = iota // no preference: don't weigh vibe or interests
	JustOutside              // shortest, simplest way out the door
	Easy                     // low effort, low interaction, no hoops
	AroundPeople             // fine being around people, no pressure to talk
	Social                   // actually want to meet people
)

var moodNames = map[string]Mood{
	"surprise": Surprise, "outside": JustOutside, "easy": Easy,
	"people": AroundPeople, "social": Social,
}

func ParseMood(s string) (Mood, error) {
	m, ok := moodNames[strings.ToLower(s)]
	if !ok {
		return 0, fmt.Errorf("mood %q: use one of outside, easy, people, social, surprise", s)
	}
	return m, nil
}

// Prefs are the person's standing preferences plus today's mood.
type Prefs struct {
	Home          geo.Point
	MaxWalk       time.Duration
	MaxPriceCents int
	Interests     []string
	Mood          Mood
	Rules         fit.Rules
}

// Pick is one recommendation, ready to show.
type Pick struct {
	Event   event.Event
	Walk    time.Duration
	Plan    fit.Result
	Block   schedule.Block
	Score   float64
	Why     []string // reasons it was picked, strongest first
	HeadsUp []string // honest caveats worth knowing before you go
}

// Funnel counts how many events each stage removed. It's for "why didn't I
// see X?" debugging, and it makes the narrowing in the brief visible.
type Funnel struct {
	Total, Cancelled, NoLocation, TooFar, TooExpensive, WrongVibe, NoTimeFit, Left int
}

// Recommend runs the whole funnel and returns every surviving event, best
// first. The caller decides how many to show (the product rule: 1 to 3).
func Recommend(events []event.Event, blocks []schedule.Block, now time.Time, p Prefs) ([]Pick, Funnel) {
	f := Funnel{Total: len(events)}
	w := weightsFor(p.Mood)
	var picks []Pick

	for _, ev := range events {
		// Stage 1: filters, cheapest checks first.
		if ev.Cancelled {
			f.Cancelled++
			continue
		}
		if ev.Venue.Location == nil {
			f.NoLocation++
			continue
		}
		walk := geo.WalkTime(p.Home, *ev.Venue.Location)
		if walk > p.MaxWalk {
			f.TooFar++
			continue
		}
		if ev.PriceCents != nil && *ev.PriceCents > p.MaxPriceCents {
			f.TooExpensive++
			continue
		}
		socialScore, allowed := socialFit(p.Mood, ev.Social)
		if !allowed {
			f.WrongVibe++
			continue
		}
		plan, block, ok := fit.FirstFit(ev, blocks, walk, p.Rules)
		if !ok {
			f.NoTimeFit++
			continue
		}

		// Stage 2: score and explain.
		pick := Pick{Event: ev, Walk: walk, Plan: plan, Block: block}
		var parts []part
		parts = append(parts, walkPart(walk, p.MaxWalk, w.walk))
		parts = append(parts, costPart(ev, p.MaxPriceCents, w.cost))
		parts = append(parts, interestPart(ev, p.Interests, w.interest))
		parts = append(parts, registrationPart(ev, w.registration))
		parts = append(parts, socialPart(ev, p.Mood, socialScore, w.social))
		parts = append(parts, soonPart(plan.LeaveHomeAt.Sub(now), w.soon))
		parts = append(parts, outdoorPart(ev, w.outdoor))

		var total, weightSum float64
		for _, pt := range parts {
			total += pt.value * pt.weight
			weightSum += pt.weight
		}
		if weightSum > 0 {
			pick.Score = total / weightSum
		}
		pick.Why = topReasons(parts, 4)
		pick.HeadsUp = headsUp(ev, plan, p.Mood)
		picks = append(picks, pick)
	}

	// Highest score first. Ties go to whatever starts sooner, then by title,
	// so the same input always gives the same order (important for tests,
	// and for trust: asking twice shouldn't give a different answer).
	sort.SliceStable(picks, func(i, j int) bool {
		a, b := picks[i], picks[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if !a.Event.Start.Equal(b.Event.Start) {
			return a.Event.Start.Before(b.Event.Start)
		}
		return a.Event.Title < b.Event.Title
	})
	f.Left = len(picks)
	return picks, f
}

// ---------------------------------------------------------------------------
// Weights: how much each factor matters, per mood. These numbers are
// judgement calls, written down so they can be argued with and changed.

type weights struct {
	walk, cost, interest, registration, social, soon, outdoor float64
}

func weightsFor(m Mood) weights {
	w := weights{walk: 3, cost: 2, interest: 2, registration: 1, social: 2, soon: 1, outdoor: 0}
	switch m {
	case JustOutside:
		w.walk = 5 // closeness is almost everything
		w.outdoor = 1
		w.interest = 1
	case Easy:
		w.registration = 2 // signing up is exactly the kind of hoop to avoid
		w.social = 3
	case Social:
		w.social = 3
	case Surprise:
		w.interest = 0 // let something outside your usual picks win
		w.social = 0
	}
	return w
}

// socialFit says whether an event's social level is acceptable for a mood,
// and how good a match it is (0..1). Unknown is allowed but never scores as
// well as a known good match — we don't promise what we don't know.
func socialFit(m Mood, s event.Social) (score float64, allowed bool) {
	v := socialTable[m][s]
	return v, v >= 0
}

// Rows are moods; columns are the event's social level. -1 means "filtered
// out": an Easy day never gets a mixer, no matter how well it scores otherwise.
var socialTable = map[Mood][4]float64{
	//            unknown  low  medium  high
	JustOutside:  {0.6, 1.0, 0.7, -1},
	Easy:         {0.4, 1.0, 0.3, -1},
	AroundPeople: {0.6, 0.9, 1.0, 0.6},
	Social:       {0.4, 0.3, 0.8, 1.0},
	Surprise:     {0.5, 0.5, 0.5, 0.5},
}

// ---------------------------------------------------------------------------
// Score parts. Each returns a value in 0..1 and, optionally, a reason that is
// only shown if the value is high enough to be worth mentioning.

type part struct {
	value, weight float64
	reason        string
}

func walkPart(walk, max time.Duration, weight float64) part {
	v := 1 - float64(walk)/float64(max)
	return part{v, weight, fmt.Sprintf("it's %d minutes away on foot", int(walk.Minutes()))}
}

func costPart(ev event.Event, maxCents int, weight float64) part {
	switch {
	case ev.IsFree():
		return part{1, weight, "it's free"}
	case ev.PriceCents == nil:
		return part{0.3, weight, ""}
	default:
		v := 0.5
		if maxCents > 0 {
			v = 0.5 * (1 - float64(*ev.PriceCents)/float64(maxCents))
		}
		return part{v, weight, ""}
	}
}

func interestPart(ev event.Event, interests []string, weight float64) part {
	for _, in := range interests {
		if ev.HasCategory(in) {
			return part{1, weight, "it matches your interest in " + strings.ToLower(in)}
		}
	}
	return part{0, weight, ""}
}

func registrationPart(ev event.Event, weight float64) part {
	switch ev.RegistrationRequired {
	case event.No:
		return part{1, weight, "no registration needed"}
	case event.Yes:
		return part{0, weight, ""}
	default:
		return part{0.4, weight, ""}
	}
}

func socialPart(ev event.Event, m Mood, score, weight float64) part {
	reason := ""
	switch {
	case ev.Social == event.SocialLow && m != Social:
		reason = "you won't be expected to talk to anyone"
	case ev.Social == event.SocialMedium && m == AroundPeople:
		reason = "people will be around, but it's easy to keep to yourself"
	case ev.Social == event.SocialHigh && m == Social:
		reason = "it's built around meeting people"
	}
	return part{score, weight, reason}
}

// soonPart prefers leaving in 20 minutes to 2 hours. Sooner than that feels
// rushed; much later and the plan is easy to talk yourself out of.
func soonPart(untilLeave time.Duration, weight float64) part {
	switch {
	case untilLeave < 20*time.Minute:
		return part{0.5, weight, ""}
	case untilLeave <= 2*time.Hour:
		return part{1, weight, fmt.Sprintf("you'd head out in about %s", roughly(untilLeave))}
	default:
		v := 1 - float64(untilLeave-2*time.Hour)/float64(4*time.Hour)
		if v < 0 {
			v = 0
		}
		return part{v, weight, ""}
	}
}

func outdoorPart(ev event.Event, weight float64) part {
	if ev.Outdoor == event.Yes {
		return part{1, weight, "it's outside"}
	}
	return part{0, weight, ""}
}

// topReasons picks the strongest few reasons that actually have text and a
// meaningful value (>= 0.5), strongest contribution first.
func topReasons(parts []part, n int) []string {
	var keep []part
	for _, p := range parts {
		if p.reason != "" && p.value >= 0.5 && p.weight > 0 {
			keep = append(keep, p)
		}
	}
	sort.SliceStable(keep, func(i, j int) bool {
		return keep[i].value*keep[i].weight > keep[j].value*keep[j].weight
	})
	var out []string
	for i := 0; i < len(keep) && i < n; i++ {
		out = append(out, keep[i].reason)
	}
	return out
}

// headsUp lists the honest caveats. The rule: if showing the pick implies
// something we don't actually know, say so here instead.
func headsUp(ev event.Event, plan fit.Result, m Mood) []string {
	var out []string
	if plan.ArriveLate > 0 {
		out = append(out, fmt.Sprintf("you'd arrive about %d min late (it's fine to walk in)", int(plan.ArriveLate.Minutes())))
	}
	if plan.LeaveEarly > 0 {
		out = append(out, fmt.Sprintf("you'd leave about %d min before the end", int(plan.LeaveEarly.Minutes())))
	}
	if ev.RegistrationRequired == event.Yes {
		out = append(out, "it needs registration")
	}
	if ev.PriceCents == nil {
		out = append(out, "the price isn't listed")
	}
	if ev.EndEstimated {
		out = append(out, "the end time is an estimate")
	}
	if ev.Social == event.SocialUnknown && (m == Easy || m == JustOutside) {
		out = append(out, "we don't know how social this one is")
	}
	return out
}

// roughly turns 75m into "an hour and 15 minutes".
func roughly(d time.Duration) string {
	mins := int(d.Round(5 * time.Minute).Minutes())
	h, m := mins/60, mins%60
	hours := "an hour"
	if h > 1 {
		hours = fmt.Sprintf("%d hours", h)
	}
	switch {
	case h == 0:
		return fmt.Sprintf("%d minutes", m)
	case m == 0:
		return hours
	default:
		return fmt.Sprintf("%s and %d minutes", hours, m)
	}
}
