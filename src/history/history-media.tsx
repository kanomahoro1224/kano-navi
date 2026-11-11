import type { Locale, Translator } from "@/types"
import { useState } from "react"
import { ArrowUpRight, ImageOff, Images } from "lucide-react"

import { mediaSrcSet } from "@/lib/media"
import { ExternalLink } from "@/site/links"
import {
  localized,
  sourceName,
  type FixedMediaItem,
  type HistoryMediaItem,
  type ReleaseCoverItem,
} from "./media"

export type { HistoryMediaItem }

/** How wide each history thumbnail shows (src/history/history.css). */
export const HISTORY_IMAGE_SIZES = {
  portrait: "(max-width: 820px) 360px, 460px",
  identity: "48px",
  milestone: "(max-width: 760px) 320px, 240px",
  gallery:
    "(max-width: 480px) 100vw, (max-width: 720px) 50vw, (max-width: 960px) 34vw, 300px",
} as const

/**
 * A Journey image or a Works cover; a missing file shows an explicit
 * unavailable state.
 * With sizes it offers the server's smaller copies; without, as in the
 * viewer, it loads the original.
 */
export function HistoryImage({
  item,
  locale,
  t,
  eager = false,
  decorative = false,
  sizes,
  focus,
}: {
  item: HistoryMediaItem
  locale: Locale
  t: Translator
  eager?: boolean
  decorative?: boolean
  sizes?: string
  /** Where a cropping box anchors the image (CSS object-position). */
  focus?: string
}) {
  const [failed, setFailed] = useState(false)
  if (failed) {
    return (
      <span
        className="history-missing"
        role={decorative ? undefined : "img"}
        aria-label={decorative ? undefined : localized(item.title, locale)}
      >
        <ImageOff aria-hidden="true" />
        <span>{t("history.imageUnavailable")}</span>
      </span>
    )
  }
  return (
    <img
      src={item.url}
      srcSet={sizes ? mediaSrcSet(item.url, item.width) : undefined}
      sizes={sizes}
      alt={decorative ? "" : localized(item.title, locale)}
      width={item.width}
      height={item.height}
      loading={eager ? "eager" : "lazy"}
      decoding="async"
      style={focus ? { objectPosition: focus } : undefined}
      onError={() => setFailed(true)}
    />
  )
}

/** Up to four timeline thumbnails; the rest are a count on the last one. */
export function MilestoneMedia({
  media,
  locale,
  t,
  onOpen,
}: {
  media: HistoryMediaItem[]
  locale: Locale
  t: Translator
  onOpen: (index: number) => void
}) {
  const shown = media.slice(0, 4)
  const hidden = media.length - shown.length
  return (
    <div className={`milestone-media count-${shown.length}`}>
      {shown.map((item, index) => (
        <button
          key={item.id}
          type="button"
          className="media-opener"
          onClick={() => onOpen(index)}
          aria-label={t("history.openImage", {
            title: localized(item.title, locale),
          })}
        >
          <HistoryImage
            item={item}
            locale={locale}
            t={t}
            decorative
            sizes={HISTORY_IMAGE_SIZES.milestone}
            focus={item.origin === "fixed" ? item.focus : undefined}
          />
          {item.frameTime ? (
            <span className="frame-time">{item.frameTime}</span>
          ) : null}
          {index === shown.length - 1 && hidden ? (
            <span className="milestone-media-more">
              <Images aria-hidden="true" />+{hidden}
            </span>
          ) : null}
        </button>
      ))}
    </div>
  )
}

export function GalleryTile({
  item,
  locale,
  t,
  onOpen,
}: {
  item: HistoryMediaItem
  locale: Locale
  t: Translator
  onOpen: () => void
}) {
  return (
    <figure className="gallery-tile">
      <button
        type="button"
        className="gallery-tile-image media-opener"
        onClick={onOpen}
        aria-label={t("history.openImage", {
          title: localized(item.title, locale),
        })}
      >
        <HistoryImage
          item={item}
          locale={locale}
          t={t}
          sizes={HISTORY_IMAGE_SIZES.gallery}
        />
        {item.frameTime ? (
          <span className="frame-time">{item.frameTime}</span>
        ) : null}
      </button>
      <figcaption>
        <span className="gallery-tile-meta">
          <span>{t(`history.mediaKind.${item.category}`)}</span>
          <span>{item.date || t("history.dateUnknown")}</span>
        </span>
        <strong>{localized(item.title, locale)}</strong>
        {item.origin === "release" ? (
          item.cover.caption ? (
            <small lang="zh-CN">{item.cover.caption}</small>
          ) : null
        ) : item.credit ? (
          <small>{item.credit}</small>
        ) : null}
      </figcaption>
    </figure>
  )
}

