import path from "node:path"
import { fileURLToPath } from "node:url"

import {
  branchPortraits,
  fixedMedia,
  historyMedia,
  type FixedMediaItem,
  type HistoryMediaItem,
  type LocalizedText,
} from "../src/history/media.ts"
import { branches, milestones } from "../src/history/milestones.ts"
import summary from "../src/works/summary.json" with { type: "json" }
import {
  apiLocales,
  worksApiImage,
  worksMimeType,
  writeApiFiles,
  type ApiText,
} from "./export-works-api.ts"

/*
 * Writes the body of GET /api/journey from the tracked Journey data
 * (docs/architecture/public-api.md#journey). It imports the page's own
 * modules, so branches, milestone order, the release covers a milestone
 * shows and their provenance come from one derivation.
 * tests/journey-api.test.ts fails when the committed file differs.
 */

const projectRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
)

export const journeyApiDirectory = path.join(
  projectRoot,
  "backend/internal/journey",
)

/** Curated text in every locale, null where the text has none. */
function textOf(
  value: Partial<LocalizedText> | null | undefined,
): ApiText | null {
  if (!value) return null
  return Object.fromEntries(
    apiLocales.map((locale) => [locale, value[locale] ?? null]),
  ) as ApiText
}

/** The same name in every locale, such as a release title. */
function sameText(value: string): ApiText {
  return Object.fromEntries(
    apiLocales.map((locale) => [locale, value]),
  ) as ApiText
}

type Milestone = (typeof milestones)[number] & {
  dateEnd?: string
  dateNote?: string
  highlight?: boolean
  release?: string
  links?: { href: string; label: string }[]
}

/** How much of a stored date is known: 2010, 2010-03 or 2010-03-28. */
function datePrecision(date: string) {
  const parts = date.split("-").length
  if (parts === 1) return "year"
  if (parts === 2) return "month"
  if (parts === 3) return "day"
  throw new Error(`Unexpected milestone date ${date}`)
}

/** A web message key such as history.sourceName.agency as its plain code. */
function labelCode(label: string) {
  const prefix = "history.sourceName."
  if (!label.startsWith(prefix))
    throw new Error(`Unexpected milestone link label ${label}`)
  return label.slice(prefix.length)
}

function fixedMediaOf(item: FixedMediaItem) {
  return {
    origin: item.origin,
    id: item.id,
    url: item.path,
    width: item.width,
    height: item.height,
    mimeType: item.mimeType,
    branches: item.branches,
    category: item.category,
    title: textOf(item.title),
    date: item.date || null,
    dateKind: item.dateKind,
    sourceKind: item.sourceKind,
    note: item.note,
    focus: "focus" in item && item.focus ? item.focus : null,
    credit: item.credit,
    creator: item.creator,
    frameTime: item.frameTime,
    sourcePage: item.sourcePage,
    sourceUrl: item.sourceUrl,
    originalStream: item.originalStream,
    releaseId: null,
    cancelled: null,
    cover: null,
  }
}

/** One image of a milestone, catalogued or a release cover from Works. */
function mediaOf(item: HistoryMediaItem) {
  if (item.origin === "fixed") return fixedMediaOf(item)
  const { cover } = item
  return {
    origin: item.origin,
    id: item.id,
    url: item.path,
    width: item.width,
    height: item.height,
    mimeType: worksMimeType(cover.media),
    branches: item.branches,
    category: item.category,
    title: sameText(item.title.en),
    date: item.date,
    dateKind: item.dateKind,
    sourceKind: cover.thirdParty ? "thirdPartyCover" : "officialCover",
    note: item.cancelled
      ? "cancelledRelease"
      : cover.thirdParty
        ? "thirdPartyCover"
        : "cover",
    focus: null,
    credit: null,
    creator: null,
    frameTime: null,
    sourcePage: cover.source ?? null,
    sourceUrl: null,
    originalStream: null,
    releaseId: item.releaseId,
    cancelled: item.cancelled,
    cover: worksApiImage(cover),
  }
}

/**
 * The date the Journey data was checked through: its catalog's newest check
 * or, for the release covers it shows, the Works collection's date.
 */
function journeyUpdatedAt() {
  return [...fixedMedia.map((item) => item.checkedAt), summary.updatedAt]
    .sort()
    .at(-1)
}

/** The body of GET /api/journey. */
export function journeyApiBody() {
  const portraits = new Map(fixedMedia.map((item) => [item.id, item]))
  return {
    updatedAt: journeyUpdatedAt(),
    branches: branches.map((branch) => {
      const portrait = portraits.get(branchPortraits[branch.id])
      return {
        id: branch.id,
        tone: branch.tone,
        since: branch.since,
        label: textOf(branch.label),
        portrait: portrait ? fixedMediaOf(portrait) : null,
      }
    }),
    milestones: (milestones as Milestone[]).map((milestone) => ({
      id: milestone.id,
      date: milestone.date,
      datePrecision: datePrecision(milestone.date),
      dateEnd: milestone.dateEnd ?? null,
      dateNote: milestone.dateNote ?? null,
      branch: milestone.branch,
      kind: milestone.kind,
      highlight: milestone.highlight ?? false,
      title: textOf(milestone.title),
      detail: textOf(milestone.detail),
      sourceUrl: milestone.source ?? null,
      links: (milestone.links ?? []).map((link) => ({
        url: link.href,
        label: labelCode(link.label),
      })),
      releaseId: milestone.release ?? null,
      media: historyMedia
        .filter((item) => item.milestoneId === milestone.id)
        .map(mediaOf),
    })),
    galleryOnly: historyMedia.filter((item) => !item.milestoneId).map(mediaOf),
  }
}

export const journeyApiFiles = { "journey.json": journeyApiBody }

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await writeApiFiles(journeyApiDirectory, journeyApiFiles)
}
