import { useEffect, useRef } from "react"
import {
  ChevronLeft,
  ChevronRight,
  Image as ImageIcon,
  ImageOff,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { formatDay, formatWeekday, japanToday } from "@/admin/format"
import type { AdminEvent, ScheduleImage } from "@/admin/types"
import { IconButton, Loading } from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"
import { shiftDay } from "@/admin/views/event-parts"
import { needsReview } from "@/admin/views/schedule-images"
import { DayHeading, EventRow, reviewOf } from "@/admin/views/schedule-parts"
import { WeekImage } from "@/admin/views/schedule-week-image"

/** Dots a day cell shows before "+N". */
const dotLimit = 4

/** The first day of the month `months` away from the month of `day`. */
export function shiftMonth(day: string, months: number) {
  const [year, month] = day.split("-").map(Number)
  return new Date(Date.UTC(year, month - 1 + months, 1))
    .toISOString()
    .slice(0, 10)
}

/** The Monday on or before a Japan date; weeks start on Monday, as on the
 * board. */
export function weekStart(day: string) {
  const weekday = new Date(`${day}T12:00:00+09:00`).getUTCDay()
  return shiftDay(day, -((weekday + 6) % 7))
}

/** Whole weeks covering the month that starts on `first`, one row each. */
function monthWeeks(first: string) {
  const start = weekStart(first)
  const end = weekStart(shiftDay(shiftMonth(first, 1), -1))
  const weeks: string[][] = []
  for (let week = start; week <= end; week = shiftDay(week, 7))
    weeks.push(Array.from({ length: 7 }, (_, index) => shiftDay(week, index)))
  return weeks
}

/** The state an entry's dot shows; the legend names each. */
function dotState(event: AdminEvent) {
  if (event.cancellationStatus === "manual_confirmed") return "is-cancelled"
  const review = reviewOf(event)
  if (review) return `is-${review.kind}`
  if (event.hiddenByPosts) return "is-hidden"
  return "is-shown"
}

/** Whether an image's dates overlap the days from `from` to `to`. */
function overlaps(image: ScheduleImage, from: string, to: string) {
  if (!image.periodStart) return false
  return (
    image.periodStart <= to && (image.periodEnd || image.periodStart) >= from
  )
}

function Legend() {
  const { t } = useAppSettings()
  return (
    <ul className="adm-cal-legend">
      {(
        [
          ["is-shown", "admin.schedule.legend.shown"],
          ["is-uncertain", "admin.schedule.review.uncertain"],
          ["is-suspected", "admin.schedule.review.suspected"],
          ["is-cancelled", "admin.event.cancelled"],
          ["is-hidden", "admin.event.hiddenByPosts"],
        ] as const
      ).map(([state, key]) => (
        <li key={state}>
          <i className={`adm-cal-dot ${state}`} aria-hidden />
          {t(key)}
        </li>
      ))}
      <li>
        <ImageIcon aria-hidden className="adm-cal-week-mark" />
        {t("admin.schedule.legend.image")}
      </li>
    </ul>
  )
}

/**
 * The schedule as a phone calendar: a Monday-first month grid on the left
 * whose days show their entries as dots coloured by review state and whose
 * weeks mark a schedule image, the selected week's images below it, and the
 * selected week's seven days on the right with their rows, review notes and
 * new entry buttons. Choosing a day selects it and its week; a double click
 * starts a new entry on it. Merged duplicates sit under the entry they are
 * merged into; with `reviewOnly` only the entries that need a look show.
 * The week's images labelled Not a schedule fold away below its others.
 * Under 1000px the grid, the images and the days stack.
 */
export function ScheduleCalendar({
  day,
  reviewOnly,
  reloadKey,
  onDay,
  onEdit,
  onMerge,
  onCreate,
  onChanged,
  onCounts,
}: {
  /** The selected Japan date; its month and week are shown. */
  day: string
  reviewOnly: boolean
  reloadKey: number
  onDay: (day: string) => void
  onEdit: (event: AdminEvent) => void
  onMerge: (event: AdminEvent) => void
  onCreate: (day: string) => void
  onChanged: () => void
  /** Reports how many shown entries need a look. */
  onCounts?: (review: number) => void
}) {
  const { t, locale } = useAppSettings()
  const { api } = useAdmin()
  const today = japanToday()
  const month = shiftMonth(day, 0)
  const weeks = monthWeeks(month)
  const from = weeks[0][0]
  const to = weeks[weeks.length - 1][6]
  const week = weekStart(day)
  const sunday = shiftDay(week, 6)
  const events = useResource(
    () => api.events({ from, to, order: "asc", pageSize: 100 }),
    [from, to, reloadKey],
  )
  const images = useResource(
    () => api.scheduleImages({ from, to }),
    [from, to, reloadKey],
  )

  const items = events.data?.items ?? []
  const merged = new Map<string, AdminEvent[]>()
  const byDay = new Map<string, AdminEvent[]>()
  let reviewCount = 0
  for (const event of items) {
    if (event.mergedInto) {
      const list = merged.get(event.mergedInto) ?? []
      list.push(event)
      merged.set(event.mergedInto, list)
      continue
    }
    const review = reviewOf(event)
    if (review) reviewCount += 1
    if (reviewOnly && !review) continue
    const list = byDay.get(event.startsOn) ?? []
    list.push(event)
    byDay.set(event.startsOn, list)
  }
  useEffect(() => {
    if (events.data) onCounts?.(reviewCount)
  }, [events.data, reviewCount, onCounts])

  const shownImages = (images.data?.items ?? []).filter(
    (image) => image.manualStatus !== "not_schedule",
  )
  const weekImages = shownImages.filter((image) =>
    overlaps(image, week, sunday),
  )
  // Images labelled Not a schedule fold away under the week's, where the
  // label can be undone.
  const excluded = (images.data?.items ?? []).filter(
    (image) =>
      image.manualStatus === "not_schedule" && overlaps(image, week, sunday),
  )
  const weekImage = (image: ScheduleImage) => (
    <WeekImage
      key={image.id}
      image={image}
      week={week}
      onWeek={onDay}
      onChanged={(next) =>
        images.setData({
          items: (images.data?.items ?? []).map((item) =>
            item.id === next.id ? next : item,
          ),
        })
      }
    />
  )

  // A day chosen on the grid scrolls into view among the week's days.
  const chosen = useRef(false)
  const agenda = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!chosen.current) return
    chosen.current = false
    agenda.current
      ?.querySelector(`[data-day="${day}"]`)
      ?.scrollIntoView({ block: "nearest", behavior: "smooth" })
  }, [day])
  const choose = (next: string) => {
    chosen.current = true
    onDay(next)
  }

  return (
    <div className="adm-planner">
      <div className="adm-planner-side">
        <div className="adm-cal" role="group" aria-busy={!events.data}>
          <div className="adm-cal-weekdays" aria-hidden>
            {weeks[0].map((weekday) => (
              <span key={weekday}>{formatWeekday(weekday, locale)}</span>
            ))}
            <span />
          </div>
          {weeks.map((row) => {
            const rowImages = shownImages.filter((image) =>
              overlaps(image, row[0], row[6]),
            )
            const pending = rowImages.some(needsReview)
            return (
              <div
                key={row[0]}
                className={`adm-cal-row${row[0] === week ? " is-selected" : ""}`}
              >
                {row.map((cell) => {
                  const list = byDay.get(cell) ?? []
                  const more = list.length - dotLimit
                  return (
                    <button
                      key={cell}
                      type="button"
                      className={`adm-cal-cell${cell.slice(0, 7) === month.slice(0, 7) ? "" : " is-outside"}${cell === today ? " is-today" : ""}${cell === day ? " is-selected" : ""}`}
                      aria-pressed={cell === day}
                      aria-label={`${formatDay(cell, locale)} · ${t("admin.schedule.day.count", { count: list.length })}`}
                      onClick={() => choose(cell)}
                      onDoubleClick={() => onCreate(cell)}
                    >
                      <span className="adm-cal-date">
                        {Number(cell.slice(8))}
                      </span>
                      <span className="adm-cal-dots" aria-hidden>
                        {list.slice(0, dotLimit).map((event) => (
                          <i
                            key={event.id}
                            className={`adm-cal-dot ${dotState(event)}`}
                          />
                        ))}
                        {more > 0 ? <small>+{more}</small> : null}
                      </span>
                    </button>
                  )
                })}
                <span
                  className={`adm-cal-week-mark-cell${pending ? " is-pending" : ""}`}
                  title={
                    rowImages.length
                      ? t(
                          pending
                            ? "admin.schedule.week.imagePending"
                            : "admin.schedule.week.imageCovered",
                        )
                      : undefined
                  }
                >
                  {rowImages.length ? (
                    <ImageIcon aria-hidden className="adm-cal-week-mark" />
                  ) : null}
                </span>
              </div>
            )
          })}
        </div>
        <Legend />
        {events.data && events.data.total > events.data.items.length ? (
          <p className="adm-muted">
            {t("admin.schedule.calendar.truncated", {
              count: events.data.items.length,
            })}
          </p>
        ) : null}
        <section
          className="adm-planner-images"
          aria-label={t("admin.schedule.week.images")}
        >
          {!images.data ? (
            <Loading />
          ) : weekImages.length ? (
            weekImages.map(weekImage)
          ) : (
            <p className="adm-week-noimage">
              <ImageOff aria-hidden />
              {t("admin.schedule.week.noImage")}
            </p>
          )}
          {excluded.length ? (
            <details className="adm-disclosure adm-planner-excluded">
              <summary>
                {t("admin.schedule.week.excluded", { count: excluded.length })}
              </summary>
              <div className="adm-planner-images">
                {excluded.map(weekImage)}
              </div>
            </details>
          ) : null}
        </section>
      </div>
      <div className="adm-planner-agenda" ref={agenda}>
        <header className="adm-planner-week">
          <IconButton
            icon={ChevronLeft}
            label={t("admin.schedule.week.previous")}
            onClick={() => onDay(shiftDay(week, -7))}
          />
          <strong aria-live="polite">
            {`${formatDay(week, locale)} – ${formatDay(sunday, locale)}`}
          </strong>
          <IconButton
            icon={ChevronRight}
            label={t("admin.schedule.week.next")}
            onClick={() => onDay(shiftDay(week, 7))}
          />
        </header>
        {!events.data ? (
          <Loading />
        ) : (
          Array.from({ length: 7 }, (_, index) => shiftDay(week, index)).map(
            (date) => {
              const list = byDay.get(date) ?? []
              return (
                <section
                  key={date}
                  data-day={date}
                  className={`adm-day adm-week-day${list.length ? "" : " is-empty"}${date === day ? " is-selected" : ""}`}
                >
                  <DayHeading
                    day={date}
                    today={today}
                    count={list.length}
                    onCreate={onCreate}
                  />
                  {list.length ? (
                    <ul className="adm-event-list">
                      {list.map((event) => (
                        <EventRow
                          key={event.id}
                          event={event}
                          merged={merged.get(event.id)}
                          onEdit={onEdit}
                          onMerge={onMerge}
                          onChanged={onChanged}
                        />
                      ))}
                    </ul>
                  ) : null}
                </section>
              )
            },
          )
        )}
      </div>
    </div>
  )
}
