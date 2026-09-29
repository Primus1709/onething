// Package event defines the one normalized Event shape that every source is
// converted into. Collectors (coming in v0.2) will each speak their source's
// format on the way in and produce this on the way out. Everything downstream
// — scheduling, ranking, the database — only ever sees this type.
package event

import (
	"fmt"
	"strings"
	"time"

	"onething/internal/geo"
)

// Tri is a yes/no answer that is allowed to be "we don't know".
//
// Most sources never say whether you can walk in late or leave early, so
// Unknown is the normal case, not an error. Using a plain bool would force us
// to pretend we know — and the explanations we show people would then lie.
type Tri int

const (
	Unknown Tri = iota // the zero value: a fresh Event is "unknown" everywhere
	Yes
	No
)

func (t Tri) String() string {
	switch t {
	case Yes:
		return "yes"
	case No:
		return "no"
	default:
		return "unknown"
	}
}

// UnmarshalText lets JSON (and later, config files) say "yes" / "no" /
// "unknown" instead of 1 / 2 / 0. encoding/json calls this automatically for
// any type that has it — that's the TextUnmarshaler interface.
func (t *Tri) UnmarshalText(b []byte) error {
	switch strings.ToLower(string(b)) {
	case "yes", "true":
		*t = Yes
	case "no", "false":
		*t = No
	case "unknown", "":
		*t = Unknown
	default:
		return fmt.Errorf("invalid yes/no/unknown value %q", b)
	}
	return nil
}

func (t Tri) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// Social is how much interacting with strangers an event expects.
// This is a property of the EVENT, never a judgement about the person.
type Social int

const (
	SocialUnknown Social = iota
	SocialLow            // you can mostly sit, watch or listen
	SocialMedium         // people around, some interaction, easy to stay quiet
	SocialHigh           // meeting people is the point (mixers, team games)
)

func (s Social) String() string {
	switch s {
	case SocialLow:
		return "low"
	case SocialMedium:
		return "medium"
	case SocialHigh:
		return "high"
	default:
		return "unknown"
	}
}

func (s *Social) UnmarshalText(b []byte) error {
	switch strings.ToLower(string(b)) {
	case "low":
		*s = SocialLow
	case "medium":
		*s = SocialMedium
	case "high":
		*s = SocialHigh
	case "unknown", "":
		*s = SocialUnknown
	default:
		return fmt.Errorf("invalid social level %q", b)
	}
	return nil
}

func (s Social) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Venue is where an event happens. Location is a pointer so that "not
// geocoded yet" (nil) is different from "at latitude 0, longitude 0".
type Venue struct {
	Name     string     `json:"name"`
	Address  string     `json:"address"`
	Location *geo.Point `json:"location"`
}

// Event is the normalized internal representation of one happening.
//
// Times are time.Time values: absolute instants that also remember the zone
// they were created in. Duration maths on them is always real elapsed time,
// which is what makes daylight-saving changes safe (see schedule tests).
type Event struct {
	SourceID      string `json:"source_id"`       // which collector produced it, e.g. "dal-libcal"
	SourceEventID string `json:"source_event_id"` // the source's own ID for it (iCal UID, etc.)
	SourceURL     string `json:"source_url"`

	Title       string `json:"title"`
	Description string `json:"description"`

	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	EndEstimated bool      `json:"end_estimated"` // the source gave no end; a collector guessed one

	Venue Venue `json:"venue"`

	// PriceCents is nil when the source doesn't say. 0 means free.
	// Money is stored in whole cents: floats can't represent $0.10 exactly.
	PriceCents *int `json:"price_cents"`

	Categories []string `json:"categories"`

	RegistrationRequired Tri    `json:"registration_required"`
	RegistrationURL      string `json:"registration_url"`

	LateEntryOK  Tri    `json:"late_entry_ok"`
	LeaveEarlyOK Tri    `json:"leave_early_ok"`
	Social       Social `json:"social"`
	Outdoor      Tri    `json:"outdoor"`

	Cancelled bool `json:"cancelled"`
}

// Duration is how long the event runs.
func (e Event) Duration() time.Duration { return e.End.Sub(e.Start) }

// IsFree reports whether we KNOW it's free. Unknown price is not free.
func (e Event) IsFree() bool { return e.PriceCents != nil && *e.PriceCents == 0 }

// HasCategory does a case-insensitive match against the event's categories.
func (e Event) HasCategory(c string) bool {
	for _, have := range e.Categories {
		if strings.EqualFold(have, c) {
			return true
		}
	}
	return false
}
