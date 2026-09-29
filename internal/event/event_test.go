package event

import (
	"encoding/json"
	"testing"
)

func TestDecodeFromJSON(t *testing.T) {
	raw := `{
		"title": "Drop-in Board Games",
		"start": "2026-09-29T17:30:00-03:00",
		"end":   "2026-09-29T19:30:00-03:00",
		"price_cents": 0,
		"late_entry_ok": "yes",
		"social": "medium"
	}`
	var e Event
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatal(err)
	}
	if e.LateEntryOK != Yes || e.Social != SocialMedium {
		t.Errorf("custom fields decoded wrong: late=%v social=%v", e.LateEntryOK, e.Social)
	}
	if e.LeaveEarlyOK != Unknown {
		t.Errorf("a missing field should stay Unknown, got %v", e.LeaveEarlyOK)
	}
	if !e.IsFree() {
		t.Error("price_cents 0 should be free")
	}
	if e.Duration().Hours() != 2 {
		t.Errorf("duration = %v", e.Duration())
	}
}

func TestMissingPriceIsNotFree(t *testing.T) {
	var e Event
	if e.IsFree() {
		t.Error("unknown price must not count as free")
	}
}

func TestBadValueIsAnError(t *testing.T) {
	var e Event
	if err := json.Unmarshal([]byte(`{"social": "extreme"}`), &e); err == nil {
		t.Error("expected an error for an invalid social level")
	}
}
