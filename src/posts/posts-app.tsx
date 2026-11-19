import type { ArchiveSnapshot, MediaItem, Post } from "@/types"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"

import { useAppSettings } from "@/app-settings"
import { cdnUrl } from "@/lib/cdn"
import { MediaDialog, stepIndex } from "@/site/media-dialog"
import { SiteFooter, SiteHeader, SkipLink } from "@/site/site-shell"
import {
  ArchiveMonthBar,
  ArchiveState,
} from "@/dashboard/components/archive-dialog"
import { monthKeyOf } from "@/dashboard/components/month-picker"
import {
  type PostArchiveLayout,
  PostArchive,
  PostArchiveFilter,
  PostArchiveLayoutToggle,
  PostMediaView,
} from "@/dashboard/components/post-wall"
import { japanToday, monthStartOf } from "@/dashboard/format"
import {
  useArchive,
  useNow,
  usePostArchiveIndex,
} from "@/dashboard/use-dashboard"
import { usePostsLocation } from "./navigation"
import "@/dashboard/board.css"
import "./posts.css"

interface ViewerImage {
  url: string
  alt?: string
  kind?: MediaItem["kind"]
  href?: string
}

type ViewerCandidate = Omit<ViewerImage, "url"> & { url: string | null }

/** This device remembers the archive's layout, as the board's dialog did. */
const POST_LAYOUT_KEY = "kano-post-archive-layout"

function readPostLayout(): PostArchiveLayout {
  try {
    return window.localStorage.getItem(POST_LAYOUT_KEY) === "list"
      ? "list"
      : "masonry"
  } catch {
    return "masonry"
  }
}

/** The first day of a `YYYY-MM` month, as a local Date. */
function monthFromKey(key: string) {
  const [year, month] = key.split("-").map(Number)
  return new Date(year, month - 1, 1)
}

