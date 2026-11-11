import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import type { ArchiveSnapshot, Video } from "../src/types.ts"
import { browser, click, render, type Rendered } from "./dom.ts"

async function loadModules() {
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  try {
    const [
      { ArchiveDialog },
      { VideoTimeline },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
      { dashboardMessages },
    ] = await Promise.all([
      vite.ssrLoadModule("/src/dashboard/components/archive-dialog.tsx"),
      vite.ssrLoadModule("/src/dashboard/components/video-shelf.tsx"),
      vite.ssrLoadModule("/src/app-settings.tsx"),
      vite.ssrLoadModule("/src/dashboard/dashboard-i18n.ts"),
    ])
    return {
      ArchiveDialog,
      VideoTimeline,
      AppSettingsProvider,
      PageMessagesProvider,
      useAppSettings,
      dashboardMessages,
    }
  } finally {
    await vite.close()
  }
}

const modules = await loadModules()

async function withAct(run: () => Promise<void>) {
  const previous = (globalThis as any).IS_REACT_ACT_ENVIRONMENT
  ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
  try {
    await run()
  } finally {
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previous
  }
}

function video(id: string, publishedAt: string): Video {
  return {
    id,
    source: "youtube",
    title: `Synthetic ${id}`,
    publishedAt,
    scheduledAt: null,
    url: `https://www.youtube.com/watch?v=${id}`,
    thumbnailUrl: null,
    thumbnailStatus: "missing",
    kind: "upload",
    isUpcoming: false,
  }
}

function snapshot(
  items: Video[],
  fields: Partial<ArchiveSnapshot<Video>> = {},
): ArchiveSnapshot<Video> {
  return {
    items,
    adjacent: { previous: null, next: null },
    total: 40,
    page: 1,
    hasNext: true,
    revision: 1,
    ...fields,
  }
}

interface TimelineProps {
  archive: ArchiveSnapshot<Video> | null
  error?: Error | null
  jump?: { month: string; nonce: number } | null
  atLatest?: boolean
  monthStart?: Date
}

/** Mounts the video dialog in timeline mode around a timeline body. */
async function mountTimeline(
  initial: TimelineProps,
  calls: {
    loadMore: number
    newer: number
    latest: number
    visible: string[]
  },
) {
  const { ArchiveDialog, VideoTimeline } = modules
  function Harness(props: TimelineProps) {
    const { locale, t } = modules.useAppSettings()
    return createElement(
      ArchiveDialog,
      {
        open: true,
        kicker: "",
        title: "Videos",
        monthStart: props.monthStart || new Date(2026, 9, 1),
        onMonthChange: () => {},
        onClose: () => {},
        picker: { counts: null },
        timeline: {
          onLatest: () => (calls.latest += 1),
          atLatest: props.atLatest ?? true,
          resetKey: "latest:",
        },
        locale,
        t,
      },
      createElement(VideoTimeline, {
        archive: props.archive,
        isLoading: false,
        error: props.error || null,
        onRetry: () => {},
        onLoadMore: () => (calls.loadMore += 1),
        monthCounts: new Map([
          ["2026-10", 12],
          ["2026-09", 1],
        ]),
        jump: props.jump || null,
        onVisibleMonth: (month: string) => calls.visible.push(month),
        onNewer: () => (calls.newer += 1),
        locale,
        now: Date.parse("2026-11-05T00:00:00Z"),
        t,
      }),
    )
  }
  const wrap = (props: TimelineProps) =>
    createElement(
      modules.AppSettingsProvider,
      null,
      createElement(
        modules.PageMessagesProvider,
        { messages: modules.dashboardMessages },
        createElement(Harness, props),
      ),
    )
  let root: Rendered | undefined
  await act(async () => {
    root = render(wrap(initial))
  })
  return {
    root: root!,
    update: (props: TimelineProps) =>
      act(async () => root!.update(wrap(props))),
  }
}

