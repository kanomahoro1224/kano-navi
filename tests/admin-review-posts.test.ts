import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import type { AdminPost, AdminTranslation } from "../src/admin/types.ts"
import { click, render, type Rendered } from "./dom.ts"

const failedTranslation: AdminTranslation = {
  status: "incomplete",
  sourceLanguage: "ja",
  machineStatus: "failed",
  lastAttemptAt: "2026-01-02T00:00:00Z",
  lastError: "output_truncated",
  requested: false,
  locales: [],
}

function post(id: string, patch: Partial<AdminPost>): AdminPost {
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
    llmStatus: "success",
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
    ...patch,
  }
}

test("failed posts name the failed step and offer that step's actions", async () => {
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
    const { ReviewView } = await vite.ssrLoadModule(
      "/src/admin/views/review.tsx",
    )
    const { AdminContext } = await vite.ssrLoadModule(
      "/src/admin/use-admin-data.ts",
    )
    const { AppSettingsProvider } = await vite.ssrLoadModule(
      "/src/app-settings.tsx",
    )
    await vite.close()
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
    const extraction = post("extraction", {
      llmStatus: "failed",
      llmLastError: "provider_error",
      llmRetryAt: "2026-01-02T03:00:00Z",
    })
    const translation = post("translation", { translation: failedTranslation })
    const queries: Record<string, unknown>[] = []
    const calls: string[] = []
    const context = {
      revision: 0,
      overview: {
        review: {
          pendingImages: 0,
          suspectedCancellations: 0,
          failedPosts: 2,
          failedExtractions: 1,
          failedTranslations: 1,
          reprocessRequested: 0,
          videoMerges: 0,
        },
      },
      refreshOverview: async () => {},
      act: async (call: () => Promise<unknown>) => call(),
      fail: (error: unknown) => {
        throw error
      },
      api: {
        posts: async (query: Record<string, unknown>) => {
          queries.push({ ...query })
          const items =
            query.llm === "failed"
              ? [extraction]
              : query.translation === "failed"
                ? [translation]
                : []
          return { items, page: 1, total: items.length, totalPages: 1 }
        },
        dismissPost: async (id: string) => {
          calls.push(`dismiss:${id}`)
          return { state: {} }
        },
        requestTranslations: async (body: { ids: string[] }) => {
          calls.push(`translate:${body.ids.join(",")}`)
          return { queued: 1, run: null, busy: false }
        },
        reprocessPost: async (id: string) => {
          calls.push(`reprocess:${id}`)
          return { state: {} }
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
            createElement(ReviewView, { tab: "posts", navigate: () => {} }),
          ),
        ),
      )
    })
    const panel = () => root!.container.querySelector(".adm-tab-panel")!
    const button = (text: string) =>
      [...panel().querySelectorAll("button")].find((item) =>
        item.textContent?.includes(text),
      )

    // Extraction failures open first, with the overview's counts.
    assert.equal(queries.at(-1)?.llm, "failed")
    const filters = [...panel().querySelectorAll('[role="radio"]')]
    assert.deepEqual(
      filters.map((item) => item.textContent),
      [
        "admin.review.posts.extraction1",
        "admin.review.posts.translation1",
        "admin.status.uncertain",
        "admin.posts.queued",
      ],
    )
    const row = () => panel().querySelector(".adm-post-row")!
    assert.match(row().textContent!, /admin\.review\.failure\.extraction/)
    assert.match(row().textContent!, /admin\.posts\.error\.provider_error/)
    assert.match(row().textContent!, /2026-01-02 00:00/)
    assert.ok(button("admin.posts.reprocess"))
    await click(button("admin.posts.dismiss")!)
    assert.deepEqual(calls, ["dismiss:extraction"])

    await click(filters[1])
    assert.equal(queries.at(-1)?.translation, "failed")
    assert.match(row().textContent!, /admin\.review\.failure\.translation/)
    assert.match(row().textContent!, /admin\.posts\.error\.output_truncated/)
    assert.doesNotMatch(
      row().textContent!,
      /admin\.review\.failure\.extraction/,
    )
    // A translation failure has nothing to dismiss or extract again.
    assert.equal(button("admin.posts.dismiss"), undefined)
    assert.equal(button("admin.posts.reprocess"), undefined)
    await click(button("admin.review.retryTranslation")!)
    assert.deepEqual(calls, ["dismiss:extraction", "translate:translation"])
  } finally {
    if (root) await act(async () => root!.unmount())
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previous
    await vite.close()
  }
})
