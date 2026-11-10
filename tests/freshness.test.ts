import assert from "node:assert/strict"
import test from "node:test"
import type { Dashboard } from "../src/types.ts"
import {
  CLOCK_SKEW_MS,
  freshnessAt,
  STALE_AFTER_MS,
  syncStateFor,
} from "../src/dashboard/freshness.ts"

const now = Date.parse("2026-01-02T00:00:00Z")
const recent = "2026-01-01T23:00:00Z"
const t = (key: string) => key

function meta(overrides: Partial<Dashboard["meta"]> = {}): Dashboard["meta"] {
  return {
    fetchedAt: recent,
    lastSync: { status: "success", startedAt: recent, finishedAt: recent },
    sourceFetchedAt: {
      x: recent,
      youtube: recent,
      bilibili: recent,
      shop: recent,
    },
    postWindowDays: 3,
    xAccounts: [],
    featuredVideoId: null,
    ...overrides,
  }
}

test("freshness ages locally at the inclusive 24-hour boundary", () => {
  assert.equal(freshnessAt("2026-01-01T00:00:00Z", now), "stale")
  assert.equal(
    freshnessAt(new Date(now - STALE_AFTER_MS + 1).toISOString(), now),
    "fresh",
  )
  const snapshot = meta()
  const current = syncStateFor(snapshot, null, now, t)
  assert.equal(current.label, "sync.current")
  assert.equal(current.tone, "leaf")
  const aged = syncStateFor(snapshot, null, now + STALE_AFTER_MS, t)
  assert.equal(aged.label, "sync.stale")
  assert.equal(aged.tone, "honey")
  assert.deepEqual(
    aged.sources.map(({ state }) => state),
    ["stale", "stale", "stale", "stale"],
  )
})

test("one out-of-date source marks the overall state; an unread one does not", () => {
  const state = syncStateFor(
    meta({
      sourceFetchedAt: {
        x: "2025-12-30T00:00:00Z",
        youtube: recent,
        bilibili: null,
        shop: recent,
      },
    }),
    null,
    now,
    t,
  )
  assert.equal(state.label, "sync.stale")
  assert.equal(state.tone, "honey")
  assert.deepEqual(
    state.sources.map(({ source, state }) => [source, state]),
    [
      ["x", "stale"],
      ["youtube", "fresh"],
      ["bilibili", "waiting"],
      ["shop", "fresh"],
    ],
  )
  const unread = syncStateFor(
    meta({ sourceFetchedAt: { x: recent, youtube: recent, shop: recent } }),
    null,
    now,
    t,
  )
  assert.equal(unread.label, "sync.current")
})

test("missing, invalid and future timestamps never look current", () => {
  for (const timestamp of [
    null,
    undefined,
    "invalid",
    "2026-01-03T00:00:00Z",
  ]) {
    assert.equal(freshnessAt(timestamp, now), "waiting")
  }
  const invalid = syncStateFor(
    meta({
      sourceFetchedAt: { x: "invalid", youtube: "2026-01-03T00:00:00Z" },
    }),
    null,
    now,
    t,
  )
  assert.deepEqual(
    invalid.sources.map(({ timestamp, state }) => [timestamp, state]),
    [
      [null, "waiting"],
      [null, "waiting"],
      [null, "waiting"],
      [null, "waiting"],
    ],
  )
  assert.equal(invalid.label, "sync.waiting")
  assert.equal(invalid.tone, "muted")
  const empty = syncStateFor(meta({ sourceFetchedAt: undefined }), null, now, t)
  assert.equal(empty.label, "sync.waiting")
  assert.deepEqual(
    empty.sources.map(({ source, timestamp }) => [source, timestamp]),
    [
      ["x", null],
      ["youtube", null],
      ["bilibili", null],
      ["shop", null],
    ],
  )
})

test("a visitor clock slightly behind the server still shows fresh reads", () => {
  const ahead = new Date(now + 2 * 60 * 1000).toISOString()
  assert.equal(freshnessAt(ahead, now), "fresh")
  assert.equal(
    freshnessAt(new Date(now + CLOCK_SKEW_MS).toISOString(), now),
    "fresh",
  )
  assert.equal(
    freshnessAt(new Date(now + CLOCK_SKEW_MS + 1).toISOString(), now),
    "waiting",
  )
  const skewed = syncStateFor(
    meta({
      sourceFetchedAt: {
        x: ahead,
        youtube: ahead,
        bilibili: ahead,
        shop: ahead,
      },
    }),
    null,
    now,
    t,
  )
  assert.equal(skewed.label, "sync.current")
  assert.deepEqual(
    skewed.sources.map(({ timestamp }) => timestamp),
    [ahead, ahead, ahead, ahead],
  )
})

test("a failed or partial run stays operator diagnosis while sources are fresh", () => {
  for (const status of ["failed", "partial"]) {
    const state = syncStateFor(
      meta({ lastSync: { status, startedAt: recent, finishedAt: recent } }),
      null,
      now,
      t,
    )
    assert.equal(state.label, "sync.current")
    assert.equal(state.tone, "leaf")
  }
  const never = syncStateFor(meta({ lastSync: null }), null, now, t)
  assert.equal(never.label, "sync.current")
})

test("an unreachable API takes precedence and keeps the source times", () => {
  const offline = syncStateFor(meta(), new Error("offline"), now, t)
  assert.equal(offline.label, "sync.apiUnavailable")
  assert.equal(offline.tone, "danger")
  assert.equal(offline.sources[0].timestamp, recent)
})
