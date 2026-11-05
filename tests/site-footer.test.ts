import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import { click, render, type Rendered } from "./dom.ts"

test("the footer's switch opens the page's notes, and it links About and the top", async () => {
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  const previousAct = (globalThis as any).IS_REACT_ACT_ENVIRONMENT
  ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
  let root: Rendered | undefined
  try {
    const [{ SiteFooter }, { AppSettingsProvider }] = await Promise.all([
      vite.ssrLoadModule("/src/site/site-shell.tsx"),
      vite.ssrLoadModule("/src/app-settings.tsx"),
    ])
    await act(async () => {
      root = render(
        createElement(
          AppSettingsProvider,
          null,
          createElement(SiteFooter, {
            disclaimer: "Synthetic page notes",
            disclaimerLabel: "Synthetic label",
          }),
        ),
      )
    })
    const footer = root!.container.querySelector("footer.site-footer")!

    // The unaffiliated line stays outside the fold; the notes sit in it.
    assert.match(footer.textContent || "", /Not affiliated/)
    const toggle = footer.querySelector("button.site-footer-toggle")!
    assert.equal(toggle.textContent, "Synthetic label")
    const notes = root!.container.ownerDocument.getElementById(
      toggle.getAttribute("aria-controls")!,
    )!
    assert.match(notes.textContent || "", /Synthetic page notes/)
    assert.equal(toggle.getAttribute("aria-expanded"), "false")
    assert.ok(notes.hidden)

    await click(toggle)
    assert.equal(toggle.getAttribute("aria-expanded"), "true")
    assert.ok(!notes.hidden)

    // The header holds the pages; the footer links only About and the top.
    assert.deepEqual(
      [...footer.querySelectorAll(".site-footer-links a")].map((link) =>
        link.getAttribute("href"),
      ),
      ["/about", "#top"],
    )
  } finally {
    await act(async () => root?.unmount())
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previousAct
    await vite.close()
  }
})
