import type { Translator } from "@/types"
import { useEffect, useRef, useState } from "react"
import { ArrowUpRight, Disc3 } from "lucide-react"

import { mediaSrcSet } from "@/lib/media"
import { ExternalLink } from "@/site/links"
import { coverUrl } from "./content"
import type { WorksCover, WorksCredit } from "./types"

/** How wide each cover shows (src/works/styles/). */
export const WORKS_COVER_SIZES = {
  card: "(max-width: 480px) 45vw, (max-width: 900px) 30vw, 200px",
  sheet: "(max-width: 720px) 90vw, 340px",
  edition: "64px",
  row: "72px",
  label: "96px",
  trio: "(max-width: 820px) 40vw, 240px",
  figure: "(max-width: 820px) 40vw, 240px",
  discPhoto: "64px",
  upload: "(max-width: 560px) 45vw, 300px",
} as const

/**
 * A packaged cover. A cover that is not on this server shows an explicit
 * unavailable state; it never falls back to the address it came from.
 */
export function WorksCoverImage({
  cover,
  alt,
  t,
  sizes,
  eager = false,
  onSettled,
}: {
  cover: WorksCover | null
  alt: string
  t: Translator
  /** Without sizes, as in the viewer, the original loads. */
  sizes?: string
  eager?: boolean
  /** Called once the image has loaded or failed to. */
  onSettled?: () => void
}) {
  // The cover that failed, so another cover in the same place still loads.
  const [failed, setFailed] = useState<string | null>(null)
  if (!cover || failed === cover.media) {
    return (
      <span
        className="works-missing"
        role={alt ? "img" : undefined}
        aria-label={alt || undefined}
      >
        <Disc3 aria-hidden="true" />
        <span>{t(cover ? "works.coverUnavailable" : "works.noCover")}</span>
      </span>
    )
  }
  const url = coverUrl(cover)
  return (
    <img
      src={url}
      srcSet={sizes ? mediaSrcSet(url, cover.width) : undefined}
      sizes={sizes}
      alt={alt}
      width={cover.width}
      height={cover.height}
      loading={eager ? "eager" : "lazy"}
      decoding="async"
      onLoad={onSettled}
      onError={() => {
        setFailed(cover.media)
        onSettled?.()
      }}
    />
  )
}

/** How long a newly loaded cover takes to fade in over the one before. */
const COVER_FADE_MS = 320

type CoverLayer = { cover: WorksCover; ready: boolean }

/**
 * A cover that changes without a gap: the one shown stays until the next has
 * loaded, which then fades in over it. A cover still loading shows a
 * skeleton, over the cover before it when there is one.
 */
export function FadingCover({
  cover,
  t,
  sizes,
  eager = false,
  skeleton = true,
}: {
  cover: WorksCover | null
  t: Translator
  sizes: string
  eager?: boolean
  /** Show the loading skeleton; a blurred backdrop has no use for one. */
  skeleton?: boolean
}) {
  const [layers, setLayers] = useState<CoverLayer[]>(() =>
    cover ? [{ cover, ready: false }] : [],
  )
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  if (cover && layers.at(-1)?.cover.media !== cover.media) {
    // Keep the last loaded cover under the new one; drop any never loaded.
    setLayers([
      ...layers.filter((layer) => layer.ready).slice(-1),
      { cover, ready: false },
    ])
  }
  if (!cover) return <WorksCoverImage cover={null} alt="" t={t} />
  const settle = (media: string) => {
    setLayers((current) =>
      current.map((layer) =>
        layer.cover.media === media ? { ...layer, ready: true } : layer,
      ),
    )
    // Once it has faded in, the covers under it go.
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(
      () =>
        setLayers((current) => {
          const top = current.findLastIndex((layer) => layer.ready)
          return top > 0 ? current.slice(top) : current
        }),
      COVER_FADE_MS,
    )
  }
  const pending = !layers.at(-1)?.ready
  return (
    <span className={`works-fade${pending && skeleton ? " is-pending" : ""}`}>
      {layers.map((layer) => (
        <span
          key={layer.cover.media}
          className={layer.ready ? "is-ready" : undefined}
        >
          <WorksCoverImage
            cover={layer.cover}
            alt=""
            t={t}
            sizes={sizes}
            eager={eager}
            onSettled={() => settle(layer.cover.media)}
          />
        </span>
      ))}
    </span>
  )
}

/**
 * Who scanned, photographed or gave the file, such as "yexca 扫描". A named
 * person with a profile links to it; the profile is not the file's source.
 */
export function CreditLine({
  credit,
  t,
}: {
  credit: WorksCredit
  t: Translator
}) {
  // Without a name value the message keeps its {name} slot to split at.
  const [before, after = ""] = t(`works.credit.${credit.role}`).split("{name}")
  const name = credit.url ? (
    <ExternalLink className="text-link" href={credit.url}>
      {credit.name}
    </ExternalLink>
  ) : (
    credit.name
  )
  return (
    <span className="works-credit">
      {before}
      {name}
      {after}
    </span>
  )
}

/** The viewer's panel: which edition, where the file came from. */
export function CoverDetails({
  cover,
  title,
  t,
}: {
  cover: WorksCover
  title: string
  t: Translator
}) {
  return (
    <div className="works-cover-details">
      <p className="kicker">{t("works.cover")}</p>
      <h2>{title}</h2>
      {cover.caption ? <p lang="zh-CN">{cover.caption}</p> : null}
      {cover.credit ? (
        <p className="works-cover-origin">
          <CreditLine credit={cover.credit} t={t} />
        </p>
      ) : null}
      <dl>
        <div>
          <dt>{t("works.coverSize")}</dt>
          <dd>
            {cover.width} × {cover.height}
          </dd>
        </div>
        <div>
          <dt>{t("works.coverFile")}</dt>
          <dd>
            {t(
              cover.thirdParty ? "works.thirdPartyFile" : "works.officialFile",
            )}
          </dd>
        </div>
      </dl>
      {cover.source ? (
        <ExternalLink className="text-link" href={cover.source}>
          {cover.sourceLabel || t("works.coverSource")}
          <ArrowUpRight aria-hidden="true" />
        </ExternalLink>
      ) : cover.sourceLabel ? (
        <p className="works-cover-origin">{cover.sourceLabel}</p>
      ) : null}
    </div>
  )
}
