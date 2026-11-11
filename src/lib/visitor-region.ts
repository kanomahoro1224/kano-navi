import { useEffect, useState } from "react"

import { pagePath } from "./page-path.ts"

const PUBLIC_PATHS = new Set(["/", "/journey", "/works", "/lineblog", "/about"])

/** Whether a pathname belongs to a public page that may show the notice. */
export function isVisitorRegionPath(pathname: string) {
  return PUBLIC_PATHS.has(pagePath(pathname))
}

/** The operator's mainland China site, as the public pages show it. */
export interface MainlandSite {
  /** Whether the edge reported a mainland China visitor. */
  mainlandChina: boolean
  providerName: string
  providerUrl: string
  mirrorUrl: string
}

/**
 * Reads a /api/visitor-region payload. The site is offered only when the
 * operator enabled it with a provider name, provider website and mirror site.
 */
export function mainlandSite(payload: unknown): MainlandSite | null {
  if (!payload || typeof payload !== "object") return null
  const candidate = payload as Record<string, unknown>
  const text = (value: unknown) =>
    typeof value === "string" ? value.trim() : ""
  const site = {
    mainlandChina: candidate.mainlandChina === true,
    providerName: text(candidate.providerName),
    providerUrl: text(candidate.providerUrl),
    mirrorUrl: text(candidate.mirrorUrl),
  }
  if (
    candidate.enabled !== true ||
    !site.providerName ||
    !site.providerUrl ||
    !site.mirrorUrl
  )
    return null
  return site
}

let pending: Promise<MainlandSite | null> | null = null

/** The toast and the footer share one request per page load. */
function loadMainlandSite() {
  pending ??= fetch("/api/visitor-region", {
    headers: { accept: "application/json" },
    cache: "no-store",
  })
    .then((response) => (response.ok ? response.json() : null))
    .then(mainlandSite)
    .catch(() => null)
  return pending
}

/** The mainland China site, or null while loading or when it is not offered. */
export function useMainlandSite(active = true) {
  const [site, setSite] = useState<MainlandSite | null>(null)

  useEffect(() => {
    if (!active) return
    let current = true
    void loadMainlandSite().then((value) => {
      if (current) setSite(value)
    })
    return () => {
      current = false
    }
  }, [active])

  return active ? site : null
}
