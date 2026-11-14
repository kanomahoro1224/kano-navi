import { cdnUrl } from "../lib/cdn.ts"
import type { Translator } from "../types.ts"
import { mediaPath, mediaUrl, personalReleases } from "../works/content.ts"
import type { WorksCover } from "../works/types.ts"
import catalog from "./media-catalog.json" with { type: "json" }
import { milestones } from "./milestones.ts"

// Journey-only images are packaged into data/journey/ under their hashed
// names (docs/development/history-archive.md). Release milestones show their
// release's covers from the Works page (data/works/, see
// docs/development/works-archive.md) instead of copies of their own. Visits
// stay local.
// `path` is the root-relative route the packaged API exports; `url` resolves
// it on read, through the CDN the page advertises, if any.
export const fixedMedia = catalog.map((item) => ({
  ...item,
  origin: "fixed" as const,
  path: `/media/journey/${item.name}`,
  get url(): string {
    return cdnUrl(this.path)
  },
}))

export type FixedMediaItem = (typeof fixedMedia)[number]

/** A cover of a release on the Works page, shown on its milestone. */
export type ReleaseCoverItem = {
  origin: "release"
  id: string
  /** The release's ID on the Works page. */
  releaseId: string
  milestoneId: string
  branches: string[]
  category: "cover"
  /** The release's title, as the Works page writes it in every locale. */
  title: LocalizedText
  date: string
  dateKind: "release"
  /** Every edition was withdrawn before sale. */
  cancelled: boolean
  cover: WorksCover
  /** The cover's root-relative route, as the packaged API names it. */
  path: string
  /** The cover's address on read, on the advertised CDN base if any. */
  readonly url: string
  width: number
  height: number
  frameTime: null
  credit: null
}

export type HistoryMediaItem = FixedMediaItem | ReleaseCoverItem

// A release that also belongs to another identity's story is listed under it.
const releaseBranches: Record<string, string[]> = {
  "ichigo-ichie": ["mklntic"],
  doppel: ["mahoro"],
}

const releaseById = new Map(
  personalReleases.map((release) => [release.id, release]),
)

export const releaseCovers: ReleaseCoverItem[] = milestones.flatMap(
  (milestone) => {
    if (!("release" in milestone) || !milestone.release) return []
    const release = releaseById.get(milestone.release)
    if (!release) return []
    return release.covers.map((cover) => ({
      origin: "release" as const,
      id: `${release.id}/${cover.media}`,
      releaseId: release.id,
      milestoneId: milestone.id,
      branches: [milestone.branch, ...(releaseBranches[milestone.id] ?? [])],
      category: "cover" as const,
      title: { en: release.title },
      date: release.date,
      dateKind: "release" as const,
      cancelled: release.cancelled,
      cover,
      path: mediaPath(cover.media),
      get url(): string {
        return mediaUrl(cover.media)
      },
      width: cover.width,
      height: cover.height,
      frameTime: null,
      credit: null,
    }))
  },
)

export const historyMedia: HistoryMediaItem[] = [
  ...fixedMedia,
  ...releaseCovers,
]

export const mediaByMilestone = historyMedia.reduce<
  Record<string, HistoryMediaItem[]>
>((groups, item) => {
  if (item.milestoneId) (groups[item.milestoneId] ??= []).push(item)
  return groups
}, {})

export const branchPortraits: Record<string, string> = {
  singer: "03_2021_official_keyvisual",
  hanayori: "01_2019_artist_intro",
  mahoro: "03_mahoro_2021_model_sheet",
  mklntic: "01_mahoro_2022_mklntic_outfit",
  sona: "12_sona_2026_official_fullbody",
}

/** Curated text keyed by locale; English is always present. */
export type LocalizedText = { en: string } & Partial<Record<string, string>>

/** Text in the visitor's locale, falling back to English. */
export function localized(value: LocalizedText, locale: string): string
export function localized(
  value: LocalizedText | null | undefined,
  locale: string,
): string | null
export function localized(
  value: LocalizedText | null | undefined,
  locale: string,
) {
  return value?.[locale] ?? value?.en ?? null
}

export function sourceName(href: string, t: Translator) {
  const host = new URL(href).hostname
  if (host.endsWith("amebaownd.com")) return t("history.sourceName.artistSite")
  if (host.endsWith("teichiku.co.jp")) return "TEICHIKU"
  if (host.endsWith("imperialrecords.jp")) return "IMPERIAL RECORDS"
  if (host.endsWith("whv-amusic.com")) return "Warner"
  if (host.endsWith("sonymusic.co.jp")) return "Sony Music"
  if (host.endsWith("youtube.com")) return "YouTube"
  if (host.endsWith("bilibili.com")) return "Bilibili"
  if (host.endsWith("x.com")) return t("history.sourceName.x")
  if (host.endsWith("booth.pm")) return "BOOTH"
  if (host === "shop.milpr.com") return t("history.sourceName.shop")
  if (host.endsWith("milpr.com")) return t("history.sourceName.agency")
  if (host.endsWith("prtimes.jp")) return "PR TIMES"
  if (host === "pixiogaming.jp") return "Pixio"
  if (host === "archive.ragtag.moe") return t("history.sourceName.archive")
  return host.replace(/^www\./u, "")
}
