import assert from "node:assert/strict"
import fs from "node:fs/promises"
import path from "node:path"
import test from "node:test"
import { isDeepStrictEqual } from "node:util"

import {
  journeyApiBody,
  journeyApiDirectory,
  journeyApiFiles,
} from "../scripts/export-journey-api.ts"
import { apiValue } from "../scripts/export-works-api.ts"
import { historyMedia } from "../src/history/media.ts"
import { milestones } from "../src/history/milestones.ts"

// The API serves the export committed into the backend; the page reads the
// same src/history and src/works data. Regenerate with npm run journey:export.
for (const [name, build] of Object.entries(journeyApiFiles)) {
  test(`backend/internal/journey/${name} matches the Journey data`, async () => {
    const committed = await fs.readFile(
      path.join(journeyApiDirectory, name),
      "utf8",
    )
    assert.ok(
      isDeepStrictEqual(JSON.parse(committed), apiValue(build())),
      `backend/internal/journey/${name} is stale: run npm run journey:export`,
    )
  })
}

test("journey API keeps the page's milestones and every image", () => {
  const body = journeyApiBody()
  assert.deepEqual(
    body.milestones.map((item) => item.id),
    milestones.map((item) => item.id),
  )
  const images = [
    ...body.milestones.flatMap((item) => item.media),
    ...body.galleryOnly,
  ]
  assert.deepEqual(
    images.map((item) => item.id).sort(),
    historyMedia.map((item) => item.id).sort(),
  )
  const release = images.find((item) => item.origin === "release")
  assert.ok(release?.cover && release.releaseId)
  assert.equal(release.cover.url, release.url)
  for (const milestone of body.milestones)
    for (const link of milestone.links)
      assert.doesNotMatch(link.label, /\./u, "link labels are plain codes")
})