export function PostsApp() {
  const { locale, t } = useAppSettings()
  const now = useNow()
  const [view, show] = usePostsLocation()
  const { index, error: indexError, retry: retryIndex } = usePostArchiveIndex()

  useEffect(() => {
    document.title = t("posts.metaTitle")
    document
      .querySelector('meta[name="description"]')
      ?.setAttribute("content", t("posts.metaDescription"))
  }, [t])

  const thisMonth = monthStartOf(japanToday())
  const account = view.account
  const filter = account?.toLowerCase() || null
  const newestKey = index?.months.at(-1)?.month ?? null
  // Without a month in the address, the newest month holding the chosen
  // account's posts.
  const shownNewest =
    (filter
      ? index?.months.findLast(({ counts }) => (counts[filter] || 0) > 0)?.month
      : null) ?? newestKey
  const monthKey = view.month ?? shownNewest
  const monthStart = monthKey ? monthFromKey(monthKey) : thisMonth
  const latestMonth =
    newestKey && monthFromKey(newestKey) > thisMonth
      ? monthFromKey(newestKey)
      : thisMonth

  // The month is known once the index names the newest one.
  const request =
    index || indexError || view.month
      ? { kind: "posts" as const, monthStart, account }
      : null
  const archive = useArchive<Post>(request, index?.revision)
  const posts = archive.archive as ArchiveSnapshot<Post> | null

  const monthCounts = useMemo(() => {
    if (!index) return null
    return new Map(
      index.months.map(({ month, total, counts }) => [
        month,
        filter ? counts[filter] || 0 : total,
      ]),
    )
  }, [index, filter])
  const monthIndex = index
    ? (index.months.find(({ month }) => month === monthKeyOf(monthStart)) ?? {
        month: monthKeyOf(monthStart),
        total: 0,
        counts: {},
      })
    : undefined
  const years = index?.months.length
    ? `${index.months[0].month.slice(0, 4)}–${index.months.at(-1)!.month.slice(0, 4)}`
    : null
  const total = index?.months.reduce((sum, month) => sum + month.total, 0)

  const setMonth = useCallback(
    (month: Date) => show({ month: monthKeyOf(month), account }),
    [show, account],
  )
  const setAccount = useCallback(
    (next: string | null) => show({ month: monthKey, account: next }),
    [show, monthKey],
  )

  const [layout, setLayoutState] = useState<PostArchiveLayout>(readPostLayout)
  const setLayout = useCallback((next: PostArchiveLayout) => {
    setLayoutState(next)
    try {
      window.localStorage.setItem(POST_LAYOUT_KEY, next)
    } catch {
      // The choice still applies until the page closes.
    }
  }, [])

  const pickerButtonRef = useRef<HTMLButtonElement>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  useEffect(() => setPickerOpen(false), [monthKey])
  // A new month or account starts at the top of the wall.
  const wallRef = useRef<HTMLDivElement>(null)
  const shownKey = useRef(`${monthKey}:${account}`)
  useEffect(() => {
    const key = `${monthKey}:${account}`
    if (shownKey.current === key) return
    shownKey.current = key
    const top = wallRef.current?.getBoundingClientRect().top
    if (top != null && top < 0)
      window.scrollBy({ top: top - 16, behavior: "instant" })
  }, [monthKey, account])

  const [viewer, setViewer] = useState<{
    items: ViewerImage[]
    index: number
  } | null>(null)
  const openImages = useCallback((items: ViewerCandidate[], at: number) => {
    const ready = items.filter((item): item is ViewerImage => Boolean(item.url))
    if (ready.length) setViewer({ items: ready, index: Math.max(0, at) })
  }, [])
  const closeViewer = useCallback(() => setViewer(null), [])
  const stepViewer = useCallback(
    (offset: number) =>
      setViewer((current) =>
        current
          ? {
              ...current,
              index: stepIndex(current.index, offset, current.items.length),
            }
          : current,
      ),
    [],
  )

  return (
    <div className="site posts" id="top">
      <SkipLink href="#posts-content">{t("posts.skip")}</SkipLink>
      <SiteHeader page="posts" />

      <main className="site-main" id="posts-content">
        <section className="posts-page" aria-labelledby="posts-title">
          <header className="posts-head">
            <p className="kicker posts-eyebrow">
              <span>{t("posts.label")}</span>
              {years ? <span>{years}</span> : null}
              {total ? <span>{t("posts.total", { count: total })}</span> : null}
            </p>
            <h1 id="posts-title">{t("posts.title")}</h1>
          </header>

          <div className="posts-bar">
            <ArchiveMonthBar
              monthStart={monthStart}
              onMonthChange={setMonth}
              latestMonth={latestMonth}
              picker={{
                counts: monthCounts,
                countLabel: (count) => t("feed.monthCount", { count }),
              }}
              pickerOpen={pickerOpen}
              setPickerOpen={setPickerOpen}
              pickerButtonRef={pickerButtonRef}
              locale={locale}
              t={t}
            />
            <div className="archive-tools">
              <PostArchiveFilter
                accounts={index?.accounts ?? []}
                bilibiliAccounts={index?.bilibiliAccounts ?? []}
                account={account}
                setAccount={setAccount}
                counts={monthIndex?.counts ?? posts?.accountCounts}
                total={monthIndex?.total ?? (account ? null : posts?.total)}
                t={t}
              />
              <PostArchiveLayoutToggle
                layout={layout}
                setLayout={setLayout}
                t={t}
              />
            </div>
          </div>

          <div className="posts-wall" ref={wallRef}>
            {indexError && !index && !posts ? (
              <ArchiveState
                error={indexError}
                onRetry={retryIndex}
                loading={false}
                empty={false}
                emptyTitle={t("feed.archiveEmpty")}
                onJump={setMonth}
                locale={locale}
                t={t}
              />
            ) : (
              <PostArchive
                archive={posts}
                layout={layout}
                isLoading={archive.isLoading || (!index && !indexError)}
                error={archive.error}
                onRetry={archive.reload}
                onLoadMore={archive.loadMore}
                onJump={setMonth}
                avatarUrl={cdnUrl("/assets/kano-avatar.jpg")}
                accountAvatars={posts?.accountAvatars ?? index?.accountAvatars}
                locale={locale}
                now={now}
                t={t}
                onOpenMedia={openImages}
              />
            )}
          </div>
        </section>
      </main>

      <SiteFooter />
      <MediaDialog
        items={viewer?.items}
        index={viewer?.index || 0}
        label={t("feed.openMedia")}
        onClose={closeViewer}
        onStep={stepViewer}
        renderImage={(item) => <PostMediaView item={item} t={t} />}
      />
    </div>
  )
}
