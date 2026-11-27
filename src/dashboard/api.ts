/**
 * The board's transport: every public API read the board and the posts page
 * make goes through these JSON readers. Callers own the URLs, payload checks
 * and state.
 */

/** Reads JSON under a request scope's signal and deadline. */
export async function readScopedJson(url: string, signal: AbortSignal) {
  const response = await fetch(url, {
    signal,
    headers: { accept: "application/json" },
    cache: "no-cache",
  })
  if (!response.ok) throw new Error(`API ${response.status}`)
  return response.json()
}

/**
 * Reads JSON under one deadline that includes headers and body consumption.
 * Aborting also rejects the logical request if a stale transport ignores
 * cancellation.
 */
export async function readJsonWithin(
  url: string,
  controller: AbortController,
  timeoutMs: number,
) {
  let deadline: number
  let onAbort: () => void
  const cancelled = new Promise<never>((_, reject) => {
    onAbort = () =>
      reject(controller.signal.reason || new Error("Request cancelled"))
    controller.signal.addEventListener("abort", onAbort, { once: true })
    deadline = window.setTimeout(
      () => controller.abort(new Error("API request timed out")),
      timeoutMs,
    )
  })
  try {
    return await Promise.race([
      cancelled,
      (async () => {
        // The snapshot carries an ETag, so an unchanged one revalidates
        // with a bodiless 304; the revision reply is never stored.
        const response = await fetch(url, {
          headers: { accept: "application/json" },
          signal: controller.signal,
          cache: "no-cache",
        })
        if (!response.ok) throw new Error(`API ${response.status}`)
        return response.json()
      })(),
    ])
  } finally {
    window.clearTimeout(deadline!)
    controller.signal.removeEventListener("abort", onAbort!)
  }
}

export type PagedSnapshot = { page: number; hasNext: boolean; revision: number }

/**
 * Rereads the pages already loaded for one range, from `first` through
 * `through`, and replaces them atomically. If a write lands between pages, it
 * restarts under the caller's deadline; it never appends pages from different
 * revisions.
 */
export async function readLoadedPages<T extends PagedSnapshot>(
  readPage: (page: number) => Promise<T>,
  current: T | null,
  join: (loaded: T, next: T) => T,
  first = 1,
  through = current?.page || 1,
): Promise<T> {
  for (let attempt = 0; attempt < 3; attempt++) {
    let next = await readPage(first)
    // Public projections such as liveStatus can expire without a stored
    // write, so even an unchanged revision must replace the loaded pages.
    const revision = next.revision
    let consistent = true
    while (next.hasNext && next.page < through) {
      const page = await readPage(next.page + 1)
      if (page.revision !== revision) {
        consistent = false
        break
      }
      next = join(next, page)
    }
    if (consistent) return next
  }
  throw new Error("Snapshot changed during refresh")
}
