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

export function eventIsUpcoming(event: ScheduleEvent, now = Date.now()) {
  const timestamp = Date.parse(event.startsAt || "")
  if (!Number.isNaN(timestamp)) return timestamp >= now
  const day = eventDateKey(event)
  if (day) return day >= dateKey(new Date(now))
  return Boolean(event.isUpcoming)
}

export const LIVE_GRACE_MS = 3 * 3600 * 1000

/** The probe-confirmed broadcast state, or null when only the start is known. */
export function confirmedLiveStatus(
  item: { liveStatus?: LiveStatus | null } | null | undefined,
): LiveStatus | null {
  const status = item?.liveStatus
  return status === "live" || status === "upcoming" || status === "ended"
    ? status
    : null
}

// A confirmed state decides first. Without one, only streams and memberships
// are presumed live without an end; other YouTube activities, such as a
// premiere, stay live only until their explicit end.
export function eventIsLive(event: ScheduleEvent, now: number) {
  const isStream = ["stream", "member"].includes(event.eventType)
  if (!isStream && event.source !== "youtube") return false
  const start = Date.parse(event.startsAt || "")
  const end = Date.parse(event.endsAt || "")
  const confirmed = confirmedLiveStatus(event)
  if (confirmed) return confirmed === "live" && !(end <= now)
  if (!(start <= now)) return false
  if (Number.isFinite(end)) return end > now
  return isStream && start > now - LIVE_GRACE_MS
}

const cancelledStatuses = [
  "cancelled",
  "canceled",
  "cancel",
  "取消",
  "已取消",
  "中止",
  "キャンセル",
]
const pendingStatuses = [
  "pending",
  "tentative",
  "unknown",
  "待确认",
  "待补充",
  "確認待ち",
  "未定",
]

