import type { CSSProperties } from "react"
import { useState } from "react"
import { Sparkles, UserRoundCheck } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import {
  TranslationToggle,
  useTranslatedText,
} from "@/components/translated-text"
import { mediaSrcSet } from "@/lib/media"
import { BilibiliLogo, ExternalLink, YouTubeLogo } from "@/site/links"
import { dayLabel, fullDate, uploadTone } from "./content"
import {
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

/**
 * The uploads posted after the collection's date, above the collection's
 * own list and outside its filters and counts. Each is marked as found
 * automatically or added by hand. Nothing shows when there are none.
 */
export function RecentUploads({
  items,
  updatedAt,
}: {
  items: WorksRecentUpload[]
  updatedAt: string
}) {
  const { t, locale } = useAppSettings()
  if (!items.length) return null
  return (
    <section className="works-recent" aria-labelledby="works-recent-title">
      <header className="works-recent-label">
        <h3 id="works-recent-title">{t("works.recent.title")}</h3>
        <small>
          {t("works.recent.since", {
            date: fullDate(updatedAt, locale),
            count: items.length,
          })}
        </small>
        <p>{t("works.recent.note")}</p>
      </header>
      <ul className="upload-grid">
        {items.map((item, index) => (
          <RecentCard key={item.id} item={item} index={index} />
        ))}
      </ul>
    </section>
  )
}
