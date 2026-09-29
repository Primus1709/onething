package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"onething/internal/event"
	"onething/internal/fit"
	"onething/internal/geo"
	"onething/internal/rank"
	"onething/internal/schedule"
)

// Config is the shape of me.json. Units are in the field names
// (minutes, dollars) because they're written by hand; the program converts
// to time.Duration and cents immediately so nothing else has to care.
type Config struct {
	Home                 geo.Point       `json:"home"`
	MaxWalkMinutes       int             `json:"max_walk_minutes"`
	MaxPriceDollars      float64         `json:"max_price_dollars"`
	Interests            []string        `json:"interests"`
	BufferMinutes        int             `json:"buffer_minutes"`
	MaxLateMinutes       int             `json:"max_late_minutes"`
	MaxLeaveEarlyMinutes int             `json:"max_leave_early_minutes"`
	MinAttendFraction    float64         `json:"min_attend_fraction"`
	Schedule             schedule.Weekly `json:"schedule"`
}

func (c Config) Validate() error {
	var errs []error
	if c.MaxWalkMinutes <= 0 {
		errs = append(errs, errors.New("max_walk_minutes must be more than 0"))
	}
	if c.MaxPriceDollars < 0 {
		errs = append(errs, errors.New("max_price_dollars can't be negative"))
	}
	if c.MinAttendFraction < 0 || c.MinAttendFraction > 1 {
		errs = append(errs, errors.New("min_attend_fraction must be between 0 and 1"))
	}
	if err := c.Schedule.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("schedule: %w", err))
	}
	return errors.Join(errs...) // nil if there were no errors
}

// Prefs converts the hand-written config into what the ranker needs.
func (c Config) Prefs(m rank.Mood) rank.Prefs {
	minutes := func(n int) time.Duration { return time.Duration(n) * time.Minute }
	return rank.Prefs{
		Home:          c.Home,
		MaxWalk:       minutes(c.MaxWalkMinutes),
		MaxPriceCents: int(c.MaxPriceDollars*100 + 0.5), // round to the nearest cent
		Interests:     c.Interests,
		Mood:          m,
		Rules: fit.Rules{
			Buffer:        minutes(c.BufferMinutes),
			MaxLate:       minutes(c.MaxLateMinutes),
			MaxLeaveEarly: minutes(c.MaxLeaveEarlyMinutes),
			MinAttend:     c.MinAttendFraction,
		},
	}
}

// decodeFile reads a JSON file into v, rejecting unknown fields so a typo
// like "max_walk_minute" is an error instead of a silently ignored setting.
func decodeFile(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close() // runs when this function returns, whichever path it takes

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func loadConfig(path string) (Config, error) {
	var c Config
	if err := decodeFile(path, &c); err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func loadEvents(path string) ([]event.Event, error) {
	var events []event.Event
	err := decodeFile(path, &events)
	return events, err
}
