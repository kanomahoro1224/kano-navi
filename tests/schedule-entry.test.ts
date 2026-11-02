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

test("a schedule entry opens its sheet, and a month entry marks it in its day", async () => {
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
      { ScheduleMonth },
      { ArchiveDialog },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
      { dashboardMessages },
    ] = await Promise.all([
      vite.ssrLoadModule("/src/dashboard/components/week-guide.tsx"),
      vite.ssrLoadModule("/src/dashboard/components/archive-dialog.tsx"),
      vite.ssrLoadModule("/src/app-settings.tsx"),
      vite.ssrLoadModule("/src/dashboard/dashboard-i18n.ts"),
    ])
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
    await act(async () => {
      root = render(
        createElement(
          AppSettingsProvider,
          null,
          createElement(
            PageMessagesProvider,
            { messages: dashboardMessages },
            createElement(Harness),
          ),
        ),
      )
    })
    const container = root!.container

    // Grid entries pick their day instead of leaving for their link.
    const chips = [...container.querySelectorAll(".month-event")]
    assert.deepEqual(
      chips.map((chip) => chip.tagName),
      ["BUTTON", "BUTTON"],
    )
    await click(chips[1])
    const cards = [...container.querySelectorAll(".month-detail .guide-event")]
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

    // A card opens its sheet, which holds the whole title and the link.
    await click(cards[0])
    const sheet = container.querySelector("dialog.sheet-dialog")
    assert.ok(sheet)
    assert.match(sheet.textContent || "", new RegExp(longTitle))
    assert.deepEqual(
      [...sheet.querySelectorAll("a")].map((link) => link.getAttribute("href")),
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
  } finally {
    await act(async () => root?.unmount())
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previousAct
    await vite.close()
  }
})
