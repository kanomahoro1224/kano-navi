import type { Locale, Video } from "../types.ts"
import {
  confirmedLiveStatus,
  LIVE_GRACE_MS,
  scheduleSortTime,
  videoTag,
  videoTime,
} from "./format.ts"

/** The board's video filter. */
export type VideoFilter = "all" | "streams" | "works" | "shorts"

/**
 * The video archive's filter groups. The API derives a video's group from its
 * calendar label and kind; the board only names them.
 */
export const VIDEO_CATEGORIES = ["game", "talk", "song", "other"] as const
export type VideoCategory = (typeof VIDEO_CATEGORIES)[number]

/**
 * Where a video stands now. `started` is a reservation past its start that no
 * watch page has confirmed yet, so it is neither on air nor an archive.
 */
export type VideoPhase = "upcoming" | "live" | "started" | "past" | "cancelled"

/**
 * A broadcast or its archive, rather than a premiere or an upload. Only the
 * form decides; what the video is about is its tag.
 */
export function videoIsStream(video: Video) {
  return video.kind === "stream"
}

/**
 * A YouTube Short, as the feed's link or an operator names it. An unknown
 * format, as for older uploads, is never guessed from the length.
 */
export function videoIsShort(video: Video) {
  return video.isShort === true
}

/**
 * Whether visitors can no longer watch a video on its own platform: it is
 * private or removed. Unknown availability is still watchable.
 */
export function videoIsGone(video: Video) {
  return video.availability === "private" || video.availability === "removed"
}

/**
 * The stage's rule for one video: a fresh confirmed state decides first;
 * without one a stream keeps the three-hour grace after its start and any
 * other video stays current only until an explicit end.
 */
export function videoPhase(video: Video, now: number): VideoPhase {
  // An operator-confirmed cancellation is neither upcoming, live nor archived.
  if (video.isCancelled) return "cancelled"
  const start = Date.parse(video.scheduledAt || "")
  const end = Date.parse(video.endsAt || "")
  const confirmed = confirmedLiveStatus(video)
  if (confirmed === "live") return end <= now ? "past" : "live"
  if (confirmed === "upcoming") return "upcoming"
  if (confirmed === "ended") return "past"
  if (!Number.isFinite(start)) return video.isUpcoming ? "upcoming" : "past"
  if (start > now) return "upcoming"
  if (Number.isFinite(end)) return end > now ? "started" : "past"
  return videoIsStream(video) && start > now - LIVE_GRACE_MS
    ? "started"
    : "past"
}

/** Whether a work leads the shelf: a song (a cover or original) or a premiere. */
function isLeadWork(video: Video) {
  return videoTag(video) === "song" || video.kind === "premiere"
}

export interface VideoSplit {
  /** Reservations, broadcasts on air and cancelled reservations, soonest first. */
  reservations: Video[]
  /** Uploads other than broadcasts and Shorts, newest first. */
  works: Video[]
  /** Broadcast archives, newest first. */
  archives: Video[]
  /** Shorts, newest first. Not works, so one leads only when featured. */
  shorts: Video[]
}

export interface VideoShelfGroups extends VideoSplit {
  /** The operator's featured video, else the newest cover or premiere. */
  lead: Video | null
  leadIsFeatured: boolean
}

const newestFirst = (a: Video, b: Video) =>
  scheduleSortTime(videoTime(b)) - scheduleSortTime(videoTime(a)) ||
  a.id.localeCompare(b.id)

/** Sorts videos into what is coming, what was uploaded and what aired. */
export function splitVideos(videos: Video[], now: number): VideoSplit {
  const reservations: Video[] = []
  const works: Video[] = []
  const archives: Video[] = []
  const shorts: Video[] = []
  for (const video of videos) {
    const phase = videoPhase(video, now)
    // A cancelled reservation stays listed until its start, then drops out.
    if (phase === "cancelled") {
      if (Date.parse(video.scheduledAt || "") > now) reservations.push(video)
    } else if (phase !== "past") reservations.push(video)
    // A Short's title can read like a stream's, so its format decides first.
    else if (videoIsShort(video)) shorts.push(video)
    else (videoIsStream(video) ? archives : works).push(video)
  }
  reservations.sort(
    (a, b) =>
      scheduleSortTime(a.scheduledAt) - scheduleSortTime(b.scheduledAt) ||
      a.id.localeCompare(b.id),
  )
  works.sort(newestFirst)
  archives.sort(newestFirst)
  shorts.sort(newestFirst)
  return { reservations, works, archives, shorts }
}

/** The shelf's lead and the rest, each video listed once. */
export function groupVideos(
  videos: Video[],
  featuredVideoId: string | null,
  now: number,
): VideoShelfGroups {
  const featured = featuredVideoId
    ? videos.find((video) => video.id === featuredVideoId) || null
    : null
  const split = splitVideos(
    videos.filter((video) => video !== featured),
    now,
  )
  let lead = featured
  if (!lead) {
    // A private or removed work never leads, as it has nothing to open. An
    // unlabelled work leads too: a missing label is no reason to pass over the
    // work that was just published for an older cover.
    const watchable = split.works.filter((video) => !videoIsGone(video))
    lead =
      watchable.find(
        (video) => isLeadWork(video) || videoTag(video) === null,
      ) ||
      watchable[0] ||
      null
    if (lead) split.works.splice(split.works.indexOf(lead), 1)
  }
  return { ...split, lead, leadIsFeatured: Boolean(featured) }
}

/** One platform's page for a video. */
export interface VideoLink {
  source: string
  url: string
}

/**
 * A video's platform links, the one its card opens first: the video itself,
 * then every merged upload of the same work. Chinese readers open a past
 * video on bilibili when a copy is merged; a reservation or broadcast keeps
 * its own platform, the only one that carries it.
 */
export function videoLinks(
  video: Video,
  locale: Locale,
  now: number,
): VideoLink[] {
  // A private or removed video keeps only its other platforms' copies.
  const links = [
    ...(videoIsGone(video) ? [] : [{ source: video.source, url: video.url }]),
    ...(video.alternates || []),
  ]
  if (locale !== "zh-CN" || videoPhase(video, now) !== "past") return links
  const index = links.findIndex((link) => link.source === "bilibili")
  if (index <= 0) return links
  return [links[index], ...links.slice(0, index), ...links.slice(index + 1)]
}

/**
 * Every platform a work is on, for its cover mark: the platforms of its links
 * in their order, led by its own platform when a private or removed video
 * left it without a link there.
 */
export function videoPlatforms(video: Video, links: VideoLink[]): string[] {
  const sources = links.map((link) => link.source)
  return videoIsGone(video) ? [video.source, ...sources] : sources
}

/**
 * Whether a video belongs to a filter; `all` keeps every video and a Short
 * belongs to `shorts` alone.
 */
export function videoMatchesFilter(video: Video, filter: VideoFilter) {
  if (filter === "all") return true
  if (videoIsShort(video)) return filter === "shorts"
  if (filter === "shorts") return false
  return videoIsStream(video) === (filter === "streams")
}
