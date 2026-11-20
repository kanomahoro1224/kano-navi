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

const states = (value: Dashboard["meta"], at = now) =>
  syncStateFor(value, at).sources.map(({ state }) => state)

test("freshness ages locally at the inclusive 24-hour boundary", () => {
  assert.equal(freshnessAt("2026-01-01T00:00:00Z", now), "stale")
  assert.equal(
    freshnessAt(new Date(now - STALE_AFTER_MS + 1).toISOString(), now),
    "fresh",
  )
  const snapshot = meta()
  assert.deepEqual(states(snapshot), ["fresh", "fresh", "fresh", "fresh"])
  assert.deepEqual(states(snapshot, now + STALE_AFTER_MS), [
    "stale",
    "stale",
    "stale",
    "stale",
  ])
})

test("each source keeps its own freshness", () => {
  const state = syncStateFor(
    meta({
      sourceFetchedAt: {
        x: "2025-12-30T00:00:00Z",
        youtube: recent,
        bilibili: null,
        shop: recent,
      },
    }),
    now,
  )
  assert.deepEqual(
    state.sources.map(({ source, state }) => [source, state]),
    [
      ["x", "stale"],
      ["youtube", "fresh"],
      ["bilibili", "waiting"],
      ["shop", "fresh"],
    ],
  )
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
    now,
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
  const empty = syncStateFor(meta({ sourceFetchedAt: undefined }), now)
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
    now,
  )
  assert.deepEqual(
    skewed.sources.map(({ timestamp }) => timestamp),
    [ahead, ahead, ahead, ahead],
  )
})

test("a failed or partial run stays operator diagnosis while sources are fresh", () => {
  for (const status of ["failed", "partial"]) {
    assert.deepEqual(
      states(
        meta({ lastSync: { status, startedAt: recent, finishedAt: recent } }),
      ),
      ["fresh", "fresh", "fresh", "fresh"],
    )
  }
  assert.deepEqual(states(meta({ lastSync: null })), [
    "fresh",
    "fresh",
    "fresh",
    "fresh",
  ])
})
