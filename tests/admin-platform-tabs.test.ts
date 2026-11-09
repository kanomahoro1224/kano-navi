import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import type { AdminPost } from "../src/admin/types.ts"
import { browser, click, render, type Rendered } from "./dom.ts"

const post: AdminPost = {
  id: "synthetic-post",
  source: "bilibili",
  accountHandle: "example",
  text: "Synthetic post",
  publishedAt: "2026-01-01T15:00:00Z",
  url: "https://example.invalid/post",
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
}

test("the Posts platform tabs narrow the query, keep the link's preset and move with the arrow keys", async () => {
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
    const { readVideoTab, videoTab } = await vite.ssrLoadModule(
      "/src/admin/views/videos.tsx",
    )
    const { AdminContext } = await vite.ssrLoadModule(
      "/src/admin/use-admin-data.ts",
    )
    const { AppSettingsProvider } = await vite.ssrLoadModule(
      "/src/app-settings.tsx",
    )
    await vite.close()
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true

    // A Videos link may name a platform, a task or both, in either order.
    assert.deepEqual(readVideoTab(""), { source: "all", view: "all" })
    assert.deepEqual(readVideoTab("bilibili"), {
      source: "bilibili",
      view: "all",
    })
    assert.deepEqual(readVideoTab("queued"), { source: "all", view: "queued" })
    assert.deepEqual(readVideoTab("youtube/upcoming"), {
      source: "youtube",
      view: "upcoming",
    })
    assert.equal(videoTab("bilibili", "queued"), "bilibili/queued")
    assert.equal(videoTab("all", "translation"), "translation")
    assert.equal(videoTab("all", "all"), "")

    const queries: Record<string, string | number>[] = []
    const visited: string[] = []
    const context = {
      revision: 0,
      fail: (error: unknown) => {
        throw error
      },
      api: {
        sources: async () => ({ x: { accounts: [{ account: "example" }] } }),
        translations: async () => ({ counts: {} }),
        posts: async (query: Record<string, string | number>) => {
          queries.push({ ...query })
          return { items: [post], page: query.page, total: 61, totalPages: 3 }
        },
      },
    }
    const view = (tab: string) =>
      createElement(
        AppSettingsProvider,
        null,
        createElement(
          AdminContext.Provider,
          { value: context },
          createElement(PostsView, {
            tab,
            navigate: (_view: string, next: string) => visited.push(next),
          }),
        ),
      )
    await act(async () => {
      root = render(view("queued-refresh"))
    })
    const find = (selector: string) => root!.container.querySelector(selector)
    const tabs = () => [
      ...root!.container.querySelectorAll<HTMLElement>('[role="tab"]'),
    ]
    const account = () =>
      [...root!.container.querySelectorAll(".adm-select")].find((select) =>
        select.textContent?.includes("admin.posts.account"),
      ) ?? null
    const latest = () => queries.at(-1)!

    // All platforms: no source, the account filter and the link's preset.
    assert.deepEqual(
      tabs().map((tab) => tab.getAttribute("aria-selected")),
      ["true", "false", "false"],
    )
    assert.equal(latest().source, "")
    assert.equal(latest().queued, "refresh")
    assert.ok(account())
    const panel = find('[role="tabpanel"]')!
    assert.equal(
      panel.getAttribute("aria-labelledby"),
      tabs()[0].getAttribute("id"),
    )

    // A tab keeps the preset in the link it opens.
    await click(tabs()[2])
    assert.equal(visited.at(-1), "bilibili/queued-refresh")
    await act(async () => {
      tabs()[0].focus()
      tabs()[0].dispatchEvent(
        new browser.KeyboardEvent("keydown", {
          key: "ArrowLeft",
          bubbles: true,
        }) as unknown as Event,
      )
    })
    assert.equal(visited.at(-1), "bilibili/queued-refresh")

    // Bilibili sends its source and has no account filter; the list keeps
    // its other filters.
    await act(async () => root!.update(view("bilibili/queued-refresh")))
    assert.equal(latest().source, "bilibili")
    assert.equal(latest().queued, "refresh")
    assert.equal(latest().page, 1)
    assert.equal(account(), null)
    assert.deepEqual(
      tabs().map((tab) => tab.tabIndex),
      [-1, -1, 0],
    )
    await click(find('[aria-label="admin.pager.next"]'))
    assert.equal(latest().page, 2)

    // Changing platform starts the list again at page 1.
    await act(async () => root!.update(view("x/queued-refresh")))
    assert.equal(latest().source, "x")
    assert.equal(latest().page, 1)
    assert.ok(account())
    await click(tabs()[0])
    assert.equal(visited.at(-1), "queued-refresh")
  } finally {
    if (root) await act(async () => root!.unmount())
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previous
    await vite.close()
  }
})
