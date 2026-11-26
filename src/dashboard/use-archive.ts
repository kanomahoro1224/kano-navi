import { useCallback, useEffect, useMemo, useRef, useState } from "react"

import type { ArchiveSnapshot } from "@/types"
import { createRequestScope } from "../lib/polling"
import { readLoadedPages, readScopedJson } from "./api"
import {
  type ArchiveRequest,
  archiveKey,
  archiveUrl,
  readArchivePage,
} from "./archive-query"
import { rememberPostTimeline, restoredPostTimeline } from "./post-archive"

/**
 * Reads one schedule month, or the post or video timeline, page by page. Like
 * the calendar it follows the board's revision: a change rereads the loaded
 * pages atomically, and a later page from another revision restarts the read
 * instead of being appended. Schedule and video archives also follow
 * successful board snapshots, whose minute/manual refresh expires time-based
 * projections without a revision change. A null request cancels the read.
 *
 * A post timeline opens on the run this document or browser last loaded for
 * the same start, without a read; it is read again when `revision`, the
 * month index's, differs from it, or is null because the index read failed.
 */
export function useArchive<T>(
  request: ArchiveRequest | null,
  revision?: number | null,
  snapshotVersion?: number,
) {
  const scope = useMemo(() => createRequestScope(), [])
  const key = request ? archiveKey(request) : null
  const [state, setState] = useState<{
    key: string | null
    snapshot: ArchiveSnapshot<T> | null
    isLoading: boolean
    error: Error | null
    /** The snapshot is a remembered one this hook has not read yet. */
    restored: boolean
  }>(() => {
    const restored =
      request?.kind === "posts" && key
        ? (restoredPostTimeline(key) as ArchiveSnapshot<T> | null)
        : null
    return {
      key: restored ? key : null,
      snapshot: restored,
      isLoading: false,
      error: null,
      restored: Boolean(restored),
    }
  })
  const keyRef = useRef(key)
  keyRef.current = key
  const requestRef = useRef(request)
  requestRef.current = request
  const stateRef = useRef(state)
  stateRef.current = state
  // Post pages carry no time-derived fields, so they follow the revision
  // alone; undefined (not known yet) and null (unknown) are targets apart.
  const refreshTarget =
    request?.kind === "posts"
      ? `posts:${revision}`
      : `${revision ?? ""}/${snapshotVersion ?? ""}`
  const refreshTargetRef = useRef(refreshTarget)
  refreshTargetRef.current = refreshTarget
  // Consume a target when its read starts. A newer target waits for active
  // work to settle; a failed target retries on the next snapshot, not in a loop.
  const requestedTargetRef = useRef(refreshTarget)

  const readPage = async (
    current: ArchiveRequest,
    page: number,
    signal: AbortSignal,
  ): Promise<ArchiveSnapshot<T>> => {
    signal.throwIfAborted()
    const result = await readScopedJson(archiveUrl(current, page), signal)
    signal.throwIfAborted()
    return readArchivePage<T>(result)
  }

  // Installs a read run; a post timeline is remembered for the next visit,
  // with the first page when this read began there.
  const settle = (
    currentKey: string | null,
    current: ArchiveRequest,
    snapshot: ArchiveSnapshot<T>,
    firstPage: ArchiveSnapshot<T> | null = null,
  ) => {
    if (current.kind === "posts" && currentKey)
      rememberPostTimeline(currentKey, current, snapshot, firstPage)
    setState({
      key: currentKey,
      snapshot,
      isLoading: false,
      error: null,
      restored: false,
    })
  }

  const load = useCallback(async () => {
    const current = requestRef.current
    const currentKey = keyRef.current
    if (!current || !currentKey) return
    requestedTargetRef.current = refreshTargetRef.current
    const loaded =
      stateRef.current.key === currentKey ? stateRef.current.snapshot : null
    setState((value) =>
      value.key === currentKey
        ? { ...value, isLoading: true }
        : {
            key: currentKey,
            snapshot: null,
            isLoading: true,
            error: null,
            restored: false,
          },
    )
    // A refresh rereads the run it has loaded, from its first page. A timeline
    // from a chosen month first reads the page after the month's too, so the
    // month always has enough below it to scroll to the top.
    const first = loaded?.firstPage ?? current.startPage ?? 1
    const through = loaded ? loaded.page : current.startPage ? first + 1 : 1
    let firstPage = null as ArchiveSnapshot<T> | null
    try {
      const read = await scope.run("archive", (signal) =>
        readLoadedPages(
          async (page) => {
            const answer = await readPage(current, page, signal)
            if (page === 1) firstPage = answer
            return answer
          },
          loaded,
          (before, page) => ({
            ...page,
            firstPage: before.firstPage ?? before.page,
            items: [...before.items, ...page.items],
          }),
          first,
          through,
        ),
      )
      if (keyRef.current !== currentKey) return
      // The first read names its page: a shorter timeline clamps it.
      settle(
        currentKey,
        current,
        { ...read, firstPage: read.firstPage ?? read.page },
        firstPage,
      )
    } catch (caught) {
      const error = caught instanceof Error ? caught : new Error(String(caught))
      if (
        scope.active &&
        keyRef.current === currentKey &&
        error.name !== "AbortError"
      )
        setState((value) => ({ ...value, isLoading: false, error }))
    }
  }, [scope])

  const loadMore = useCallback(async () => {
    const current = requestRef.current
    const { key: currentKey, snapshot } = stateRef.current
    if (
      !current ||
      !snapshot?.hasNext ||
      currentKey !== keyRef.current ||
      scope.busy("archive")
    )
      return
    setState((value) => ({ ...value, isLoading: true }))
    try {
      const next = await scope.run("archive", (signal) =>
        readPage(current, snapshot.page + 1, signal),
      )
      if (keyRef.current !== currentKey) return
      // A write moved the pages: reread the loaded ones instead of mixing.
      if (
        next.revision !== snapshot.revision ||
        next.page !== snapshot.page + 1
      ) {
        await load()
        return
      }
      settle(currentKey, current, {
        ...next,
        firstPage: snapshot.firstPage,
        items: [...snapshot.items, ...next.items],
      })
    } catch (caught) {
      const error = caught instanceof Error ? caught : new Error(String(caught))
      if (
        scope.active &&
        keyRef.current === currentKey &&
        error.name !== "AbortError"
      )
        setState((value) => ({ ...value, isLoading: false, error }))
    }
  }, [scope, load])

  // Reads the page before the first loaded one and puts its newer records
  // above the loaded ones; a write in between rereads the whole run from that
  // page instead.
  const loadNewer = useCallback(async () => {
    const current = requestRef.current
    const { key: currentKey, snapshot } = stateRef.current
    const first = snapshot?.firstPage ?? 1
    if (
      !current ||
      !snapshot ||
      first <= 1 ||
      currentKey !== keyRef.current ||
      scope.busy("archive")
    )
      return
    setState((value) => ({ ...value, isLoading: true }))
    try {
      const newer = await scope.run("archive", (signal) =>
        readPage(current, first - 1, signal),
      )
      if (keyRef.current !== currentKey) return
      if (newer.revision !== snapshot.revision || newer.page !== first - 1) {
        // Reread the whole run from the newer page, so the request still
        // brings its newer records.
        requestedTargetRef.current = refreshTargetRef.current
        const read = await scope.run("archive", (signal) =>
          readLoadedPages(
            (page) => readPage(current, page, signal),
            snapshot,
            (before, page) => ({
              ...page,
              firstPage: before.firstPage ?? before.page,
              items: [...before.items, ...page.items],
            }),
            first - 1,
            snapshot.page,
          ),
        )
        if (keyRef.current !== currentKey) return
        settle(currentKey, current, {
          ...read,
          firstPage: read.firstPage ?? read.page,
        })
        return
      }
      settle(currentKey, current, {
        ...snapshot,
        firstPage: newer.page,
        items: [...newer.items, ...snapshot.items],
      })
    } catch (caught) {
      const error = caught instanceof Error ? caught : new Error(String(caught))
      if (
        scope.active &&
        keyRef.current === currentKey &&
        error.name !== "AbortError"
      )
        setState((value) => ({ ...value, isLoading: false, error }))
    }
  }, [scope])

  useEffect(() => {
    scope.activate()
    return () => scope.dispose()
  }, [scope])

  useEffect(() => {
    // A remembered post timeline waits for the revision instead of a read.
    const { key: shownKey, restored } = stateRef.current
    if (key && !(restored && shownKey === key)) void load()
    return () => scope.cancelKey("archive")
  }, [key, load, scope])

  useEffect(() => {
    const { key: currentKey, snapshot, restored } = stateRef.current
    if (
      currentKey !== keyRef.current ||
      !snapshot ||
      requestedTargetRef.current === refreshTarget ||
      stateRef.current.isLoading ||
      scope.busy("archive")
    )
      return
    // A post run already at the index's revision is current. Without the
    // revision, only a remembered run is read again.
    if (
      requestRef.current?.kind === "posts" &&
      (revision === undefined ||
        (revision === null ? !restored : snapshot.revision === revision))
    ) {
      requestedTargetRef.current = refreshTarget
      return
    }
    void load()
  }, [refreshTarget, state.isLoading, load, scope])

  const current = state.key === key ? state : null
  return {
    archive: current?.snapshot || null,
    isLoading: Boolean(key) && (!current || current.isLoading),
    error: current?.error || null,
    reload: load,
    loadMore,
    loadNewer,
  }
}
