package rank

import (
	"strings"
	"testing"
	"time"

	"onething/internal/event"
	"onething/internal/fit"
	"onething/internal/geo"
	"onething/internal/schedule"
)

var halifax, _ = time.LoadLocation("America/Halifax")

func tue(hh, mm int) time.Time { return time.Date(2026, 9, 29, hh, mm, 0, 0, halifax) }

func cents(n int) *int { return &n }

var home = geo.Point{Lat: 44.6390, Lng: -63.5860}

// near is a point roughly an 8-minute walk from home.
var near = &geo.Point{Lat: 44.6373, Lng: -63.5916}

// far is roughly a 35-minute walk from home.
var far = &geo.Point{Lat: 44.6236, Lng: -63.5677}

func prefs(m Mood) Prefs {
	return Prefs{
		Home:          home,
		MaxWalk:       30 * time.Minute,
		MaxPriceCents: 1000,
		Interests:     []string{"science"},
		Mood:          m,
		Rules:         fit.Rules{Buffer: 10 * time.Minute, MinAttend: 0.5},
	}
}

var afternoon = []schedule.Block{{Start: tue(16, 0), End: tue(20, 0)}}

func ev(title string, opts ...func(*event.Event)) event.Event {
	e := event.Event{
		Title: title,
		Start: tue(17, 30), End: tue(18, 30),
		Venue:      event.Venue{Name: "Somewhere", Location: near},
		PriceCents: cents(0),
	}
	for _, o := range opts {
		o(&e)
	}
	return e
}

func TestEasyMoodPrefersLowSocialAndNeverShowsHigh(t *testing.T) {
	events := []event.Event{
		ev("Mixer", func(e *event.Event) { e.Social = event.SocialHigh }),
		ev("Lecture", func(e *event.Event) {
			e.Social = event.SocialLow
			e.RegistrationRequired = event.No
		}),
		ev("Board games", func(e *event.Event) { e.Social = event.SocialMedium }),
	}
	picks, f := Recommend(events, afternoon, tue(16, 5), prefs(Easy))

	if f.WrongVibe != 1 {
		t.Errorf("the mixer should be filtered out on an easy day; funnel = %+v", f)
	}
	if len(picks) != 2 || picks[0].Event.Title != "Lecture" {
		t.Fatalf("want Lecture first, got %v", titles(picks))
	}
	why := strings.Join(picks[0].Why, "; ")
	for _, want := range []string{"expected to talk", "free"} {
		if !strings.Contains(why, want) {
			t.Errorf("reasons %q should mention %q", why, want)
		}
	}
}

func TestSocialMoodPrefersHighSocial(t *testing.T) {
	events := []event.Event{
		ev("Lecture", func(e *event.Event) { e.Social = event.SocialLow }),
		ev("Mixer", func(e *event.Event) { e.Social = event.SocialHigh }),
	}
	picks, _ := Recommend(events, afternoon, tue(16, 5), prefs(Social))
	if len(picks) == 0 || picks[0].Event.Title != "Mixer" {
		t.Fatalf("want Mixer first, got %v", titles(picks))
	}
}

// The honesty rule: we never claim "you can sit and listen" unless the event
// is actually known to be low-social. Unknown gets a heads-up instead.
func TestUnknownSocialIsNeverDescribedAsEasy(t *testing.T) {
	picks, _ := Recommend([]event.Event{ev("Mystery")}, afternoon, tue(16, 5), prefs(Easy))
	if len(picks) != 1 {
		t.Fatalf("unknown social level should still be allowed, got %d picks", len(picks))
	}
	if strings.Contains(strings.Join(picks[0].Why, " "), "expected to talk") {
		t.Error("claimed low interaction without knowing")
	}
	if !strings.Contains(strings.Join(picks[0].HeadsUp, " "), "don't know how social") {
		t.Errorf("expected a heads-up about unknown social level, got %v", picks[0].HeadsUp)
	}
}

func TestFunnelCountsEachFilter(t *testing.T) {
	events := []event.Event{
		ev("Cancelled", func(e *event.Event) { e.Cancelled = true }),
		ev("Nowhere", func(e *event.Event) { e.Venue.Location = nil }),
		ev("Far", func(e *event.Event) { e.Venue.Location = far }),
		ev("Pricey", func(e *event.Event) { e.PriceCents = cents(3500) }),
		ev("Tonight", func(e *event.Event) { e.Start, e.End = tue(21, 0), tue(22, 0) }),
		ev("Keeper"),
	}
	picks, f := Recommend(events, afternoon, tue(16, 5), prefs(Surprise))
	want := Funnel{Total: 6, Cancelled: 1, NoLocation: 1, TooFar: 1, TooExpensive: 1, NoTimeFit: 1, Left: 1}
	if f != want {
		t.Fatalf("funnel = %+v\n  want %+v", f, want)
	}
	if picks[0].Event.Title != "Keeper" {
		t.Fatalf("got %v", titles(picks))
	}
}

func TestUnknownPriceIsAllowedButFlagged(t *testing.T) {
	picks, _ := Recommend([]event.Event{ev("No price", func(e *event.Event) { e.PriceCents = nil })},
		afternoon, tue(16, 5), prefs(Surprise))
	if len(picks) != 1 {
		t.Fatal("unknown price should not be filtered out")
	}
	if !strings.Contains(strings.Join(picks[0].HeadsUp, " "), "price isn't listed") {
		t.Errorf("want a price heads-up, got %v", picks[0].HeadsUp)
	}
}

func TestSameInputSameOrder(t *testing.T) {
	events := []event.Event{ev("B"), ev("A"), ev("C")} // identical except title
	first, _ := Recommend(events, afternoon, tue(16, 5), prefs(Surprise))
	for i := 0; i < 5; i++ {
		again, _ := Recommend(events, afternoon, tue(16, 5), prefs(Surprise))
		if strings.Join(titles(again), ",") != strings.Join(titles(first), ",") {
			t.Fatal("ranking is not deterministic")
		}
	}
	if strings.Join(titles(first), ",") != "A,B,C" {
		t.Errorf("ties should break by title, got %v", titles(first))
	}
}

func titles(ps []Pick) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Event.Title)
	}
	return out
}
