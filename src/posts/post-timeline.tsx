import type {
  ArchiveSnapshot,
  Locale,
  MediaItem,
  Post,
  Translator,
} from "@/types"
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { ChevronUp } from "lucide-react"

import { EmptyNote } from "@/site/site-shell"
import {
  ArchiveMore,
  ArchiveState,
} from "@/dashboard/components/archive-dialog"
import {
  ARCHIVE_SKELETON,
  PostNote,
  PostSkeleton,
} from "@/dashboard/components/post-wall"
import { formatLongDate, formatMonth, formatWeekday } from "@/dashboard/format"
import { groupPostsByDay } from "./timeline"

/** How far below the viewport's top a month header still counts as shown. */
const VISIBLE_MONTH_OFFSET = 140
/** How far below the sticky controls a day's top still counts as shown. */
const VISIBLE_DAY_SLACK = 8

function dayFromKey(key: string) {
  const [year, month, day] = key.split("-").map(Number)
  return new Date(year, month - 1, day)
}

/**
 * Keeps an element at a viewport offset while the posts around it settle:
 * images above it that finish loading would push it away. Each resize of the
 * list re-aligns it, for a second at most, and the hold ends as soon as the
 * reader scrolls or presses a key.
 */
function holdInView(
  list: HTMLElement,
  find: () => HTMLElement | null,
  top: number,
) {
  const events = ["wheel", "touchstart", "pointerdown", "keydown"]
  const align = () => {
    const element = find()
    const offset = element ? element.getBoundingClientRect().top - top : 0
    if (Math.abs(offset) >= 1)
      window.scrollBy({ top: offset, behavior: "instant" })
  }
  const observer =
    typeof ResizeObserver === "function" ? new ResizeObserver(align) : null
  const stop = () => {
    window.clearTimeout(timer)
    observer?.disconnect()
    for (const name of events) window.removeEventListener(name, stop)
  }
  for (const name of events)
    window.addEventListener(name, stop, { passive: true })
  observer?.observe(list)
  const timer = window.setTimeout(stop, 1000)
  align()
  return stop
}

function postCard(root: HTMLElement, id: string) {
  for (const card of root.querySelectorAll<HTMLElement>("[data-post-id]"))
    if (card.dataset.postId === id) return card
  return null
}

/**
 * The posts page's column: stored posts newest first, under a heading per
 * Japan month and a date per Japan day. The next page loads as the end
 * scrolls near and the month and day in view are reported as the page
 * scrolls; `jump`
 * scrolls a loaded month to the top once it is loaded. A timeline that starts
 * from an earlier month offers the newer posts above it; when they load
 * above, the post that was first stays where it was on screen.
 */
