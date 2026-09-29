# Decisions — v0.1

Every choice in this codebase, why it was made, what it costs, and when to rethink it. You own these now. If you disagree with one, that's a good sign — change it and update this file.

Each entry has the same four parts:

- **Decision**: what we did.
- **Why**: the reasoning.
- **Cost**: what we gave up.
- **Revisit when**: the signal that it's time to change it.

---

## Scope

### D1. Build the engine before the collectors

**Decision.** v0.1 is the "brain" only: free-time calculation, the fit check, ranking, explanations, and a terminal loop. Events come from a hand-written JSON file. There is no scraping and no database connection yet.

**Why.** Everything else feeds this part. If the fit logic is wrong, perfect data is useless. Sample data also lets us create the awkward cases on purpose (a cancelled event, a missing location, an event just after your free time) instead of waiting for real feeds to produce them.

**Cost.** The recommendations aren't real yet.

**Revisit when.** Now. This is v0.2 (see "What's next" at the bottom).

### D2. Standard library only, no external packages

**Decision.** v0.1 imports nothing outside Go's standard library.

**Why.** You can clone it and run `go test ./...` with nothing to install or break. It also means every line you read is either ours or Go's.

**Cost.** None yet. The database driver (pgx) will be the first dependency in v0.2.

**Revisit when.** v0.2.

### D3. Folder layout: `cmd/` for programs, `internal/` for logic

**Decision.**

```
cmd/onething/        the terminal program (reads files, prints text)
internal/event/      the normalized Event type
internal/geo/        distance and walking time
internal/schedule/   weekly schedule → free blocks
internal/fit/        can I get there, be there, and get back?
internal/rank/       filters, scores, explanations
db/migrations/       SQL schema
sample/              fake events and a sample person
```

Dependencies only point one way: `rank` uses `fit`, `fit` uses `schedule` and `event`, and nothing in `internal/` knows about files, flags or the terminal.

**Why.** When the REST API arrives, it will call `rank.Recommend` exactly the way the terminal program does. Only `cmd/` changes. Go also enforces `internal/`: code outside this module can't import it.

**Cost.** More folders than a single-file program.

**Revisit when.** A package grows past about 500 lines, or two packages keep needing each other.

---

## Data and honesty

### D4. "Unknown" is a real value, not a missing one

**Decision.** Yes/no facts use a three-way `Tri` type (Yes / No / Unknown), with Unknown as the default. Price is `*int`, where nil means "not listed" and 0 means "free". A venue's location is `*geo.Point`, where nil means "not geocoded yet".

**Why.** Most sources never say whether you can walk in late, or how social an event is. A plain true/false would force us to guess, and then the explanation might say "no registration needed" when we don't actually know that. The product's promise is fewer surprises. That requires never pretending to know something.

**Cost.** Every place that reads these fields has to handle three cases instead of two.

**Revisit when.** Never, really. This is a core principle.

### D5. Money is stored in whole cents

**Decision.** `PriceCents int`, and `price_cents integer` in SQL.

**Why.** Decimal amounts like 0.10 can't be stored exactly as floating-point numbers, so adding prices slowly drifts. Integers in cents can't drift.

**Cost.** You convert when displaying (divide by 100).

**Revisit when.** Never.

### D6. How time is handled

**Decision.**

- Events and free blocks are `time.Time` values: exact moments in time.
- The weekly schedule is written in clock time ("14:30 on Tuesdays"). It is turned into exact moments only once we know the date.
- The database uses `timestamptz`, which stores an exact moment.
- The time zone database is built into the program (`_ "time/tzdata"`), so `America/Halifax` works on any machine.

**Why.** Daylight saving time ends at 2:00 AM on November 1, 2026. That night, midnight to 4 AM on the clock is really five hours long. Subtracting exact moments gets this right automatically. Subtracting clock readings would be off by an hour. The test `TestFreeTimeAcrossDaylightSavingChange` proves this.

