import { useCallback, useEffect, useMemo, useRef, useState } from "react"

import { createRequestScope } from "../lib/polling"
import {
  emptyDashboard,
  mergeSnapshot,
  readCachedSnapshot,
  writeCachedSnapshot,
} from "./snapshot-cache"
import {
  type LoadResult,
  type SnapshotSyncHandlers,
  startSnapshotSync,
} from "./snapshot-sync"
import { useWeekCalendar } from "./use-week-calendar"

// The board's and the posts page's data hooks. The request logic lives in
// the sibling modules this one composes; pages import it from here.
export {
  type ArchiveKind,
  type ArchiveRequest,
  archiveRange,
  TIMELINE_PAGE_SIZE,
  timelineStartPage,
} from "./archive-query"
export {
  type PostArchiveIndex,
  type PostTimelineStart,
  rememberedPostTimeline,
  usePostArchiveIndex,
} from "./post-archive"
export { useArchive } from "./use-archive"
export { useArchiveMonths } from "./use-archive-months"
export { useNow } from "./use-now"
export { weekRange } from "./use-week-calendar"

const unavailable = async (): Promise<LoadResult> => ({
  ok: false,
  error: new Error("Page is unavailable"),
})

/**
 * The board's snapshot and its week calendar. `startSnapshotSync` reads the
 * snapshot and follows its revision; `useWeekCalendar` reads the selected
 * week, which the snapshot coordinator keeps current. A visit starts from the last snapshot this
 * browser stored, refreshing it, rather than from the loading placeholder.
 */
export function useDashboard() {
  const [cached] = useState(readCachedSnapshot)
  const shownCacheRef = useRef(cached != null)
  const [dashboard, setDashboard] = useState(cached || emptyDashboard)
  const [snapshotVersion, setSnapshotVersion] = useState(0)
  const [status, setStatus] = useState<{
    isLoading: boolean
    isRefreshing: boolean
    error: Error | null
  }>({
    isLoading: !cached,
    isRefreshing: false,
    error: null,
  })
  const scope = useMemo(() => createRequestScope(), [])
  const reloadRef = useRef(unavailable)
  const load = useCallback(() => reloadRef.current(), [])
  // The calendar's handlers, read when a snapshot or revision arrives. The
  // snapshot effect is declared before the calendar hook, so the snapshot
  // read starts before the calendar's first read.
  const calendarSync = useRef<SnapshotSyncHandlers["calendar"] | null>(null)

  useEffect(() => {
    scope.activate()
    const sync = startSnapshotSync({
      onReadStart: (hasLoaded) =>
        setStatus((current) => ({
          ...current,
          isLoading: !hasLoaded && !shownCacheRef.current,
          isRefreshing: true,
        })),
      onSnapshot: (payload) => {
        setDashboard(mergeSnapshot(payload))
        writeCachedSnapshot(payload)
        setSnapshotVersion((version) => version + 1)
        setStatus({ isLoading: false, isRefreshing: false, error: null })
      },
      onReadFailed: (error) =>
        setStatus({ isLoading: false, isRefreshing: false, error }),
      calendar: {
        afterSnapshot: (revision, later) =>
          calendarSync.current?.afterSnapshot(revision, later),
        followRevision: (revision) =>
          calendarSync.current?.followRevision(revision),
      },
    })
    reloadRef.current = sync.reload
    return () => {
      sync.stop()
      scope.dispose()
      reloadRef.current = unavailable
    }
  }, [scope])

  const {
    afterSnapshot,
    followRevision,
    state: calendar,
  } = useWeekCalendar(scope)
  calendarSync.current = { afterSnapshot, followRevision }

  return {
    dashboard,
    snapshotVersion,
    ...status,
    reload: load,
    ...calendar,
  }
}
