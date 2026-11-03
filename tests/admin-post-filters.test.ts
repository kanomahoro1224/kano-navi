import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import type { AdminPost } from "../src/admin/types.ts"
import { click, enter, render, submit, type Rendered } from "./dom.ts"

test("post date and queue filters reach server queries, survive pagination and clear independently", async () => {
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
    const post: AdminPost = {
      id: "synthetic-post",
      source: "x",
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
      scheduleImage: null,
    }
    const queries: Record<string, string | number>[] = []
    const context = {
      revision: 0,
      fail: (error: unknown) => {
        throw error
      },
      api: {
        sources: async () => undefined,
        translations: async () => ({ counts: {} }),
        posts: async (query: Record<string, string | number>) => {
          queries.push({ ...query })
          return { items: [post], page: query.page, total: 61, totalPages: 3 }
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
            createElement(PostsView, {
              tab: "queued-refresh",
              navigate: () => {},
            }),
          ),
        ),
      )
    })
    const find = (selector: string) => root!.container.querySelector(selector)
    const findAll = (selector: string) => [
      ...root!.container.querySelectorAll(selector),
    ]
    const input = (key: string) => find(`[aria-label="${key}"]`)
    const change = (key: string, value: string) => enter(input(key)!, value)
    const latest = () => queries.at(-1)!

    // A queue link from the overview opens the list on that kind of request.
    assert.equal(queries[0].queued, "refresh")

    await change("admin.schedule.filter.from", "2026-01-02")
    await change("admin.schedule.filter.to", "2026-01-03")
    assert.equal(latest().from, "2026-01-02")
    assert.equal(latest().to, "2026-01-03")
    assert.equal(latest().page, 1)
    await click(input("admin.pager.next"))
    assert.equal(latest().page, 2)
    assert.equal(latest().from, "2026-01-02")
    assert.equal(latest().to, "2026-01-03")
    // A page number and the jump field page the same query; a page past the
    // end goes to the last one.
    const pageButton = (number: number) =>
      findAll(".adm-pager-page").find(
        (button) => button.textContent === String(number),
      )
    await click(pageButton(3))
    assert.equal(latest().page, 3)
    await enter(find('[inputmode="numeric"]')!, "99")
    await submit(find(".adm-pager-jump"))
    assert.equal(latest().page, 3)
    assert.equal(latest().from, "2026-01-02")
    // Picking a choice from a filter list narrows the query from page 1.
    await click(findAll('[aria-haspopup="listbox"]')[1])
    await click(
      findAll('[role="option"]').find((option) => option.id.endsWith("-5")),
    )
    assert.equal(latest().llm, "failed")
    assert.equal(latest().page, 1)
    await click(input("admin.pager.next"))
    assert.equal(latest().page, 2)
    await change("admin.posts.search", "needle")
    await submit(find(".adm-filters"))
    assert.equal(latest().search, "needle")
    assert.equal(latest().page, 1)
    assert.equal(latest().from, "2026-01-02")
    assert.equal(latest().to, "2026-01-03")
    await change("admin.schedule.filter.to", "")
    assert.equal(latest().from, "2026-01-02")
    assert.equal(latest().to, "")
    await change("admin.schedule.filter.from", "")
    assert.equal(latest().from, "")
    assert.equal(latest().search, "needle")
    assert.equal(latest().queued, "refresh")
    // The queue filter offers any open request.
    await click(findAll('[aria-haspopup="listbox"]')[3])
    await click(
      findAll('[role="option"]').find((option) => option.id.endsWith("-1")),
    )
    assert.equal(latest().queued, "any")
    assert.equal(latest().page, 1)
  } finally {
    if (root) await act(async () => root!.unmount())
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previous
    await vite.close()
  }
})
