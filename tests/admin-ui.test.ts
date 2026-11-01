import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement, useState } from "react"
import { createServer } from "vite"
import { browser, click, enter, render, submit, type Rendered } from "./dom.ts"

const overview = {
  activeRun: null,
  recentRuns: [],
  scheduler: { enabled: false, running: false, tickSeconds: 30 },
  timers: { scheduled: 0, nextRunAt: null },
  sources: {
    x: { skipped: true, lastSuccessAt: null, lastStep: null },
    youtube: { skipped: true, lastSuccessAt: null, lastStep: null },
    bilibili: { skipped: true, lastSuccessAt: null, lastStep: null },
  },
  review: {
    pendingImages: 0,
    suspectedCancellations: 0,
    failedPosts: 0,
    reprocessRequested: 0,
    videoMerges: 0,
  },
  counts: { upcomingEvents: 0, events: 0, posts: 0, videos: 0 },
}

const visitorRegion = {
  enabled: false,
  providerName: "",
  providerUrl: "",
  mirrorUrl: "",
}

const variantStatus = {
  checkedAt: "2026-03-02T03:00:00.000Z",
  groups: [
    { group: "lineblog", originals: 4, ready: 4, unhashed: 0, widths: [] },
    { group: "history", originals: 3, ready: 1, unhashed: 2, widths: [] },
    { group: "media", originals: 0, ready: 0, unhashed: 0, widths: [] },
  ],
}

function syncSettings() {
  return {
    settings: {
      xBootstrapDays: 7,
      xDiscoveryLimit: 50,
      xMaxStatusRequests: 12,
      xRefreshKnown: 1,
      youtubeMaxDetailRequests: 12,
      fetchMaxPages: 4,
      fetchTimeoutMs: 7000,
      fetchRequestDelayMs: 0,
      translationScanLimit: 20,
      translationLookbackDays: 30,
      llmModelAttempts: 3,
    },
    fields: [
      ["xBootstrapDays", 1, 30],
      ["xDiscoveryLimit", 1, 200],
      ["xMaxStatusRequests", 0, 50],
      ["xRefreshKnown", 0, 5],
      ["youtubeMaxDetailRequests", 1, 50],
      ["fetchMaxPages", 1, 20],
      ["fetchTimeoutMs", 1000, 60000],
      ["fetchRequestDelayMs", 0, 10000],
      ["translationScanLimit", 1, 200],
      ["translationLookbackDays", 1, 3650],
      ["llmModelAttempts", 1, 10],
    ].map(([field, min, max]) => ({ field, default: min, min, max })),
  }
}

