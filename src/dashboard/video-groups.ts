import type { DashboardProjection, Locale, Video } from "../types.ts"
import { dateKey, videoTime } from "./format.ts"

/** The board's video filter. */
export type VideoFilter = "all" | "streams" | "works" | "shorts"

/**
 * The video archive's filter groups. The API derives a video's group from its
 * calendar label and kind; the board only names them.
 */
export const VIDEO_CATEGORIES = ["game", "talk", "song", "other"] as const
export type VideoCategory = (typeof VIDEO_CATEGORIES)[number]

/** A video's Japan month, `YYYY-MM`, or "" when it has no time. */
export function videoMonthKey(video: Video) {
  return dateKey(videoTime(video)).slice(0, 7)
}

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

/** Where a video stands, as the server's projection judged it. */
export function videoPhase(video: Video): VideoPhase {
  return video.view.phase
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
  /** The server's lead: the operator's featured video, else its chosen work. */
  lead: Video | null
  leadIsFeatured: boolean
}

const shelfLists = {
  reservation: "reservations",
  work: "works",
  archive: "archives",
  short: "shorts",
} as const

/**
 * Sorts videos into what is coming, what was uploaded and what aired, in the
 * projection's shelf order. A video the shelf lists leave out, such as the
 * lead in a filtered view, joins the end of its server-assigned group; one
 * without a group stays off the shelf.
 */
export function splitVideos(
  videos: Video[],
  projection: DashboardProjection,
): VideoSplit {
  const byId = new Map(videos.map((video) => [video.id, video]))
  const take = (ids: string[]) =>
    ids.flatMap((id) => {
      const video = byId.get(id)
      return video ? [video] : []
    })
  const { shelf } = projection
  const split: VideoSplit = {
    reservations: take(shelf.reservations),
    works: take(shelf.works),
    archives: take(shelf.archives),
    shorts: take(shelf.shorts),
  }
  const listed = new Set(Object.values(split).flat())
  for (const video of videos) {
    const group = video.view.shelfGroup
    if (group && !listed.has(video)) split[shelfLists[group]].push(video)
  }
  return split
}

/** The shelf's lead and the rest, each video listed once. */
export function groupVideos(
  videos: Video[],
  projection: DashboardProjection,
): VideoShelfGroups {
  const { leadId, leadIsFeatured } = projection.shelf
  const lead = leadId
    ? videos.find((video) => video.id === leadId) || null
    : null
  const split = splitVideos(
    videos.filter((video) => video !== lead),
    projection,
  )
  return { ...split, lead, leadIsFeatured: Boolean(lead && leadIsFeatured) }
}

/** Earlier works beside the lead when no reservation is shown. */
export const SIDE_WORK_SLOTS = 4
/** Items beside the lead when reservations are shown, tickets and works. */
export const SIDE_MIXED_SLOTS = 5
/** Tickets beside the lead, so an earlier work always keeps a slot. */
export const SIDE_TICKET_SLOTS = 4

export interface SideSlots {
  /** Reservation tickets shown, soonest first. */
  tickets: number
  /** Earlier works shown, newest first. */
  works: number
  /** Reservations cut from the end of the ticket list. */
  hidden: number
}

/**
 * How many reservations and earlier works the All view's side group shows:
 * four works alone, five items in all beside reservations, and at most four
 * tickets with the rest behind the overflow control.
 */
export function sideSlots(reservations: number, works: number): SideSlots {
  const total = Math.max(0, reservations)
  const available = Math.max(0, works)
  if (!total)
    return {
      tickets: 0,
      works: Math.min(available, SIDE_WORK_SLOTS),
      hidden: 0,
    }
  const tickets = Math.min(total, SIDE_TICKET_SLOTS)
  return {
    tickets,
    works: Math.min(available, SIDE_MIXED_SLOTS - tickets),
    hidden: total - tickets,
  }
}

/** One platform's page for a video. */
export interface VideoLink {
  source: string
  url: string
  /** The platform is broadcasting this entry now. */
  live?: boolean
}

/**
 * A video's platform links, the one its card opens first: the video itself,
 * then every merged upload of the same work. Chinese readers open a past
 * video on bilibili when a copy is merged; a reservation or broadcast keeps
 * its own platform, the only one that carries it.
 */
export function videoLinks(video: Video, locale: Locale): VideoLink[] {
  // A private or removed video keeps only its other platforms' copies.
  const links = [
    ...(videoIsGone(video) ? [] : [{ source: video.source, url: video.url }]),
    ...(video.alternates || []),
  ]
  if (locale !== "zh-CN" || videoPhase(video) !== "past") return links
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
