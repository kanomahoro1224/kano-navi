import assert from "node:assert/strict"
import crypto from "node:crypto"
import fs from "node:fs/promises"
import os from "node:os"
import path from "node:path"
import test, { type TestContext } from "node:test"

import {
  mediaIdForSourceUrl,
  packageHistoryMedia,
  sha256ForContent,
  type HistoryCatalogItem,
} from "../scripts/package-history-media.ts"
import {
  fixedMedia,
  historyMedia,
  mediaByMilestone,
  releaseCovers,
} from "../src/history/media.ts"
import { milestones } from "../src/history/milestones.ts"
import { personalReleases } from "../src/works/content.ts"
import worksMedia from "../src/works/media-catalog.json" with { type: "json" }

async function fixture(t: TestContext) {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "kano-history-test-"))
  const content = Buffer.concat([
    Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j1WQAAAAASUVORK5CYII=",
      "base64",
    ),
    crypto.randomBytes(16),
  ])
  const sourceUrl = "https://images.example.invalid/history.png"
  const mediaId = mediaIdForSourceUrl(sourceUrl)
  assert.ok(mediaId)
  const image: HistoryCatalogItem = {
    id: "fixture-image",
    filename: "history.png",
    sourceUrl,
    mediaId,
    sha256: sha256ForContent(content),
    byteSize: content.length,
    mimeType: "image/png",
  }
  await fs.writeFile(path.join(root, image.filename), content)
  const records = [{ filename: image.filename, file: image.filename }]
  const saveManifest = () =>
    fs.writeFile(
      path.join(root, "manifest.json"),
      JSON.stringify({ images: records }),
    )
  await saveManifest()
  t.after(async () => {
    assert.equal(path.dirname(root), path.resolve(os.tmpdir()))
    assert.ok(path.basename(root).startsWith("kano-history-test-"))
    await fs.rm(root, { recursive: true, force: true })
  })
  return { root, content, image, records, saveManifest }
}

test("history catalog preserves distinct source identities, dates, and milestone references", () => {
  const milestoneIds = new Set(milestones.map((item) => item.id))
  assert.equal(
    new Set(historyMedia.map((item) => item.id)).size,
    historyMedia.length,
  )
  assert.equal(
    new Set(fixedMedia.map((item) => item.mediaId)).size,
    fixedMedia.length,
  )
  for (const item of fixedMedia) {
    assert.equal(item.mediaId, mediaIdForSourceUrl(item.sourceUrl))
    assert.ok(!item.milestoneId || milestoneIds.has(item.milestoneId))
    assert.match(
      item.url,
      /^\/assets\/history\/[a-zA-Z0-9_.-]+\?v=[a-f0-9]{64}$/u,
    )
    assert.ok(item.title.en && item.title.ja && item.title["zh-CN"])
    assert.notEqual(item.category, "cover", item.id)
  }
  const frames = fixedMedia.filter((item) => item.frameTime)
  assert.equal(frames.length, 4)
  assert.ok(
    frames.every(
      (item) =>
        item.sourceKind === "streamArchive" &&
        item.originalStream?.includes("youtube.com/watch?") === true &&
        item.dateKind === "stream",
    ),
  )
  const portrait = fixedMedia.find(
    (item) => item.id === "12_sona_2026_official_fullbody",
  )
  assert.ok(portrait)
  assert.equal(portrait.dateKind, "capture")
  assert.equal(portrait.date, "2026-09-30")
})

test("every fixed history image ships with its catalog bytes and identity", async () => {
  for (const item of fixedMedia) {
    const content = await fs.readFile(
      new URL(`../public/assets/history/${item.filename}`, import.meta.url),
    )
    assert.equal(content.length, item.byteSize, item.id)
    assert.equal(sha256ForContent(content), item.sha256, item.id)
  }
  const shipped = await fs.readdir(
    new URL("../public/assets/history/", import.meta.url),
  )
  assert.deepEqual(
    shipped.sort(),
    fixedMedia.map((item) => item.filename).sort(),
  )
})