**Cost.** Overnight commitments (for example, a shift from 22:00 to 02:00) are rejected for now.

**Revisit when.** Someone with a night shift uses it.

---

## The fit check (`internal/fit`)

### D7. Every trip starts and ends at one home base

**Decision.** The plan is: leave home, walk to the event, attend, walk home, and be back before the free block ends.

**Why.** It's the simplest model that is always safe. If your next class is actually on the way, we left you extra time, not too little.

**Cost.** It's conservative. A 6:00 PM event near campus, before a 7:30 PM class on campus, might get rejected even though it would work.

**Revisit when.** You add locations to commitments. The fit check would then use "where am I coming from" and "where am I going next" instead of home.

### D8. Walking only, estimated from straight-line distance

**Decision.** Walking time = straight-line distance × 1.3 ÷ 80 metres per minute, rounded **up** to the next whole minute.

**Why.** It needs no API key, no network and no cost, and it's close enough on the Halifax peninsula. 80 m/min is a relaxed pace. Rounding up means we err toward you arriving early.

**Cost.** It's wrong across water (Dartmouth), wrong on steep Citadel Hill streets, and there's no transit.

**Revisit when.** Users go beyond the peninsula. The next step would be Halifax Transit's schedule data. Only `geo.WalkTime` needs replacing.

### D9. Buffers, arriving late, leaving early, and the 50% rule

**Decision.**

- The buffer applies at both ends of the trip: time to get ready before leaving, and spare time before your next commitment.
- Arriving late or leaving early is allowed **only** if the event says it's fine (`Yes`). `Unknown` is treated as No.
- Even then, it must be within your own limits (`max_late_minutes`, `max_leave_early_minutes`).
- If you'd miss more than half the event, it's not suggested (`min_attend_fraction`).

**Why.** Walking into a lecture late is exactly the kind of moment that makes someone decide not to go. With unknown information we choose to suggest less rather than create an awkward situation.

**Cost.** Real events with unknown late-entry rules will only be suggested if you can arrive on time.

**Revisit when.** Collectors can tag drop-in events (library drop-ins, markets) as `late_entry_ok: yes`. That tagging rule belongs in a collector, not here.

---

## Ranking (`internal/rank`)

### D10. Filter first, then score

**Decision.** Hard rules remove events first, cheapest checks first: cancelled → no location → too far → too expensive → wrong vibe → doesn't fit your time. Only the survivors get a score. The counts are kept in a `Funnel`; the `-why-not` flag shows them.

**Why.** It's the funnel from the brief, made visible. When you're asking "why didn't it show me X?", the funnel answers in one line.

- An **unknown price** passes the filter but gets a lower score and a heads-up. Most library events don't list a price, and dropping them all would lose your best source.
- An **unknown location** is dropped, because we can't check time or distance without it.

**Cost.** Filters are hard cutoffs. An event 31 minutes away is gone even if it's perfect.

**Revisit when.** People say "I'd have walked 5 more minutes for that."

### D11. Hand-picked weights, and the explanation comes from the score itself

**Decision.** The score is a weighted average of seven parts, each between 0 and 1: walk, cost, interest, registration, social, how soon you'd leave, and outdoor. Each mood changes the weights. The social table decides which event social levels each mood allows. Each part carries its own sentence, so "Why this one" lists the strongest parts that actually contributed.

**Why.** The explanation can't disagree with the ranking, because both come from the same numbers. If a pick looks wrong, you can find the exact weight responsible.

**Cost.** The numbers are judgement calls. They are written down in `weightsFor` and `socialTable` so they can be argued with.

**Revisit when.** You've used it for real and a pick felt wrong. Change one number, add a test for that case, and repeat.

### D12. The honesty rule for explanations

**Decision.** A reason is only shown if it's backed by known data. For example, "you won't be expected to talk to anyone" appears only when the event's social level is `low`, never when it's `unknown`. Anything we're unsure about goes in "Heads up" instead.

