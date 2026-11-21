import type {
  LiveStatus,
  SaleWindow,
  ScheduleEvent,
  Translator,
  Video,
} from "../types.ts"

export const JAPAN_TIME_ZONE = "Asia/Tokyo"

/** Anything `new Date()` accepts; empty and invalid values format as blank. */
type DateInput = Date | string | number | null | undefined

const intlLocales: Record<string, string> = {
  "zh-CN": "zh-CN",
  ja: "ja-JP",
  en: "en-US",
}

export function intlLocale(locale: string) {
  return intlLocales[locale] || intlLocales.en
}

const formatterCache = new Map<string, Intl.DateTimeFormat>()

function formatter(locale: string, options: Intl.DateTimeFormatOptions) {
  const key = `${locale}|${JSON.stringify(options)}`
  let cached = formatterCache.get(key)
  if (!cached) {
    cached = new Intl.DateTimeFormat(intlLocale(locale), options)
    formatterCache.set(key, cached)
  }
  return cached
}

function parts(
  date: Date,
  locale: string,
  options: Intl.DateTimeFormatOptions,
) {
  const result: Partial<Record<Intl.DateTimeFormatPartTypes, string>> = {}
  for (const part of formatter(locale, options).formatToParts(date))
    result[part.type] = part.value
  return result
}

function toDate(value: DateInput) {
  if (!value) return null
  const date = value instanceof Date ? value : new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

const pad = (value: number) => String(value).padStart(2, "0")

// Calendar math uses local Date objects whose Y/M/D mirror the Japan date, so
// week navigation is independent of the visitor's own time zone.
export function dateKey(value: DateInput) {
  const date = toDate(value)
  if (!date) return ""
  const { year, month, day } = parts(date, "en", {
    timeZone: JAPAN_TIME_ZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  })
  return `${year}-${month}-${day}`
}

export function localDateKey(date: Date) {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

export function japanToday() {
  const [year, month, day] = dateKey(new Date()).split("-").map(Number)
  return new Date(year, month - 1, day)
}

export function addDays(date: Date, amount: number) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate() + amount)
}

export function startOfWeek(date: Date) {
  return addDays(date, -((date.getDay() + 6) % 7))
}

export function weekStartKey(value: DateInput) {
  const key = dateKey(value)
  if (!key) return ""
  const [year, month, day] = key.split("-").map(Number)
  return localDateKey(startOfWeek(new Date(year, month - 1, day)))
}

export function formatWeekRange(weekStart: Date, locale: string) {
  const weekEnd = addDays(weekStart, 6)
  const format = formatter(locale, { month: "short", day: "numeric" })
  return typeof format.formatRange === "function"
    ? format.formatRange(weekStart, weekEnd)
    : `${format.format(weekStart)} – ${format.format(weekEnd)}`
}

export function formatMonth(date: Date, locale: string) {
  return formatter(locale, { year: "numeric", month: "long" }).format(date)
}

/** A month with its year, short where the locale has a short name: "Jan 2026". */
export function formatShortMonth(date: Date, locale: string) {
  return formatter(locale, { year: "numeric", month: "short" }).format(date)
}

/** A month's short name alone: "Jan", "1月". */
export function formatMonthName(date: Date, locale: string) {
  return formatter(locale, { month: "short" }).format(date)
}

/** A weekday's short name: "Fri", "金", "周五". */
export function formatWeekday(date: Date, locale: string) {
  return formatter(locale, { weekday: "short" }).format(date)
}

/** A calendar date's day of the month with its short weekday. */
export function formatDayWeekday(date: Date, locale: string) {
  return formatter(locale, { day: "numeric", weekday: "short" }).format(date)
}

/** The first day of a calendar date's month, as a local Date. */
export function monthStartOf(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), 1)
}

export function formatLongDate(date: Date, locale: string) {
  return formatter(locale, {
    month: "long",
    day: "numeric",
    weekday: "short",
  }).format(date)
}

/** "08.29 21:00" in Japan time. */
export function formatStamp(value: DateInput) {
  const date = toDate(value)
  if (!date) return "—"
  const p = parts(date, "en", {
    timeZone: JAPAN_TIME_ZONE,
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  })
  return `${p.month}.${p.day} ${p.hour}:${p.minute}`
}