/** Provenance panel for the shared image dialog. */
export function ImageDetails({
  item,
  locale,
  t,
}: {
  item: HistoryMediaItem
  locale: Locale
  t: Translator
}) {
  return item.origin === "release" ? (
    <ReleaseCoverDetails item={item} t={t} />
  ) : (
    <FixedImageDetails item={item} locale={locale} t={t} />
  )
}

/**
 * A release cover's panel, with the Works page's provenance: the file's page,
 * whether it is the publisher's or a third party's, and who scanned or gave
 * it.
 */
function ReleaseCoverDetails({
  item,
  t,
}: {
  item: ReleaseCoverItem
  t: Translator
}) {
  const { cover } = item
  const credit = cover.credit
  const note = item.cancelled
    ? "cancelledRelease"
    : cover.thirdParty
      ? "thirdPartyCover"
      : "cover"
  return (
    <div className="image-details">
      <p className="kicker">{t("history.mediaKind.cover")}</p>
      <h2>{item.title.en}</h2>
      {cover.caption ? <p lang="zh-CN">{cover.caption}</p> : null}
      <dl>
        <div>
          <dt>{t("history.dateKind.release")}</dt>
          <dd>{item.date}</dd>
        </div>
        <div>
          <dt>{t("history.imageSize")}</dt>
          <dd>
            {cover.width} × {cover.height} ·{" "}
            {cover.media.split(".").pop()?.toUpperCase()}
          </dd>
        </div>
        <div>
          <dt>{t("history.imageSource")}</dt>
          <dd>
            {t(
              cover.thirdParty
                ? "history.sourceKind.thirdPartyCover"
                : "history.sourceKind.officialCover",
            )}
          </dd>
        </div>
        {credit ? (
          <div>
            <dt>{t("history.creator")}</dt>
            <dd>
              {credit.url && credit.name ? (
                <ExternalLink className="text-link" href={credit.url}>
                  {t(`history.coverCredit.${credit.role}`, {
                    name: credit.name,
                  })}
                </ExternalLink>
              ) : (
                t(`history.coverCredit.${credit.role}`, {
                  name: credit.name ?? "",
                })
              )}
            </dd>
          </div>
        ) : null}
      </dl>
      <p className="image-details-note">{t(`history.imageNote.${note}`)}</p>
      <div className="image-details-links">
        <a
          className="pill-button"
          href={`/works?release=${encodeURIComponent(item.releaseId)}`}
        >
          {t("history.openRelease")}
        </a>
        {cover.source ? (
          <ExternalLink className="text-link" href={cover.source}>
            {t("history.openSource")} · {sourceName(cover.source, t)}
            <ArrowUpRight aria-hidden="true" />
          </ExternalLink>
        ) : cover.sourceLabel ? (
          <span className="text-link">{cover.sourceLabel}</span>
        ) : null}
      </div>
      <p className="image-details-rights">{t("history.imageRights")}</p>
    </div>
  )
}

function FixedImageDetails({
  item,
  locale,
  t,
}: {
  item: FixedMediaItem
  locale: Locale
  t: Translator
}) {
  return (
    <div className="image-details">
      <p className="kicker">{t(`history.mediaKind.${item.category}`)}</p>
      <h2>{localized(item.title, locale)}</h2>
      <dl>
        <div>
          <dt>{t(`history.dateKind.${item.dateKind}`)}</dt>
          <dd>{item.date || t("history.dateUnknown")}</dd>
        </div>
        <div>
          <dt>{t("history.imageSize")}</dt>
          <dd>
            {item.width} × {item.height} ·{" "}
            {item.mimeType.slice(6).toUpperCase()}
          </dd>
        </div>
        <div>
          <dt>{t("history.imageSource")}</dt>
          <dd>{t(`history.sourceKind.${item.sourceKind}`)}</dd>
        </div>
        {item.creator ? (
          <div>
            <dt>{t("history.creator")}</dt>
            <dd>{item.creator}</dd>
          </div>
        ) : null}
        {item.frameTime ? (
          <div>
            <dt>{t("history.frameTime")}</dt>
            <dd>{item.frameTime}</dd>
          </div>
        ) : null}
      </dl>
      {item.note ? (
        <p className="image-details-note">
          {t(`history.imageNote.${item.note}`)}
        </p>
      ) : null}
      <div className="image-details-links">
        <ExternalLink className="pill-button" href={item.sourcePage}>
          {t("history.openSource")} · {sourceName(item.sourcePage, t)}
          <ArrowUpRight aria-hidden="true" />
        </ExternalLink>
        <ExternalLink className="text-link" href={item.sourceUrl}>
          {t(
            item.sourceKind === "streamArchive"
              ? "history.originalStream"
              : item.frameTime
                ? "history.videoAtFrame"
                : "history.sourceImage",
          )}
          <ArrowUpRight aria-hidden="true" />
        </ExternalLink>
      </div>
      {item.credit ? (
        <p className="image-details-credit">{item.credit}</p>
      ) : null}
      <p className="image-details-rights">{t("history.imageRights")}</p>
    </div>
  )
}
