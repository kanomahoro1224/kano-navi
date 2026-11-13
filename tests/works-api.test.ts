import assert from "node:assert/strict"
import fs from "node:fs/promises"
import path from "node:path"
import test from "node:test"
import { isDeepStrictEqual } from "node:util"

import {
  apiValue,
  worksApiDirectory,
  worksApiFiles,
  worksApiImage,
  worksApiText,
} from "../scripts/export-works-api.ts"
import worksMedia from "../src/works/media-catalog.json" with { type: "json" }

// The API serves the export committed into the backend; the page reads the
// same src/works data. Regenerate with npm run works:export.
for (const [name, build] of Object.entries(worksApiFiles)) {
  test(`backend/internal/works/${name} matches the Works data`, async () => {
    const committed = await fs.readFile(
      path.join(worksApiDirectory, name),
      "utf8",
    )
    assert.ok(
      isDeepStrictEqual(JSON.parse(committed), apiValue(build())),
      `backend/internal/works/${name} is stale: run npm run works:export`,
    )
  })
}

test("works API text renders phrases in every locale", () => {
  assert.equal(worksApiText(""), null)
  assert.equal(worksApiText([]), null)
  assert.equal(worksApiText(undefined), null)
  assert.deepEqual(worksApiText(["初回限定盤", { phrase: "cover" }]), {
    ja: "初回限定盤ジャケット",
    "zh-CN": "初回限定盤封面",
    en: "初回限定盤 cover",
  })
  assert.deepEqual(worksApiText("缶バッジ"), {
    ja: "缶バッジ",
    "zh-CN": "缶バッジ",
    en: "缶バッジ",
  })
})

test("works API images carry the catalog type and provenance", () => {
  const { name, width, height, mimeType } = worksMedia[0]
  assert.deepEqual(
    worksApiImage({
      media: name,
      width,
      height,
      caption: [{ phrase: "disc" }],
      sourceLabel: "Example",
      credit: { role: "scan", name: "Example" },
      thirdParty: true,
    }),
    {
      url: `/media/works/${name}`,
      width,
      height,
      mimeType,
      caption: worksApiText([{ phrase: "disc" }]),
      sourceUrl: null,
      sourceLabel: "Example",
      credit: { role: "scan", name: "Example", url: null },
      thirdParty: true,
    },
  )
  // A cutout has no provenance of its own.
  assert.equal(worksApiImage({ media: name, width, height }).thirdParty, null)
  assert.throws(
    () => worksApiImage({ media: "000000000000.png", width: 1, height: 1 }),
    /not in the Works media catalog/u,
  )
})