export function PostTimeline({
  archive,
  isLoading,
  error,
  onRetry,
  onLoadMore,
  onNewer,
  monthCounts,
  jump,
  onVisibleMonth,
  onVisibleDay,
  avatarUrl,
  accountAvatars,
  locale,
  now,
  t,
  onOpenMedia,
}: {
  archive: ArchiveSnapshot<Post> | null
  isLoading: boolean
  error: Error | null
  onRetry: () => unknown
  onLoadMore: () => unknown
  onNewer: () => void
  /** Posts per `YYYY-MM` for the chosen account; null while unknown. */
  monthCounts: ReadonlyMap<string, number> | null
  /** A loaded month to scroll to; a new nonce scrolls again. */
  jump: { month: string; nonce: number } | null
  onVisibleMonth: (month: string) => void
  /** The `YYYY-MM-DD` Japan day whose posts reach the sticky controls. */
  onVisibleDay?: (day: string | null) => void
  avatarUrl: string | null
  accountAvatars?: Record<string, string | null>
  locale: Locale
  now: number
  t: Translator
  onOpenMedia: (media: MediaItem[], index: number) => void
}) {
  const listRef = useRef<HTMLDivElement>(null)
  const items = archive?.items
  const months = useMemo(() => groupPostsByDay(items || []), [items])
  const monthList = months.map((group) => group.month).join(",")

  const reportRef = useRef(onVisibleMonth)
  reportRef.current = onVisibleMonth
  const reportDayRef = useRef(onVisibleDay)
  reportDayRef.current = onVisibleDay
  const shownRef = useRef<string | null>(null)
  const shownDayRef = useRef<string | null>(null)
  // The month in view is the last one whose header has reached the top; the
  // day in view is the last one that has reached the sticky controls.
  useEffect(() => {
    const list = listRef.current
    if (!list || !monthList) return undefined
    let frame = 0
    const measure = () => {
      frame = 0
      const dayLine =
        (parseFloat(getComputedStyle(list).getPropertyValue("--posts-top")) ||
          0) + VISIBLE_DAY_SLACK
      let day: string | null = null
      for (const section of list.querySelectorAll<HTMLElement>("[data-day]")) {
        if (section.getBoundingClientRect().top > dayLine) break
        day = section.dataset.day || null
      }
      if (day !== shownDayRef.current) {
        shownDayRef.current = day
        reportDayRef.current?.(day)
      }
      let shown: string | undefined
      for (const section of list.querySelectorAll<HTMLElement>(
        "[data-month]",
      )) {
        if (shown && section.getBoundingClientRect().top > VISIBLE_MONTH_OFFSET)
          break
        shown = section.dataset.month
      }
      if (shown && shown !== shownRef.current) {
        shownRef.current = shown
        reportRef.current(shown)
      }
    }
    const onScroll = () => {
      if (!frame) frame = window.requestAnimationFrame(measure)
    }
    measure()
    window.addEventListener("scroll", onScroll, { passive: true })
    return () => {
      window.removeEventListener("scroll", onScroll)
      if (frame) window.cancelAnimationFrame(frame)
    }
  }, [monthList])

  // A jump waits for its month to load, then scrolls there once.
  const jumpedRef = useRef<number | null>(null)
  useEffect(() => {
    if (!jump || jumpedRef.current === jump.nonce) return
    const list = listRef.current
    const section = list?.querySelector<HTMLElement>(
      `[data-month="${jump.month}"]`,
    )
    if (!list || !section) return
    jumpedRef.current = jump.nonce
    shownRef.current = jump.month
    reportRef.current(jump.month)
    section.scrollIntoView?.({ block: "start", behavior: "instant" })
    // A month near the end of the loaded posts cannot reach the top until the
    // next page loads, so the hold aims at where the month belongs.
    const month = jump.month
    return holdInView(
      list,
      () => list.querySelector<HTMLElement>(`[data-month="${month}"]`),
      parseFloat(getComputedStyle(section).scrollMarginTop) || 0,
    )
  }, [jump, monthList])

  // Where the first post stood before newer ones loaded above it.
  const keepRef = useRef<{ id: string; top: number } | null>(null)
  const firstId = items?.[0]?.id
  const firstPage = archive?.firstPage ?? 1
  useLayoutEffect(() => {
    const keep = keepRef.current
    if (!keep || keep.id === firstId) return
    keepRef.current = null
    const list = listRef.current
    if (!list) return
    return holdInView(list, () => postCard(list, keep.id), keep.top)
  }, [firstId])
  // Whether the reader asked for newer posts and they are still loading; a
  // background reread neither relabels nor disables the button, so it keeps
  // keyboard focus.
  const [newerPending, setNewerPending] = useState(false)
  useEffect(() => {
    if (isLoading) return
    keepRef.current = null
    setNewerPending(false)
  }, [isLoading, newerPending])
  const showNewer = () => {
    if (isLoading) return
    const card =
      firstId && listRef.current ? postCard(listRef.current, firstId) : null
    keepRef.current =
      card && firstPage > 1
        ? { id: firstId!, top: card.getBoundingClientRect().top }
        : null
    setNewerPending(true)
    onNewer()
  }

  return (
    <div className="posts-timeline" ref={listRef}>
      <ArchiveState
        error={error}
        onRetry={onRetry}
        loading={false}
        empty={false}
        emptyTitle=""
        onJump={() => {}}
        locale={locale}
        t={t}
      />
      {!archive && isLoading ? (
        <div className="posts-day is-skeleton" aria-hidden="true">
          <span className="posts-day-date" />
          <div className="posts-day-list">
            {ARCHIVE_SKELETON.slice(0, 4).map((card, index) => (
              <PostSkeleton key={index} {...card} />
            ))}
          </div>
        </div>
      ) : null}
      {archive && !archive.items.length ? (
        <EmptyNote title={t("posts.empty")} />
      ) : null}
      {firstPage > 1 ? (
        <div className="archive-more is-newer">
          <button
            type="button"
            className="pill-button"
            aria-disabled={isLoading || undefined}
            aria-busy={newerPending || undefined}
            onClick={showNewer}
          >
            <ChevronUp aria-hidden="true" />
            {newerPending ? t("common.refreshing") : t("posts.newer")}
          </button>
        </div>
      ) : null}
      {months.map(({ month, days }) => {
        const [year, monthNumber] = month.split("-").map(Number)
        const count =
          monthCounts?.get(month) ??
          days.reduce((sum, day) => sum + day.posts.length, 0)
        return (
          <section
            key={month}
            className="posts-month"
            data-month={month}
            aria-labelledby={`posts-month-${month}`}
          >
            <h2 className="posts-month-head" id={`posts-month-${month}`}>
              <span>
                {formatMonth(new Date(year, monthNumber - 1, 1), locale)}
              </span>
              <small>{t("feed.monthCount", { count })}</small>
            </h2>
            {days.map(({ day, posts }) => {
              const date = dayFromKey(day)
              return (
                <section
                  key={day}
                  className="posts-day"
                  data-day={day}
                  aria-label={formatLongDate(date, locale)}
                >
                  <time
                    className="posts-day-date"
                    dateTime={day}
                    aria-hidden="true"
                  >
                    <span className="posts-day-number">{date.getDate()}</span>
                    <span className="posts-day-weekday">
                      {formatWeekday(date, locale)}
                    </span>
                  </time>
                  <div className="posts-day-list">
                    {posts.map((post) => (
                      <div key={post.id} data-post-id={post.id}>
                        <PostNote
                          post={post}
                          avatarUrl={avatarUrl}
                          accountAvatars={accountAvatars}
                          absoluteTime="clock"
                          clamp={false}
                          locale={locale}
                          now={now}
                          t={t}
                          onOpenMedia={onOpenMedia}
                        />
                      </div>
                    ))}
                  </div>
                </section>
              )
            })}
          </section>
        )
      })}
      {archive ? (
        <ArchiveMore
          hasNext={archive.hasNext}
          isLoading={isLoading}
          label={t("posts.loadMore")}
          onLoadMore={onLoadMore}
          auto={{ failed: Boolean(error), loaded: archive.items.length }}
          t={t}
        />
      ) : null}
    </div>
  )
}
