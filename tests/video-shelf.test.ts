import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import type { Video } from "../src/types.ts"
import { click, render, type Rendered } from "./dom.ts"

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
      { VideoShelf },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
      { dashboardMessages },
    ] = await Promise.all([
      vite.ssrLoadModule("/src/dashboard/components/video-shelf.tsx"),
      vite.ssrLoadModule("/src/app-settings.tsx"),
      vite.ssrLoadModule("/src/dashboard/dashboard-i18n.ts"),
    ])
    return {
      VideoShelf,
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
const now = Date.parse("2026-10-25T12:00:00Z")
const hour = 3600 * 1000

function video(id: string, fields: Partial<Video>): Video {
  return {
    id,
    source: "youtube",
    title: `Synthetic ${id}`,
    publishedAt: null,
    scheduledAt: null,
    url: `https://www.youtube.com/watch?v=${id}`,
    thumbnailUrl: null,
    thumbnailStatus: "missing",
    kind: "upload",
    isUpcoming: false,
    ...fields,
    view: fields.isUpcoming
      ? {
          phase: "upcoming",
          formCode: "upcoming",
          shelfGroup: "reservation",
          latestEligible: false,
          focusEligible: true,
          tagContext: "stream",
        }
      : {
          phase: "past",
          formCode: "video",
          shelfGroup: "work",
          latestEligible: true,
          focusEligible: false,
          tagContext: "work",
        },
  }
}

/** The server's shelf for these videos: the first work leads. */
function shelfOf(videos: Video[]) {
  const reservations = videos.filter((item) => item.isUpcoming)
  const [lead, ...works] = videos.filter((item) => !item.isUpcoming)
  return {
    version: 1,
    nextTransitionAt: null,
    focus: { eventIds: [], videoIds: [] },
    spotlight: null,
    sales: [],
    shelf: {
      leadId: lead?.id ?? null,
      leadIsFeatured: false,
      reservations: reservations.map((item) => item.id),
      works: works.map((item) => item.id),
      archives: [],
      shorts: [],
    },
  }
}

function shelfVideos(reservations: number) {
  const reserved = Array.from({ length: reservations }, (_, index) =>
    video(`reserved-${index + 1}`, {
      kind: "stream",
      isUpcoming: true,
      scheduledAt: new Date(now + (index + 1) * hour).toISOString(),
    }),
  )
  const works = Array.from({ length: 6 }, (_, index) =>
    video(`work-${index + 1}`, {
      tag: "song",
      publishedAt: new Date(now - (index + 1) * 24 * hour).toISOString(),
    }),
  )
  return [...reserved, ...works]
}

async function mountShelf(videos: Video[]) {
  const { VideoShelf } = modules
  function Harness() {
    const { locale, t } = modules.useAppSettings()
    return createElement(VideoShelf, {
      videos,
      projection: shelfOf(videos),
      focus: null,
      isLoading: false,
      onOpenArchive: () => {},
      locale,
      now,
      t,
    })
  }
  let root: Rendered | undefined
  await act(async () => {
    root = render(
      createElement(
        modules.AppSettingsProvider,
        null,
        createElement(
          modules.PageMessagesProvider,
          { messages: modules.dashboardMessages },
          createElement(Harness),
        ),
      ),
    )
  })
  return root!
}

function withAct(run: () => Promise<void>) {
  return async () => {
    const previous = (globalThis as any).IS_REACT_ACT_ENVIRONMENT
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
    try {
      await run()
    } finally {
      ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previous
    }
  }
}

const side = (root: Rendered) => {
  const column = root.container.querySelector(".videos-side")
  assert.ok(column)
  return {
    tickets: column.querySelectorAll(".video-tickets > li").length,
    works: column.querySelectorAll(".video-list > li").length,
    toggle: column.querySelector<HTMLButtonElement>(
      ".videos-subhead-row > button",
    ),
  }
}

test(
  "the side group folds reservations past four behind a toggle",
  withAct(async () => {
    const root = await mountShelf(shelfVideos(6))
    try {
      let shown = side(root)
      assert.equal(shown.tickets, 4)
      assert.equal(shown.works, 1)
      const toggle = shown.toggle
      assert.ok(toggle)
      assert.equal(toggle.textContent, "+2")
      assert.equal(toggle.getAttribute("aria-expanded"), "false")
      assert.equal(toggle.getAttribute("aria-label"), "Show all 6 reservations")
      // The toggle is the heading's sibling and controls the ticket list.
      assert.equal(toggle.closest("h3"), null)
      const list = root.container.querySelector(".video-tickets")
      assert.ok(list?.id)
      assert.equal(toggle.getAttribute("aria-controls"), list.id)

      await click(toggle)
      shown = side(root)
      assert.equal(shown.tickets, 6)
      assert.equal(shown.works, 1)
      assert.equal(toggle.getAttribute("aria-expanded"), "true")
      assert.equal(toggle.getAttribute("aria-label"), "Show fewer reservations")
      assert.equal(toggle.textContent, "")

      await click(toggle)
      shown = side(root)
      assert.equal(shown.tickets, 4)
      assert.equal(toggle.getAttribute("aria-expanded"), "false")
    } finally {
      await act(async () => root.unmount())
    }
  }),
)

test(
  "the side group shows four works alone and no toggle within its slots",
  withAct(async () => {
    for (const [reservations, tickets, works] of [
      [0, 0, 4],
      [3, 3, 2],
      [4, 4, 1],
    ] as const) {
      const root = await mountShelf(shelfVideos(reservations))
      try {
        const shown = side(root)
        assert.equal(shown.tickets, tickets, `${reservations} tickets`)
        assert.equal(shown.works, works, `${reservations} works`)
        assert.equal(shown.toggle, null, `${reservations} toggle`)
      } finally {
        await act(async () => root.unmount())
      }
    }
  }),
)
