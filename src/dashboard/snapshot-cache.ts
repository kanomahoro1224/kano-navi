import { fallbackProfile, POST_WINDOW_DAYS } from "./content"
import type { Dashboard } from "@/types"

/**
 * The projection version this board renders. A snapshot with any other
 * version, or none, is never shown: the board reads a fresh one instead.
 */
const PROJECTION_VERSION = 1

export const emptyDashboard: Dashboard = {
  profile: fallbackProfile,
  projection: {
    version: PROJECTION_VERSION,
    nextTransitionAt: null,
    focus: { eventIds: [], videoIds: [] },
    spotlight: null,
    sales: [],
    shelf: {
      leadId: null,
      leadIsFeatured: false,
      reservations: [],
      works: [],
      archives: [],
      shorts: [],
    },
  },
  summary: {
    nextEvent: null,
    nextStream: null,
    latestVideo: null,
    latestPost: null,
    counts: null,
  },
  posts: [],
  events: [],
  videos: [],
  focus: null,
  timeline: [],
  resources: [],
  assets: [],
  scheduleImages: [],
  liveNow: null,
  meta: {
    fetchedAt: null,
    sourceFetchedAt: { x: null, youtube: null, bilibili: null, shop: null },
    lastSync: null,
    postWindowDays: POST_WINDOW_DAYS,
    xAccounts: ["kano_2525", "_Kanotic"],
    featuredVideoId: null,
  },
}

/** A snapshot payload that carries the projection this board renders. */
export type SupportedSnapshot = Partial<Dashboard> &
  Pick<Dashboard, "projection">

/**
 * The last snapshot this browser read, shown on the next visit until the
 * first read replaces it, when it carries the supported projection.
 */
const SNAPSHOT_CACHE_KEY = "kano-dashboard-snapshot-v1"
/** An older stored snapshot or post archive read is dropped instead of shown. */
export const STORED_READ_MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000

/** Whether a snapshot payload carries the projection this board renders. */
export function hasSupportedProjection(
  payload: Partial<Dashboard> | null | undefined,
): payload is SupportedSnapshot {
  return payload?.projection?.version === PROJECTION_VERSION
}

/** A snapshot payload over the defaults, as the board renders it. */
export function mergeSnapshot(payload: SupportedSnapshot): Dashboard {
  return {
    ...emptyDashboard,
    ...payload,
    profile: { ...fallbackProfile, ...(payload.profile || {}) },
    summary: { ...emptyDashboard.summary, ...(payload.summary || {}) },
    meta: { ...emptyDashboard.meta, ...(payload.meta || {}) },
  }
}

export function readCachedSnapshot(): Dashboard | null {
  try {
    const stored = JSON.parse(
      window.localStorage.getItem(SNAPSHOT_CACHE_KEY) || "null",
    )
    const snapshot = stored?.snapshot
    const age = Date.now() - stored?.savedAt
    if (
      !(age >= 0 && age <= STORED_READ_MAX_AGE_MS) ||
      !snapshot ||
      typeof snapshot !== "object" ||
      !hasSupportedProjection(snapshot) ||
      (["posts", "events", "videos", "timeline", "resources"] as const).some(
        (key) => key in snapshot && !Array.isArray(snapshot[key]),
      )
    )
      return null
    // A stored broadcast may have ended; only a fresh read says it is live.
    return mergeSnapshot({ ...snapshot, liveNow: null })
  } catch {
    return null
  }
}

export function writeCachedSnapshot(snapshot: unknown) {
  try {
    window.localStorage.setItem(
      SNAPSHOT_CACHE_KEY,
      JSON.stringify({ savedAt: Date.now(), snapshot }),
    )
  } catch {
    // Without storage the next visit starts from the placeholder.
  }
}
