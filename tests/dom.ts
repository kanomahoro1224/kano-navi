import { Window } from "happy-dom"
import { act, type ReactNode } from "react"

// One happy-dom window per test file. It loads no scripts, styles or pages,
// so a rendered view never reaches the network.
export const browser = new Window({
  url: "https://example.test/",
  settings: {
    disableJavaScriptFileLoading: true,
    handleDisabledFileLoadingAsSuccess: true,
    navigation: { disableMainFrameNavigation: true },
  },
})
Object.assign(globalThis, { window: browser, document: browser.document })

// Tests see happy-dom through the standard DOM types.
const page = browser.document as unknown as Document
const PageEvent = browser.Event as unknown as typeof Event

// React DOM detects the browser events it supports when it loads, so it is
// imported only once the window is in place.
const { createRoot } = await import("react-dom/client")

/**
 * Renders into a container in the test window. Like React's own root, the
 * calls belong inside act().
 */
export function render(element: ReactNode) {
  const container = page.createElement("div")
  page.body.append(container)
  const root = createRoot(container)
  root.render(element)
  return {
    container,
    update: (next: ReactNode) => root.render(next),
    unmount: () => {
      root.unmount()
      container.remove()
    },
  }
}

export type Rendered = ReturnType<typeof render>

/** Enters a value the way typing does, so React's onChange sees it. */
export async function enter(input: Element, value: string) {
  const prototype = Object.getPrototypeOf(input)
  const setter = Object.getOwnPropertyDescriptor(prototype, "value")?.set
  if (!setter) throw new Error(`${input.tagName} has no value`)
  await act(async () => {
    setter.call(input, value)
    input.dispatchEvent(new PageEvent("input", { bubbles: true }))
  })
}

export async function click(element: Element | null | undefined) {
  if (!element) throw new Error("nothing to click")
  await act(async () => (element as HTMLElement).click())
}

/** Submits a form's handlers without happy-dom's navigation. */
export async function submit(form: Element | null | undefined) {
  if (!form) throw new Error("no form to submit")
  await act(async () => {
    form.dispatchEvent(
      new PageEvent("submit", { bubbles: true, cancelable: true }),
    )
  })
}
