import { streamIdentity } from "../lib/stream-identity.ts"
import {
  cleanVideoTitle,
  confirmedLiveStatus,
  eventIsLive,
  eventIsUpcoming,
  eventLink,
  eventLinks,
  scheduleSortTime,
  type EventPlatformLink,
} from "./format.ts"
import { leadingCandidate, videoIsInFocus } from "./derive-dashboard.ts"
import type {
  Dashboard,
  DashboardProjection,
  LiveNow,
  ScheduleEvent,
  Video,
} from "../types.ts"

type SpotlightSummary = Pick<
  Dashboard["summary"],
  "nextEvent" | "nextStream" | "latestVideo"
>

export type SpotlightLink = "watch" | "announcement" | "open"

interface SpotlightCandidate {
  mode: "next" | "latest"
  time: string | null
  dateOnly?: string | null
  title: string
  detail?: string | null
  url: string | null
  link: SpotlightLink | null
  thumbnailUrl?: string | null
  video?: Video
  event?: ScheduleEvent
  /** An unannounced broadcast the platform reports live now. */
  liveNow?: LiveNow
  /**
   * The platform links an entry of the same broadcast adds to a video, such
   * as the bilibili room of a simulcast.
   */
  extraLinks?: EventPlatformLink[]
  projectionIsLive?: boolean
  membersChip?: boolean
}

export interface SpotlightItem extends SpotlightCandidate {
  layout: "full" | "compact"
}

const watchHosts = ["youtube.com", "youtu.be", "twitch.tv", "live.bilibili.com"]
const announcementHosts = ["x.com", "twitter.com"]

function hostMatches(host: string, domains: string[]) {
  return domains.some(
    (domain) => host === domain || host.endsWith(`.${domain}`),
  )
}

/** What following the ticket opens: the stream, its announcement, or a page. */
export function spotlightLink(
  url: string | null | undefined,
): SpotlightLink | null {
  if (!url) return null
  let host = ""
  try {
    host = new URL(url).hostname.toLowerCase()
  } catch {
    return "open"
  }
  if (hostMatches(host, watchHosts)) return "watch"
  if (hostMatches(host, announcementHosts)) return "announcement"
  return "open"
}

/** Whether an entry of the video's broadcast is live on another platform. */
function extraLive(item: SpotlightCandidate) {
  return Boolean(item.extraLinks?.some((link) => link.live))
}

// Only artwork earns the full ticket; a schedule entry without it would leave
// an empty art band, so it gets the compact date ticket instead.
function withLayout(item: SpotlightCandidate): SpotlightItem {
  return { ...item, layout: item.thumbnailUrl ? "full" : "compact" }
}

/**
 * What the stage shows: the next or live stream or entry, an unannounced
 * live broadcast (`liveNow`, already checked against the entries), or else
 * the latest video.
 */