/** upcoming | live | open | done | pending | cancelled | cancellation_review */
export function eventStatus(event: ScheduleEvent, now = Date.now()) {
  if (event.view?.state) return event.view.state
  if (event.cancellationStatus === "manual_confirmed") return "cancelled"
  if (event.cancellationStatus === "llm_suspected") return "cancellation_review"
  const status = String(event.status || "")
    .trim()
    .toLowerCase()
  if (cancelledStatuses.includes(status) || status.startsWith("cancel"))
    return "cancelled"
  // A sale period follows its window, never a broadcast state.
  if (eventIsSale(event)) return saleStatus(event, now)
  // The stage's On air rule, so the schedule never calls it ended meanwhile.
  if (eventIsLive(event, now)) return "live"
  // A waiting room past its reserved time has not started, as the stage shows.
  if (confirmedLiveStatus(event) === "upcoming") return "upcoming"
  if (eventIsUpcoming(event, now)) return "upcoming"
  if (event.timePrecision === "unknown" || pendingStatuses.includes(status))
    return "pending"
  return "done"
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
  now?: number,
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
  if (lead < 0 && locale === "zh-CN" && now !== undefined)
    lead =
      eventStatus(event, now) === "done"
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

export function eventTypeKey(event: ScheduleEvent) {
  const type = String(event.eventType || "").toLowerCase()
  return ["stream", "member", "event", "release", "video", "sale"].includes(
    type,
  )
    ? `event.type.${type}`
    : null
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
 * The calendar entries of an event. A sale period shows at its ends rather
 * than on every day it runs: a period that closes on a later date gives an
 * opening and a closing entry, a deadline its closing entry alone, and any
 * other sale its opening. The closing entry carries the close as its date and
 * time. Every other event is its own single entry.
 */
export function scheduleEntries(event: ScheduleEvent): ScheduleEvent[] {
  if (event.view?.calendarEntries?.length) {
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
  if (!eventIsSale(event)) return [event]
  const window = saleWindow(event)
  const closing: ScheduleEvent = {
    ...event,
    startsOn: window.endsOn || window.startsOn,
    startsAt: window.endsAt,
    timePrecision: window.endsAt ? "exact" : "unknown",
    saleMark: "closes",
    saleWindow: window,
  }
  if (saleIsDeadline(window)) return [closing]
  const opening: ScheduleEvent = {
    ...event,
    saleMark: "opens",
    saleWindow: window,
  }
  return window.endsOn && window.endsOn > window.startsOn
    ? [opening, closing]
    : [opening]
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

/** How far ahead the on-sale list looks for a sale that has not opened. */
export const SALE_LOOKAHEAD_MS = 7 * 86_400_000

/**
 * The sales the board lists as on sale now or opening within a week:
 * cancelled ones and those under cancellation review stay out, open ones
 * come first by their close, then the others by their opening.
 */
export function currentSales(
  events: ScheduleEvent[],
  now: number,
  limit = Infinity,
) {
  const listed = events.filter((event) => {
    if (!eventIsSale(event)) return false
    const status = eventStatus(event, now)
    return (
      status === "open" ||
      (status === "upcoming" && saleOpensAt(event) <= now + SALE_LOOKAHEAD_MS)
    )
  })
  const open = (event: ScheduleEvent) => saleStatus(event, now) === "open"
  return listed
    .sort(
      (a, b) =>
        Number(open(b)) - Number(open(a)) ||
        (open(a)
          ? saleClosesAt(a) - saleClosesAt(b)
          : saleOpensAt(a) - saleOpensAt(b)) ||
        a.id.localeCompare(b.id),
    )
    .slice(0, limit)
}

/**
 * upcoming before a sale opens, open until it closes, done after. A deadline
 * counts as open until it passes, and a period without a close as open for
 * its opening day.
 */
export function saleStatus(event: ScheduleEvent, now: number) {
  if (now < saleOpensAt(event)) return "upcoming"
  return now < saleClosesAt(event) ? "open" : "done"
}

export const eventTags = ["live", "game", "talk", "song", "video"] as const
export type EventTag = (typeof eventTags)[number]

/** The entry's activity label, or null when it is unlabelled or unknown. */
export function eventTag(event: ScheduleEvent): EventTag | null {
  const tag = String(event.tag || "").toLowerCase()
  return (eventTags as readonly string[]).includes(tag)
    ? (tag as EventTag)
    : null
}

/**
 * The kind chips of an entry. A label says what the activity is and the type
 * how it is held: a label replaces the plain stream chip, a members-only,
 * event or release chip stays beside it, and a label that repeats its type
 * is dropped. `work` marks a release (a premiere) or an upload, where a song
 * is a cover or original rather than a singing stream.
 */
export function eventKinds(event: ScheduleEvent) {
  if (event.view) {
    return {
      typeKey: event.view.typeCode ? `event.type.${event.view.typeCode}` : null,
      tag: event.view.activityCode as EventTag | null,
      work: event.view.isWork,
    }
  }
  const type = String(event.eventType || "").toLowerCase()
  let tag = eventTag(event)
  if (
    (tag === "live" && (type === "stream" || type === "member")) ||
    (tag === "video" && type === "video")
  )
    tag = null
  const typeKey = tag && type === "stream" ? null : eventTypeKey(event)
  return { typeKey, tag, work: type === "release" || type === "video" }
}

/**
 * Whether a members-only entry adds its members chip beside the label that
 * took the type's place (`displaced`): an activity label or a video's form.
 * A video whose availability chip already tells who can watch it needs none.
 */
export function membersChipBeside(
  event: ScheduleEvent | null | undefined,
  video: Video | null | undefined,
  displaced: boolean,
) {
  const availability = video?.availability
  return (
    String(event?.eventType || "").toLowerCase() === "member" &&
    displaced &&
    availability !== "members" &&
    availability !== "private" &&
    availability !== "removed"
  )
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

// A stream past its waiting room may still be on air: a reported end that has
// passed closes it, as in videoPhase; otherwise a confirmed broadcast
// decides, then its start within the live grace and before its end.
function streamMayBeOnAir(video: Video, now: number) {
  const start = Date.parse(video.scheduledAt || "")
  const end = Date.parse(video.endsAt || "")
  if (end <= now) return false
  const confirmed = confirmedLiveStatus(video)
  if (confirmed) return confirmed === "live"
  if (!(start <= now)) return false
  if (Number.isFinite(end)) return end > now
  return start > now - LIVE_GRACE_MS
}

// A premiere before or during its showing, by videoPhase's rule: a fresh
// confirmed state decides, else a start still ahead or an end not yet passed.
function premiereIsCurrent(video: Video, now: number) {
  const start = Date.parse(video.scheduledAt || "")
  const end = Date.parse(video.endsAt || "")
  const confirmed = confirmedLiveStatus(video)
  if (confirmed === "live") return !(end <= now)
  if (confirmed) return confirmed === "upcoming"
  if (!Number.isFinite(start)) return video.isUpcoming === true
  return start > now || end > now
}

/**
 * A video's form chip: a Short, then a stream's waiting room, then its form.
 * A stream that may be on air reads as a stream and one past that as its
 * archive. A premiere is named only while it is waiting or airing, by the
 * same rule as `videoPhase`; once it has aired it is an ordinary video with
 * no form chip (null). What the video is about is its tag chip.
 */
export function videoFormLabel(
  video: Video,
  t: Translator,
  now = Date.now(),
): string | null {
  if (video.view) {
    if (!video.view.formCode) return null
    return t(
      video.view.formCode === "short"
        ? "media.kind.short"
        : video.view.formCode === "upcoming"
          ? "media.kind.upcoming"
          : video.view.formCode === "stream"
            ? "event.type.stream"
            : video.view.formCode === "archive"
              ? "media.kind.liveArchive"
              : video.view.formCode === "premiere"
                ? "media.kind.premiere"
                : "media.kind.video",
    )
  }
  if (video?.isShort === true) return t("media.kind.short")
  const kind = String(video?.kind || "")
  if (kind === "stream") {
    if (video.isUpcoming) return t("media.kind.upcoming")
    return t(
      streamMayBeOnAir(video, now)
        ? "event.type.stream"
        : "media.kind.liveArchive",
    )
  }
  if (kind === "premiere")
    return premiereIsCurrent(video, now) ? t("media.kind.premiere") : null
  return t("media.kind.video")
}

/** A video's tag chip label, or null when it has no tag. */
export function videoTagLabel(video: Video, t: Translator) {
  const tag = videoTag(video)
  if (!tag) return null
  const context =
    video.view?.tagContext || (video.kind === "stream" ? "stream" : "work")
  return t(context === "stream" ? `event.tag.${tag}` : workTagKeys[tag])
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
