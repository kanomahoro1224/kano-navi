import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import { dashboardMessages } from "../src/dashboard/dashboard-i18n.ts"
import { render, type Rendered } from "./dom.ts"

const room = "https://live.example.invalid/100000001"

test("the stage shows an unannounced broadcast as live with its watch link", async () => {
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
      { NowStage },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
      { fallbackProfile },
    ] = await Promise.all([
      vite.ssrLoadModule("/src/dashboard/components/now-stage.tsx"),
      vite.ssrLoadModule("/src/app-settings.tsx"),
      vite.ssrLoadModule("/src/dashboard/content.ts"),
    ])
    function Harness() {
      const { locale, t } = useAppSettings()
      return createElement(NowStage, {
        profile: fallbackProfile,
        summary: {
          nextEvent: null,
          nextStream: null,
          latestVideo: null,
          latestPost: null,
          counts: null,
        },
        liveNow: {
          platform: "bilibili",
          url: room,
          title: "Synthetic unannounced stream",
          startedAt: "2026-10-21T11:40:00Z",
        },
        resources: [],
        counts: { posts: 0, upcomingEvents: 0, videos: 0 },
        postWindowDays: 14,
        videoWindowDays: 30,
        syncState: { tone: "leaf", label: "Synced", sources: [] },
        isLoading: false,
        locale,
        now: Date.parse("2026-10-21T12:00:00Z"),
        t,
      })
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
    const ticket = root!.container.querySelector(".ticket")!
    assert.ok(ticket.classList.contains("is-live"))
    assert.match(
      ticket.querySelector(".ticket-onair")?.textContent || "",
      /On air/u,
    )
    assert.equal(
      ticket.querySelector(".ticket-head .tag")?.textContent,
      "Live now on bilibili",
    )
    assert.equal(
      ticket.querySelector(".ticket-title a")?.getAttribute("href"),
      room,
    )
    assert.match(
      ticket.querySelector(".ticket-link")?.textContent || "",
      /Watch/u,
    )
    // A reported broadcast is not presumed from its start time.
    assert.equal(ticket.querySelector(".ticket-note"), null)
  } finally {
    await act(async () => root?.unmount())
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previousAct
    await vite.close()
  }
})
