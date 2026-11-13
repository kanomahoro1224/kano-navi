// Writes backend/internal/lineblog/archive.json, the LINE BLOG archive as the
// public API serves it, from the tracked files under src/lineblog/ only. It
// reuses the page's own rules (months, neighbours, readable alts, pictographs,
// links between articles), so the board and the API cannot disagree;
// tests/lineblog-api.test.ts fails when the committed file is out of date.
import fs from "node:fs/promises"
import path from "node:path"
import { fileURLToPath } from "node:url"

import {
  ORIGINAL_BLOG,
  archiveMonths,
  articleHref,
  meaningfulAlt,
  mediaUrl,
  neighbours,
  postDate,
  posts,
} from "../src/lineblog/content.ts"
import { isEmojiMedia } from "../src/lineblog/post-media.ts"
import type {
  LineblogBlock,
  LineblogComment,
  LineblogImage,
  LineblogInline,
  LineblogMedia,
  LineblogPostFile,
} from "../src/lineblog/types.ts"

const projectRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
)

export const lineblogExportFile = path.join(
  projectRoot,
  "backend/internal/lineblog/archive.json",
)

type Localized = { ja: string; "zh-CN": string | null; en: string | null }

export interface ApiMedia {
  url: string
  width: number | null
  height: number | null
  mimeType: string
}

export interface ApiImage {
  type: "image"
  /** null when the archive never captured the picture. */
  image: ApiMedia | null
  alt: string | null
  emoji: boolean
  href: string | null
  sourceUrl: string | null
  remoteUrl: string | null
  recovery: "stand-in" | "tweet" | null
}

export type ApiInline =
  | { type: "text"; text: string }
  | {
      type: "link"
      href: string
      articleId: string | null
      content: ApiInline[]
    }
  | ApiImage

export type ApiBlock =
  | {
      type: "paragraph"
      content: ApiInline[]
      translations: { "zh-CN": ApiInline[][] | null; en: ApiInline[][] | null }
    }
  | { type: "quote"; content: ApiInline[] }
  | { type: "rule" }
  | { type: "video"; video: ApiMedia }
  | {
      type: "youtube"
      videoId: string
      watchUrl: string
      video: ApiMedia | null
      cover: ApiMedia | null
      coverSourceUrl: string | null
      archiveUrl: string | null
    }
  | {
      type: "card"
      title: string
      url: string
      articleId: string | null
      archiveUrl: string | null
      description: string | null
      image: ApiImage | null
    }

export interface ApiComment {
  number: number
  author: string
  posted: string
  body: string
  rating: { icon: "star" | "face"; value: number } | null
}

export interface LineblogExport {
  updatedAt: string
  originalBlogUrl: string
  posts: {
    id: string
    published: string
    title: Localized
    thumbnail: ApiMedia | null
    commentCount: number
  }[]
  months: { month: string; total: number }[]
  articles: {
    id: string
    post: {
      id: string
      published: string
      title: Localized
      url: string
      archiveUrl: string
      commentCount: number
      blocks: ApiBlock[]
    }
    previousId: string | null
    nextId: string | null
    comments: ApiComment[]
  }[]
}

export interface LineblogSources {
  catalog: LineblogMedia[]
  files: Map<string, LineblogPostFile>
  comments: Map<string, LineblogComment[]>
}

async function readJson<T>(file: string): Promise<T> {
  return JSON.parse(await fs.readFile(file, "utf8")) as T
}

async function readDirectory<T>(directory: string) {
  const out = new Map<string, T>()
  for (const name of (await fs.readdir(directory)).sort()) {
    const match = /^(\d+)\.json$/u.exec(name)
    if (!match) throw new Error(`Unexpected file ${name} in ${directory}`)
    out.set(match[1], await readJson<T>(path.join(directory, name)))
  }
  return out
}

/** The tracked article files, comments files and media catalog. */
export async function readLineblogSources(
  root = projectRoot,
): Promise<LineblogSources> {
  const dir = path.join(root, "src/lineblog")
  return {
    catalog: await readJson(path.join(dir, "media-catalog.json")),
    files: await readDirectory(path.join(dir, "posts")),
    comments: await readDirectory(path.join(dir, "comments")),
  }
}

/** The archived article a link opens on the page, if it is one. */
function articleId(href: string) {
  const local = articleHref(href)
  return local ? new URLSearchParams(local.split("?")[1]).get("article") : null
}

/** The Japan date of an ISO instant, from the page's own date line. */
function japanDate(iso: string) {
  return postDate(iso).slice(0, 10).replaceAll("/", "-")
}

function localized(item: {
  title: string
  titleZh?: string
  titleEn?: string
}): Localized {
  return {
    ja: item.title,
    "zh-CN": item.titleZh ?? null,
    en: item.titleEn ?? null,
  }
}

