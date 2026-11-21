import {
  cleanVideoTitle,
  eventLink,
  eventLinks,
  type EventPlatformLink,
} from "./format.ts"
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
  /** Whether the projection reads the ticket On air. */
  isLive: boolean
  /** Whether the projection adds a members chip beside the ticket's label. */
  membersChip: boolean
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

// Only artwork earns the full ticket; a schedule entry without it would leave
// an empty art band, so it gets the compact date ticket instead.
function withLayout(item: SpotlightCandidate): SpotlightItem {
  return { ...item, layout: item.thumbnailUrl ? "full" : "compact" }
}

/**
 * What the stage shows, as the projection names it: the next or live stream
 * or entry, an unannounced live broadcast (`liveNow`), or else the latest
 * video. The summary supplies the named records.
 */
export function pickSpotlight(
  summary: SpotlightSummary | null | undefined,
  liveNow: LiveNow | null | undefined,
  projection: DashboardProjection,
): SpotlightItem | null {
  const projected = projection.spotlight
  if (!projected) return null
  if (projected.liveNow && liveNow) {
    return withLayout({
      mode: "next",
      time: liveNow.startedAt,
      title: cleanVideoTitle(liveNow.title),
      url: liveNow.url,
      link: "watch",
      liveNow,
      isLive: projected.isLive,
      membersChip: projected.membersChip,
    })
  }
  if (projected.mode === "latest") {
    const latest = summary?.latestVideo
    if (!latest) return null
    return {
      mode: "latest",
      layout: "full",
      time: latest.publishedAt || latest.scheduledAt,
      title: cleanVideoTitle(latest.title),
      url: latest.url,
      link: "watch",
      thumbnailUrl: latest.thumbnailUrl,
      video: latest,
      isLive: projected.isLive,
      membersChip: false,
    }
  }
  const video = projected.videoId
    ? [summary?.nextStream, summary?.latestVideo].find(
        (item) => item?.id === projected.videoId,
      ) || null
    : null
  const event =
    projected.eventId && summary?.nextEvent?.id === projected.eventId
      ? summary.nextEvent
      : null
  if (video) {
    // The video's ticket keeps the other platforms of its entry.
    const sources = [
      video.source,
      ...(video.alternates || []).map((link) => link.source),
    ]
    return withLayout({
      mode: "next",
      time: video.scheduledAt,
      title: cleanVideoTitle(video.title),
      url: video.url,
      link: "watch",
      thumbnailUrl: video.thumbnailUrl,
      video,
      ...(event
        ? {
            extraLinks: eventLinks(event).filter(
              (link) => !sources.includes(link.source),
            ),
          }
        : {}),
      isLive: projected.isLive,
      membersChip: projected.membersChip,
    })
  }
  if (!event) return null
  const url = eventLink(event)
  return withLayout({
    mode: "next",
    time: event.startsAt || null,
    dateOnly: event.startsAt ? null : event.startsOn,
    title: cleanVideoTitle(event.title),
    detail: event.detail,
    url,
    link: spotlightLink(url),
    event,
    isLive: projected.isLive,
    membersChip: projected.membersChip,
  })
}

/** Whether a ticket reads On air, as the projection says, for a timed one. */
export function spotlightIsLive(item: SpotlightCandidate | null | undefined) {
  return item?.mode === "next" && Boolean(item.time) && item.isLive
}

/** Whether the ticket is an entry under way without a broadcast on air. */
export function spotlightIsOngoing(
  item: SpotlightCandidate | null | undefined,
) {
  return (
    item?.mode === "next" &&
    !item.isLive &&
    item.event?.view.state === "ongoing"
  )
}

/**
 * The latest video the projection shows under a compact ticket, so the stage
 * keeps a picture.
 */
export function pickCompanion(
  summary: SpotlightSummary | null | undefined,
  item: SpotlightItem | null | undefined,
  projection: DashboardProjection,
) {
  if (item?.layout !== "compact") return null
  const video = summary?.latestVideo
  const id = projection.spotlight?.companionVideoId
  if (!video || !id || video.id !== id) return null
  return {
    time: video.publishedAt || video.scheduledAt,
    title: cleanVideoTitle(video.title),
    url: video.url,
    thumbnailUrl: video.thumbnailUrl,
    video,
  }
}
