import type { Dashboard, Translator } from "../types.ts"

// A local warning threshold, independent of the server's synchronization timer.
export const STALE_AFTER_MS = 24 * 60 * 60 * 1000
// A visitor clock a little behind the server still shows a fresh read; a
// timestamp further ahead is not trusted.
export const CLOCK_SKEW_MS = 5 * 60 * 1000

export const SYNC_SOURCES = ["x", "youtube", "bilibili", "shop"] as const
export type SyncSource = (typeof SYNC_SOURCES)[number]

type Freshness = "fresh" | "stale" | "waiting"

export interface SyncState {
  tone: "leaf" | "honey" | "danger" | "muted"
  label: string
  sources: {
    source: SyncSource
    timestamp: string | null
    state: Freshness
  }[]
}

export function freshnessAt(
  timestamp: string | null | undefined,
  now: number,
): Freshness {
  const at = Date.parse(timestamp || "")
  if (!Number.isFinite(at) || at > now + CLOCK_SKEW_MS) return "waiting"
  return now - at >= STALE_AFTER_MS ? "stale" : "fresh"
}

/**
 * The overall state is what a visitor can tell from the page: whether the API
 * answers and how old the sources' last successful reads are. A run's outcome
 * is operator diagnosis; a failing source shows here once its read ages out.
 * Recomputed from the retained snapshot when the local clock changes.
 */
export function syncStateFor(
  meta: Dashboard["meta"],
  error: unknown,
  now: number,
  t: Translator,
): SyncState {
  const sources = SYNC_SOURCES.map((source) => {
    const timestamp = meta.sourceFetchedAt?.[source] ?? null
    const state = freshnessAt(timestamp, now)
    return { source, timestamp: state === "waiting" ? null : timestamp, state }
  })
  let tone: SyncState["tone"] = "leaf"
  let key = "sync.current"
  if (error) {
    tone = "danger"
    key = "sync.apiUnavailable"
  } else if (sources.every(({ state }) => state === "waiting")) {
    tone = "muted"
    key = "sync.waiting"
  } else if (sources.some(({ state }) => state === "stale")) {
    tone = "honey"
    key = "sync.stale"
  }
  return { tone, label: t(key), sources }
}
