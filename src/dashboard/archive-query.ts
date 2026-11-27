import type { ArchiveMonth, ArchiveSnapshot } from "@/types"
import { addDays, localDateKey, startOfWeek } from "./format"
import type { VideoCategory } from "./video-groups"

// What an archive read asks for and how its answers are checked: the
// addresses, ranges and keys of the schedule month, post and video timeline
// reads and their month indexes. Nothing here reads.

export type ArchiveKind = "schedule" | "posts" | "videos"

export interface ArchiveRequest {
  kind: ArchiveKind
  /** The schedule month shown; the post and video timelines ignore it. */
  monthStart: Date
  /** Post timelines only: an X handle or Bilibili mid, or null for all. */
  account?: string | null
  /** Post timelines only: category ("all" | "post" | "reply" | "repost"). */
  postKind?: string | null
  /** Video timelines only: one category, or null for every video. */
  category?: VideoCategory | null
  /**
   * Post and video timelines: the month the timeline starts from; null starts
   * from the newest record. With `startPage`, the timeline reads the
   * newest-first pages from the one holding that month, so the newer pages
   * before it can load later; without it, from that month's last day, as
   * nothing newer.
   */
  anchor?: Date | null
  /** Post and video timelines: the page of the whole timeline holding `anchor`. */
  startPage?: number
}

/** Records per timeline page; the page numbers count in these. */
export const TIMELINE_PAGE_SIZE = 30

/**
 * The page of a newest-first post or video timeline that holds a month's
 * newest record: every record of the later months comes before it. The month
 * index counts per filter key (a video category, or a post account's lowercase
 * handle), so a chosen filter counts its own records alone.
 */
export function timelineStartPage(
  months: readonly ArchiveMonth[],
  month: string,
  filter: string | null | undefined,
) {
  let newer = 0
  for (const entry of months)
    if (entry.month > month)
      newer += filter ? entry.counts[filter] || 0 : entry.total
  return Math.floor(newer / TIMELINE_PAGE_SIZE) + 1
}

/**
 * Inclusive Japan dates a month archive reads: the Monday-first weeks that
 * cover the month for the schedule grid (at most six, the API bound), or the
 * month's own days for posts and videos.
 */
export function archiveRange(kind: ArchiveKind, monthStart: Date) {
  const first = new Date(monthStart.getFullYear(), monthStart.getMonth(), 1)
  const last = new Date(first.getFullYear(), first.getMonth() + 1, 0)
  return kind === "schedule"
    ? {
        from: localDateKey(startOfWeek(first)),
        to: localDateKey(addDays(startOfWeek(last), 6)),
      }
    : { from: localDateKey(first), to: localDateKey(last) }
}

/** The inclusive Japan date the video timeline ends on, or "" for none. */
function timelineUntil(anchor: Date | null | undefined) {
  return anchor
    ? localDateKey(new Date(anchor.getFullYear(), anchor.getMonth() + 1, 0))
    : ""
}

export function archiveUrl(request: ArchiveRequest, page: number) {
  if (request.kind === "schedule") {
    const { from, to } = archiveRange(request.kind, request.monthStart)
    return `/api/calendar?from=${from}&to=${to}&page=${page}`
  }
  const until = request.startPage ? "" : timelineUntil(request.anchor)
  const filter =
    request.kind === "videos"
      ? request.category
        ? `&category=${request.category}`
        : ""
      : `${request.account ? `&account=${encodeURIComponent(request.account)}` : ""}${
          request.postKind
            ? `&kind=${encodeURIComponent(request.postKind)}`
            : ""
        }`
  return `/api/${request.kind}?page=${page}&pageSize=${TIMELINE_PAGE_SIZE}${until ? `&until=${until}` : ""}${filter}`
}

export const rangeKey = ({ from, to }: { from: string; to: string }) =>
  `${from}/${to}`

/** What identifies one archive read: its month or timeline start and filter. */
export function archiveKey(request: ArchiveRequest) {
  const span =
    request.kind !== "schedule"
      ? request.startPage
        ? `from-page:${request.startPage}`
        : `until:${timelineUntil(request.anchor)}`
      : rangeKey(archiveRange(request.kind, request.monthStart))
  const kindPart = request.postKind ? `:${request.postKind}` : ""
  return `${request.kind}:${span}:${request.account || ""}:${request.category || ""}${kindPart}`
}

/** One archive page answer, or an error when it is not one. */
export function readArchivePage<T>(result: any): ArchiveSnapshot<T> {
  if (
    !Array.isArray(result?.items) ||
    !result.adjacent ||
    !Number.isInteger(result.total) ||
    !Number.isInteger(result.revision) ||
    !Number.isInteger(result.page)
  )
    throw new Error("Invalid archive page")
  return result
}

const monthKeyPattern = /^\d{4}-(0[1-9]|1[0-2])$/

/** Where each month index lives and which field holds its per-key counts. */
export const monthIndexes = {
  posts: { url: "/api/posts/months", counts: "accountCounts" },
  videos: { url: "/api/videos/months", counts: "categoryCounts" },
} as const

export function readArchiveMonth(
  value: unknown,
  countsField: string,
): ArchiveMonth | null {
  if (!value || typeof value !== "object") return null
  const month = value as Record<string, unknown>
  const counts = month[countsField]
  if (
    typeof month.month !== "string" ||
    !monthKeyPattern.test(month.month) ||
    !Number.isInteger(month.total) ||
    !counts ||
    typeof counts !== "object"
  )
    return null
  return {
    month: month.month,
    total: month.total as number,
    counts: counts as Record<string, number>,
  }
}