const items = [
  video("oct-b", "2026-10-20T03:00:00Z"),
  video("oct-a", "2026-10-01T03:00:00Z"),
  // 2026-09-30T15:00Z is October 1 in Japan; the next one is September.
  video("oct-edge", "2026-09-30T15:00:00Z"),
  video("sep", "2026-09-30T14:59:00Z"),
  video("old", "2015-02-10T03:00:00Z"),
]

test("the video timeline lists every loaded video under its Japan month", async () => {
  await withAct(async () => {
    const calls = { loadMore: 0, newer: 0, latest: 0, visible: [] as string[] }
    // Lay the months out as a browser would: the first at the body's top.
    const Element = (browser as any).HTMLElement.prototype
    const measure = Element.getBoundingClientRect
    const tops: Record<string, number> = { "2026-10": 20, "2026-09": 900 }
    Element.getBoundingClientRect = function (this: HTMLElement) {
      const month = this.getAttribute("data-month")
      return { top: month ? (tops[month] ?? 1600) : 0 }
    }
    const { root } = await mountTimeline({ archive: snapshot(items) }, calls)
    Element.getBoundingClientRect = measure
    try {
      const { container } = root
      const sections = Array.from(container.querySelectorAll(".video-month"))
      assert.deepEqual(
        sections.map((section) => section.getAttribute("data-month")),
        ["2026-10", "2026-09", "2015-02"],
      )
      assert.deepEqual(
        sections.map(
          (section) =>
            Array.from(section.querySelectorAll(".archive-card-title")).length,
        ),
        [3, 1, 1],
      )
      // A month's count comes from the index, else from what is loaded.
      assert.deepEqual(
        sections.map((section) => section.querySelector("h3")?.textContent),
        [
          "October 202612 videos",
          "September 20261 video",
          "February 20151 video",
        ],
      )
      assert.equal(
        container.querySelector(".video-timeline-status")?.textContent,
        "Showing 5 of 40",
      )
      // The month in view is reported once the sections exist.
      assert.equal(calls.visible[0], "2026-10")
      assert.equal(container.querySelector(".archive-more.is-newer"), null)
      const more = container.querySelector<HTMLButtonElement>(
        ".archive-more:not(.is-newer) button",
      )
      assert.equal(more?.textContent, "Load more videos")
      await click(more)
      assert.equal(calls.loadMore, 1)
    } finally {
      await act(async () => root.unmount())
    }
  })
})

test("the timeline dialog swaps the month arrows for a Latest button", async () => {
  await withAct(async () => {
    const calls = { loadMore: 0, newer: 0, latest: 0, visible: [] as string[] }
    const { root, update } = await mountTimeline(
      {
        archive: snapshot(items.slice(3), {
          adjacent: { previous: null, next: "2026-10-01" },
          hasNext: false,
          total: 2,
        }),
        atLatest: false,
        monthStart: new Date(2026, 8, 1),
      },
      calls,
    )
    try {
      const { container } = root
      const bar = container.querySelector(".archive-month")!
      assert.equal(
        bar.querySelector('[aria-label="Previous month"]'),
        null,
        "no previous-month arrow",
      )
      assert.equal(bar.querySelector('[aria-label="Next month"]'), null)
      const label = bar.querySelector(".archive-month-label.is-picker")
      assert.equal(
        label?.querySelector(".archive-month-long")?.textContent,
        "September 2026",
      )
      // Phones show the short name in its place.
      assert.equal(
        label?.querySelector(".archive-month-short")?.textContent,
        "Sep 2026",
      )
      // A label that follows the scroll is not announced on every change.
      assert.equal(label?.querySelector("[aria-live]"), null)
      const latest = Array.from(
        bar.querySelectorAll<HTMLButtonElement>(".pill-button"),
      ).find((button) => button.textContent === "Latest")
      assert.equal(latest?.disabled, false)
      await click(latest)
      assert.equal(calls.latest, 1)

      // A timeline that starts at an earlier month offers the newer videos.
      const newer = container.querySelector(".archive-more.is-newer button")
      assert.equal(newer?.textContent, "Show newer videos")
      await click(newer)
      assert.equal(calls.newer, 1)
      // The last page leaves no load control.
      assert.equal(
        container.querySelector(".archive-more:not(.is-newer)"),
        null,
      )

      // A timeline started at a later page offers the pages before it.
      await update({
        archive: snapshot(items.slice(3), { firstPage: 3 }),
        atLatest: false,
      })
      await click(container.querySelector(".archive-more.is-newer button"))
      assert.equal(calls.newer, 2)

      await update({
        archive: snapshot(items),
        atLatest: true,
      })
      assert.equal(container.querySelector(".archive-more.is-newer"), null)
      assert.equal(
        Array.from(
          bar.querySelectorAll<HTMLButtonElement>(".pill-button"),
        ).find((button) => button.textContent === "Latest")?.disabled,
        true,
      )
    } finally {
      await act(async () => root.unmount())
    }
  })
})

