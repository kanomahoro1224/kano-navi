import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import type { AdminPost } from "../src/admin/types.ts"
import { click, render, type Rendered } from "./dom.ts"

function post(id: string, patch: Partial<AdminPost> = {}): AdminPost {
  return {
    id,
    source: "x",
    accountHandle: "example",
    text: `Synthetic ${id}`,
    publishedAt: "2026-01-01T15:00:00Z",
    url: `https://example.invalid/${id}`,
    mediaUrl: null,
    mediaAlt: null,
    media: [],
    mediaStatus: "none",
    llmStatus: "never",
    llmRoute: null,
    llmLastAttemptAt: null,
    llmLastError: null,
    llmReprocessRequested: false,
    llmRetryAt: null,
    llmRetryStopped: false,
    translation: null,
    quote: null,
    quoteHiddenAt: null,
    refreshRequestedAt: null,
    hiddenAt: null,
    promotedAt: null,
    scheduleImage: null,
    ...patch,
  }
}

test("every own X post in the Posts list refreshes from the source on its own or in a selection", async () => {
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  const previous = (globalThis as any).IS_REACT_ACT_ENVIRONMENT
  let root: Rendered | undefined
  try {
    const { PostsView } = await vite.ssrLoadModule("/src/admin/views/posts.tsx")
    const { AdminContext } = await vite.ssrLoadModule(
      "/src/admin/use-admin-data.ts",
    )
    const { AppSettingsProvider } = await vite.ssrLoadModule(
      "/src/app-settings.tsx",
    )
    await vite.close()
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true

    const items = [
      post("plain"),
      post("queued", { refreshRequestedAt: "2026-01-02T00:00:00Z" }),
      // A quote card keeps only its own actions.
      post("quoting", {
        quote: {
          id: "quoted",
          authorHandle: "other",
          authorName: null,
          text: "Quoted synthetic",
          publishedAt: null,
          url: "https://example.invalid/quoted",
          unavailable: false,
          media: [],
        },
      }),
      post("promoted", { promotedAt: "2026-01-02T00:00:00Z" }),
      post("dynamic", { source: "bilibili" }),
    ]
    const calls: string[] = []
    const notices: string[] = []
    const context = {
      revision: 0,
      act: async (call: () => Promise<unknown>) => call(),
      notify: (key: string, values?: { count?: number }) =>
        notices.push(`${key}:${values?.count}`),
      fail: (error: unknown) => {
        throw error
      },
      api: {
        sources: async () => ({ x: { accounts: [{ account: "example" }] } }),
        translations: async () => ({ counts: {} }),
        posts: async () => ({
          items,
          page: 1,
          total: items.length,
          totalPages: 1,
        }),
        refreshPost: async (id: string) => {
          calls.push(`refresh:${id}`)
          return { state: {} }
        },
        refreshPosts: async (ids: string[]) => {
          calls.push(`refresh:${ids.join(",")}`)
          return { queued: ids.length }
        },
      },
    }
    await act(async () => {
      root = render(
        createElement(
          AppSettingsProvider,
          null,
          createElement(
            AdminContext.Provider,
            { value: context },
            createElement(PostsView, { tab: "", navigate: () => {} }),
          ),
        ),
      )
    })
    const rows = () => [
      ...root!.container.querySelectorAll<HTMLElement>(".adm-post"),
    ]
    const row = (id: string) =>
      rows().find((item) => item.textContent?.includes(`Synthetic ${id}`))!
    const refresh = (id: string) =>
      row(id).querySelector<HTMLButtonElement>(
        '.adm-post-controls [aria-label="admin.posts.refresh"]',
      )
    const pick = (id: string) =>
      row(id).querySelector<HTMLInputElement>(".adm-post-select input")
    const bar = () => root!.container.querySelector(".adm-select-bar")!

    // Own X posts carry the action whether or not they quote; a promoted
    // status and a Bilibili dynamic do not.
    assert.equal(rows().length, 5)
    assert.ok(refresh("plain"))
    assert.ok(refresh("quoting"))
    assert.equal(refresh("promoted"), null)
    assert.equal(refresh("dynamic"), null)
    assert.equal(
      row("quoting").querySelector(
        '.adm-post-quote [aria-label="admin.posts.refresh"]',
      ),
      null,
    )
    // A queued refresh shows its badge and cannot be queued twice.
    assert.equal(refresh("queued")!.disabled, true)
    assert.match(row("queued").textContent!, /admin\.posts\.refreshQueued/)
    assert.doesNotMatch(
      row("plain").textContent!,
      /admin\.posts\.refreshQueued/,
    )
    await click(refresh("plain"))
    assert.deepEqual(calls, ["refresh:plain"])

    // Only own X posts can be picked; the bar's box picks all of them and its
    // action queues them in one request.
    assert.equal(pick("promoted"), null)
    assert.equal(pick("dynamic"), null)
    const action = () =>
      [...bar().querySelectorAll("button")].find((item) =>
        item.textContent?.includes("admin.posts.refresh"),
      )!
    assert.equal(action().disabled, true)
    await click(bar().querySelector("input"))
    assert.deepEqual(
      ["plain", "queued", "quoting"].map((id) => pick(id)!.checked),
      [true, true, true],
    )
    await click(action())
    assert.equal(calls.at(-1), "refresh:plain,queued,quoting")
    assert.equal(notices.at(-1), "admin.notice.refreshesQueued:3")
    assert.equal(pick("plain")!.checked, false)
  } finally {
    if (root) await act(async () => root!.unmount())
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previous
    await vite.close()
  }
})
