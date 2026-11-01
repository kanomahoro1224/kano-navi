import assert from "node:assert/strict"
import test from "node:test"
import { mainlandSite } from "../src/lib/visitor-region.ts"

const configured = {
  enabled: true,
  mainlandChina: false,
  providerName: " Example Host ",
  providerUrl: "https://host.example.invalid/",
  mirrorUrl: "https://mirror.example.invalid/",
}

test("an enabled site is offered to every visitor, with the region flag", () => {
  assert.deepEqual(mainlandSite(configured), {
    mainlandChina: false,
    providerName: "Example Host",
    providerUrl: "https://host.example.invalid/",
    mirrorUrl: "https://mirror.example.invalid/",
  })
  assert.equal(
    mainlandSite({ ...configured, mainlandChina: true })?.mainlandChina,
    true,
  )
})

test("a disabled or incomplete site is not offered", () => {
  assert.equal(mainlandSite({ enabled: false, mainlandChina: false }), null)
  assert.equal(mainlandSite({ ...configured, enabled: false }), null)
  assert.equal(mainlandSite({ ...configured, providerName: "  " }), null)
  assert.equal(mainlandSite({ ...configured, providerUrl: undefined }), null)
  assert.equal(mainlandSite({ ...configured, mirrorUrl: 42 }), null)
  assert.equal(mainlandSite(null), null)
  assert.equal(mainlandSite("enabled"), null)
})
