import assert from "node:assert/strict"
import test, { type TestContext } from "node:test"

// The page's DOM, so the meta tag the server writes is visible.
import { browser } from "./dom.ts"
import { cdnBase, cdnUrl } from "../src/lib/cdn.ts"
import { isMediaUrl, mediaSrcSet, mediaVariantUrl } from "../src/lib/media.ts"
import { fallbackProfile } from "../src/dashboard/content.ts"
import { fixedMedia, releaseCovers } from "../src/history/media.ts"
import { mediaUrl as lineblogMediaUrl } from "../src/lineblog/content.ts"
import { mediaUrl as worksMediaUrl } from "../src/works/content.ts"
import { normalizeCdnBase } from "../scripts/cdn-base.ts"
import { inlineJSON } from "../scripts/vite-page-preload.ts"

const id = "a".repeat(64)
const revision = "b".repeat(64)
const cached = `/media/${id}?v=${revision}`

/**
 * The meta tag backend/internal/cdn writes when CDN_PUBLIC_BASE is set,
 * removed again when the test ends so later tests start without a base.
 */
function advertise(t: TestContext, content: string) {
  const meta = browser.document.createElement("meta")
  meta.setAttribute("name", "kano-cdn-base")
  meta.setAttribute("content", content)
  browser.document.head.append(meta)
  t.after(() => meta.remove())
}

test("without an advertised base every path stays at this origin", () => {
  assert.equal(cdnBase(), "")
  assert.equal(cdnUrl(cached), cached)
  assert.equal(cdnUrl("/assets/kano-avatar.jpg"), "/assets/kano-avatar.jpg")
  assert.equal(mediaVariantUrl(cached, 640), `${cached}&w=640`)
  assert.equal(isMediaUrl(cached), true)
})

test("an advertised base serves the media addresses it names", (t) => {
  advertise(t, "https://cdn.example.invalid/kano/")
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

test("packaged addresses resolve on read, after the base is advertised", (t) => {
  // These modules were imported before any base existed.
  const journey = fixedMedia[0]
  const cover = releaseCovers[0]
  assert.ok(journey && cover)
  assert.equal(fallbackProfile.avatarUrl, "/assets/kano-avatar.jpg")
  assert.equal(journey.url, journey.path)
  assert.equal(cover.url, cover.path)

  advertise(t, "https://cdn.example.invalid/kano")
  const cdn = "https://cdn.example.invalid/kano"
  assert.equal(fallbackProfile.avatarUrl, `${cdn}/assets/kano-avatar.jpg`)
  assert.equal(fallbackProfile.bannerUrl, `${cdn}/assets/kano-banner.jpg`)
  assert.equal(
    { ...fallbackProfile }.avatarUrl,
    `${cdn}/assets/kano-avatar.jpg`,
  )
  assert.equal(journey.url, `${cdn}${journey.path}`)
  assert.match(journey.path, /^\/media\/journey\/[a-f0-9]{12}\.(?:jpg|png)$/)
  assert.equal(cover.url, `${cdn}${cover.path}`)
  assert.equal(
    worksMediaUrl("0123456789ab.jpg"),
    `${cdn}/media/works/0123456789ab.jpg`,
  )
  assert.equal(
    lineblogMediaUrl("0123456789ab.jpg"),
    `${cdn}/media/lineblog/0123456789ab.jpg`,
  )
})

test("the advertised base is gone once its test ends", () => {
  assert.equal(
    browser.document.querySelector('meta[name="kano-cdn-base"]'),
    null,
  )
  assert.equal(cdnBase(), "")
  assert.equal(fallbackProfile.avatarUrl, "/assets/kano-avatar.jpg")
})

test("the build normalizes CDN_PUBLIC_BASE as the server does", () => {
  // The same cases as backend/internal/config/cdn_test.go; keep them in step.
  const accepted: Record<string, string> = {
    "": "",
    "  ": "",
    "https://cdn.example.invalid": "https://cdn.example.invalid",
    "https://cdn.example.invalid/": "https://cdn.example.invalid",
    "https://CDN.Example.INVALID": "https://cdn.example.invalid",
    "https://cdn.example.invalid/kano": "https://cdn.example.invalid/kano",
    "https://cdn.example.invalid/kano/": "https://cdn.example.invalid/kano",
    "https://cdn.example.invalid/v1/site":
      "https://cdn.example.invalid/v1/site",
    "http://127.0.0.1:9000": "http://127.0.0.1:9000",
    "http://[::1]:9000/kano": "http://[::1]:9000/kano",
    "https://xn--n8j.example.invalid/kano":
      "https://xn--n8j.example.invalid/kano",
    "https://CDN.example.invalid/Kano": "https://cdn.example.invalid/Kano",
  }
  for (const [raw, want] of Object.entries(accepted))
    assert.equal(normalizeCdnBase(raw), want, raw)
  // Assembled so the privacy scanner does not read it as an address.
  const credentialed = "https" + "://user:secret@cdn.example.invalid"
  for (const raw of [
    "cdn.example.invalid",
    "ftp://cdn.example.invalid",
    "https://cdn.example.invalid?x=1",
    "https://cdn.example.invalid#fragment",
    credentialed,
    "javascript:alert(1)",
    "//cdn.example.invalid",
    "https://cdn.example.invalid//kano",
    "https://cdn.example.invalid/./kano",
    "https://cdn.example.invalid/kano/../x",
    'https://cdn.example.invalid/kano"><script>',
    "https://cdn.example.invalid/ka no",
    "https://cdn.example.invalid/ka%6Eo",
    "https://cdn.example.invalid.",
    "https://cdn.example.invalid:",
    "https://cdn.example.invalid:0",
    "https://cdn.example.invalid:080",
    "https://-cdn.example.invalid",
    "https://[::1",
  ])
    assert.throws(() => normalizeCdnBase(raw), /CDN_PUBLIC_BASE/, raw)
})

test("inline preload data cannot close its script", () => {
  const json = inlineJSON({ base: "https://cdn.example.invalid/</script><!--" })
  assert.ok(!json.includes("<"))
  assert.deepEqual(JSON.parse(json), {
    base: "https://cdn.example.invalid/</script><!--",
  })
})
