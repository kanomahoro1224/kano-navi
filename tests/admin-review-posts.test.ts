import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import type { AdminPost, AdminTranslation } from "../src/admin/types.ts"
import { browser, click, render, type Rendered } from "./dom.ts"

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

test("failed posts name the failed step and offer its actions for one post or a selection", async () => {
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  const previous = (globalThis as any).IS_REACT_ACT_ENVIRONMENT
  let root: Rendered | undefined
  let compactRoot: Rendered | undefined
  try {
    const { PostActions, ReviewView } = await vite.ssrLoadModule(
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
    const failed = [
      extraction,
      post("extraction2", { llmStatus: "failed" }),
      post("extraction3", { llmStatus: "failed" }),
    ]
    const translation = post("translation", { translation: failedTranslation })
    const queued = post("queued", {
      llmStatus: "queued",
      llmReprocessRequested: true,
    })
    const queries: Record<string, unknown>[] = []
    const calls: string[] = []
    const notices: string[] = []
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
      notify: (key: string, values?: { count?: number }) =>
        notices.push(`${key}:${values?.count}`),
      fail: (error: unknown) => {
        throw error
      },
      api: {
        posts: async (query: Record<string, unknown>) => {
          queries.push({ ...query })
          const items =
            query.llm === "failed"
              ? failed
              : query.translation === "failed"
                ? [translation]
                : query.reprocess
                  ? [queued]
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
        reprocessPosts: async (ids: string[]) => {
          calls.push(`reprocess:${ids.join(",")}`)
          return { queued: ids.length }
        },
        dismissPosts: async (ids: string[]) => {
          calls.push(`dismiss:${ids.join(",")}`)
          return { dismissed: ids.length }
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
    const find = (scope: Element, text: string) =>
      [...scope.querySelectorAll("button")].find((item) =>
        item.textContent?.includes(text),
      )
    const row = () => panel().querySelector(".adm-post-row")!
    const button = (text: string) => find(row(), text)
    const bar = () => panel().querySelector(".adm-select-bar")!
    const barButton = (text: string) => find(bar(), text)!
    const picks = () => [
      ...panel().querySelectorAll<HTMLInputElement>(".adm-post-pick input"),
    ]
    const checked = () => picks().map((input) => input.checked)

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
    assert.match(row().textContent!, /admin\.review\.failure\.extraction/)
    assert.match(row().textContent!, /admin\.posts\.error\.provider_error/)
    assert.match(row().textContent!, /2026-01-02 00:00/)
    assert.ok(button("admin.posts.reprocess"))
    await click(button("admin.posts.dismiss")!)
    assert.deepEqual(calls, ["dismiss:extraction"])

    // A click picks one row and a shift-click the range from it; the bar
    // applies its action to the whole selection in one request.
    assert.equal(barButton("admin.posts.dismiss").disabled, true)
    await click(picks()[0])
    await act(async () => {
      picks()[2].dispatchEvent(
        new browser.MouseEvent("click", {
          bubbles: true,
          shiftKey: true,
        }) as unknown as Event,
      )
    })
    assert.deepEqual(checked(), [true, true, true])
    await click(barButton("admin.review.bulkReprocess"))
    assert.equal(calls.at(-1), "reprocess:extraction,extraction2,extraction3")
    assert.equal(notices.at(-1), "admin.notice.postsQueued:3")
    assert.deepEqual(checked(), [false, false, false])
    // The bar's box selects the page, and clearing empties it again.
    await click(bar().querySelector("input"))
    assert.deepEqual(checked(), [true, true, true])
    await click(
      bar().querySelector('[aria-label="admin.review.clearSelection"]'),
    )
    assert.deepEqual(checked(), [false, false, false])
    await click(picks()[1])
    await click(barButton("admin.posts.dismiss"))
    assert.equal(calls.at(-1), "dismiss:extraction2")
    assert.equal(notices.at(-1), "admin.notice.postsDismissed:1")

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
    assert.equal(calls.at(-1), "translate:translation")
    await click(picks()[0])
    await click(barButton("admin.review.retryTranslation"))
    assert.equal(notices.at(-1), "admin.notice.translationsQueued:1")

    // Queued posts have no bulk action, so nothing is selectable.
    await click(filters[3])
    assert.match(row().textContent!, /Synthetic queued/)
    assert.equal(panel().querySelector(".adm-select-bar"), null)
    assert.deepEqual(picks(), [])

    // Compact actions honor the same reprocess switch as the full actions.
    await act(async () => {
      compactRoot = render(
        createElement(
          AppSettingsProvider,
          null,
          createElement(
            AdminContext.Provider,
            { value: context },
            createElement(PostActions, {
              post: translation,
              onDone: () => {},
              compact: true,
              reprocess: false,
            }),
          ),
        ),
      )
    })
    assert.equal(
      compactRoot!.container.querySelector(
        '[aria-label="admin.posts.reprocess"]',
      ),
      null,
    )
  } finally {
    if (root) await act(async () => root!.unmount())
    if (compactRoot) await act(async () => compactRoot!.unmount())
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previous
    await vite.close()
  }
})
