import type { Locale, Translator } from "@/types"
import type { KeyboardEvent } from "react"
import { useEffect, useRef, useState } from "react"
import { ChevronLeft, ChevronRight } from "lucide-react"

import {
  formatMonth,
  formatMonthName,
  japanToday,
  monthStartOf,
} from "../format"

const COLUMNS = 4

/** A month's index key, `YYYY-MM`, as the month index stores it. */
export function monthKeyOf(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}`
}

/**
 * The years the picker offers: from the oldest indexed month (or the shown
 * year) to the latest month worth opening. Without an index the earlier end
 * stays open.
 */
function yearBounds(
  shown: Date,
  counts: ReadonlyMap<string, number> | null,
  latestMonth?: Date,
) {
  let min: number | null = null
  if (counts) {
    for (const [key, count] of counts)
      if (count > 0) {
        const year = Number(key.slice(0, 4))
        if (min == null || year < min) min = year
      }
    min = Math.min(min ?? shown.getFullYear(), shown.getFullYear())
  }
  const max = latestMonth
    ? Math.max(latestMonth.getFullYear(), shown.getFullYear())
    : null
  return { min, max }
}

/**
 * The month dialog's jump grid: a year stepper over twelve months. With a
 * month index, months without records are dimmed and skipped; months after
 * `latestMonth` are never offered. Arrow keys move between months, Page Up
 * and Page Down change the year, and Escape closes it.
 */
export function MonthPicker({
  id,
  shown,
  latestMonth,
  counts,
  countLabel,
  onChoose,
  onClose,
  locale,
  t,
}: {
  id: string
  /** The month the dialog shows. */
  shown: Date
  latestMonth?: Date
  /** Records per `YYYY-MM`; null when the index is unknown. */
  counts: ReadonlyMap<string, number> | null
  countLabel?: (count: number) => string
  onChoose: (month: Date) => void
  onClose: () => void
  locale: Locale
  t: Translator
}) {
  const [year, setYear] = useState(shown.getFullYear())
  const [focusIndex, setFocusIndex] = useState(shown.getMonth())
  const gridRef = useRef<HTMLDivElement>(null)
  const refocus = useRef(true)
  const { min, max } = yearBounds(shown, counts, latestMonth)
  const thisMonth = monthStartOf(japanToday())

  useEffect(() => {
    if (!refocus.current) return
    refocus.current = false
    gridRef.current
      ?.querySelectorAll<HTMLButtonElement>("button")
      [focusIndex]?.focus()
  }, [year, focusIndex])

  const months = Array.from({ length: 12 }, (_, index) => {
    const date = new Date(year, index, 1)
    const count = counts ? (counts.get(monthKeyOf(date)) ?? 0) : null
    const isShown = date.getTime() === shown.getTime()
    const afterLatest = latestMonth ? date > latestMonth : false
    const available = !afterLatest && (count == null || count > 0 || isShown)
    return { date, count, isShown, available }
  })
  const yearTotal = counts
    ? months.reduce((sum, month) => sum + (month.count ?? 0), 0)
    : null

  const stepYear = (offset: number) => {
    const next = year + offset
    if ((min != null && next < min) || (max != null && next > max)) return
    setYear(next)
  }

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      event.preventDefault()
      event.stopPropagation()
      onClose()
      return
    }
    const inGrid = gridRef.current?.contains(event.target as Node)
    let next = focusIndex
    switch (event.key) {
      case "ArrowLeft":
        next = focusIndex - 1
        break
      case "ArrowRight":
        next = focusIndex + 1
        break
      case "ArrowUp":
        next = focusIndex - COLUMNS
        break
      case "ArrowDown":
        next = focusIndex + COLUMNS
        break
      case "Home":
        next = 0
        break
      case "End":
        next = 11
        break
      case "PageUp":
      case "PageDown":
        event.preventDefault()
        refocus.current = Boolean(inGrid)
        stepYear(event.key === "PageUp" ? -1 : 1)
        return
      default:
        return
    }
    if (!inGrid) return
    event.preventDefault()
    refocus.current = true
    setFocusIndex(Math.min(11, Math.max(0, next)))
  }

  return (
    <div
      id={id}
      className="month-picker"
      role="dialog"
      aria-label={t("common.chooseMonth")}
      onKeyDown={onKeyDown}
    >
      <div className="month-picker-year">
        <button
          type="button"
          className="round-button"
          disabled={min != null && year <= min}
          onClick={() => stepYear(-1)}
          aria-label={t("common.previousYear")}
          title={t("common.previousYear")}
        >
          <ChevronLeft aria-hidden="true" />
        </button>
        <p className="month-picker-year-label" aria-live="polite">
          <strong>{year}</strong>
          {yearTotal != null && countLabel ? (
            <span>{countLabel(yearTotal)}</span>
          ) : null}
        </p>
        <button
          type="button"
          className="round-button"
          disabled={max != null && year >= max}
          onClick={() => stepYear(1)}
          aria-label={t("common.nextYear")}
          title={t("common.nextYear")}
        >
          <ChevronRight aria-hidden="true" />
        </button>
      </div>
      <div className="month-picker-grid" ref={gridRef}>
        {months.map(({ date, count, isShown, available }, index) => {
          const label = formatMonth(date, locale)
          return (
            <button
              type="button"
              key={index}
              className={`month-picker-month${
                date.getTime() === thisMonth.getTime() ? " is-today" : ""
              }`}
              tabIndex={index === focusIndex ? 0 : -1}
              aria-disabled={available ? undefined : true}
              aria-current={isShown ? "date" : undefined}
              aria-label={
                count != null && countLabel
                  ? `${label}, ${countLabel(count)}`
                  : label
              }
              onFocus={() => setFocusIndex(index)}
              onClick={() => {
                if (!available) return
                if (isShown) onClose()
                else onChoose(date)
              }}
            >
              <span>{formatMonthName(date, locale)}</span>
              {count != null ? (
                <span className="month-picker-count" aria-hidden="true">
                  {count || "·"}
                </span>
              ) : null}
            </button>
          )
        })}
      </div>
    </div>
  )
}
