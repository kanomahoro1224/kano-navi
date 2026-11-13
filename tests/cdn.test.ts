import assert from "node:assert/strict"
import test from "node:test"

// The page's DOM, so the meta tag the server writes is visible.
import { browser } from "./dom.ts"
import { cdnBase, cdnUrl } from "../src/lib/cdn.ts"
import { isMediaUrl, mediaSrcSet, mediaVariantUrl } from "../src/lib/media.ts"

const id = "a".repeat(64)
const revision = "b".repeat(64)
const cached = `/media/${id}?v=${revision}`

/** The meta tag backend/internal/cdn writes when CDN_PUBLIC_BASE is set. */
function advertise(content: string) {
  const meta = browser.document.createElement("meta")
  meta.setAttribute("name", "kano-cdn-base")
  meta.setAttribute("content", content)
  browser.document.head.append(meta)
}

test("without an advertised base every path stays at this origin", () => {
  assert.equal(cdnBase(), "")
  assert.equal(cdnUrl(cached), cached)
  assert.equal(cdnUrl("/assets/kano-avatar.jpg"), "/assets/kano-avatar.jpg")
  assert.equal(mediaVariantUrl(cached, 640), `${cached}&w=640`)
  assert.equal(isMediaUrl(cached), true)
})

test("an advertised base serves the media addresses it names", () => {
  advertise("https://cdn.example.invalid/kano/")
  assert.equal(cdnBase(), "https://cdn.example.invalid/kano")
  assert.equal(cdnUrl(cached), `https://cdn.example.invalid/kano${cached}`)
  // Absolute addresses and relative ones are not paths of this site.
  assert.equal(
    cdnUrl("https://pbs.twimg.com/media/x.jpg"),
    "https://pbs.twimg.com/media/x.jpg",
  )
  const onCdn = cdnUrl(cached)
  assert.equal(isMediaUrl(onCdn), true)
  assert.equal(isMediaUrl("/assets/kano-banner.jpg"), false)
  assert.equal(
    mediaVariantUrl(onCdn, 640),
    `https://cdn.example.invalid/kano${cached}&w=640`,
  )
  assert.equal(
    mediaSrcSet(onCdn, 1000),
    [
      `https://cdn.example.invalid/kano${cached}&w=320 320w`,
      `https://cdn.example.invalid/kano${cached}&w=640 640w`,
      `https://cdn.example.invalid/kano${cached}&w=960 960w`,
      `https://cdn.example.invalid/kano${cached} 1000w`,
    ].join(", "),
  )
})
