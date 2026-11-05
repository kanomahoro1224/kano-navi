import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import { browser, click, render, type Rendered } from "./dom.ts"

const PageKeyboardEvent =
  browser.KeyboardEvent as unknown as typeof KeyboardEvent

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
      { usePostMonths },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
      { dashboardMessages },
    ] = await Promise.all([
      vite.ssrLoadModule("/src/dashboard/components/archive-dialog.tsx"),
      vite.ssrLoadModule("/src/dashboard/use-dashboard.ts"),
      vite.ssrLoadModule("/src/app-settings.tsx"),
      vite.ssrLoadModule("/src/dashboard/dashboard-i18n.ts"),
    ])
    return {
      ArchiveDialog,
      usePostMonths,
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

/** Mounts a post month dialog with a jump grid over the given counts. */
async function mountDialog(
  counts: Map<string, number> | null,
  onMonthChange: (month: Date) => void,
  onClose: () => void,
) {
  const { ArchiveDialog, AppSettingsProvider, PageMessagesProvider } = modules
  function Harness() {
    const { locale, t } = modules.useAppSettings()
    return createElement(
      ArchiveDialog,
      {
        open: true,
        kicker: "",
        title: "Posts",
        monthStart: new Date(2026, 9, 1),
        latestMonth: new Date(2026, 10, 1),
        onMonthChange,
        onClose,
        picker: {
          counts,
          countLabel: (count: number) => t("feed.monthCount", { count }),
        },
        locale,
        t,
      },
      createElement("p", null, "Body"),
    )
  }
  let root: Rendered | undefined
  await act(async () => {
    root = render(
      createElement(
        AppSettingsProvider,
        null,
        createElement(
          PageMessagesProvider,
          { messages: modules.dashboardMessages },
          createElement(Harness),
        ),
      ),
    )
  })
  return root!
}

const months = (container: Element) =>
  Array.from(container.querySelectorAll(".month-picker-month"))
const enabled = (container: Element) =>
  months(container)
    .filter((button) => button.getAttribute("aria-disabled") !== "true")
    .map((button) => button.textContent)

test("the month label opens a jump grid over months that hold posts", async () => {
  await withAct(async () => {
    const chosen: Date[] = []
    let closed = 0
    const counts = new Map([
      ["2025-03", 4],
      ["2026-09", 2],
      ["2026-10", 5],
    ])
    const root = await mountDialog(
      counts,
      (month) => chosen.push(month),
      () => (closed += 1),
    )
    try {
      const { container } = root
      const trigger = container.querySelector(".archive-month-label.is-picker")
      assert.equal(trigger?.getAttribute("aria-expanded"), "false")
      assert.equal(container.querySelector(".month-picker"), null)
      await click(trigger)
      assert.equal(trigger?.getAttribute("aria-expanded"), "true")

      // Months without posts and months after the latest are not offered.
      assert.equal(months(container).length, 12)
      assert.deepEqual(enabled(container), ["Sep2", "Oct5"])
      const october = months(container)[9]
      assert.equal(october.getAttribute("aria-current"), "date")
      assert.equal(browser.document.activeElement, october)
      const year = () =>
        container.querySelector(".month-picker-year-label strong")?.textContent
      const [previousYear, nextYear] = Array.from(
        container.querySelectorAll<HTMLButtonElement>(
          ".month-picker-year .round-button",
        ),
      )
      assert.equal(year(), "2026")
      assert.equal(nextYear.disabled, true)

      // The year stops at the oldest indexed month.
      await click(previousYear)
      assert.equal(year(), "2025")
      assert.equal(previousYear.disabled, true)
      assert.deepEqual(enabled(container), ["Mar4"])
      assert.match(
        months(container)[2].getAttribute("aria-label") || "",
        /4 posts/,
      )
      await click(months(container)[1])
      assert.equal(chosen.length, 0)

      await click(months(container)[2])
      assert.deepEqual(
        chosen.map((month) => [month.getFullYear(), month.getMonth()]),
        [[2025, 2]],
      )
      assert.equal(container.querySelector(".month-picker"), null)
      assert.equal(browser.document.activeElement, trigger)

      // Escape closes only the grid, not the dialog.
      await click(trigger)
      await act(async () => {
        ;(
          browser.document.activeElement as unknown as HTMLElement | null
        )?.dispatchEvent(
          new PageKeyboardEvent("keydown", { key: "Escape", bubbles: true }),
        )
      })
      assert.equal(container.querySelector(".month-picker"), null)
      assert.equal(closed, 0)
    } finally {
      await act(async () => root.unmount())
    }
  })
})

test("without a month index every month up to the latest is offered", async () => {
  await withAct(async () => {
    const root = await mountDialog(
      null,
      () => {},
      () => {},
    )
    try {
      const { container } = root
      await click(container.querySelector(".archive-month-label.is-picker"))
      assert.equal(enabled(container).length, 11)
      assert.equal(months(container)[11].getAttribute("aria-disabled"), "true")
      assert.equal(container.querySelector(".month-picker-count"), null)
      const previousYear = container.querySelector<HTMLButtonElement>(
        ".month-picker-year .round-button",
      )
      assert.equal(previousYear?.disabled, false)
    } finally {
      await act(async () => root.unmount())
    }
  })
})

test("the month index loads while enabled, follows the revision and survives failures", async (t) => {
  await withAct(async () => {
    const requests: {
      url: string
      resolve: (response: Response) => void
      reject: (error: Error) => void
    }[] = []
    t.mock.method(
      globalThis,
      "fetch",
      (url: RequestInfo | URL) =>
        new Promise<Response>((resolve, reject) =>
          requests.push({ url: String(url), resolve, reject }),
        ),
    )
    let value: unknown
    function Probe(props: { enabled: boolean; revision: number }) {
      value = modules.usePostMonths(props.enabled, props.revision)
      return null
    }
    let root: Rendered | undefined
    await act(async () => {
      root = render(createElement(Probe, { enabled: false, revision: 1 }))
    })
    try {
      assert.equal(requests.length, 0)
      await act(async () =>
        root!.update(createElement(Probe, { enabled: true, revision: 1 })),
      )
      assert.equal(requests[0].url, "/api/posts/months")
      const month = (key: string, total: number) => ({
        month: key,
        total,
        accountCounts: { kano_2525: total },
      })
      await act(async () =>
        requests[0].resolve({
          ok: true,
          status: 200,
          json: async () => ({
            months: [month("2026-09", 2), month("2026-13", 1), { month: 3 }],
            revision: 1,
          }),
        } as Response),
      )
      assert.deepEqual(value, [month("2026-09", 2)])

      await act(async () =>
        root!.update(createElement(Probe, { enabled: true, revision: 2 })),
      )
      assert.equal(requests.length, 2)
      await act(async () => requests[1].reject(new Error("Synthetic outage")))
      assert.deepEqual(value, [month("2026-09", 2)])
    } finally {
      await act(async () => root?.unmount())
    }
  })
})
