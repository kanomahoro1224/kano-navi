import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement, useState } from "react"
import { createServer } from "vite"
import { browser, click, render, type Rendered } from "./dom.ts"

const event = (id: string, overrides: Record<string, unknown>) => ({
  id,
  source: "x",
  title: `Synthetic entry ${id}`,
  detail: null,
  startsOn: "2026-10-09",
  startsAt: null,
  endsAt: null,
  timezone: "Asia/Tokyo",
  timePrecision: "date",
  status: null,
  eventType: "stream",
  tag: null,
  url: null,
  provenance: "automatic",
  manualLocked: false,
  isUpcoming: false,
  cancellationStatus: "none",
  ...overrides,
})

/**
 * Loads the schedule's modules through Vite and hands them to `run`, which
 * renders with `mount` inside the board's settings and messages.
 */
async function withSchedule(
  run: (modules: {
    ScheduleMonth: any
    WeekGuide: any
    ArchiveDialog: any
    useAppSettings: any
    mount: (element: ReturnType<typeof createElement>) => Promise<Rendered>
  }) => Promise<void>,
) {
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
    const [
      { ScheduleMonth, WeekGuide },
      { ArchiveDialog },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
      { dashboardMessages },
    ] = await Promise.all([
      vite.ssrLoadModule("/src/dashboard/components/week-guide.tsx"),
      vite.ssrLoadModule("/src/dashboard/components/archive-dialog.tsx"),
      vite.ssrLoadModule("/src/app-settings.tsx"),
      vite.ssrLoadModule("/src/dashboard/dashboard-i18n.ts"),
    ])
    const mount = async (element: ReturnType<typeof createElement>) => {
      await act(async () => {
        root = render(
          createElement(
            AppSettingsProvider,
            null,
            createElement(
              PageMessagesProvider,
              { messages: dashboardMessages },
              element,
            ),
          ),
        )
      })
      return root!
    }
    await run({
      ScheduleMonth,
      WeekGuide,
      ArchiveDialog,
      useAppSettings,
      mount,
    })
  } finally {
    await act(async () => root?.unmount())
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previousAct
    await vite.close()
  }
}

test("the week folds past entries and names no ended status", async () => {
  await withSchedule(async ({ WeekGuide, useAppSettings, mount }) => {
    const timed = (id: string, startsAt: string, more = {}) =>
      event(id, {
        startsOn: startsAt.slice(0, 10),
        startsAt,
        timePrecision: "minute",
        tag: "game",
        url: `https://video.example.test/watch/${id}`,
        ...more,
      })
    const events = [
      timed("yesterday", "2026-10-07T13:00:00+09:00", {
        liveStatus: "ended",
      }),
      timed("earlier", "2026-10-08T10:00:00+09:00", {
        endsAt: "2026-10-08T12:00:00+09:00",
      }),
      timed("onAir", "2026-10-08T20:00:00+09:00"),
      timed("tomorrow", "2026-10-09T20:00:00+09:00"),
    ]
    const range = { from: "2026-10-05", to: "2026-10-11" }
    function Harness() {
      const { locale, t } = useAppSettings()
      return createElement(WeekGuide, {
        events,
        scheduleImages: [],
        weekStart: new Date(2026, 9, 5),
        setWeekStart: () => {},
        range,
        shownRange: range,
        adjacent: { previous: null, next: null },
        error: null,
        onRetry: () => {},
        total: events.length,
        hasNext: false,
        onLoadMore: () => {},
        dayCounts: {},
        isLoading: false,
        onOpenImage: () => {},
        onOpenMonth: () => {},
        locale,
        now: Date.parse("2026-10-08T21:00:00+09:00"),
        t,
      })
    }
    const { container } = await mount(createElement(Harness))

    const cards = [...container.querySelectorAll(".guide-days .guide-event")]
    // An entry of an earlier day, and today's once it ends, fold to their
    // time and title; the one on air and the coming one stay whole.
    assert.deepEqual(
      cards.map((card) => card.classList.contains("is-folded")),
      [true, true, false, false],
    )
    for (const card of cards.slice(0, 2)) {
      assert.equal(card.querySelector(".guide-event-tags"), null)
      assert.ok(card.querySelector(".guide-event-time svg"))
    }
    assert.match(cards[2].textContent || "", /On air/)
    assert.doesNotMatch(container.textContent || "", /Ended/)

    // A folded entry still opens its sheet, which has no ended status.
    await click(cards[0].querySelector(".guide-event-title button"))
    const sheet = container.querySelector("dialog.sheet-dialog")
    assert.ok(sheet)
    assert.match(sheet.textContent || "", /Gameplay/)
    assert.doesNotMatch(sheet.textContent || "", /Ended/)
  })
})

