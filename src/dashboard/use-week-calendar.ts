import { useCallback, useEffect, useRef, useState } from "react"

import type { CalendarSnapshot } from "@/types"
import { type createRequestScope, startPolling } from "../lib/polling"
import { readLoadedPages, readScopedJson } from "./api"
import { rangeKey } from "./archive-query"
import { addDays, japanToday, localDateKey, startOfWeek } from "./format"

/** How often a failed calendar read retries while its week is shown. */
const CALENDAR_RETRY_MS = 8000

/** Inclusive Japan-date range of the board's calendar: one Monday-first week. */
export function weekRange(weekStart: Date) {
  return {
    from: localDateKey(weekStart),
    to: localDateKey(addDays(weekStart, 6)),
  }
}

/**
 * The board's one-week calendar: the selected week, its paged read and its
 * retry. The snapshot coordinator keeps it current through `afterSnapshot`
 * and `followRevision`; the scope is the board's, activated and disposed by
 * the board hook.
 */
export function useWeekCalendar(scope: ReturnType<typeof createRequestScope>) {
  const [weekStart, setWeekStart] = useState(() => startOfWeek(japanToday()))
  const range = weekRange(weekStart)
  const visibleKey = rangeKey(range)
  const [calendar, setCalendar] = useState<CalendarSnapshot>({
    events: [],
    scheduleImages: [],
    adjacent: { previous: null, next: null },
    total: 0,
    page: 1,
    hasNext: false,
    revision: 0,
    dayCounts: {},
  })
  // The week the calendar snapshot belongs to. While another week loads, the
  // board keeps this one rendered instead of pairing its data with new dates.
  const [calendarShownRange, setCalendarShownRange] = useState<{
    from: string
    to: string
  } | null>(null)
  const [calendarLoading, setCalendarLoading] = useState(true)
  const [calendarFailure, setCalendarFailure] = useState<{
    key: string
    error: Error
  } | null>(null)
  const calendarError =
    calendarFailure?.key === visibleKey ? calendarFailure.error : null
  const rangeRef = useRef(range)
  rangeRef.current = range
  const calendarRef = useRef(calendar)
  calendarRef.current = calendar
  const calendarKeyRef = useRef<string | null>(null)
  const calendarRefreshRef = useRef(false)
  const loadCalendar = useCallback(async () => {
    const { from, to } = rangeRef.current
    const key = rangeKey(rangeRef.current)
    const current = calendarKeyRef.current === key ? calendarRef.current : null
    calendarRefreshRef.current = false
    // Only a week without a loaded snapshot shows the placeholder; a refresh
    // keeps the current week rendered and swaps its data in place.
    if (!current) setCalendarLoading(true)
    try {
      const payload = await scope.run("calendar", async (signal) => {
        const readPage = async (page: number): Promise<CalendarSnapshot> => {
          signal.throwIfAborted()
          const result = await readScopedJson(
            `/api/calendar?from=${from}&to=${to}&page=${page}`,
            signal,
          )
          signal.throwIfAborted()
          if (
            !Array.isArray(result.events) ||
            !result.adjacent ||
            !Number.isInteger(result.total) ||
            !Number.isInteger(result.revision) ||
            result.page !== page
          )
            throw new Error("Invalid calendar snapshot")
          return result
        }
        return readLoadedPages(readPage, current, (loaded, page) => ({
          ...page,
          events: [...loaded.events, ...page.events],
        }))
      })
      if (key !== rangeKey(rangeRef.current)) return
      calendarKeyRef.current = key
      calendarRef.current = payload
      setCalendar(payload)
      setCalendarShownRange({ from, to })
      setCalendarFailure(null)
      setCalendarLoading(false)
      return { ok: true }
    } catch (caught) {
      const error = caught instanceof Error ? caught : new Error(String(caught))
      if (
        scope.active &&
        key === rangeKey(rangeRef.current) &&
        error.name !== "AbortError"
      ) {
        setCalendarFailure({ key, error })
        setCalendarLoading(false)
      }
      return { ok: false }
    }
  }, [scope])
  const loadMoreCalendar = useCallback(async () => {
    const current = calendarRef.current
    if (!current.hasNext || scope.busy("calendar")) return
    const { from, to } = rangeRef.current
    const key = rangeKey(rangeRef.current)
    if (calendarKeyRef.current !== key) return
    try {
      const next = await scope.run("calendar", (signal) =>
        readScopedJson(
          `/api/calendar?from=${from}&to=${to}&page=${current.page + 1}`,
          signal,
        ),
      )
      if (key !== rangeKey(rangeRef.current)) return
      if (next.revision !== current.revision) {
        await loadCalendar()
        return
      }
      const combined = { ...next, events: [...current.events, ...next.events] }
      calendarRef.current = combined
      setCalendar(combined)
      setCalendarFailure(null)
    } catch (caught) {
      const error = caught instanceof Error ? caught : new Error(String(caught))
      if (
        scope.active &&
        key === rangeKey(rangeRef.current) &&
        error.name !== "AbortError"
      )
        setCalendarFailure({ key, error })
    }
  }, [scope, loadCalendar])
  const calendarRetryRef = useRef(false)
  calendarRetryRef.current = Boolean(calendarError)

  // What the snapshot coordinator calls. A failed week waits for its retry,
  // and active calendar work finishes first.
  const afterSnapshot = useCallback(
    (revision: number | null, later: boolean) => {
      calendarRefreshRef.current ||= later
      if (
        !scope.busy("calendar") &&
        !calendarRetryRef.current &&
        (calendarRefreshRef.current ||
          calendarKeyRef.current !== rangeKey(rangeRef.current) ||
          revision !== calendarRef.current.revision)
      )
        void loadCalendar()
    },
    [scope, loadCalendar],
  )
  const followRevision = useCallback(
    (revision: number) => {
      if (
        (calendarRefreshRef.current ||
          revision !== calendarRef.current.revision) &&
        !scope.busy("calendar") &&
        !calendarRetryRef.current
      )
        void loadCalendar()
    },
    [scope, loadCalendar],
  )

  useEffect(() => {
    void loadCalendar()
    const stop = startPolling(
      async () => {
        if (
          document.visibilityState === "hidden" ||
          scope.busy("calendar") ||
          !calendarRetryRef.current
        )
          return
        const result = await loadCalendar()
        if (!result?.ok) throw new Error("calendar_unavailable")
      },
      { delay: () => CALENDAR_RETRY_MS },
    )
    return () => {
      stop()
      scope.cancelKey("calendar")
    }
  }, [visibleKey, loadCalendar, scope])

  return {
    afterSnapshot,
    followRevision,
    state: {
      calendar,
      calendarShownRange,
      // The selected week is loading until its own snapshot arrives; a
      // failed read stops reporting it until a retry starts.
      calendarLoading:
        calendarShownRange == null ||
        rangeKey(calendarShownRange) !== visibleKey
          ? calendarLoading || !calendarError
          : false,
      calendarError,
      reloadCalendar: loadCalendar,
      loadMoreCalendar,
      weekStart,
      setWeekStart,
      calendarRange: range,
    },
  }
}
