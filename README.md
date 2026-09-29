# onething — v0.1

A Halifax event finder whose job is to help you decide, not to help you browse. You tell it you're free and what you're up for. It finds **one** realistic thing that fits your day, and tells you why it picked that one.

v0.1 is the engine only: free-time calculation, the "can I actually make it?" check, ranking, and explanations. The events are sample data. Real feeds come next (see `DECISIONS.md` → "What's next").

## Run it

You need Go 1.24 or newer ([go.dev/dl](https://go.dev/dl)). Nothing else.

```sh
go test ./...
go run ./cmd/onething -now "2026-09-29 16:05" -mood easy -why-not
```

`-now` pretends it's that time, because the sample events are on Tuesday, September 29, 2026. `-mood` can be `outside`, `easy`, `people`, `social` or `surprise`. `-why-not` shows how many events each filter removed.

What you'll see:

```
Looked at 13 events:
  - 1 cancelled
  - 1 no known location yet
  - 1 too far to walk
  - 2 over your price limit
  - 2 not the vibe you asked for
  - 2 don't fit your free time
  = 4 left to choose from

You have about 4 hours free (4:05 PM – 8:00 PM).

  Public Lecture: Mapping the Ocean Floor
  Killam Memorial Library · 8-minute walk
  5:30 PM – 6:30 PM (an hour) · leave by 5:22 PM, home by 6:38 PM

  Why this one: you won't be expected to talk to anyone, it's 8 minutes away on foot, it's free and it matches your interest in science.

  [1] I'll go   [2] Show me something else   [3] Not today
```

Things to try:

- `-mood social`: the answer changes to the board games.
- `-now "2026-09-29 19:40"`: too late for anything, so it says so, calmly.
- Edit `sample/me.json` (your home, schedule and limits) and see what changes.

## How a request flows

```
me.json ──► schedule.FreeBlocks ──► schedule.FromNow ──┐
                                                       ▼
events.json ─────────────────────────────────► rank.Recommend
                                                 │ for each event:
                                                 │   filters (cancelled, location, walk, price, vibe)
                                                 │   fit.FirstFit  (travel + buffers vs. your free time)
                                                 │   score + reasons
                                                 ▼
                                   best-first list ──► show 1, up to 3
```

## Reading order

If you read the code, go in this order. Each file only depends on the ones before it.

1. `internal/event/event.go`: what an event *is*. Note the `Tri` type (yes / no / unknown).
2. `internal/geo/geo.go`: about 50 lines. Distance and walking time.
3. `internal/schedule/schedule.go`: turns a weekly schedule into today's free blocks.
4. `internal/fit/fit.go`: **the heart of the app.** Read `Check` slowly.
5. `internal/fit/fit_test.go`: the brief's own example, as a test, plus every edge case.
6. `internal/rank/rank.go`: filters, weights, and where the "Why this one" sentences come from.
7. `cmd/onething/`: file reading and terminal printing. Deliberately boring.
8. `db/migrations/0001_init.sql`: the database shape for v0.2.

Then read **`DECISIONS.md`**. It explains every choice above, and lists the open questions that are yours to answer.

## Go ideas used in this code

A short guide to what you'll run into.

- **Packages and folders.** Each folder is one package. Names starting with a capital letter (`Check`, `Event`) are visible to other packages; lowercase names (`noFit`, `part`) are private to their package.
- **`internal/`.** Go won't let code outside this project import these packages.
- **Structs and methods.** `func (b Block) Duration()` attaches a method to `Block`. The `(b Block)` part is called the receiver.
- **Zero values.** Every type has a default: `0`, `""`, `false`, `nil`. `Tri`'s zero value is `Unknown` on purpose, so an event starts as "we don't know" everywhere.
- **`iota`.** Numbers constants 0, 1, 2… in a `const` block. It's how the enums (`Tri`, `Social`, `Mood`) are made.
- **Pointers as "maybe".** `*int` is either `nil` (not set) or points at a value. It's used for price and location.
- **Errors are values.** Functions return `(result, error)`, and callers check `if err != nil`. Nothing is thrown.
- **Interfaces are automatic.** Any type with an `UnmarshalText` method counts as a text unmarshaler, and `encoding/json` will use it. That's how `"yes"` in JSON becomes `event.Yes`.
- **`defer`.** Runs a line when the function returns, whichever way it returns. It's used to close files.
- **Table-driven tests.** A slice of cases plus one loop with `t.Run`. Adding a case is one more entry in the slice.
- **`_ "time/tzdata"`.** An import used only for its side effect (embedding the time zone data), not for any names.

## Try the schema

If you have PostgreSQL installed:

```sh
createdb onething
psql -d onething -f db/migrations/0001_init.sql
```

## About the sample data

The venues and their rough coordinates are real Halifax places. **The events are made up**, for testing. The "Harbourside Clay Studio" venue is invented too.