export function pickSpotlight(
  summary: SpotlightSummary | null | undefined,
  now: number,
  liveNow?: LiveNow | null,
  projection?: DashboardProjection,
): SpotlightItem | null {
  const projected = projection?.spotlight
  if (projected) {
    if (projected.liveNow && liveNow) {
      return withLayout({
        mode: "next",
        time: liveNow.startedAt,
        title: cleanVideoTitle(liveNow.title),
        url: liveNow.url,
        link: "watch",
        liveNow,
        projectionIsLive: projected.isLive,
        membersChip: projected.membersChip,
      })
    }
    const video = projected.videoId
      ? [summary?.nextStream, summary?.latestVideo].find(
          (item) => item?.id === projected.videoId,
        ) || null
      : null
    const event = projected.eventId
      ? summary?.nextEvent?.id === projected.eventId
        ? summary.nextEvent
        : null
      : null
    if (projected.mode === "latest" && summary?.latestVideo) {
      return {
        mode: "latest",
        layout: "full",
        time:
          summary.latestVideo.publishedAt || summary.latestVideo.scheduledAt,
        title: cleanVideoTitle(summary.latestVideo.title),
        url: summary.latestVideo.url,
        link: "watch",
        thumbnailUrl: summary.latestVideo.thumbnailUrl,
        video: summary.latestVideo,
        projectionIsLive: projected.isLive,
      }
    }
    if (video || event) {
      const duplicate = Boolean(video && event)
      const url = video?.url || (event ? eventLink(event) : null)
      const item: SpotlightCandidate = video
        ? {
            mode: "next",
            time: video.scheduledAt,
            title: cleanVideoTitle(video.title),
            url,
            link: "watch",
            thumbnailUrl: video.thumbnailUrl,
            video,
            projectionIsLive: projected.isLive,
            membersChip: projected.membersChip,
          }
        : {
            mode: "next",
            time: event?.startsAt || null,
            dateOnly: event?.startsAt ? null : event?.startsOn,
            title: cleanVideoTitle(event?.title),
            detail: event?.detail,
            url,
            link: spotlightLink(url),
            event: event || undefined,
            projectionIsLive: projected.isLive,
            membersChip: projected.membersChip,
          }
      if (duplicate && event && video) {
        const sources = [
          video.source,
          ...(video.alternates || []).map((link) => link.source),
        ]
        item.extraLinks = eventLinks(event).filter(
          (link) => !sources.includes(link.source),
        )
      }
      return withLayout(item)
    }
  }
  const candidates: SpotlightCandidate[] = []
  if (liveNow) {
    candidates.push({
      mode: "next",
      time: liveNow.startedAt,
      title: cleanVideoTitle(liveNow.title),
      url: liveNow.url,
      link: "watch",
      liveNow,
    })
  }
  if (summary?.nextStream?.scheduledAt) {
    const video = summary.nextStream
    candidates.push({
      mode: "next",
      time: video.scheduledAt,
      title: cleanVideoTitle(video.title),
      url: video.url,
      link: "watch",
      thumbnailUrl: video.thumbnailUrl,
      video,
    })
  }
  if (summary?.nextEvent) {
    const event = summary.nextEvent
    const duplicate = candidates.find(
      (candidate) =>
        event.url &&
        candidate.url &&
        streamIdentity(candidate.url) === streamIdentity(event.url),
    )
    // The video's ticket keeps the entry's other platforms.
    if (duplicate?.video) {
      const sources = [
        duplicate.video.source,
        ...(duplicate.video.alternates || []).map((link) => link.source),
      ]
      const extra = eventLinks(event).filter(
        (link) => !sources.includes(link.source),
      )
      if (extra.length) duplicate.extraLinks = extra
    }
    if (!duplicate) {
      // A Bilibili stream's ticket opens its live room once known.
      const url = eventLink(event)
      candidates.push({
        mode: "next",
        time: event.startsAt || null,
        dateOnly: event.startsAt ? null : event.startsOn,
        title: cleanVideoTitle(event.title),
        detail: event.detail,
        url,
        link: spotlightLink(url),
        event,
      })
    }
  }
  const upcoming = candidates
    .filter((candidate) => {
      if (candidate.liveNow) return true
      if (candidate.event)
        return (
          eventIsUpcoming(candidate.event, now) ||
          eventIsLive(candidate.event, now)
        )
      return candidate.video
        ? videoIsInFocus(candidate.video, now) || extraLive(candidate)
        : false
    })
    .sort(
      (a, b) =>
        scheduleSortTime(a.time, a.dateOnly) -
        scheduleSortTime(b.time, b.dateOnly),
    )
  const lead = leadingCandidate(
    upcoming,
    now,
    (candidate) => Date.parse(candidate.time || ""),
    (candidate) =>
      Date.parse((candidate.event || candidate.video)?.endsAt || ""),
    (candidate) =>
      Boolean(candidate.liveNow) ||
      confirmedLiveStatus(candidate.event || candidate.video) === "live" ||
      extraLive(candidate),
  )
  if (lead) return withLayout(lead)

  const video = summary?.latestVideo
  if (video) {
    return {
      mode: "latest",
      layout: "full",
      time: video.publishedAt || video.scheduledAt,
      title: cleanVideoTitle(video.title),
      url: video.url,
      link: "watch",
      thumbnailUrl: video.thumbnailUrl,
      video,
    }
  }
  return null
}

/**
 * Whether a ticket reads On air. A probe-confirmed state wins; only without
 * one does a started time count as live.
 */
export function spotlightIsLive(
  item: SpotlightCandidate | null | undefined,
  now: number,
) {
  if (item?.mode !== "next" || !item.time) return false
  if (item.projectionIsLive !== undefined) return item.projectionIsLive
  if (item.liveNow || extraLive(item)) return true
  const confirmed = confirmedLiveStatus(item.event || item.video)
  if (confirmed) return confirmed === "live"
  return Date.parse(item.time) <= now
}

/** The latest video shown under a compact ticket, so the stage keeps a picture. */
export function pickCompanion(
  summary: SpotlightSummary | null | undefined,
  item: SpotlightItem | null | undefined,
  projection?: DashboardProjection,
) {
  if (item?.layout !== "compact") return null
  const video = summary?.latestVideo
  if (!video || video.url === item.url) return null
  if (projection?.spotlight && !projection.spotlight.companionVideoId)
    return null
  if (
    projection?.spotlight?.companionVideoId &&
    projection.spotlight.companionVideoId !== video.id
  )
    return null
  return {
    time: video.publishedAt || video.scheduledAt,
    title: cleanVideoTitle(video.title),
    url: video.url,
    thumbnailUrl: video.thumbnailUrl,
    video,
  }
}
