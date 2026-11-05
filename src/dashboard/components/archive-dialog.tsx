import type { Locale, Translator } from "@/types"
import type { ReactNode } from "react"
import { useEffect, useRef, useState } from "react"
import { ChevronDown, ChevronLeft, ChevronRight, X } from "lucide-react"

import { EmptyNote, Placeholder } from "@/site/site-shell"
import { formatMonth, japanToday, monthStartOf } from "../format"
import { MonthPicker } from "./month-picker"

/** A month dialog's jump grid: its month index and how a count reads. */
export interface ArchivePicker {
  /** Records per `YYYY-MM`; null while unknown, which offers every month. */
  counts: ReadonlyMap<string, number> | null
  countLabel?: (count: number) => string
}

/**
 * A dialog that scrolls through every month instead of showing one: its month
 * label follows the scroll, and the month arrows give way to a Latest button.
 */
export interface ArchiveTimeline {
  /** Returns to the newest records. */
  onLatest: () => void
  /** Whether the newest records are already at the top of the view. */
  atLatest: boolean
  /** Scrolls the body back to the top whenever it changes. */
  resetKey: string
}

function shiftMonth(date: Date, offset: number) {
  return new Date(date.getFullYear(), date.getMonth() + offset, 1)
}

function dateFromKey(key: string) {
  const [year, month, day] = key.split("-").map(Number)
  return new Date(year, month - 1, day)
}

/**
 * Native modal sheet for browsing one month of stored records, or with
 * `timeline` a scrolling list of every month. Escape and the backdrop close
 * it, focus returns to the opener, and an image viewer opened from inside
 * stacks above it.
 */
