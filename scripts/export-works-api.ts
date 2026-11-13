import fs from "node:fs/promises"
import path from "node:path"
import { fileURLToPath } from "node:url"

import * as prettier from "prettier"

import { createTranslator } from "../src/i18n.ts"
import { personalReleases } from "../src/works/content.ts"
import mediaCatalog from "../src/works/media-catalog.json" with { type: "json" }
import participation from "../src/works/participation.json" with { type: "json" }
import summary from "../src/works/summary.json" with { type: "json" }
import uploads from "../src/works/uploads.json" with { type: "json" }
import { worksPhraseMessages } from "../src/works/works-phrases.ts"
import { worksText } from "../src/works/works-text.ts"
import type {
  DiscImage,
  ParticipationRelease,
  PersonalEdition,
  PersonalRelease,
  StoreBonus,
  StoreGroup,
  Upload,
  UploadVideos,
  WorksCover,
  WorksImage,
  WorksMedia,
  WorksText,
  WorksTrack,
} from "../src/works/types.ts"

/*
 * Writes the bodies of GET /api/works and GET /api/works/upload-list from the
 * tracked Works data (docs/architecture/public-api.md#works-collection). It
 * reads only src/ and renders every phrase-coded text with the page's own
 * worksText and translators, so the board and the API share one derivation.
 * tests/works-api.test.ts fails when the committed files differ.
 */

const projectRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
)

export const worksApiDirectory = path.join(
  projectRoot,
  "backend/internal/works",
)

/** The locales of every localized text, in the order the API writes them. */
export const apiLocales = ["ja", "zh-CN", "en"] as const

export type ApiText = Record<(typeof apiLocales)[number], string | null>

const translators = Object.fromEntries(
  apiLocales.map((locale) => [
    locale,
    createTranslator(locale, worksPhraseMessages),
  ]),
) as Record<(typeof apiLocales)[number], ReturnType<typeof createTranslator>>

/**
 * Collection wording in every locale as the page renders it, or null when the
 * collection gives none.
 */
export function worksApiText(text: WorksText | undefined): ApiText | null {
  if (
    text === undefined ||
    text === "" ||
    (Array.isArray(text) && !text.length)
  )
    return null
  return Object.fromEntries(
    apiLocales.map((locale) => [
      locale,
      worksText(text, translators[locale]) || null,
    ]),
  ) as ApiText
}

const mimeTypes = new Map(
  (mediaCatalog as WorksMedia[]).map((item) => [item.name, item.mimeType]),
)

/** The packaged MIME type of a Works image; every image must be catalogued. */
export function worksMimeType(name: string) {
  const mimeType = mimeTypes.get(name)
  if (!mimeType) throw new Error(`${name} is not in the Works media catalog`)
  return mimeType
}

/**
 * A Works image as the API serves it: its local URL and size, and the
 * caption and provenance the page shows. A cutout has no provenance of its
 * own, so its thirdParty is null.
 */
export function worksApiImage(image: WorksCover | WorksImage) {
  const cover: Partial<WorksCover> = image
  return {
    url: `/media/works/${image.media}`,
    width: image.width,
    height: image.height,
    mimeType: worksMimeType(image.media),
    caption: worksApiText(cover.caption),
    sourceUrl: cover.source ?? null,
    sourceLabel: cover.sourceLabel ?? null,
    credit: cover.credit
      ? {
          role: cover.credit.role,
          name: cover.credit.name ?? null,
          url: cover.credit.url ?? null,
        }
      : null,
    thirdParty: cover.thirdParty ?? null,
  }
}

/** An upload's videos, as platform IDs in the upload list's shape. */
function listenOf(listen: UploadVideos | undefined) {
  if (!listen) return null
  return {
    niconico: listen.niconico,
    youtube: listen.youtube,
    youtubeMembers: listen.youtubeMembers,
    bilibili: listen.bilibili,
  }
}

function trackOf(track: WorksTrack) {
  return {
    disc: track.disc,
    discLabel: worksApiText(track.discLabel),
    no: track.no,
    title: track.title,
    credit: worksApiText(track.credit),
    note: worksApiText(track.note),
    listen: listenOf(track.listen),
  }
}

const discOf = (item: DiscImage) => ({
  disc: item.disc,
  image: worksApiImage(item.image),
})

