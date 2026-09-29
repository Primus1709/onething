// Command onething is the v0.1 "Give me one thing" loop in a terminal.
//
// Usage:
//
//	go run ./cmd/onething -now "2026-09-29 16:05" -mood easy
//
// It reads your settings (sample/me.json) and a list of events
// (sample/events.json), works out your free time for today, runs the funnel,
// and shows ONE option. You answer 1, 2 or 3.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "time/tzdata" // embed the time zone database so "America/Halifax" works on any machine

	"onething/internal/rank"
	"onething/internal/schedule"
)

const zone = "America/Halifax"

func main() {
	// main stays tiny: all the work is in run(), which returns an error.
	// That keeps one place for "print the problem and exit with code 1".
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	mePath := flag.String("me", "sample/me.json", "your settings and weekly schedule")
	eventsPath := flag.String("events", "sample/events.json", "events to choose from")
	nowFlag := flag.String("now", "", `pretend it's this time, e.g. "2026-09-29 16:05" (default: real now)`)
	moodFlag := flag.String("mood", "surprise", "what you're up for: outside, easy, people, social, surprise")
	whyNot := flag.Bool("why-not", false, "also show how many events each filter removed")
	flag.Parse()

	loc, err := time.LoadLocation(zone)
	if err != nil {
		return err
	}
	now := time.Now().In(loc)
	if *nowFlag != "" {
		now, err = time.ParseInLocation("2006-01-02 15:04", *nowFlag, loc)
		if err != nil {
			return fmt.Errorf(`-now must look like "2026-09-29 16:05"`)
		}
	}
	mood, err := rank.ParseMood(*moodFlag)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(*mePath)
	if err != nil {
		return err
	}
	events, err := loadEvents(*eventsPath)
	if err != nil {
		return err
	}

	// Today's free time, from now until the end of your day.
	blocks := schedule.FromNow(cfg.Schedule.FreeBlocks(now, loc), now)
	picks, funnel := rank.Recommend(events, blocks, now, cfg.Prefs(mood))

	if *whyNot {
		printFunnel(funnel)
	}
	if len(picks) == 0 {
		fmt.Println("Nothing realistic fits the rest of today. That's okay.")
		if len(blocks) == 0 {
			fmt.Println("(You don't have free time left today by your schedule.)")
		}
		return nil
	}
	return loop(picks, loc)
}

// loop shows one pick at a time. "Show me something else" moves down the
// ranked list; it stops at three, because the brief says "one to three
// realistic options", not "scroll forever".
const maxShown = 3

func loop(picks []rank.Pick, loc *time.Location) error {
	in := bufio.NewScanner(os.Stdin)
	for i := 0; i < len(picks) && i < maxShown; i++ {
		fmt.Println(strings.TrimRight(describe(picks[i], loc), "\n"))
		fmt.Print("\n  [1] I'll go   [2] Show me something else   [3] Not today\n> ")

		if !in.Scan() {
			return in.Err() // input closed (e.g. Ctrl-D): just stop quietly
		}
		switch strings.TrimSpace(in.Text()) {
		case "1":
			p := picks[i]
			fmt.Printf("\nGreat. Head out by %s.", clock(p.Plan.LeaveHomeAt, loc))
			if p.Event.SourceURL != "" {
				fmt.Printf(" Details: %s", p.Event.SourceURL)
			}
			fmt.Println()
			return nil
		case "3":
			// No tracking, no score, no streak. Nothing happens. On purpose.
			fmt.Println("\nNo problem.")
			return nil
		default: // "2", or anything else: show the next one
			fmt.Println()
		}
	}
	fmt.Println("That's everything that fits today. Staying in is a fine answer too.")
	return nil
}

func describe(p rank.Pick, loc *time.Location) string {
	var b strings.Builder
	ev := p.Event

	fmt.Fprintf(&b, "You have about %s free (%s – %s).\n\n",
		roughly(p.Block.Duration()), clock(p.Block.Start, loc), clock(p.Block.End, loc))

	fmt.Fprintf(&b, "  %s\n", ev.Title)
	fmt.Fprintf(&b, "  %s · %d-minute walk\n", ev.Venue.Name, int(p.Walk.Minutes()))
	fmt.Fprintf(&b, "  %s – %s (%s) · leave by %s, home by %s\n",
		clock(ev.Start, loc), clock(ev.End, loc), roughly(ev.Duration()),
		clock(p.Plan.LeaveHomeAt, loc), clock(p.Plan.HomeBy, loc))

	if len(p.Why) > 0 {
		fmt.Fprintf(&b, "\n  Why this one: %s.\n", joinNaturally(p.Why))
	}
	if len(p.HeadsUp) > 0 {
		fmt.Fprintf(&b, "  Heads up: %s.\n", joinNaturally(p.HeadsUp))
	}
	return b.String()
}

func printFunnel(f rank.Funnel) {
	fmt.Printf("Looked at %d events:\n", f.Total)
	rows := []struct {
		n     int
		label string
	}{
		{f.Cancelled, "cancelled"},
		{f.NoLocation, "no known location yet"},
		{f.TooFar, "too far to walk"},
		{f.TooExpensive, "over your price limit"},
		{f.WrongVibe, "not the vibe you asked for"},
		{f.NoTimeFit, "don't fit your free time"},
	}
	for _, r := range rows {
		if r.n > 0 {
			fmt.Printf("  - %d %s\n", r.n, r.label)
		}
	}
	fmt.Printf("  = %d left to choose from\n\n", f.Left)
}

func clock(t time.Time, loc *time.Location) string { return t.In(loc).Format("3:04 PM") }

// roughly rounds to the nearest half hour for "about 3 hours" style text.
func roughly(d time.Duration) string {
	if d < 55*time.Minute {
		return fmt.Sprintf("%d minutes", int(d.Round(5*time.Minute).Minutes()))
	}
	halfHours := int(d.Round(30*time.Minute) / (30 * time.Minute))
	switch {
	case halfHours <= 1:
		return fmt.Sprintf("%d minutes", int(d.Round(5*time.Minute).Minutes()))
	case halfHours == 2:
		return "an hour"
	case halfHours == 3:
		return "an hour and a half"
	case halfHours%2 == 0:
		return fmt.Sprintf("%d hours", halfHours/2)
	default:
		return fmt.Sprintf("%d and a half hours", halfHours/2)
	}
}

// joinNaturally turns ["a", "b", "c"] into "a, b and c".
func joinNaturally(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}