/** The public LINE BLOG archive, in the shape backend/internal/lineblog embeds. */
export function lineblogExport(sources: LineblogSources): LineblogExport {
  const catalog = new Map(sources.catalog.map((item) => [item.name, item]))

  const media = (name: string | undefined): ApiMedia | null => {
    if (!name) return null
    const item = catalog.get(name)
    if (!item) throw new Error(`Media ${name} is not in the catalog`)
    return {
      url: mediaUrl(name),
      width: item.width ?? null,
      height: item.height ?? null,
      mimeType: item.mimeType,
    }
  }

  const image = (value: LineblogImage): ApiImage => ({
    type: "image",
    image: media(value.media),
    alt: meaningfulAlt(value.alt),
    emoji: isEmojiMedia(value.media ? catalog.get(value.media) : undefined),
    href: value.href ?? null,
    sourceUrl: value.source ?? null,
    remoteUrl: value.remote ?? null,
    recovery: value.recovery ?? null,
  })

  const inlines = (content: LineblogInline[]): ApiInline[] =>
    content.map((inline) => {
      switch (inline.type) {
        case "text":
          return { type: "text", text: inline.text }
        case "link":
          return {
            type: "link",
            href: inline.href,
            articleId: articleId(inline.href),
            content: inlines(inline.content),
          }
        case "image":
          return image(inline)
        default:
          throw new Error(
            `Unknown inline type ${(inline as { type: string }).type}`,
          )
      }
    })

  const paragraphs = (value: LineblogInline[][] | undefined) =>
    value ? value.map(inlines) : null

  const block = (value: LineblogBlock): ApiBlock => {
    switch (value.type) {
      case "p":
        return {
          type: "paragraph",
          content: inlines(value.content),
          translations: {
            "zh-CN": paragraphs(value.zh),
            en: paragraphs(value.en),
          },
        }
      case "quote":
        return { type: "quote", content: inlines(value.content) }
      case "hr":
        return { type: "rule" }
      case "video":
        return { type: "video", video: media(value.video)! }
      case "youtube":
        return {
          type: "youtube",
          videoId: value.videoId,
          watchUrl: value.watch,
          video: media(value.video),
          cover: media(value.cover),
          coverSourceUrl: value.coverSource ?? null,
          archiveUrl: value.archive ?? null,
        }
      case "card":
        return {
          type: "card",
          title: value.title,
          url: value.url,
          articleId: articleId(value.url),
          archiveUrl: value.archive ?? null,
          description: value.description ?? null,
          image: value.image ? image(value.image) : null,
        }
      default:
        throw new Error(
          `Unknown block type ${(value as { type: string }).type}`,
        )
    }
  }

  for (const id of sources.files.keys())
    if (!posts.some((post) => post.id === id))
      throw new Error(`Article file ${id} is not in the post index`)
  for (const id of sources.comments.keys())
    if (!sources.files.has(id))
      throw new Error(`Comments file ${id} has no article`)

  const comments = (id: string): ApiComment[] =>
    (sources.comments.get(id) ?? []).map((comment) => ({
      number: comment.number,
      author: comment.author,
      posted: comment.posted,
      body: comment.body,
      rating: comment.rating
        ? { icon: comment.rating.icon, value: comment.rating.value }
        : null,
    }))

  const articles = posts.map((summary) => {
    const file = sources.files.get(summary.id)
    if (!file || file.post.id !== summary.id)
      throw new Error(`Article ${summary.id} has no article file`)
    const { post } = file
    const { newer, older } = neighbours(post.id)
    const archived = comments(post.id)
    return {
      id: post.id,
      post: {
        id: post.id,
        published: post.published,
        title: localized(post),
        url: post.url,
        archiveUrl: post.archive,
        commentCount: archived.length,
        blocks: post.blocks.map(block),
      },
      previousId: older?.id ?? null,
      nextId: newer?.id ?? null,
      comments: archived,
    }
  })

  const instants = [
    ...posts.map((post) => post.published),
    ...articles.flatMap((article) =>
      article.comments.map((comment) => comment.posted),
    ),
  ]
  const updatedAt = instants.map(japanDate).sort().at(-1)
  if (!updatedAt) throw new Error("The archive holds no posts")

  return {
    updatedAt,
    originalBlogUrl: ORIGINAL_BLOG,
    posts: posts.map((summary, index) => ({
      id: summary.id,
      published: summary.published,
      title: localized(summary),
      thumbnail: media(summary.thumbnail),
      commentCount: articles[index].post.commentCount,
    })),
    months: archiveMonths.flatMap((year) =>
      year.months.map((month) => ({ month: month.month, total: month.count })),
    ),
    articles,
  }
}

export function serializeLineblogExport(value: LineblogExport) {
  return `${JSON.stringify(value, null, 2)}\n`
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  try {
    const value = lineblogExport(await readLineblogSources())
    const temporary = `${lineblogExportFile}.tmp`
    await fs.writeFile(temporary, serializeLineblogExport(value))
    await fs.rename(temporary, lineblogExportFile)
    console.log(
      `Exported ${value.posts.length} LINE BLOG articles and ${value.articles.reduce((sum, article) => sum + article.comments.length, 0)} comments.`,
    )
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error))
    process.exitCode = 1
  }
}