export function ArchiveDialog({
  open,
  kicker,
  title,
  summary,
  monthStart,
  onMonthChange,
  latestMonth,
  picker,
  timeline,
  toolbar,
  onClose,
  children,
  locale,
  t,
}: {
  open: boolean
  kicker: ReactNode
  title: string
  summary?: ReactNode
  monthStart: Date
  onMonthChange: (month: Date) => void
  /** The last month worth opening; later months are not offered. */
  latestMonth?: Date
  /** Turns the month label into a button that opens a jump grid. */
  picker?: ArchivePicker
  /** Shows `monthStart` as the scrolled-to month of a continuous list. */
  timeline?: ArchiveTimeline
  toolbar?: ReactNode
  onClose: () => void
  children: ReactNode
  locale: Locale
  t: Translator
}) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)
  const bodyRef = useRef<HTMLDivElement>(null)
  const pickerRef = useRef<HTMLDivElement>(null)
  const pickerButtonRef = useRef<HTMLButtonElement>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  const closePicker = (refocus: boolean) => {
    setPickerOpen(false)
    if (refocus) pickerButtonRef.current?.focus()
  }

  // A press outside the month controls closes the jump grid.
  useEffect(() => {
    if (!pickerOpen) return undefined
    const onPointerDown = (event: PointerEvent) => {
      if (!pickerRef.current?.contains(event.target as Node))
        setPickerOpen(false)
    }
    document.addEventListener("pointerdown", onPointerDown)
    return () => document.removeEventListener("pointerdown", onPointerDown)
  }, [pickerOpen])

  useEffect(() => {
    const dialog = dialogRef.current
    if (!open || !dialog) return undefined
    const previous = document.activeElement as HTMLElement | null
    if (!dialog.open) dialog.showModal?.()
    closeRef.current?.focus()
    document.body.classList.add("has-sheet")
    return () => {
      if (dialog.open) dialog.close()
      document.body.classList.remove("has-sheet")
      previous?.focus?.()
    }
  }, [open])

  // A timeline's month follows the scroll, so only its reset key rewinds it.
  const resetKey =
    timeline?.resetKey ?? `${monthStart.getFullYear()}-${monthStart.getMonth()}`
  useEffect(() => {
    bodyRef.current?.scrollTo?.({ top: 0 })
    setPickerOpen(false)
  }, [resetKey])

  if (!open) return null
  const thisMonth = monthStartOf(japanToday())
  const isCurrent = monthStart.getTime() === thisMonth.getTime()
  const atLatest = latestMonth ? monthStart >= latestMonth : false
  const label = formatMonth(monthStart, locale)

  return (
    <dialog
      ref={dialogRef}
      className="archive-dialog"
      aria-labelledby="archive-dialog-title"
      onCancel={(event) => {
        event.preventDefault()
        // Escape closes an open jump grid before the dialog.
        if (pickerOpen) closePicker(true)
        else onClose()
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <div className="archive-dialog-frame">
        <header className="archive-dialog-head">
          <div className="archive-dialog-heading">
            <p className="kicker">{kicker}</p>
            <h2 id="archive-dialog-title" className="archive-dialog-title">
              {title}
            </h2>
          </div>
          <button
            ref={closeRef}
            type="button"
            className="round-button"
            onClick={onClose}
            aria-label={t("common.close")}
            title={t("common.close")}
          >
            <X aria-hidden="true" />
          </button>
          <div className="archive-dialog-bar">
            <div
              className="archive-month"
              role="group"
              aria-label={label}
              ref={pickerRef}
            >
              {timeline ? null : (
                <button
                  type="button"
                  className="round-button"
                  onClick={() => onMonthChange(shiftMonth(monthStart, -1))}
                  aria-label={t("common.previousMonth")}
                  title={t("common.previousMonth")}
                >
                  <ChevronLeft aria-hidden="true" />
                </button>
              )}
              {picker ? (
                <button
                  ref={pickerButtonRef}
                  type="button"
                  className="archive-month-label is-picker"
                  aria-haspopup="dialog"
                  aria-expanded={pickerOpen}
                  aria-controls={
                    pickerOpen ? "archive-month-picker" : undefined
                  }
                  title={t("common.chooseMonth")}
                  onClick={() => setPickerOpen((open) => !open)}
                >
                  {/* A scrolled month changes too often to announce. */}
                  <span aria-live={timeline ? undefined : "polite"}>
                    {label}
                  </span>
                  <ChevronDown aria-hidden="true" />
                </button>
              ) : (
                <strong className="archive-month-label" aria-live="polite">
                  {label}
                </strong>
              )}
              {timeline ? (
                <button
                  type="button"
                  className="pill-button"
                  disabled={timeline.atLatest}
                  onClick={timeline.onLatest}
                >
                  {t("common.latest")}
                </button>
              ) : (
                <>
                  <button
                    type="button"
                    className="round-button"
                    disabled={atLatest}
                    onClick={() => onMonthChange(shiftMonth(monthStart, 1))}
                    aria-label={t("common.nextMonth")}
                    title={t("common.nextMonth")}
                  >
                    <ChevronRight aria-hidden="true" />
                  </button>
                  <button
                    type="button"
                    className="pill-button"
                    disabled={isCurrent}
                    onClick={() => onMonthChange(thisMonth)}
                  >
                    {t("common.thisMonth")}
                  </button>
                </>
              )}
              {summary ? (
                <span className="archive-summary">{summary}</span>
              ) : null}
              {picker && pickerOpen ? (
                <MonthPicker
                  id="archive-month-picker"
                  shown={monthStart}
                  latestMonth={latestMonth}
                  counts={picker.counts}
                  countLabel={picker.countLabel}
                  onChoose={(month) => {
                    closePicker(true)
                    onMonthChange(month)
                  }}
                  onClose={() => closePicker(true)}
                  locale={locale}
                  t={t}
                />
              ) : null}
            </div>
            {toolbar}
          </div>
        </header>
        <div className="archive-dialog-body" ref={bodyRef}>
          {children}
        </div>
      </div>
    </dialog>
  )
}

/**
 * Shared states of an archive body: an error with retry, the first load, and
 * an empty month that offers the nearest months holding records.
 */
export function ArchiveState({
  error,
  onRetry,
  loading,
  empty,
  emptyTitle,
  hint,
  adjacent,
  onJump,
  locale,
  t,
}: {
  error: Error | null
  onRetry: () => unknown
  loading: boolean
  empty: boolean
  emptyTitle: string
  hint?: string
  adjacent?: { previous: string | null; next: string | null }
  onJump: (month: Date) => void
  locale: Locale
  t: Translator
}) {
  const previous = adjacent?.previous ? dateFromKey(adjacent.previous) : null
  const next = adjacent?.next ? dateFromKey(adjacent.next) : null
  return (
    <>
      {error ? (
        <div className="inline-alert" role="alert">
          <span>{t("error.api")}</span>
          <button type="button" className="pill-button" onClick={onRetry}>
            {t("common.retry")}
          </button>
        </div>
      ) : null}
      {loading ? (
        <Placeholder lines={4} />
      ) : empty ? (
        <EmptyNote title={emptyTitle} hint={hint}>
          {previous || next ? (
            <div className="archive-jumps">
              {previous ? (
                <button
                  type="button"
                  className="pill-button"
                  onClick={() => onJump(monthStartOf(previous))}
                >
                  <ChevronLeft aria-hidden="true" />
                  {t("common.jumpToMonth", {
                    month: formatMonth(previous, locale),
                  })}
                </button>
              ) : null}
              {next ? (
                <button
                  type="button"
                  className="pill-button"
                  onClick={() => onJump(monthStartOf(next))}
                >
                  {t("common.jumpToMonth", {
                    month: formatMonth(next, locale),
                  })}
                  <ChevronRight aria-hidden="true" />
                </button>
              ) : null}
            </div>
          ) : null}
        </EmptyNote>
      ) : null}
    </>
  )
}

/**
 * The archive's "load more" control, disabled while a page is in flight. With
 * `auto`, it also loads the next page once it scrolls near the visible area;
 * the button stays for keyboards and for a failed read, which never retries
 * on its own.
 */
export function ArchiveMore({
  hasNext,
  isLoading,
  label,
  onLoadMore,
  auto,
  t,
}: {
  hasNext: boolean
  isLoading: boolean
  label: string
  onLoadMore: () => unknown
  /** Loads on approach unless a read failed. */
  auto?: { failed: boolean; loaded: number }
  t: Translator
}) {
  const ref = useRef<HTMLDivElement>(null)
  const loadRef = useRef(onLoadMore)
  loadRef.current = onLoadMore
  const armed = Boolean(auto && hasNext && !isLoading && !auto.failed)
  // Observing anew after each page reports whether the control is still in
  // reach, so a short page keeps loading until the view fills.
  useEffect(() => {
    const node = ref.current
    if (!armed || !node || typeof IntersectionObserver !== "function")
      return undefined
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          observer.disconnect()
          void loadRef.current()
        }
      },
      {
        root: node.closest(".archive-dialog-body"),
        rootMargin: "0px 0px 600px 0px",
      },
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [armed, auto?.loaded])
  if (!hasNext) return null
  return (
    <div className="archive-more" ref={ref}>
      <button
        type="button"
        className="pill-button"
        disabled={isLoading}
        onClick={onLoadMore}
      >
        {isLoading ? t("common.refreshing") : label}
      </button>
    </div>
  )
}