test("a schedule entry opens its sheet, and a month entry marks it in its day", async () => {
  await withSchedule(
    async ({ ScheduleMonth, ArchiveDialog, useAppSettings, mount }) => {
      const longTitle =
        "Synthetic stream whose title runs well past three lines of a narrow card"
      const items = [
        event("first", {
          title: longTitle,
          startsAt: "2026-10-09T10:00:00Z",
          timePrecision: "minute",
          url: "https://video.example.test/watch/first",
        }),
        event("second", {
          startsAt: "2026-10-09T12:00:00Z",
          timePrecision: "minute",
          provenance: "manual",
          url: "https://video.example.test/watch/second",
        }),
      ]
      const archive = {
        items,
        adjacent: { previous: null, next: null },
        total: items.length,
        page: 1,
        hasNext: false,
        revision: 1,
        dayCounts: { "2026-10-09": items.length },
      }
      const monthStart = new Date(2026, 9, 1)
      let closed = 0
      function Harness() {
        const { locale, t } = useAppSettings()
        const [details, setDetails] = useState(true)
        return createElement(
          ArchiveDialog,
          {
            open: true,
            kicker: "",
            title: "Month",
            monthStart,
            onMonthChange: () => {},
            onClose: () => (closed += 1),
            locale,
            t,
          },
          createElement(ScheduleMonth, {
            monthStart,
            showDetails: details,
            onShowDetails: setDetails,
            archive,
            isLoading: false,
            error: null,
            onRetry: () => {},
            onLoadMore: () => {},
            onJump: () => {},
            onOpenWeek: () => {},
            locale,
            now: Date.parse("2026-10-20T00:00:00+09:00"),
            t,
          }),
        )
      }
      const { container } = await mount(createElement(Harness))

      // Grid entries pick their day instead of leaving for their link.
      const chips = [...container.querySelectorAll(".month-event")]
      assert.deepEqual(
        chips.map((chip) => chip.tagName),
        ["BUTTON", "BUTTON"],
      )
      await click(chips[1])
      const cards = [
        ...container.querySelectorAll(".month-detail .guide-event"),
      ]
      assert.equal(cards.length, 2)
      assert.ok(cards.every((card) => card.classList.contains("is-full")))
      assert.deepEqual(
        cards.map((card) => card.classList.contains("is-highlighted")),
        [false, true],
      )

      // Cards name their provenance by icon, with the words kept for screen
      // readers and the tooltip.
      for (const card of cards) {
        const mark = card.querySelector(".guide-provenance")!
        assert.ok(mark.getAttribute("title"))
        assert.equal(
          mark.querySelector(".sr-only")?.textContent,
          mark.getAttribute("title"),
        )
      }

      // The arrow in a card's corner follows the link without the sheet.
      assert.deepEqual(
        cards.map((card) =>
          card.querySelector("a.guide-event-open")?.getAttribute("href"),
        ),
        [
          "https://video.example.test/watch/first",
          "https://video.example.test/watch/second",
        ],
      )

      // Grid titles take more lines in a day with fewer entries.
      assert.equal(
        container.querySelector(".month-events")?.getAttribute("data-count"),
        "2",
      )

      // The legend names every activity icon that grid entries show alone.
      assert.equal(
        container.querySelectorAll(".guide-legend .guide-legend-tag svg")
          .length,
        5,
      )

      // A card opens its sheet, which holds the whole title and the link.
      await click(cards[0])
      const sheet = container.querySelector("dialog.sheet-dialog")
      assert.ok(sheet)
      assert.match(sheet.textContent || "", new RegExp(longTitle))
      assert.deepEqual(
        [...sheet.querySelectorAll("a")].map((link) =>
          link.getAttribute("href"),
        ),
        ["https://video.example.test/watch/first"],
      )

      // Escape closes the sheet alone, not the month dialog beneath it.
      await act(async () => {
        sheet.dispatchEvent(
          new browser.Event("cancel", { cancelable: true }) as unknown as Event,
        )
      })
      assert.equal(container.querySelector("dialog.sheet-dialog"), null)
      assert.equal(closed, 0)
      assert.ok(container.querySelector("dialog.archive-dialog"))
    },
  )
})