const storeBonusOf = (bonus: StoreBonus) => ({
  store: worksApiText(bonus.store),
  item: worksApiText(bonus.item),
  images: bonus.images.map(worksApiImage),
})

const storeGroupOf = (group: StoreGroup) => ({
  item: worksApiText(group.item),
  links: group.links.map((link) => ({
    platform: link.platform,
    url: link.url,
    hiRes: link.hiRes,
  })),
})

function editionOf(edition: PersonalEdition) {
  return {
    name: worksApiText(edition.name),
    date: edition.date,
    cancelled: edition.cancelled,
    catalog: worksApiText(edition.catalog),
    jan: edition.jan,
    price: worksApiText(edition.price),
    format: worksApiText(edition.format),
    packaging: worksApiText(edition.packaging),
    covers: edition.covers.map(worksApiImage),
    bonuses: edition.bonuses.map(worksApiText),
    bonusImages: edition.bonusImages.map(worksApiImage),
    storeBonuses: edition.storeBonuses.map(storeBonusOf),
    stores: edition.stores.map(storeGroupOf),
    tracks: edition.tracks.map(trackOf),
    discs: edition.discs.map(discOf),
    printedDisc: edition.printedDisc,
  }
}

function personalOf(release: PersonalRelease) {
  return {
    id: release.id,
    date: release.date,
    title: release.title,
    format: release.format,
    digital: release.digital,
    cancelled: release.cancelled,
    unit: release.unit,
    label: worksApiText(release.label),
    catalog: worksApiText(release.catalog),
    covers: release.covers.map(worksApiImage),
    disc: release.disc ? worksApiImage(release.disc) : null,
    printedDisc: release.printedDisc,
    cutout: release.cutout ? worksApiImage(release.cutout) : null,
    tracks: release.tracks.map(trackOf),
    discs: release.discs.map(discOf),
    editions: release.editions.map(editionOf),
    storeBonuses: release.storeBonuses.map(storeBonusOf),
    stores: release.stores.map(storeGroupOf),
  }
}

function participationOf(release: ParticipationRelease) {
  return {
    id: release.id,
    date: release.date,
    title: release.title,
    roles: release.roles,
    forms: release.forms,
    unit: release.unit,
    label: release.label,
    event: release.event,
    catalog: release.catalog,
    sourceUrl: release.source || null,
    cover: release.cover ? worksApiImage(release.cover) : null,
    tracks: release.tracks.map((track) => ({
      disc: track.disc,
      no: track.no,
      title: track.title,
      credit: track.credit,
      kano: track.kano,
      kanoCredits: track.kanoCredits,
    })),
  }
}

/** The body of GET /api/works. */
export function worksApiBody() {
  const { updatedAt, ...counts } = summary
  return {
    updatedAt,
    summary: counts,
    personal: personalReleases.map(personalOf),
    participation: (participation as ParticipationRelease[]).map(
      participationOf,
    ),
  }
}

/** The body of GET /api/works/upload-list: the upload list as packaged. */
export function worksUploadListBody() {
  return {
    updatedAt: summary.updatedAt,
    uploads: (uploads as Upload[]).map((upload) => ({
      id: upload.id,
      date: upload.date,
      song: upload.song,
      originalArtist: upload.originalArtist,
      kind: upload.kind,
      with: upload.with,
      niconico: upload.niconico,
      youtube: upload.youtube,
      youtubeMembers: upload.youtubeMembers,
      bilibili: upload.bilibili,
      bilibiliTitle: upload.bilibiliTitle,
      albums: upload.albums,
      tracks: upload.tracks,
    })),
  }
}

/** An API body as JSON reads it back, for comparing with a committed file. */
export function apiValue(value: unknown): unknown {
  return JSON.parse(JSON.stringify(value))
}

export const worksApiFiles = {
  "catalog.json": worksApiBody,
  "upload-list.json": worksUploadListBody,
}

export async function writeApiFiles(
  directory: string,
  files: Record<string, () => unknown>,
) {
  for (const [name, build] of Object.entries(files)) {
    const file = path.join(directory, name)
    const temporary = `${file}.tmp`
    // Committed as the repository formats JSON, so format checks pass.
    const options = await prettier.resolveConfig(file)
    await fs.writeFile(
      temporary,
      await prettier.format(JSON.stringify(build()), {
        ...options,
        filepath: file,
      }),
    )
    await fs.rename(temporary, file)
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await writeApiFiles(worksApiDirectory, worksApiFiles)
}