export function formatJapanTime(value: DateInput) {
  const date = toDate(value)
  if (!date) return ""
  const p = parts(date, "en", {
    timeZone: JAPAN_TIME_ZONE,
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  })
  return `${p.hour}:${p.minute}`
}

export function formatJapanDay(value: DateInput, locale: string) {
  const date = toDate(value)
  if (!date) return ""
  return formatter(locale, {
    timeZone: JAPAN_TIME_ZONE,
    month: "short",
    day: "numeric",
    weekday: "short",
  }).format(date)
}

/** The Japan month, day and weekday of an instant, for a date block. */
export function japanDateParts(value: DateInput, locale: string) {
  const date = toDate(value)
  if (!date) return null
  const pick = (options: Intl.DateTimeFormatOptions) =>
    formatter(locale, { timeZone: JAPAN_TIME_ZONE, ...options }).format(date)
  return {
    day: pick({ day: "numeric" }).replace(/\D/gu, ""),
    month: pick({ month: "short" }),
    weekday: pick({ weekday: "short" }),
  }
}

export function formatVisitorTime(value: DateInput, locale: string) {
  const date = toDate(value)
  if (!date) return ""
  return formatter(locale, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZoneName: "short",
  }).format(date)
}

export function visitorIsInJapan() {
  try {
    return (
      Intl.DateTimeFormat().resolvedOptions().timeZone === JAPAN_TIME_ZONE ||
      new Date().getTimezoneOffset() === -540
    )
  } catch {
    return false
  }
}

const relativeCache = new Map<string, Intl.RelativeTimeFormat>()

export function formatRelative(
  value: DateInput,
  locale: string,
  now = Date.now(),
) {
  const date = toDate(value)
  if (!date) return ""
  let format = relativeCache.get(locale)
  if (!format) {
    format = new Intl.RelativeTimeFormat(intlLocale(locale), {
      numeric: "auto",
    })
    relativeCache.set(locale, format)
  }
  const seconds = Math.round((date.getTime() - now) / 1000)
  const abs = Math.abs(seconds)
  if (abs < 60) return format.format(0, "second")
  if (abs < 3600) return format.format(Math.round(seconds / 60), "minute")
  if (abs < 86400) return format.format(Math.round(seconds / 3600), "hour")
  if (abs < 86400 * 30) return format.format(Math.round(seconds / 86400), "day")
  return format.format(Math.round(seconds / (86400 * 30)), "month")
}

export function countdownParts(value: DateInput, now = Date.now()) {
  const date = toDate(value)
  if (!date) return null
  const remaining = date.getTime() - now
  if (remaining <= 0) return { past: true, days: 0, hours: 0, minutes: 0 }
  const totalMinutes = Math.floor(remaining / 60000)
  return {
    past: false,
    days: Math.floor(totalMinutes / 1440),
    hours: Math.floor((totalMinutes % 1440) / 60),
    minutes: totalMinutes % 60,
  }
}

