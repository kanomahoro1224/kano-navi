import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import { browser, click, render, type Rendered } from "./dom.ts"

const overview = {
  activeRun: null,
  recentRuns: [],
  scheduler: { enabled: false, running: false, tickSeconds: 30 },
  timers: { scheduled: 0, nextRunAt: null },
  sources: Object.fromEntries(
    ["x", "x_related", "youtube", "bilibili", "bilibili_dynamics", "shop"].map(
      (source) => [
        source,
        { skipped: true, lastSuccessAt: null, lastStep: null },
      ],
    ),
  ),
  review: {
    pendingImages: 0,
    suspectedCancellations: 0,
    unlistedEvents: 0,
    uncertainEvents: 0,
    bilibiliForms: 0,
    failedPosts: 0,
    failedExtractions: 0,
    failedTranslations: 0,
    videoMerges: 0,
  },
  backlog: {
    extraction: 0,
    postRefresh: 0,
    postTranslation: 0,
    videoRefresh: 0,
    videoTranslation: 0,
  },
  counts: { upcomingEvents: 0, events: 0, posts: 0, videos: 0 },
}

const worksCovers = {
  total: 0,
  covered: 0,
  due: 0,
  waiting: 0,
  uncovered: 0,
  skipped: false,
  checkedAt: "2026-03-02T03:00:00.000Z",
  uploads: [],
}

function installBrowser(hash: string) {
  const previous = {
    fetch: globalThis.fetch,
    act: (globalThis as any).IS_REACT_ACT_ENVIRONMENT,
  }
  browser.happyDOM.setURL(new URL("/admin" + hash, "https://example.test").href)
  browser.localStorage.clear()
  ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
  return { previous }
}

function restoreBrowser(
  previous: ReturnType<typeof installBrowser>["previous"],
) {
  globalThis.fetch = previous.fetch
  ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previous.act
}

test("the phone menu names the current section with the counts and opens every group", async () => {
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  const installed = installBrowser("#overview")
  globalThis.fetch = async (input) => {
    const path = String(input).replace(/^\/api\/admin/, "")
    const payload =
      path === "/session"
        ? {
            authenticated: true,
            mode: "development",
            requiresPassword: false,
            requiresCode: false,
          }
        : path === "/overview"
          ? {
              ...overview,
              alerts: { open: 1 },
              review: { ...overview.review, failedPosts: 2 },
            }
          : path === "/media/works-covers"
            ? worksCovers
            : {}
    return new Response(JSON.stringify(payload), {
      status: 200,
      headers: { "content-type": "application/json" },
    })
  }
  let root: Rendered | undefined
  try {
    const [{ AdminApp }, { AppSettingsProvider, PageMessagesProvider }, copy] =
      await Promise.all([
        vite.ssrLoadModule("/src/admin/index.tsx"),
        vite.ssrLoadModule("/src/app-settings.tsx"),
        vite.ssrLoadModule("/src/admin-i18n.ts"),
      ])
    await act(async () => {
      root = render(
        createElement(
          AppSettingsProvider,
          null,
          createElement(
            PageMessagesProvider,
            { messages: copy.adminMessages },
            createElement(AdminApp),
          ),
        ),
      )
      await new Promise((resolve) => setImmediate(resolve))
    })
    const trigger = () =>
      root!.container.querySelector<HTMLButtonElement>(".adm-nav-trigger")!
    // The button names the view and, once the overview arrives, carries the
    // Review queue's and Alerts' counts together.
    for (let tick = 0; tick < 20 && !trigger().querySelector("em"); tick++)
      await act(async () => {
        await new Promise((resolve) => setImmediate(resolve))
      })
    assert.equal(trigger().querySelector("strong")?.textContent, "Overview")
    assert.equal(trigger().querySelector("em")?.textContent, "3")
    assert.equal(trigger().getAttribute("aria-expanded"), "false")
    const sheet = () =>
      root!.container.querySelector<HTMLDialogElement>("dialog.adm-nav-sheet")!
    assert.equal(sheet().open, false)

    await click(trigger())
    assert.equal(sheet().open, true)
    assert.equal(trigger().getAttribute("aria-expanded"), "true")
    assert.deepEqual(
      [...sheet().querySelectorAll(".adm-nav-group > p")].map(
        (group) => group.textContent,
      ),
      ["Today", "Content", "Pipeline", "System"],
    )
    const items = [...sheet().querySelectorAll(".adm-nav-item")]
    assert.equal(items.length, 15)
    assert.equal(
      sheet().querySelector(".adm-nav-item.is-active span")?.textContent,
      "Overview",
    )

    // Picking a section opens it and closes the sheet.
    await act(async () => {
      await click(items.find((item) => item.textContent === "Works"))
      await new Promise((resolve) => setImmediate(resolve))
    })
    assert.equal(browser.location.hash, "#works")
    assert.equal(sheet().open, false)
    assert.equal(trigger().querySelector("strong")?.textContent, "Works")
  } finally {
    await act(async () => root?.unmount())
    restoreBrowser(installed.previous)
    await vite.close()
  }
})
