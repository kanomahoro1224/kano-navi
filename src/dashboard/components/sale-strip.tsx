import type { Locale, ScheduleEvent, Translator } from "@/types"
import { ArrowUpRight } from "lucide-react"

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
import { TranslatedTitle } from "./translated-title"

/** A Japan day, with its time when the sale states one. */
function when(instant: number, timed: boolean, locale: Locale, t: Translator) {
  const day = formatJapanDay(instant, locale)
  return timed
    ? `${day} ${formatJapanTime(instant)} ${t("common.timezoneShort")}`
    : day
}

/**
 * The sales on sale now or opening within a week, between the stage and the
 * schedule: what can be bought and until when. The stage keeps to
 * activities, so this is where a period lasting weeks stays visible. It
 * renders nothing when no sale qualifies.
 */
export function SaleStrip({
  events,
  salesIds,
  locale,
  now,
  t,
}: {
  events: ScheduleEvent[]
  salesIds?: string[]
  locale: Locale
  now: number
  t: Translator
}) {
  const sales = salesIds
    ? salesIds
        .map((id) => events.find((event) => event.id === id) || null)
        .filter((event): event is ScheduleEvent => event !== null)
    : currentSales(events, now)
  if (!sales.length) return null
  return (
    <section className="sale-strip" aria-labelledby="sale-strip-title">
      <h2 className="kicker" id="sale-strip-title">
        {t("sales.title")}
      </h2>
      <ol className="sale-strip-list">
        {sales.map((event) => {
          const open = saleStatus(event, now) === "open"
          const window = saleWindow(event)
          const line = open
            ? t("sales.closes", {
                date: when(
                  saleClosesAt(event),
                  Boolean(window.endsAt),
                  locale,
                  t,
                ),
              })
            : t("sales.opens", {
                date: when(
                  saleOpensAt(event),
                  Boolean(window.startsAt),
                  locale,
                  t,
                ),
              })
          return (
            <li key={event.id}>
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
            </li>
          )
        })}
      </ol>
    </section>
  )
}
