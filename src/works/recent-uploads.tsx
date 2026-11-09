import type { CSSProperties } from "react"
import { useEffect, useRef, useState } from "react"
import { ChevronDown, Sparkles, UserRoundCheck } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import {
  TranslationToggle,
  useTranslatedText,
} from "@/components/translated-text"
import { mediaSrcSet } from "@/lib/media"
import { BilibiliLogo, ExternalLink, YouTubeLogo } from "@/site/links"
import { dayLabel, fullDate, uploadTone } from "./content"
import {
  foldedCount,
  japanDay,
  recentLinks,
  type WorksRecentUpload,
} from "./use-works-uploads"
import { WORKS_COVER_SIZES } from "./works-media"

const platformMarks = { youtube: YouTubeLogo, bilibili: BilibiliLogo }

/** The video's saved thumbnail, or a printed sleeve with its title. */
function RecentArt({
  item,
  title,
}: {
  item: WorksRecentUpload
  title: string
}) {
  const [failed, setFailed] = useState(false)
  const cover = item.cover
  if (cover && !failed)
    return (
      <img
        src={cover.url}
        srcSet={mediaSrcSet(cover.url, cover.width)}
        sizes={WORKS_COVER_SIZES.upload}
        alt=""
        width={cover.width ?? undefined}
        height={cover.height ?? undefined}
        loading="lazy"
        decoding="async"
        onError={() => setFailed(true)}
      />
    )
  return (
    <span
      className={`upload-print is-tone-${uploadTone(item.id)}`}
      aria-hidden="true"
    >
      <span className="upload-print-kind" />
      <span className="upload-print-title">{title}</span>
      <span className="upload-print-year">
        {japanDay(item.publishedAt).slice(0, 4)}
      </span>
    </span>
  )
}

function RecentCard({
  item,
  index,
}: {
  item: WorksRecentUpload
  index: number
}) {
  const { t } = useAppSettings()
  const state = useTranslatedText(
    item.title,
    item.translations,
    item.sourceLanguage,
  )
  const links = recentLinks(item)
  const day = japanDay(item.publishedAt)
  const Origin = item.origin === "operator" ? UserRoundCheck : Sparkles
  const watch = (platform: "youtube" | "bilibili") =>
    t("works.watchOn", {
      platform: t(`works.platform.${platform}`),
      title: item.title,
    })
  return (
    <li className="upload-card" style={{ "--order": index } as CSSProperties}>
      <ExternalLink
        className="upload-art"
        href={item.url}
        aria-label={watch(item.source)}
        tabIndex={-1}
      >
        <RecentArt item={item} title={item.title} />
      </ExternalLink>
      <div className="upload-copy">
        <div className="upload-head">
          <p className="works-meta">
            {day ? <time dateTime={day}>{dayLabel(day)}</time> : null}
          </p>
          <p className="upload-links">
            {links.map((link) => {
              const Mark = platformMarks[link.platform]
              return (
                <ExternalLink
                  key={link.href}
                  className={`upload-link is-${link.platform}`}
                  href={link.href}
                  aria-label={watch(link.platform)}
                  title={t(`works.platform.${link.platform}`)}
                >
                  <Mark aria-hidden="true" />
                </ExternalLink>
              )
            })}
          </p>
        </div>
        <h4
          className={`upload-title${state.available ? " translated-title" : ""}`}
        >
          <span lang={state.lang}>{state.text}</span>
          <TranslationToggle state={state} original={item.title} t={t} />
        </h4>
        <p
          className={`works-recent-origin is-${item.origin}`}
          title={t(`works.recent.originHint.${item.origin}`)}
        >
          <Origin aria-hidden="true" />
          {t(`works.recent.origin.${item.origin}`)}
        </p>
      </div>
    </li>
  )
}

/** How many columns a grid lays out now, following its width; measured
 * once the grid is shown. */
function useGridColumns(shown: boolean) {
  const ref = useRef<HTMLUListElement>(null)
  const [columns, setColumns] = useState(1)
  useEffect(() => {
    const grid = ref.current
    if (!grid) return
    const measure = () =>
      setColumns(
        getComputedStyle(grid).gridTemplateColumns.split(" ").filter(Boolean)
          .length || 1,
      )
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(grid)
    return () => observer.disconnect()
  }, [shown])
  return [ref, columns] as const
}

/**
 * The uploads posted after the collection's date, above the collection's
 * own list and outside its filters and counts. Each is marked as found
 * automatically or added by hand. Two rows show until the reader opens the
 * rest; the server sends the newest 48 of total. Nothing shows when there
 * are none.
 */
export function RecentUploads({
  items,
  total,
  updatedAt,
}: {
  items: WorksRecentUpload[]
  total: number
  updatedAt: string
}) {
  const { t, locale } = useAppSettings()
  const [grid, columns] = useGridColumns(items.length > 0)
  const [open, setOpen] = useState(false)
  if (!items.length) return null
  const folded = foldedCount(columns)
  const shown = open ? items : items.slice(0, folded)
  return (
    <section className="works-recent" aria-labelledby="works-recent-title">
      <header className="works-recent-label">
        <h3 id="works-recent-title">{t("works.recent.title")}</h3>
        <small>
          {t("works.recent.since", {
            date: fullDate(updatedAt, locale),
            count: total,
          })}
        </small>
      </header>
      <div className="works-recent-body">
        <ul ref={grid} id="works-recent-list" className="upload-grid">
          {shown.map((item, index) => (
            <RecentCard key={item.id} item={item} index={index} />
          ))}
        </ul>
        {items.length > folded ? (
          <button
            type="button"
            className="pill-button works-recent-more"
            aria-expanded={open}
            aria-controls="works-recent-list"
            onClick={() => setOpen((value) => !value)}
          >
            {open
              ? t("works.recent.fewer")
              : t("works.recent.more", { count: items.length - folded })}
            <ChevronDown aria-hidden="true" />
          </button>
        ) : null}
        {open && total > items.length ? (
          <p className="works-recent-limit">
            {t("works.recent.limit", { count: items.length })}
          </p>
        ) : null}
      </div>
    </section>
  )
}
