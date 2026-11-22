import type { ArchiveMonth, Locale, Translator } from "@/types"
import { useMemo } from "react"

import { formatMonth, formatMonthName } from "@/dashboard/format"
import { activityLevel, activityScale, activityYears } from "./timeline"

/**
 * Every year of the archive as a row of twelve months, shaded by how many
 * posts the chosen account made in each, and the year's total. A month
 * holding posts opens the timeline there; the month in view is outlined.
 */
export function ActivityMap({
  months,
  filter,
  shown,
  onChoose,
  locale,
  t,
}: {
  months: readonly ArchiveMonth[]
  /** The chosen account's lowercase handle, or null for every account. */
  filter: string | null
  /** The `YYYY-MM` month in view. */
  shown: string | null
  onChoose: (month: string) => void
  locale: Locale
  t: Translator
}) {
  const years = useMemo(() => activityYears(months, filter), [months, filter])
  const scale = useMemo(
    () => activityScale(years.flatMap((row) => row.months.map((n) => n || 0))),
    [years],
  )
  const number = useMemo(() => new Intl.NumberFormat(locale), [locale])
  const shownYear = shown ? Number(shown.slice(0, 4)) : null
  return (
    <nav className="posts-map" aria-label={t("posts.months")}>
      <div className="posts-map-names" aria-hidden="true">
        <span />
        {[0, 5, 11].map((index) => (
          <span
            key={index}
            className={index === 11 ? "is-last" : undefined}
            style={{ gridColumn: index + 2 }}
          >
            {formatMonthName(new Date(2000, index, 1), locale)}
          </span>
        ))}
        <span className="posts-map-total">Σ</span>
      </div>
      <ol className="posts-map-years">
        {years.map(({ year, months: counts, total }) => (
          <li
            key={year}
            className={`posts-map-year${year === shownYear ? " is-shown" : ""}${total ? "" : " is-empty"}`}
          >
            <span className="posts-map-label">{year}</span>
            {counts.map((count, index) => {
              const key = `${year}-${String(index + 1).padStart(2, "0")}`
              if (count === null)
                return <span key={key} className="posts-map-cell is-future" />
              const label = `${formatMonth(new Date(year, index, 1), locale)} · ${t("feed.monthCount", { count })}`
              const level = activityLevel(count, scale)
              const current = key === shown
              return count ? (
                <button
                  key={key}
                  type="button"
                  className={`posts-map-cell level-${level}`}
                  aria-label={label}
                  aria-current={current ? "date" : undefined}
                  title={label}
                  onClick={() => onChoose(key)}
                />
              ) : (
                <span
                  key={key}
                  className={`posts-map-cell level-0${current ? " is-current" : ""}`}
                  title={label}
                />
              )
            })}
            <span className="posts-map-total">{number.format(total)}</span>
          </li>
        ))}
      </ol>
    </nav>
  )
}
