import type { Translator } from "@/types"
import { useState } from "react"
import { ArrowUpRight, Disc3 } from "lucide-react"

import { mediaSrcSet } from "@/lib/media"
import { ExternalLink } from "@/site/links"
import { coverUrl } from "./content"
import type { WorksCover } from "./types"

/** How wide each cover shows (src/works/works.css). */
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
}: {
  cover: WorksCover | null
  alt: string
  t: Translator
  /** Without sizes, as in the viewer, the original loads. */
  sizes?: string
  eager?: boolean
}) {
  const [failed, setFailed] = useState(false)
  if (!cover || failed) {
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
      onError={() => setFailed(true)}
    />
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