test("a jump scrolls its loaded month into view, and the first month to the top", async () => {
  await withAct(async () => {
    const calls = { loadMore: 0, newer: 0, latest: 0, visible: [] as string[] }
    const { root, update } = await mountTimeline(
      { archive: snapshot(items) },
      calls,
    )
    const Element = (browser as any).HTMLElement.prototype
    const scrolled: string[] = []
    const previousInto = Element.scrollIntoView
    const previousTo = Element.scrollTo
    Element.scrollIntoView = function (this: HTMLElement) {
      scrolled.push(`into:${this.getAttribute("data-month")}`)
    }
    Element.scrollTo = function (this: HTMLElement) {
      if (this.classList.contains("archive-dialog-body")) scrolled.push("top")
    }
    try {
      await update({
        archive: snapshot(items),
        jump: { month: "2015-02", nonce: 1 },
      })
      await update({
        archive: snapshot(items),
        jump: { month: "2026-10", nonce: 2 },
      })
      // A month not loaded yet waits, and scrolls once it arrives.
      await update({
        archive: snapshot(items),
        jump: { month: "2020-01", nonce: 3 },
      })
      assert.deepEqual(scrolled, ["into:2015-02", "top"])
      const later = [...items, video("jan", "2020-01-10T03:00:00Z")]
      await update({
        archive: snapshot(later),
        jump: { month: "2020-01", nonce: 3 },
      })
      await update({
        archive: snapshot([...later, video("older", "2019-05-10T03:00:00Z")]),
        jump: { month: "2020-01", nonce: 3 },
      })
      assert.deepEqual(scrolled, ["into:2015-02", "top", "into:2020-01"])
    } finally {
      Element.scrollIntoView = previousInto
      Element.scrollTo = previousTo
      await act(async () => root.unmount())
    }
  })
})

test("a failed read keeps the load button and never loads on its own", async () => {
  await withAct(async () => {
    const observed: unknown[] = []
    const previous = (globalThis as any).IntersectionObserver
    ;(globalThis as any).IntersectionObserver = class {
      constructor(callback: (entries: { isIntersecting: boolean }[]) => void) {
        observed.push(callback)
      }
      observe() {
        ;(observed.at(-1) as (entries: unknown[]) => void)([
          { isIntersecting: true },
        ])
      }
      disconnect() {}
    }
    const calls = { loadMore: 0, newer: 0, latest: 0, visible: [] as string[] }
    try {
      const { root, update } = await mountTimeline(
        { archive: snapshot(items) },
        calls,
      )
      try {
        // In reach of the end: the next page loads without a press.
        assert.equal(calls.loadMore, 1)
        await update({
          archive: snapshot(items),
          error: new Error("Synthetic outage"),
        })
        assert.equal(calls.loadMore, 1)
        assert.ok(root.container.querySelector('[role="alert"]'))
        assert.ok(root.container.querySelector(".archive-more button"))
      } finally {
        await act(async () => root.unmount())
      }
    } finally {
      ;(globalThis as any).IntersectionObserver = previous
    }
  })
})
