import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import { render, type Rendered } from "./dom.ts"

test("a members-only cover adds YouTube's members star beside the YouTube mark", async () => {
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
    const { PlatformBadge } = await vite.ssrLoadModule(
      "/src/dashboard/components/platform-links.tsx",
    )
    await act(async () => {
      root = render(
        createElement(PlatformBadge, {
          platforms: ["youtube", "bilibili"],
          members: true,
        }),
      )
    })
    const marks = [...root!.container.querySelectorAll(".platform-badge-mark")]
    assert.equal(marks.length, 2)
    // The star follows the YouTube mark only; a bilibili copy is no
    // members-only video.
    assert.equal(marks[0].querySelectorAll(".platform-members").length, 1)
    assert.equal(marks[1].querySelectorAll(".platform-members").length, 0)

    await act(async () => {
      root!.update(createElement(PlatformBadge, { platforms: ["youtube"] }))
    })
    assert.equal(
      root!.container.querySelectorAll(".platform-members").length,
      0,
    )
  } finally {
    await act(async () => root?.unmount())
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previousAct
    await vite.close()
  }
})
