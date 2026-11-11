import { useEffect, useState } from "react"
import {
  ArrowRight,
  CalendarPlus,
  ChevronLeft,
  ChevronRight,
  ListChecks,
  Search,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { formatMonth, japanToday } from "@/admin/format"
import { type AdminEvent } from "@/admin/types"
import {
  Button,
  DateRange,
  Empty,
  IconButton,
  Loading,
  PageHeader,
  Pager,
  Panel,
  Segmented,
} from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"
import { EventMergeDialog, shiftDay } from "@/admin/views/event-parts"
import {
  ScheduleCalendar,
  shiftMonth,
  weekStart,
} from "@/admin/views/schedule-calendar"
import { ScheduleImageList } from "@/admin/views/schedule-images"
import { DayHeading, EventRow, groupByDay } from "@/admin/views/schedule-parts"
import { EventDialog } from "@/admin/views/event-dialog"

type Scope = "upcoming" | "past" | "all"

/** The date range each scope tab sets; any other range is custom. */
function scopeRange(scope: Scope, today: string) {
  if (scope === "upcoming") return { from: today, to: "" }
  if (scope === "past") return { from: "", to: shiftDay(today, -1) }
  return { from: "", to: "" }
}

function scopeOf(from: string, to: string, today: string): Scope | "" {
  return (
    (["upcoming", "past", "all"] as const).find((scope) => {
      const range = scopeRange(scope, today)
      return range.from === from && range.to === to
    }) ?? ""
  )
}

type ScheduleMode = "calendar" | "list"

/** What the list shows: the entries or the schedule images. */
type Records = "entries" | "images"

type Status = "all" | "unconfirmed" | "suspected" | "deleted"

/** Each status tab is a review task; All keeps merged duplicates, dimmed. */
const statusFilters: Record<Status, Record<string, string>> = {
  all: {},
  unconfirmed: { provenance: "unconfirmed" },
  suspected: { cancellation: "llm_suspected" },
  deleted: { deleted: "only" },
}

const modeKey = "adm-schedule-view"

/** The view this browser used last, the calendar by default; storage may be
 * unavailable. */
function storedMode(): ScheduleMode {
  try {
    return window.localStorage.getItem(modeKey) === "list" ? "list" : "calendar"
  } catch {
    return "calendar"
  }
}

export function ScheduleView({ tab, navigate }: ViewProps) {
  const { t, locale } = useAppSettings()
  const { api, overview } = useAdmin()
  const today = japanToday()
  const [mode, setMode] = useState(storedMode)
  const [records, setRecords] = useState<Records>("entries")
  // The calendar shows the selected day's month and week.
  const [day, setDay] = useState(today)
  const [reviewOnly, setReviewOnly] = useState(false)
  // A month tab (YYYY-MM) opens the calendar on that month, a date tab
  // (YYYY-MM-DD) on that day, a look tab (look-YYYY-MM-DD) on that day with
  // only what needs a look, and the images tab lists the schedule images.
  useEffect(() => {
    if (tab === "images") {
      setMode("list")
      setRecords("images")
    } else if (/^\d{4}-\d{2}$/u.test(tab)) {
      setMode("calendar")
      setDay(`${tab}-01`)
    } else if (/^(?:look-)?\d{4}-\d{2}-\d{2}$/u.test(tab)) {
      setMode("calendar")
      setDay(tab.slice(-10))
      setReviewOnly(tab.startsWith("look-"))
    }
  }, [tab])
  const [reviewCount, setReviewCount] = useState<number | undefined>()
  const [status, setStatus] = useState<Status>("all")
  const [search, setSearch] = useState("")
  const [query, setQuery] = useState("")
  // The list opens on today onward.
  const [range, setRange] = useState(() => scopeRange("upcoming", today))
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<
    { event: AdminEvent | null; day?: string } | undefined
  >(undefined)
  const [merging, setMerging] = useState<AdminEvent | null>(null)
  // Bumped after a dialog saves, so whichever view is shown reads again.
  const [reloadKey, setReloadKey] = useState(0)
  const reload = () => setReloadKey((value) => value + 1)
  const scope = scopeOf(range.from, range.to, today)
  // While only what needs a look shows, the weeks that hold some lead from
  // one to the next, round to the earliest after the last.
  const attention = useResource(
    () =>
      mode === "calendar" && reviewOnly
        ? api.scheduleAttention()
        : Promise.resolve(null),
    [mode, reviewOnly, reloadKey],
  )
  const lookWeeks = (attention.data?.weeks ?? [])
    .map((week) => week.weekStart)
    .filter((week) => week !== weekStart(day))
  const nextWeek =
    lookWeeks.find((week) => week > weekStart(day)) ?? lookWeeks[0]
  const queued = overview
    ? overview.review.pendingImages +
      overview.review.suspectedCancellations +
      overview.review.uncertainEvents
    : 0
  const showImages = mode === "list" && records === "images"
  const events = useResource(
    () =>
      mode === "list" && records === "entries"
        ? api.events({
            ...statusFilters[status],
            search: query,
            ...range,
            page,
            pageSize: 50,
            // A range with a start reads forward from it; the past and all
            // dates read back from the newest.
            order: range.from ? "asc" : "desc",
          })
        : Promise.resolve(null),
    [mode, records, status, query, range, page, reloadKey],
  )
  /** Switches the view; a search's switch to the list is not remembered. */
  const changeMode = (next: ScheduleMode, remember = true) => {
    if (tab) navigate("schedule")
    setMode(next)
    if (!remember) return
    try {
      window.localStorage.setItem(modeKey, next)
    } catch {
      // The choice simply is not remembered.
    }
  }
  // Moving leaves a dated address behind, so a later link opens it again.
  const showDay = (next: string) => {
    setDay(next)
    if (tab) navigate("schedule")
  }
  // Paging months selects the first day, or today in the current month.
  const showMonth = (months: number) => {
    const first = shiftMonth(day, months)
    showDay(first.slice(0, 7) === today.slice(0, 7) ? today : first)
  }
  const changeRange = (patch: { from?: string; to?: string }) => {
    setRange((current) => ({ ...current, ...patch }))
    setPage(1)
  }
  // The calendar has no search of its own: a search lists its matches on
  // every date.
  const submitSearch = () => {
    if (mode === "calendar") {
      if (!search.trim()) return
      setQuery(search)
      setStatus("all")
      setRange(scopeRange("all", today))
      setPage(1)
      changeMode("list", false)
      return
    }
    if (search === query) return
    setQuery(search)
    setPage(1)
  }
  const data = events.data

  return (
    <div className="adm-view">
      <PageHeader
        title={t("admin.schedule.title")}
        actions={
          <Button
            variant="primary"
            icon={CalendarPlus}
            onClick={() =>
              setEditing({
                event: null,
                day: mode === "calendar" ? day : undefined,
              })
            }
          >
            {t("admin.schedule.create")}
          </Button>
        }
      />
      <Panel>
        <div className="adm-schedule-bar">
          <Segmented
            label={t("admin.schedule.view")}
            value={mode}
            options={[
              { value: "calendar", label: t("admin.schedule.view.calendar") },
              { value: "list", label: t("admin.schedule.view.list") },
            ]}
            onChange={(next) => changeMode(next)}
          />
          {mode === "list" ? (
            <Segmented
              label={t("admin.schedule.records")}
              value={records}
              options={[
                {
                  value: "entries",
                  label: t("admin.schedule.records.entries"),
                },
                { value: "images", label: t("admin.schedule.records.images") },
              ]}
              onChange={(next) => {
                if (tab) navigate("schedule")
                setRecords(next)
              }}
            />
          ) : null}
          {showImages ? null : mode === "list" ? (
            <div className="adm-schedule-dates">
              <Segmented
                label={t("admin.schedule.scope")}
                value={scope}
                options={(["upcoming", "past", "all"] as const).map(
                  (value) => ({
                    value,
                    label: t(`admin.schedule.scope.${value}`),
                  }),
                )}
                onChange={(value) => {
                  if (value) changeRange(scopeRange(value, today))
                }}
              />
              <DateRange
                from={range.from}
                to={range.to}
                onChange={changeRange}
              />
            </div>
          ) : (
            <div className="adm-month-nav">
              <IconButton
                icon={ChevronLeft}
                label={t("admin.schedule.month.previous")}
                onClick={() => showMonth(-1)}
              />
              <strong aria-live="polite">{formatMonth(day, locale)}</strong>
              <IconButton
                icon={ChevronRight}
                label={t("admin.schedule.month.next")}
                onClick={() => showMonth(1)}
              />
              <Button
                size="sm"
                variant="ghost"
                disabled={day === today}
                onClick={() => showDay(today)}
              >
                {t("admin.schedule.day.today")}
              </Button>
            </div>
          )}
        </div>
        {showImages ? null : (
          <div className="adm-list-tools">
            {mode === "list" ? (
              <Segmented
                label={t("admin.schedule.status")}
                value={status}
                options={(
                  ["all", "unconfirmed", "suspected", "deleted"] as const
                ).map((value) => ({
                  value,
                  label: t(`admin.schedule.status.${value}`),
                }))}
                onChange={(value) => {
                  setStatus(value)
                  setPage(1)
                }}
              />
            ) : (
              <Segmented
                label={t("admin.schedule.status")}
                value={reviewOnly ? "review" : "all"}
                options={[
                  { value: "all", label: t("admin.schedule.status.all") },
                  {
                    value: "review",
                    label: t("admin.schedule.review.filter"),
                    count: reviewCount || undefined,
                  },
                ]}
                onChange={(value) => {
                  if (tab) navigate("schedule")
                  setReviewOnly(value === "review")
                }}
              />
            )}
            {mode === "calendar" && reviewOnly ? (
              <div className="adm-look-nav">
                <Button
                  size="sm"
                  variant="ghost"
                  icon={ArrowRight}
                  disabled={!nextWeek}
                  onClick={() => nextWeek && showDay(nextWeek)}
                >
                  {t("admin.schedule.review.nextWeek")}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  icon={ListChecks}
                  onClick={() => navigate("review", "schedule")}
                >
                  {t("admin.schedule.review.queue", { count: queued })}
                </Button>
              </div>
            ) : null}
            <form
              className="adm-search"
              role="search"
              onSubmit={(event) => {
                event.preventDefault()
                submitSearch()
              }}
            >
              <Search aria-hidden />
              <input
                type="search"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                onBlur={() => {
                  if (mode === "list") submitSearch()
                }}
                placeholder={t("admin.schedule.search")}
                aria-label={t("admin.schedule.search")}
              />
            </form>
          </div>
        )}
        {showImages ? (
          <ScheduleImageList
            filters={["active", "approved", "undated", "rejected"]}
            navigate={navigate}
          />
        ) : mode === "calendar" ? (
          <ScheduleCalendar
            day={day}
            reviewOnly={reviewOnly}
            reloadKey={reloadKey}
            onDay={showDay}
            onEdit={(event) => setEditing({ event })}
            onMerge={setMerging}
            onCreate={(chosen) => setEditing({ event: null, day: chosen })}
            onChanged={reload}
            onCounts={setReviewCount}
          />
        ) : !data ? (
          <Loading />
        ) : data.items.length ? (
          <>
            <div className="adm-schedule-days">
              {groupByDay(data.items).map(({ day: date, events: items }) => (
                <section key={date} className="adm-day">
                  <DayHeading
                    day={date}
                    today={today}
                    count={items.length}
                    onCreate={(chosen) =>
                      setEditing({ event: null, day: chosen })
                    }
                  />
                  <ul className="adm-event-list">
                    {items.map((event) => (
                      <EventRow
                        key={event.id}
                        event={event}
                        onEdit={(chosen) => setEditing({ event: chosen })}
                        onMerge={setMerging}
                        onChanged={reload}
                      />
                    ))}
                  </ul>
                </section>
              ))}
            </div>
            <Pager
              page={data.page}
              totalPages={data.totalPages}
              total={data.total}
              onPage={setPage}
            />
          </>
        ) : (
          <Empty icon={CalendarPlus} title={t("admin.schedule.empty")}>
            {t("admin.schedule.emptyHint")}
          </Empty>
        )}
      </Panel>
      {editing !== undefined ? (
        <EventDialog
          event={editing.event}
          day={editing.day}
          onClose={(saved) => {
            setEditing(undefined)
            if (saved) reload()
          }}
        />
      ) : null}
      {merging ? (
        <EventMergeDialog
          event={merging}
          onClose={(merged) => {
            setMerging(null)
            if (merged) reload()
          }}
        />
      ) : null}
    </div>
  )
}