export function formatCompact(value: unknown, locale: string) {
  const number = Number(value)
  if (!Number.isFinite(number) || number <= 0) return ""
  return new Intl.NumberFormat(intlLocale(locale), {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(number)
}

// ---------------------------------------------------------------------------
// Events

export function eventDateKey(event: ScheduleEvent) {
  return event.startsOn || dateKey(event.startsAt)
}

/** The probe-confirmed broadcast state, or null when only the start is known. */
export function confirmedLiveStatus(
  item: { liveStatus?: LiveStatus | null } | null | undefined,
): LiveStatus | null {
  const status = item?.liveStatus
  return status === "live" || status === "upcoming" || status === "ended"
    ? status
    : null
}

/**
 * The server's state of an entry: upcoming, live, open, done, pending,
 * cancelled or cancellation_review.
 */
export function eventStatus(event: ScheduleEvent) {
  return event.view.state
}

/** One platform link of an entry, marked while that platform is live. */
export interface EventPlatformLink {
  source: string
  url: string
  live?: boolean
}

/**
 * An entry's platform links, as videos order theirs: a live platform
 * first, Bilibili first for zh-CN once the entry is over, its own platform
 * first otherwise. A platform without a link is left out.
 */
export function eventLinks(
  event: ScheduleEvent,
  locale?: string,
): EventPlatformLink[] {
  const links: EventPlatformLink[] = (event.links || []).flatMap((link) =>
    link.url
      ? [
          {
            source: link.platform,
            url: link.url,
            ...(link.liveStatus === "live" ? { live: true } : {}),
          },
        ]
      : [],
  )
  let lead = links.findIndex((link) => link.live)
  if (lead < 0 && locale === "zh-CN")
    lead =
      eventStatus(event) === "done"
        ? links.findIndex((link) => link.source === "bilibili")
        : -1
  if (lead <= 0) return links
  return [links[lead], ...links.slice(0, lead), ...links.slice(lead + 1)]
}

/**
 * The link an entry follows: its first platform link (a live platform's,
 * else its own platform's), or for a snapshot without links a stream's
 * Bilibili live room once known, else its own link (the source post or
 * operator's link).
 */
export function eventLink(event: ScheduleEvent) {
  const [first] = eventLinks(event)
  if (first) return first.url
  if (event.liveUrl && ["stream", "member"].includes(event.eventType))
    return event.liveUrl
  return event.url
}

/**
 * The entry's own link (its announcement, or an operator's link) when no
 * platform link is that page; it is never labelled as a platform.
 */
export function eventAnnouncement(
  event: ScheduleEvent,
  links: EventPlatformLink[],
): string | null {
  if (!event.url || links.some((link) => link.url === event.url)) return null
  return event.url
}

/**
 * Whether an entry shows its platform links: more than one, or one beside
 * an announcement of its own, which then keeps the open arrow.
 */
export function eventShowsPlatformLinks(
  event: ScheduleEvent,
  links: EventPlatformLink[],
) {
  return (
    links.length > 1 ||
    (links.length === 1 && Boolean(eventAnnouncement(event, links)))
  )
}

/** The platforms an entry is shown on, for its mark. */
export function eventPlatforms(event: ScheduleEvent): string[] {
  return (event.platforms || []).filter((platform) => platform !== "unknown")
}

export function eventIsManual(event: ScheduleEvent) {
  return event.provenance === "manual" || Boolean(event.manualLocked)
}

// ---------------------------------------------------------------------------
// Sale periods

export function eventIsSale(event: Pick<ScheduleEvent, "eventType">) {
  return String(event.eventType || "").toLowerCase() === "sale"
}

/** The stored period of a sale entry, also from one of its calendar entries. */
export function saleWindow(event: ScheduleEvent): SaleWindow {
  return (
    event.saleWindow || {
      startsOn: event.startsOn,
      startsAt: event.startsAt,
      endsOn: event.endsOn ?? null,
      endsAt: event.endsAt,
    }
  )
}

/**
 * A deadline whose opening is not known: the close date is the start date
 * and there is no start time.
 */
function saleIsDeadline(window: SaleWindow) {
  return window.endsOn === window.startsOn && !window.startsAt
}

/**
 * The calendar entries of an event, as the server lists them: a sale shows
 * at its opening and its close, and each of its entries keeps the whole
 * period.
 */
export function scheduleEntries(event: ScheduleEvent): ScheduleEvent[] {
  const window = eventIsSale(event) ? saleWindow(event) : null
  return event.view.calendarEntries.map((entry) => ({
    ...event,
    ...(window ? { saleWindow: window } : {}),
    startsOn: entry.startsOn,
    startsAt: entry.startsAt,
    timePrecision: entry.timePrecision,
    ...(entry.saleMark ? { saleMark: entry.saleMark } : {}),
  }))
}

const japanDayStart = (day: string) => Date.parse(`${day}T00:00:00+09:00`)
const japanDayEnd = (day: string) => Date.parse(`${day}T23:59:59.999+09:00`)

/** The instant a sale opens; -Infinity for a deadline whose opening is unknown. */
export function saleOpensAt(event: ScheduleEvent) {
  const window = saleWindow(event)
  return saleIsDeadline(window)
    ? -Infinity
    : Date.parse(window.startsAt || "") || japanDayStart(window.startsOn)
}

/** The instant a sale closes: its close time, else the end of its close day. */
export function saleClosesAt(event: ScheduleEvent) {
  const window = saleWindow(event)
  return (
    Date.parse(window.endsAt || "") ||
    japanDayEnd(window.endsOn || window.startsOn)
  )
}

export const eventTags = ["live", "game", "talk", "song", "video"] as const
export type EventTag = (typeof eventTags)[number]

/**
 * The kind chips of an entry, as the server codes them: the type chip's
 * message key, the activity label, and whether the entry is a work, where a
 * song is a cover or original rather than a singing stream.
 */
export function eventKinds(event: ScheduleEvent) {
  return {
    typeKey: event.view.typeCode ? `event.type.${event.view.typeCode}` : null,
    tag: event.view.activityCode as EventTag | null,
    work: event.view.isWork,
  }
}

/** A label's message key: a work's song reads as music, any other as singing. */
export function eventTagKey(tag: EventTag, work: boolean) {
  return work && tag === "song" ? "media.kind.music" : `event.tag.${tag}`
}

export function sortEvents(events: ScheduleEvent[]) {
  return [...events].sort(
    (a, b) =>
      scheduleSortTime(a.startsAt, a.startsOn) -
        scheduleSortTime(b.startsAt, b.startsOn) ||
      String(a.id || "").localeCompare(String(b.id || "")),
  )
}

// Date-only entries follow timed entries on the same Japan date. This is a
// sorting boundary only; it does not supply an event start or countdown.
export function scheduleSortTime(
  time: string | null | undefined,
  dateOnly: string | null = null,
) {
  const instant = Date.parse(time || "")
  if (Number.isFinite(instant)) return instant
  const dayEnd = Date.parse(`${dateOnly}T23:59:59.999+09:00`)
  return Number.isFinite(dayEnd) ? dayEnd : Infinity
}

// ---------------------------------------------------------------------------
// Videos

export const videoTags = ["game", "talk", "song"] as const
export type VideoTag = (typeof videoTags)[number]

// A work's tag names what it is (a song); a stream's names what it was (a
// singing stream), in the schedule's words.
const workTagKeys: Record<VideoTag, string> = {
  game: "media.kind.gameplay",
  talk: "media.kind.talk",
  song: "media.kind.music",
}

/** The video's content tag (game, talk or song), or null for other. */
export function videoTag(video: Video): VideoTag | null {
  const tag = String(video?.tag || "")
  return (videoTags as readonly string[]).includes(tag)
    ? (tag as VideoTag)
    : null
}

const formKeys: Record<NonNullable<Video["view"]["formCode"]>, string> = {
  short: "media.kind.short",
  upcoming: "media.kind.upcoming",
  stream: "event.type.stream",
  archive: "media.kind.liveArchive",
  premiere: "media.kind.premiere",
  video: "media.kind.video",
}

/**
 * A video's form chip from the server's form code, or null for a video
 * without one, such as an aired premiere. What the video is about is its tag
 * chip.
 */
export function videoFormLabel(video: Video, t: Translator): string | null {
  const code = video.view.formCode
  return code ? t(formKeys[code]) : null
}

/** A video's tag chip label, or null when it has no tag. */
export function videoTagLabel(video: Video, t: Translator) {
  const tag = videoTag(video)
  if (!tag) return null
  return t(
    video.view.tagContext === "stream" ? `event.tag.${tag}` : workTagKeys[tag],
  )
}

/** Strip the trailing 【鹿乃まほろ/ミリプロ】 credit that every title repeats. */
export function cleanVideoTitle(title: string | null | undefined) {
  return String(title || "")
    .replace(/\s*【\s*鹿乃まほろ\s*[/／]\s*ミリプロ\s*】\s*/gu, " ")
    .trim()
}

export function videoTime(video: Video) {
  return video.scheduledAt || video.publishedAt || null
}

/** A video length as the platforms show it: "4:49" or "2:01:05". */
export function formatDuration(seconds: number | null | undefined) {
  const total = Math.floor(Number(seconds))
  if (!Number.isFinite(total) || total <= 0) return ""
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const rest = pad(total % 60)
  return hours ? `${hours}:${pad(minutes)}:${rest}` : `${minutes}:${rest}`
}

/** The same length as an HTML duration string, such as "PT2H1M5S". */
export function durationAttribute(seconds: number) {
  const total = Math.floor(seconds)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  return `PT${hours ? `${hours}H` : ""}${minutes ? `${minutes}M` : ""}${total % 60}S`
}
