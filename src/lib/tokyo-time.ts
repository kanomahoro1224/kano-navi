/**
 * Japan time for the whole frontend: every time the site shows is in
 * Asia/Tokyo, whatever the visitor's zone. Pages build their date formats
 * from these cached formatters instead of their own `Intl.DateTimeFormat`.
 */
export const TOKYO_TIME_ZONE = "Asia/Tokyo"

/** Anything `new Date()` accepts; empty and invalid values have no date. */
export type DateInput = Date | string | number | null | undefined

const intlLocales: Record<string, string> = {
  "zh-CN": "zh-CN",
  ja: "ja-JP",
  en: "en-US",
}

/** The Intl locale of a site locale; any other value formats as English. */
export function intlLocale(locale: string) {
  return intlLocales[locale] || intlLocales.en
}

const formatters = new Map<string, Intl.DateTimeFormat>()

/**
 * A cached formatter for a site locale. It formats in the browser's zone
 * unless the options name one; the board's calendar math formats local dates
 * that mirror the Japan date this way.
 */
export function dateFormatter(
  locale: string,
  options: Intl.DateTimeFormatOptions,
) {
  const key = `${locale}|${JSON.stringify(options)}`
  let cached = formatters.get(key)
  if (!cached) {
    cached = new Intl.DateTimeFormat(intlLocale(locale), options)
    formatters.set(key, cached)
  }
  return cached
}

/** A cached formatter for a site locale in Japan time. */
export function tokyoFormatter(
  locale: string,
  options: Intl.DateTimeFormatOptions,
) {
  return dateFormatter(locale, { timeZone: TOKYO_TIME_ZONE, ...options })
}

export function toDate(value: DateInput) {
  if (!value) return null
  const date = value instanceof Date ? value : new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

export type TokyoParts = Partial<Record<Intl.DateTimeFormatPartTypes, string>>

/** The Japan-time fields of an instant, in English digits; null without one. */
export function tokyoParts(
  value: DateInput,
  options: Intl.DateTimeFormatOptions,
): TokyoParts | null {
  const date = toDate(value)
  if (!date) return null
  const result: TokyoParts = {}
  for (const part of tokyoFormatter("en", options).formatToParts(date))
    result[part.type] = part.value
  return result
}

/** An instant's Japan date as "YYYY-MM-DD", or "" when it has none. */
export function tokyoDay(value: DateInput) {
  const p = tokyoParts(value, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  })
  return p ? `${p.year}-${p.month}-${p.day}` : ""
}

/** An instant's Japan wall-clock time as "HH:mm", or "" when it has none. */
export function tokyoClock(value: DateInput) {
  const p = tokyoParts(value, {
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  })
  return p ? `${p.hour}:${p.minute}` : ""
}
