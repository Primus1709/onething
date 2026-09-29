-- 0001_init.sql — first version of the database.
--
-- Not used by the Go code yet: v0.2 adds the first collector, which is the
-- first thing that needs to store events. Writing the schema now forces the
-- Event shape to be thought through, and it's checked against a real
-- PostgreSQL (see README: "Try the schema").
--
-- Migrations are numbered files that only ever move forward. To change
-- something later, add 0002_..., never edit this file after it has run.

BEGIN;

-- Where events come from. One row per collector.
CREATE TABLE sources (
    id          text PRIMARY KEY,                -- short stable name, e.g. 'dal-libcal'
    name        text NOT NULL,
    kind        text NOT NULL CHECK (kind IN ('api', 'open_data', 'ical', 'rss', 'html')),
    url         text NOT NULL,
    enabled     boolean NOT NULL DEFAULT true,
    last_run_at timestamptz,                     -- last attempt
    last_ok_at  timestamptz                      -- last success: a gap between these = a broken collector
);

-- Places. Kept separate from events because many events share a venue,
-- and geocoding a venue once is cheaper and more consistent than per event.
CREATE TABLE venues (
    id      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name    text NOT NULL,
    address text NOT NULL DEFAULT '',
    lat     double precision CHECK (lat BETWEEN -90 AND 90),
    lng     double precision CHECK (lng BETWEEN -180 AND 180),
    CHECK ((lat IS NULL) = (lng IS NULL)),       -- both or neither
    UNIQUE (name, address)
);

CREATE TABLE events (
    id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    -- Identity within the source. Re-collecting the same event updates this
    -- row instead of inserting a duplicate (an "upsert" on this pair).
    source_id             text NOT NULL REFERENCES sources (id),
    source_event_id       text NOT NULL,
    source_url            text,
    UNIQUE (source_id, source_event_id),

    title                 text NOT NULL,
    description           text NOT NULL DEFAULT '',

    -- timestamptz stores an absolute instant (internally UTC). Halifax local
    -- time is applied only when displaying. Never use plain "timestamp" here.
    starts_at             timestamptz NOT NULL,
    ends_at               timestamptz NOT NULL,
    end_estimated         boolean NOT NULL DEFAULT false,
    CHECK (ends_at > starts_at),

    venue_id              bigint REFERENCES venues (id),

    -- NULL = the source didn't say. 0 = free. Whole cents, never floats.
    price_cents           integer CHECK (price_cents >= 0),

    categories            text[] NOT NULL DEFAULT '{}',

    -- yes / no / unknown answers. 'unknown' is the honest default.
    registration_required text NOT NULL DEFAULT 'unknown' CHECK (registration_required IN ('yes', 'no', 'unknown')),
    registration_url      text,
    late_entry_ok         text NOT NULL DEFAULT 'unknown' CHECK (late_entry_ok  IN ('yes', 'no', 'unknown')),
    leave_early_ok        text NOT NULL DEFAULT 'unknown' CHECK (leave_early_ok IN ('yes', 'no', 'unknown')),
    outdoor               text NOT NULL DEFAULT 'unknown' CHECK (outdoor        IN ('yes', 'no', 'unknown')),
    social                text NOT NULL DEFAULT 'unknown' CHECK (social IN ('low', 'medium', 'high', 'unknown')),

    cancelled             boolean NOT NULL DEFAULT false,

    -- Change tracking. content_hash is a hash of the fields we care about,
    -- so a collector can tell "unchanged" from "updated" without comparing
    -- every column. If an event stops appearing in its source, last_seen_at
    -- stops moving — that's how we'll notice silent cancellations.
    content_hash          text NOT NULL,
    first_seen_at         timestamptz NOT NULL DEFAULT now(),
    last_seen_at          timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

-- The main query is "upcoming, not cancelled, in this time range".
-- A partial index only covers the rows that query can ever return.
CREATE INDEX events_upcoming_idx ON events (starts_at) WHERE NOT cancelled;

COMMIT;