**Why.** One false reassurance ("it said I wouldn't have to talk to anyone") costs more trust than ten accurate suggestions earn. `TestUnknownSocialIsNeverDescribedAsEasy` guards this rule.

**Cost.** Explanations are shorter when data is thin.

**Revisit when.** Never. Only the data should get better.

### D13. Same input, same answer

**Decision.** Ties are broken by start time, then by title. There's no randomness, and "Surprise me" just turns off the interest and social weights.

**Why.** Asking twice and getting a different answer feels arbitrary, and it makes bugs impossible to reproduce.

**Cost.** "Surprise" isn't very surprising.

**Revisit when.** You want variety. The fix is randomness you can repeat on purpose (for example, choosing based on today's date), not truly random picks.

### D14. At most three picks, and "Not today" does nothing

**Decision.** "Show me something else" stops after three picks. "Not today" prints "No problem." and exits. Nothing is recorded.

**Why.** Both come straight from the brief: fewer, better options, and no penalty for saying no.

**Cost.** None.

**Revisit when.** Never for "Not today". If you ever record "I'll go" to improve suggestions, keep it optional and never show it back as a count.

### D15. Today's mood is never stored

**Decision.** Mood is a command-line flag for this one run only.

**Why.** It's a temporary preference, not something to build a profile from.

**Cost.** None.

**Revisit when.** Only if the person asks for a default mood. Even then, store their chosen default, not a history of moods.

---

## Storage and config

### D16. Schema choices (`db/migrations/0001_init.sql`)

- **Venues are their own table.** Many events share a venue. Geocode it once and every event benefits.
- **An event's identity is `(source_id, source_event_id)`.** Collecting the same feed twice updates the existing row instead of adding a duplicate. This is called an "upsert". Duplicates *across* sources (the same talk on Dal's calendar and on Eventbrite) are a separate, harder problem, saved for later.
- **Fixed value lists use `text` + `CHECK`, not Postgres `ENUM` types.** Adding a value later is a one-line constraint change.
- **`content_hash` and `last_seen_at`.** These let a collector tell "unchanged" from "updated", and notice an event that quietly disappeared from its source (probably cancelled).
- **A partial index on `starts_at` covering only non-cancelled events.** It is shaped for the query the app will run most often.
- **Numbered migrations that only move forward.** Once `0001` has run anywhere, it's never edited; changes go in `0002_...`.

The schema was applied to a real PostgreSQL 16. Tests confirmed that it rejects backwards times, invalid social levels, venues with only one coordinate, and events from unknown sources.

### D17. Settings live in one JSON file, with no accounts

**Decision.** `sample/me.json` holds your home location, limits and weekly schedule. Unknown field names are rejected, so a typo like `max_walk_minute` is an error instead of a setting that silently does nothing.

**Why.** The brief says there are no accounts until they're needed. One person, one file.

**Cost.** One user per copy of the file.

**Revisit when.** You build the web frontend and a second person wants to use it.

---

## Open questions — yours to decide

1. **What does "Just get me outside" mean?** Out of the house, or outdoors? Right now it means mostly out of the house, with a small outdoor bonus. In the sample data, the indoor lecture beats the garden walk on that mood. Is that what you want?
2. **Should events with an unknown social level appear on "Something easy" days?** Right now they're allowed, but score low and come with a heads-up.
3. **Do the weights feel right?** Use it for a week of real days, then decide.
4. **The name.** `onething` is a placeholder. To rename, change the first line of `go.mod` and the `onething/...` import paths.

## What's next (v0.2)

1. **First real collector:** Dalhousie Libraries' LibCal iCal feed. It's a single URL with a documented format, which makes it the easiest real source.
2. **Store events in Postgres** with an upsert on `(source_id, source_event_id)`.
3. **Change the program** to read today's events from the database instead of `sample/events.json`.

Note: the current release of pgx (the Postgres driver) requires Go 1.25 or newer, so install a recent Go before starting v0.2.
