import { useEffect, useMemo, useState } from "react"

import type { ArchiveMonth } from "@/types"
import { createRequestScope } from "../lib/polling"
import { readScopedJson } from "./api"
import { monthIndexes, readArchiveMonth } from "./archive-query"

/**
 * Reads the month index of the post archive or the video timeline while
 * `enabled`: every Japan month holding records, oldest first. It rereads when
 * the board's revision changes. A failed read keeps the known index, and with
 * none `months` stays null, so the picker offers every month.
 */
export function useArchiveMonths(
  kind: "posts" | "videos",
  enabled: boolean,
  revision?: number | null,
) {
  const scope = useMemo(() => createRequestScope(), [])
  const [state, setState] = useState<{
    kind: string
    months: ArchiveMonth[]
  } | null>(null)

  useEffect(() => {
    scope.activate()
    return () => scope.dispose()
  }, [scope])

  useEffect(() => {
    if (!enabled) return undefined
    const index = monthIndexes[kind]
    scope
      .run("months", async (signal) => {
        const result = await readScopedJson(index.url, signal)
        if (!Array.isArray(result?.months))
          throw new Error("Invalid month index")
        return (result.months as unknown[])
          .map((month) => readArchiveMonth(month, index.counts))
          .filter((month): month is ArchiveMonth => month !== null)
      })
      .then(
        (months) => setState({ kind, months }),
        () => {
          // A failed read keeps a known index; the picker works without one.
        },
      )
    return () => scope.cancelKey("months")
  }, [kind, enabled, revision, scope])

  return state?.kind === kind ? state.months : null
}
