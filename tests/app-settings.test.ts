import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import { browser, render, type Rendered } from "./dom.ts"

test("blocked browser storage keeps every page rendering", async () => {
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  const previousAct = (globalThis as any).IS_REACT_ACT_ENVIRONMENT
  ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
  // A browser that blocks site data throws on reading window.localStorage.
  const descriptor = Object.getOwnPropertyDescriptor(browser, "localStorage")
  Object.defineProperty(browser, "localStorage", {
    configurable: true,
    get() {
      throw new DOMException("blocked", "SecurityError")
    },
  })
  let root: Rendered | undefined
  try {
    const { AppSettingsProvider, useAppSettings } = await vite.ssrLoadModule(
      "/src/app-settings.tsx",
    )
    function Probe() {
      const { locale, setIsDark } = useAppSettings()
      return createElement(
        "button",
        { onClick: () => setIsDark(true) },
        `locale:${locale}`,
      )
    }
    await act(async () => {
      root = render(
        createElement(AppSettingsProvider, null, createElement(Probe)),
      )
    })
    const button = root!.container.querySelector("button")
    assert.equal(button?.textContent, "locale:en")
    await act(async () => (button as HTMLElement).click())
    assert.equal(document.documentElement.classList.contains("dark"), true)
  } finally {
    await act(async () => root?.unmount())
    if (descriptor) Object.defineProperty(browser, "localStorage", descriptor)
    else delete (browser as any).localStorage
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previousAct
    await vite.close()
  }
})