test("release milestones show their Works release's packaged covers", () => {
  const packaged = new Set(worksMedia.map((item) => item.name))
  const releases = new Map(personalReleases.map((item) => [item.id, item]))
  const linked = milestones.filter((item) => "release" in item)
  assert.ok(linked.length > 0)
  for (const milestone of linked) {
    assert.ok("release" in milestone)
    const release = releases.get(milestone.release)
    assert.ok(release, milestone.id)
    const covers = releaseCovers.filter(
      (item) => item.milestoneId === milestone.id,
    )
    assert.deepEqual(
      covers.map((item) => item.cover.media),
      release.covers.map((cover) => cover.media),
      milestone.id,
    )
    for (const cover of covers) {
      assert.ok(packaged.has(cover.cover.media), cover.id)
      assert.equal(cover.url, `/media/works/${cover.cover.media}`)
      assert.equal(cover.releaseId, release.id)
      assert.equal(cover.date, release.date)
      assert.ok(cover.branches.includes(milestone.branch))
    }
  }
  assert.ok(
    milestones.every(
      (item) =>
        item.kind !== "release" ||
        "release" in item ||
        !personalReleases.some(
          (release) => release.date === item.date && !release.unit,
        ),
    ),
    "a release milestone on a Works release date names that release",
  )
})

test("the first album and cancelled EP keep their distinct release and publication dates", () => {
  const firstAlbum = milestones.find((item) => item.id === "bambino")
  assert.ok(firstAlbum)
  assert.ok(firstAlbum.source)
  assert.equal(firstAlbum.date, "2011-01-16")
  const [firstCover] = mediaByMilestone.bambino
  assert.ok(firstCover?.origin === "release")
  assert.equal(firstCover.date, firstAlbum.date)
  assert.equal(firstCover.cover.thirdParty, true)
  assert.ok(firstCover.cover.source)
  assert.notEqual(
    new URL(firstAlbum.source).hostname,
    new URL(firstCover.cover.source).hostname,
  )

  const mv = milestones.find((item) => item.id === "ichigo-ichie")
  assert.ok(mv)
  assert.equal(mv.date, "2022-10-07")
  assert.equal(mv.branch, "singer")
  assert.ok("links" in mv)
  assert.ok(
    mv.links?.some(
      (link) => link.label === "history.sourceName.cancelledRelease",
    ),
  )
  const covers = mediaByMilestone[mv.id]
  assert.equal(covers.length, 2)
  for (const cover of covers) {
    assert.ok(cover.origin === "release")
    assert.equal(cover.cancelled, true)
    assert.deepEqual(cover.branches, ["singer", "mklntic"])
  }
})

test("history packaging validates the archive before publishing fixed images", async (t) => {
  const { root, content, image } = await fixture(t)
  const destination = path.join(root, "packaged")
  assert.deepEqual(await packageHistoryMedia(root, destination, [image]), {
    packaged: 1,
    bytes: content.length,
  })
  const packagedFile = path.join(destination, image.filename)
  assert.deepEqual(await fs.readFile(packagedFile), content)
  await fs.writeFile(
    path.join(root, image.filename),
    Buffer.alloc(content.length),
  )
  await assert.rejects(
    packageHistoryMedia(root, destination, [image]),
    /identity mismatch/u,
  )
  assert.deepEqual(await fs.readFile(packagedFile), content)
})

test("history packaging rejects traversal, wrong MIME, and changed size before writing", async (t) => {
  const { root, image, records, saveManifest } = await fixture(t)
  const destination = path.join(root, "packaged")
  records[0].file = "../outside.png"
  await saveManifest()
  await assert.rejects(
    packageHistoryMedia(root, destination, [image]),
    /unsafe archive image path/u,
  )
  records[0].file = image.filename
  await saveManifest()
  await assert.rejects(
    packageHistoryMedia(root, destination, [
      { ...image, mimeType: "image/gif" },
    ]),
    /identity mismatch/u,
  )
  await assert.rejects(
    packageHistoryMedia(root, destination, [
      { ...image, byteSize: image.byteSize + 1 },
    ]),
    /size mismatch/u,
  )
  await assert.rejects(fs.access(destination))
})
