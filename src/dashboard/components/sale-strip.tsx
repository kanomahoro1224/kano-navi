import type { Locale, ScheduleEvent, Translator } from "@/types"
import { ArrowUpRight, X } from "lucide-react"
import { useState, useSyncExternalStore } from "react"

import { followCardLink } from "@/site/links"
import {
  cleanVideoTitle,
  currentSales,
  formatJapanDay,
  formatJapanTime,
  saleClosesAt,
  saleOpensAt,
  saleStatus,
  saleWindow,
} from "../format"
import { SheetDialog } from "./sheet-dialog"
import { TranslatedTitle } from "./translated-title"

/**
 * Where the sales sit on the stage: `main` under a compact ticket, as a row,
 * or `side` under the profile card beside a full ticket, as a stack.
 */
export type SalePlacement = "main" | "side"

/** How many sales show in place; every one opens in the sheet. */
const SALE_LIMIT = { main: 3, side: 2 }

/** The width at which the stage has its two columns. */
const stageColumns = "(min-width: 981px)"

function useStageColumns() {
  return useSyncExternalStore(
    (onChange) => {
      const media = window.matchMedia(stageColumns)
      media.addEventListener("change", onChange)
      return () => media.removeEventListener("change", onChange)
    },
    () => window.matchMedia(stageColumns).matches,
    () => true,
  )
}

/** A Japan day, with its time when the sale states one. */
function when(instant: number, timed: boolean, locale: Locale, t: Translator) {
  const day = formatJapanDay(instant, locale)
  return timed
    ? `${day} ${formatJapanTime(instant)} ${t("common.timezoneShort")}`
    : day
}

function SaleCard({
  event,
  locale,
  now,
  t,
}: {
  event: ScheduleEvent
  locale: Locale
  now: number
  t: Translator
}) {
  const open = saleStatus(event, now) === "open"
  const window = saleWindow(event)
  const line = open
    ? t("sales.closes", {
        date: when(saleClosesAt(event), Boolean(window.endsAt), locale, t),
      })
    : t("sales.opens", {
        date: when(saleOpensAt(event), Boolean(window.startsAt), locale, t),
      })
  return (
    <div
      className={`sale-card tone-lavender${event.url ? " link-card" : ""}`}
      onClick={event.url ? followCardLink : undefined}
    >
      <span className="sale-card-head">
        <span className={`tag${open ? " tone-leaf" : ""}`}>
          {t(open ? "schedule.status.open" : "sales.soon")}
        </span>
        <span className="sale-card-when">{line}</span>
      </span>
      <TranslatedTitle
        className="sale-card-title"
        title={cleanVideoTitle(event.title)}
        translations={event.translations}
        sourceLanguage={event.sourceLanguage}
        href={event.url}
        t={t}
      />
      {event.url ? (
        <ArrowUpRight className="sale-card-open" aria-hidden="true" />
      ) : null}
    </div>
  )
}

/**
 * The sales on sale now or opening within a week: what can be bought and
 * until when. The stage keeps to activities, so this is where a period
 * lasting weeks stays visible. On wide screens it fills the shorter stage
 * column; narrower, it follows the stage. A few show in place and every one
 * opens in a sheet. It renders nothing when no sale qualifies.
 */
export function SaleStrip({
  events,
  salesIds,
  placement,
  locale,
  now,
  t,
}: {
  events: ScheduleEvent[]
  salesIds?: string[]
  placement: SalePlacement
  locale: Locale
  now: number
  t: Translator
}) {
  const [sheetOpen, setSheetOpen] = useState(false)
  const columns = useStageColumns()
  const limit = SALE_LIMIT[columns ? placement : "main"]
  const sales = salesIds
    ? salesIds
        .map((id) => events.find((event) => event.id === id) || null)
        .filter((event): event is ScheduleEvent => event !== null)
    : currentSales(events, now)
  if (!sales.length) return null
  const closeSheet = () => setSheetOpen(false)
  return (
    <section
      className={`sale-strip is-${placement}`}
      aria-labelledby="sale-strip-title"
    >
      <header className="sale-strip-head">
        <h2 className="kicker" id="sale-strip-title">
          {t("sales.title")}
        </h2>
        {sales.length > limit ? (
          <button
            type="button"
            className="sale-strip-all"
            onClick={() => setSheetOpen(true)}
          >
            {t("sales.all", { count: sales.length })}
          </button>
        ) : null}
      </header>
      <ol className="sale-strip-list">
        {sales.slice(0, limit).map((event) => (
          <li key={event.id}>
            <SaleCard event={event} locale={locale} now={now} t={t} />
          </li>
        ))}
      </ol>
      {sheetOpen ? (
        <SheetDialog label={t("sales.title")} onClose={closeSheet}>
          <div className="sale-sheet">
            <header className="sale-sheet-head">
              <h2 className="kicker">{t("sales.title")}</h2>
              <button
                type="button"
                className="round-button sheet-dialog-close"
                onClick={closeSheet}
                aria-label={t("common.close")}
                title={t("common.close")}
              >
                <X aria-hidden="true" />
              </button>
            </header>
            <ol className="sale-sheet-list">
              {sales.map((event) => (
                <li key={event.id}>
                  <SaleCard event={event} locale={locale} now={now} t={t} />
                </li>
              ))}
            </ol>
          </div>
        </SheetDialog>
      ) : null}
    </section>
  )
}
