import type { Dashboard, LiveNow, ScheduleEvent, Video } from "../types.ts"
import { streamIdentity } from "../lib/stream-identity.ts"
import {
  confirmedLiveStatus,
  eventIsLive,
  eventIsSale,
  eventIsUpcoming,
  eventStatus,
  LIVE_GRACE_MS,
  sortEvents,
  scheduleSortTime,
} from "./format.ts"
import { videoIsGone, videoIsShort } from "./video-groups.ts"

/** How close a later reservation must be to replace a presumed-live item. */
export const LIVE_YIELD_MS = 30 * 60 * 1000

/**
 * The operator's featured video. A snapshot from before the setting existed
 * names it through the focus copy instead.
 */
export function featuredVideoIdOf(snapshot: Dashboard): string | null {
  return Object.hasOwn(snapshot.meta, "featuredVideoId")
    ? snapshot.meta.featuredVideoId
    : (snapshot.focus?.videoId ?? null)
}

/**
 * The first of start-ordered focus candidates. A started candidate without an
 * explicit end or a confirmed live state is only presumed live, so it yields
 * to a later timed candidate that has started or starts within LIVE_YIELD_MS.
 */
export function leadingCandidate<T>(
  candidates: T[],
  now: number,
  startOf: (candidate: T) => number,
  endOf: (candidate: T) => number,
  confirmedLive: (candidate: T) => boolean = () => false,
): T | null {
  const lead = candidates.find((candidate, index) => {
    const start = startOf(candidate)
    if (!(start <= now) || Number.isFinite(endOf(candidate))) return true
    if (confirmedLive(candidate)) return true
    return !candidates.slice(index + 1).some((later) => {
      const next = startOf(later)
      return next > start && next - now <= LIVE_YIELD_MS
    })
  })
  return lead ?? null
}

/**
 * How early before an entry's start its broadcast may open, and how late
 * after it the broadcast may start.
 */
export const LIVE_NOW_EARLY_MS = 2 * 3600 * 1000
export const LIVE_NOW_LATE_MS = 3 * 3600 * 1000

/**
 * Whether an entry is the unannounced broadcast's own: a live stream or
 * membership entry shown on that platform whose broadcast started from two
 * hours before to three hours after the entry's start. Links never decide
 * it, since an entry links to its source post.
 */
export function eventMatchesLiveNow(event: ScheduleEvent, liveNow: LiveNow) {
  const platforms: string[] = event.platforms?.length
    ? event.platforms
    : [event.platform || ""]
  if (!platforms.includes(liveNow.platform)) return false
  if (!["stream", "member"].includes(event.eventType)) return false
  if (confirmedLiveStatus(event) !== "live") return false
  const start = Date.parse(event.startsAt || "")
  const started = Date.parse(liveNow.startedAt)
  return (
    started >= start - LIVE_NOW_EARLY_MS && started <= start + LIVE_NOW_LATE_MS
  )
}

/** The live-now item, unless an entry already shows that broadcast. */
export function visibleLiveNow(
  liveNow: LiveNow | null | undefined,
  events: ScheduleEvent[],
): LiveNow | null {
  if (!liveNow?.url) return null
  return events.some((event) => eventMatchesLiveNow(event, liveNow))
    ? null
    : liveNow
}

export function videoIsInFocus(video: Video, now: number) {
  if (video.isCancelled) return false
  const start = Date.parse(video.scheduledAt || "")
  const end = Date.parse(video.endsAt || "")
  const confirmed = confirmedLiveStatus(video)
  if (confirmed === "live") return !(end <= now)
  if (confirmed === "ended") return false
  if (Number.isFinite(end)) return end > now && Number.isFinite(start)
  return Number.isFinite(start)
    ? start > now - LIVE_GRACE_MS
    : Boolean(video.isUpcoming)
}

/** Recompute time-dependent state from the local snapshot, without network work. */
export function deriveDashboardAt(snapshot: Dashboard, now: number): Dashboard {
  const events = snapshot.events.map((event) => ({
    ...event,
    isUpcoming: eventIsUpcoming(event, now),
  }))
  const streamEnds = new Map<string, string>()
  for (const event of events) {
    if (event.url && event.endsAt && Number.isFinite(Date.parse(event.endsAt)))
      streamEnds.set(streamIdentity(event.url), event.endsAt)
  }
  const videos = snapshot.videos.map((video) => ({
    ...video,
    endsAt: streamEnds.get(streamIdentity(video.url)) || video.endsAt || null,
    // A waiting room past its reserved time is still upcoming, and a
    // confirmed broadcast is not.
    isUpcoming:
      video.liveStatus === "upcoming" ||
      (!confirmedLiveStatus(video) &&
        (video.scheduledAt
          ? Date.parse(video.scheduledAt) >= now
          : Boolean(video.isUpcoming))),
  }))
  const eligible = events.filter(
    (event) =>
      !["cancelled", "cancellation_review"].includes(eventStatus(event, now)),
  )
  const upcomingEvents = eligible.filter((event) => event.isUpcoming)
  // A sale period is never the next event: the stage shows activities, and
  // an order window lasting weeks would hold it.
  const focusEvents = sortEvents(
    eligible.filter(
      (event) =>
        !eventIsSale(event) && (event.isUpcoming || eventIsLive(event, now)),
    ),
  )
  const focusVideos = videos
    .filter((video) => videoIsInFocus(video, now))
    .sort(
      (a, b) =>
        scheduleSortTime(a.scheduledAt) - scheduleSortTime(b.scheduledAt) ||
        a.id.localeCompare(b.id),
    )
  // The stage falls back to the newest upload that is not a Short, unless
  // the operator featured that Short, and that visitors can still watch.
  const featuredId = featuredVideoIdOf(snapshot)
  const latestVideos = videos
    .filter(
      (video) =>
        !video.isUpcoming &&
        (!videoIsShort(video) || video.id === featuredId) &&
        !videoIsGone(video),
    )
    .sort(
      (a, b) =>
        scheduleSortTime(b.scheduledAt || b.publishedAt) -
          scheduleSortTime(a.scheduledAt || a.publishedAt) ||
        a.id.localeCompare(b.id),
    )
  const nextEvent = snapshot.summary.nextEvent
  return {
    ...snapshot,
    events,
    videos,
    liveNow: visibleLiveNow(
      snapshot.liveNow,
      nextEvent ? [...events, nextEvent] : events,
    ),
    summary: {
      ...snapshot.summary,
      nextEvent: leadingCandidate(
        focusEvents,
        now,
        (event) => Date.parse(event.startsAt || ""),
        (event) => Date.parse(event.endsAt || ""),
        (event) => confirmedLiveStatus(event) === "live",
      ),
      nextStream: leadingCandidate(
        focusVideos,
        now,
        (video) => Date.parse(video.scheduledAt || ""),
        (video) => Date.parse(video.endsAt || ""),
        (video) => confirmedLiveStatus(video) === "live",
      ),
      latestVideo: latestVideos[0] || null,
      counts: snapshot.summary.counts
        ? {
            ...snapshot.summary.counts,
            upcomingEvents: snapshot.meta.eventWindow
              ? Math.max(
                  0,
                  snapshot.summary.counts.upcomingEvents +
                    upcomingEvents.length -
                    snapshot.events.filter(
                      (event) =>
                        event.isUpcoming &&
                        !["cancelled", "cancellation_review"].includes(
                          eventStatus(event, now),
                        ),
                    ).length,
                )
              : upcomingEvents.length,
          }
        : null,
    },
  }
}
