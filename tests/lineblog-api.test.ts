import assert from "node:assert/strict"
import fs from "node:fs/promises"
import test from "node:test"

import {
  type ApiBlock,
  type ApiImage,
  type ApiInline,
  lineblogExport,
  lineblogExportFile,
  readLineblogSources,
  serializeLineblogExport,
} from "../scripts/export-lineblog.ts"
import { neighbours, postMonth, posts } from "../src/lineblog/content.ts"

const exported = lineblogExport(await readLineblogSources())

function images(content: ApiInline[]): ApiImage[] {
  return content.flatMap((inline) =>
    inline.type === "image"
      ? [inline]
      : inline.type === "link"
        ? images(inline.content)
        : [],
  )
}

function blockImages(block: ApiBlock): ApiImage[] {
  if (block.type === "paragraph")
    return [
      ...images(block.content),
      ...[block.translations["zh-CN"], block.translations.en].flatMap(
        (paragraphs) => (paragraphs ?? []).flatMap(images),
      ),
    ]
  if (block.type === "quote") return images(block.content)
  if (block.type === "card" && block.image) return [block.image]
  return []
}

test("the server's LINE BLOG archive matches the tracked page data", async () => {
  assert.equal(
    await fs.readFile(lineblogExportFile, "utf8"),
    serializeLineblogExport(exported),
    "backend/internal/lineblog/archive.json is out of date; run npm run lineblog:export",
  )
})

test("the API lists the page's articles, months and neighbours", () => {
  assert.deepEqual(
    exported.posts.map((post) => post.id),
    posts.map((post) => post.id),
  )
  assert.deepEqual(
    exported.articles.map((article) => article.id),
    posts.map((post) => post.id),
  )
  assert.equal(
    exported.months.reduce((sum, month) => sum + month.total, 0),
    posts.length,
  )
  for (const month of exported.months)
    assert.equal(
      month.total,
      posts.filter((post) => postMonth(post.published) === month.month).length,
    )
  for (const article of exported.articles) {
    const { newer, older } = neighbours(article.id)
    assert.equal(article.previousId, older?.id ?? null)
    assert.equal(article.nextId, newer?.id ?? null)
  }
  assert.equal(exported.articles.at(-1)?.previousId, null)
  assert.equal(exported.articles[0].nextId, null)
  exported.posts.forEach((post, index) => {
    assert.equal(post.commentCount, exported.articles[index].comments.length)
    assert.deepEqual(post.title, exported.articles[index].post.title)
  })
  assert.match(exported.updatedAt, /^\d{4}-\d{2}-\d{2}$/u)
})

test("the API is locale-neutral and serves media from this host", () => {
  const text = JSON.stringify(exported)
  for (const post of exported.posts) {
    assert.deepEqual(Object.keys(post.title), ["ja", "zh-CN", "en"])
    assert.ok(post.title.ja)
  }
  // Every media object points at the archive route with its type.
  for (const match of text.matchAll(/"url":"(\/media\/[^"]*)"/gu))
    assert.match(
      match[1],
      /^\/media\/lineblog\/[0-9a-f]{12}\.(?:jpg|png|mp4)$/u,
    )
  const allImages = exported.articles.flatMap((article) =>
    article.post.blocks.flatMap(blockImages),
  )
  for (const image of allImages) {
    // An image is either archived or keeps only its original address.
    assert.ok(image.image !== null || image.remoteUrl !== null)
    if (image.image)
      assert.ok(image.image.width !== null && image.image.mimeType)
  }
  for (const article of exported.articles)
    for (const block of article.post.blocks) {
      if (block.type === "paragraph")
        for (const paragraphs of [
          block.translations["zh-CN"],
          block.translations.en,
        ])
          assert.ok(
            paragraphs === null ||
              paragraphs.every((paragraph) => Array.isArray(paragraph)),
          )
      if (block.type === "youtube")
        assert.ok(block.watchUrl.startsWith("https://www.youtube.com/"))
    }
  for (const article of exported.articles)
    for (const comment of article.comments)
      assert.ok(
        comment.rating === null ||
          (["star", "face"].includes(comment.rating.icon) &&
            comment.rating.value >= 1 &&
            comment.rating.value <= 5),
      )
})