function installBrowser(hash: string, pathname = "/admin") {
  const previous = {
    fetch: globalThis.fetch,
    act: (globalThis as any).IS_REACT_ACT_ENVIRONMENT,
  }
  browser.happyDOM.setURL(new URL(pathname + hash, "https://example.test").href)
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

test("the real admin Site route saves visitor-region settings and makes image copies", async () => {
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  const installed = installBrowser("#site")
  const requests: { path: string; method: string; body?: any }[] = []
  const saved = {
    enabled: true,
    providerName: "Synthetic provider",
    providerUrl: "https://provider.example.test",
    mirrorUrl: "https://mirror.example.test",
  }
  globalThis.fetch = async (input, options: RequestInit = {}) => {
    const url = String(input)
    const path = url.replace(/^\/api\/admin/, "")
    const method = options.method || "GET"
    const body = options.body ? JSON.parse(String(options.body)) : undefined
    requests.push({ path, method, body })
    const payload =
      method === "PUT" && path === "/settings/visitor-region"
        ? body
        : path === "/session"
          ? {
              authenticated: true,
              mode: "development",
              requiresPassword: false,
            }
          : path === "/overview"
            ? overview
            : path === "/settings/visitor-region"
              ? visitorRegion
              : path.startsWith("/media/variants")
                ? variantStatus
                : method === "POST" && path === "/runs"
                  ? { run: { id: "synthetic-run" } }
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
    assert.match(root!.container.textContent, /Mainland China site suggestion/)
    const name = root!.container.querySelector(
      'input[placeholder="e.g. a provider"]',
    )
    const [providerUrl, mirrorUrl] = root!.container.querySelectorAll(
      'input[placeholder="https://"]',
    )
    assert.ok(name)
    assert.ok(providerUrl)
    assert.ok(mirrorUrl)
    await enter(name, "New provider")
    await enter(providerUrl, "https://provider.example.test")
    await enter(mirrorUrl, "https://mirror.example.test")
    const switchInput = root!.container.querySelector('[role="switch"]')
    assert.equal((switchInput as HTMLInputElement | null)?.checked, false)
    await click(switchInput)
    await submit(root!.container.querySelector("form"))
    const update = requests.find(
      (request) =>
        request.method === "PUT" && request.path === "/settings/visitor-region",
    )
    assert.deepEqual(update?.body, {
      ...saved,
      providerName: "New provider",
    })
    // The copies card shows each group's ready images; Check again asks for
    // a new listing and Make missing copies queues the variants run.
    const text = root!.container.textContent
    assert.match(text, /LINE BLOG archive4\/ 4 imagesAll copies made/)
    assert.match(text, /Journey images1\/ 3 images2 without copies/)
    const button = (label: string) =>
      [...root!.container.querySelectorAll("button")].find(
        (element) => element.textContent === label,
      )
    await click(button("Check again"))
    assert.ok(
      requests.some(
        (request) =>
          request.method === "GET" &&
          request.path === "/media/variants?refresh=1",
      ),
    )
    await click(button("Make missing copies"))
    const run = requests.find(
      (request) => request.method === "POST" && request.path === "/runs",
    )
    assert.deepEqual(run?.body, { kind: "variants" })
  } finally {
    await act(async () => root?.unmount())
    restoreBrowser(installed.previous)
    await vite.close()
  }
})

test("admin sync-setting drafts survive a save in the other Sources column", async () => {
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  const installed = installBrowser("#sources")
  let root: Rendered | undefined
  try {
    const [{ AdminContext }, { AppSettingsProvider }, { SyncSettingsForm }] =
      await Promise.all([
        vite.ssrLoadModule("/src/admin/use-admin-data.ts"),
        vite.ssrLoadModule("/src/app-settings.tsx"),
        vite.ssrLoadModule("/src/admin/views/sync-settings.tsx"),
      ])
    let current = syncSettings()
    const api = {
      updateSyncSettings: async (body: Record<string, number>) => {
        const settings = { ...current.settings }
        for (const [field, value] of Object.entries(body))
          settings[field as keyof typeof settings] = value
        current = { ...current, settings }
        return current
      },
    }
    const admin = {
      api,
      act: async (work: () => Promise<unknown>) => work(),
      revision: 0,
      fail() {},
    }
    function Harness() {
      const [data, setData] = useState(current)
      return createElement(
        AdminContext.Provider,
        { value: admin as any },
        createElement(
          "div",
          null,
          createElement(SyncSettingsForm, {
            group: "x",
            section: "timed",
            data,
            onSaved: setData,
          }),
          createElement(SyncSettingsForm, {
            group: "x",
            section: "manual",
            data,
            onSaved: setData,
            rows: true,
          }),
        ),
      )
    }
    await act(async () => {
      root = render(
        createElement(AppSettingsProvider, null, createElement(Harness)),
      )
      await new Promise((resolve) => setImmediate(resolve))
    })
    const inputs = () => root!.container.querySelectorAll("input")
    const timed = () => inputs()[0]
    const manual = () => inputs()[4]
    assert.equal(manual().value, "4")
    assert.equal(timed().value, "12")
    await enter(manual(), "9")
    await enter(timed(), "20")
    await submit(root!.container.querySelector("form"))
    assert.equal(manual().value, "9")
  } finally {
    await act(async () => root?.unmount())
    restoreBrowser(installed.previous)
    await vite.close()
  }
})

test("the public visitor-region toast follows the persisted CN response", async () => {
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  const installed = installBrowser("", "/")
  globalThis.fetch = async () =>
    new Response(
      JSON.stringify({
        enabled: true,
        mainlandChina: true,
        providerName: "Synthetic provider",
        providerUrl: "https://provider.example.test",
        mirrorUrl: "https://mirror.example.test",
      }),
      { status: 200, headers: { "content-type": "application/json" } },
    )
  let root: Rendered | undefined
  try {
    const [{ VisitorRegionToast }, { AppSettingsProvider }] = await Promise.all(
      [
        vite.ssrLoadModule("/src/components/visitor-region-toast.tsx"),
        vite.ssrLoadModule("/src/app-settings.tsx"),
      ],
    )
    await act(async () => {
      root = render(
        createElement(
          AppSettingsProvider,
          null,
          createElement(VisitorRegionToast),
        ),
      )
      await new Promise((resolve) => setImmediate(resolve))
    })
    const links = [...root!.container.querySelectorAll("a")]
    assert.deepEqual(
      links.map((link) => link.getAttribute("href")),
      ["https://provider.example.test", "https://mirror.example.test"],
    )
    assert.match(root!.container.textContent, /Synthetic provider/)
    await click(root!.container.querySelector("button"))
    assert.equal(root!.container.querySelectorAll("aside").length, 0)
  } finally {
    await act(async () => root?.unmount())
    restoreBrowser(installed.previous)
    await vite.close()
  }
})
