import type { ComponentType } from "react"
import type { Dashboard, Locale, Profile, Translator } from "@/types"
import {
  ArrowUpRight,
  BadgeCheck,
  Play,
  Radio,
  RefreshCw,
  ShoppingBag,
  Sparkles,
} from "lucide-react"

import {
  BilibiliLogo,
  ExternalLink,
  followCardLink,
  XLogo,
  YouTubeLogo,
} from "@/site/links"
import { eventDetailText } from "@/lib/event-text"
import { mediaSrcSet } from "@/lib/media"
import {
  confirmedLiveStatus,
  countdownParts,
  eventIsManual,
  eventKinds,
  membersChipBeside,
  eventTag,
  eventTagKey,
  eventTypeKey,
  formatJapanDay,
  formatJapanTime,
  formatRelative,
  formatStamp,
  formatVisitorTime,
  japanDateParts,
  videoFormLabel,
  videoTag,
  videoTagLabel,
  visitorIsInJapan,
  eventLinks,
  eventPlatforms,
  eventShowsPlatformLinks,
  type VideoTag,
} from "../format"
import { pickCompanion, pickSpotlight, spotlightIsLive } from "../spotlight"
import type { SyncSource, SyncState } from "../freshness"
import {
  videoIsGone,
  videoLinks,
  videoPlatforms,
  type VideoLink,
} from "../video-groups"
import {
  EventTagIcon,
  MembersChip,
  VideoAvailabilityChip,
  VideoTagChip,
} from "./event-tag"
import { PlatformBadge, PlatformLinks } from "./platform-links"
import { TranslatedTitle } from "./translated-title"

/** Each source's mark and brand name; the shop has neither, so its name is
 * translated (sync.source.shop). */
const SOURCES: Record<
  SyncSource,
  {
    name: string | null
    Icon: ComponentType<{ "aria-hidden"?: "true" }>
  }
> = {
  x: { name: "X", Icon: XLogo },
  youtube: { name: "YouTube", Icon: YouTubeLogo },
  bilibili: { name: "bilibili", Icon: BilibiliLogo },
  shop: { name: null, Icon: ShoppingBag },
}

/**
 * A ticket's platform links: a video's, with the other platforms of an
 * entry of the same broadcast, or an entry's. A live platform leads.
 */
function ticketLinks(
  item: SpotlightItem,
  locale: Locale,
  now: number,
): VideoLink[] {
  if (!item.video) {
    if (!item.event) return []
    const links = eventLinks(item.event, locale, now)
    return eventShowsPlatformLinks(item.event, links) ? links : []
  }
  // The video's own link is marked while the probe confirms it live.
  const live = confirmedLiveStatus(item.video) === "live"
  const links: VideoLink[] = [
    ...videoLinks(item.video, locale, now).map((link) =>
      live && link.url === item.video?.url ? { ...link, live } : link,
    ),
    ...(item.extraLinks || []),
  ]
  const lead = links.findIndex((link) => link.live)
  if (lead <= 0) return links
  return [links[lead], ...links.slice(0, lead), ...links.slice(lead + 1)]
}

function dateOnlyParts(key: string, locale: Locale) {
  const [year, month, day] = key.split("-").map(Number)
  const date = new Date(Date.UTC(year, month - 1, day, 3))
  return japanDateParts(date.toISOString(), locale)
}

/**
 * The countdown under the stub, or with inline set the copy beside the time
 * that phones show instead; the stylesheet shows one of the two.
 */
function Countdown({
  time,
  now,
  t,
  inline = false,
}: {
  time: string
  now: number
  t: Translator
  inline?: boolean
}) {
  const parts = countdownParts(time, now)
  if (!parts || parts.past) return null
  const cells = parts.days
    ? [
        [parts.days, t("countdown.unitDays")],
        [parts.hours, t("countdown.unitHours")],
      ]
    : [
        [parts.hours, t("countdown.unitHours")],
        [parts.minutes, t("countdown.unitMinutes")],
      ]
  return (
    <span className={`ticket-countdown${inline ? " is-inline" : ""}`}>
      <span className="ticket-countdown-label">{t("spotlight.startsIn")}</span>
      <span className="ticket-countdown-cells">
        {cells.map(([value, unit]) => (
          <span key={unit} className="ticket-countdown-cell">
            <strong>{String(value).padStart(2, "0")}</strong>
            <small>{unit}</small>
          </span>
        ))}
      </span>
    </span>
  )
}

type SpotlightItem = NonNullable<ReturnType<typeof pickSpotlight>>

const platformNames: Record<string, string> = { bilibili: "bilibili" }
type Companion = NonNullable<ReturnType<typeof pickCompanion>>

function TicketHead({
  item,
  isLive,
  now,
  t,
}: {
  item: SpotlightItem
  isLive: boolean
  now: number
  t: Translator
}) {
  const typeKey = item.event ? eventTypeKey(item.event) : null
  // The video's tag (which already follows its entry's label) or else the
  // entry's gameplay, talk or singing label outranks the form; a plain
  // stream or upload label says no more than the type already does.
  const label = item.event ? eventTag(item.event) : null
  const entryWork = item.event ? eventKinds(item.event).work : false
  const videoActivity = item.video ? videoTag(item.video) : null
  const activity: VideoTag | null =
    videoActivity ??
    (label === "game" || label === "talk" || label === "song" ? label : null)
  const tag = item.liveNow
    ? t("spotlight.liveNow", { platform: platformNames[item.liveNow.platform] })
    : item.video && videoActivity
      ? videoTagLabel(item.video, t)
      : activity
        ? t(eventTagKey(activity, entryWork))
        : // A reservation on air on another platform is a stream on air.
          (item.video &&
            (isLive && item.video.isUpcoming
              ? t("event.type.stream")
              : videoFormLabel(item.video, t, now))) ||
          (typeKey
            ? t(typeKey)
            : eventDetailText(item.detail, t) || t("schedule.publicEvent"))
  // A members-only entry keeps saying so beside its activity or video form,
  // as the schedule does.
  const membersChip = membersChipBeside(
    item.event,
    item.video,
    Boolean(activity || item.video),
  )
  const manual = item.event ? eventIsManual(item.event) : false
  return (
    <span className="ticket-head">
      {isLive ? (
        <span className="ticket-onair">
          <Radio aria-hidden="true" />
          {t("spotlight.onAir")}
        </span>
      ) : (
        <span className="kicker">
          {t(item.mode === "next" ? "spotlight.next" : "spotlight.latest")}
        </span>
      )}
      <span className="tag tone-berry">
        {item.liveNow ? <BilibiliLogo aria-hidden="true" /> : null}
        {activity ? (
          <EventTagIcon
            tag={activity}
            work={
              videoActivity && item.video
                ? item.video.kind !== "stream"
                : entryWork
            }
          />
        ) : null}
        {tag}
      </span>
      {membersChip ? <MembersChip t={t} /> : null}
      {item.video ? <VideoAvailabilityChip video={item.video} t={t} /> : null}
      {item.event ? (
        <span className={`ticket-provenance${manual ? " is-manual" : ""}`}>
          {manual ? (
            <BadgeCheck aria-hidden="true" />
          ) : (
            <Sparkles aria-hidden="true" />
          )}
          {t(manual ? "schedule.sourceManual" : "schedule.sourceAutomatic")}
        </span>
      ) : null}
    </span>
  )
}

function TicketWhen({
  item,
  locale,
  now,
  t,
}: {
  item: SpotlightItem
  locale: Locale
  now: number
  t: Translator
}) {
  if (item.mode !== "next") {
    return (
      <span className="ticket-when">
        <small>{formatRelative(item.time, locale, now)}</small>
      </span>
    )
  }
  if (!item.time) {
    return (
      <span className="ticket-when">
        <strong>
          {item.dateOnly} · {t("common.pendingConfirmation")}
        </strong>
      </span>
    )
  }
  return (
    <span className="ticket-when">
      <strong>
        <span className="sr-only">{formatJapanDay(item.time, locale)} </span>
        {formatJapanTime(item.time)} {t("common.timezoneShort")}
      </strong>
      {!visitorIsInJapan() ? (
        <small>
          {t("spotlight.localTime", {
            time: formatVisitorTime(item.time, locale),
          })}
        </small>
      ) : null}
    </span>
  )
}

function DateBlock({ item, locale }: { item: SpotlightItem; locale: Locale }) {
  const parts = item.time
    ? japanDateParts(item.time, locale)
    : item.dateOnly
      ? dateOnlyParts(item.dateOnly, locale)
      : null
  if (!parts) return null
  return (
    <span className="ticket-date" aria-hidden="true">
      <small>{parts.month}</small>
      <strong>{parts.day}</strong>
      <small>{parts.weekday}</small>
    </span>
  )
}

/** A stream or video with artwork: the picture leads, the stub follows. */
function FullTicket({
  item,
  isLive,
  locale,
  now,
  t,
}: {
  item: SpotlightItem
  isLive: boolean
  locale: Locale
  now: number
  t: Translator
}) {
  const links = ticketLinks(item, locale, now)
  // A private or removed video has no page of its own left to open.
  const url =
    links[0]?.url || (item.video && videoIsGone(item.video) ? null : item.url)
  return (
    <div
      className={`ticket is-${isLive ? "live" : item.mode}${item.thumbnailUrl ? " has-art" : ""}${url ? " link-card" : ""}`}
      onClick={url ? followCardLink : undefined}
    >
      <span
        className={`ticket-art${item.video && !url ? " is-closed" : ""}`}
        aria-hidden="true"
      >
        {item.thumbnailUrl ? (
          <img
            src={item.thumbnailUrl}
            srcSet={mediaSrcSet(item.thumbnailUrl)}
            sizes="(max-width: 960px) 100vw, 60vw"
            alt=""
          />
        ) : (
          <span className="ticket-art-fallback" />
        )}
        <PlatformBadge
          platforms={
            item.video
              ? videoPlatforms(item.video, links)
              : item.event
                ? eventPlatforms(item.event)
                : []
          }
        />
        {item.link === "watch" ? (
          <span className="ticket-play">
            <Play fill="currentColor" />
          </span>
        ) : null}
      </span>
      <span className="ticket-stub">
        <TicketHead item={item} isLive={isLive} now={now} t={t} />
        <span className="ticket-body">
          {item.mode === "next" ? (
            <DateBlock item={item} locale={locale} />
          ) : null}
          <span className="ticket-copy">
            <TranslatedTitle
              className="ticket-title"
              title={item.title}
              translations={(item.video || item.event)?.translations}
              sourceLanguage={(item.video || item.event)?.sourceLanguage}
              href={url}
              t={t}
            />
            <span className="ticket-when-row">
              <TicketWhen item={item} locale={locale} now={now} t={t} />
              {item.mode === "next" && item.time && !isLive ? (
                <Countdown time={item.time} now={now} t={t} inline />
              ) : null}
            </span>
            {links.length > (item.video ? 1 : 0) ? (
              <PlatformLinks
                links={links}
                title={item.title}
                variant="chip"
                t={t}
              />
            ) : null}
          </span>
        </span>
        {item.mode === "next" && item.time && !isLive ? (
          <Countdown time={item.time} now={now} t={t} />
        ) : null}
        {isLive &&
        !item.liveNow &&
        !confirmedLiveStatus(item.event || item.video) ? (
          <span className="ticket-note">{t("spotlight.started")}</span>
        ) : null}
        {url ? (
          <span className="ticket-open">
            <span className="ticket-open-label">{t("spotlight.open")}</span>
            <ArrowUpRight aria-hidden="true" />
          </span>
        ) : null}
      </span>
    </div>
  )
}

/** A schedule entry without artwork: the date is the picture. */
function CompactTicket({
  item,
  isLive,
  locale,
  now,
  t,
}: {
  item: SpotlightItem
  isLive: boolean
  locale: Locale
  now: number
  t: Translator
}) {
  const links = ticketLinks(item, locale, now)
  // A video's ticket follows the platform that is live, as its links lead.
  const url = (item.video && links[0]?.url) || item.url
  return (
    <div
      className={`ticket is-compact is-${isLive ? "live" : item.mode}${url ? " link-card" : ""}`}
      onClick={url ? followCardLink : undefined}
    >
      <span className="ticket-side">
        <DateBlock item={item} locale={locale} />
      </span>
      <span className="ticket-stub">
        <TicketHead item={item} isLive={isLive} now={now} t={t} />
        <span className="ticket-copy">
          <TranslatedTitle
            className="ticket-title"
            title={item.title}
            translations={(item.video || item.event)?.translations}
            sourceLanguage={(item.video || item.event)?.sourceLanguage}
            href={url}
            t={t}
          />
          <span className="ticket-when-row">
            <TicketWhen item={item} locale={locale} now={now} t={t} />
            {item.time && !isLive ? (
              <Countdown time={item.time} now={now} t={t} inline />
            ) : null}
          </span>
          {links.length > (item.video ? 1 : 0) ? (
            <PlatformLinks
              links={links}
              title={item.title}
              variant="chip"
              t={t}
            />
          ) : null}
        </span>
        {item.time && !isLive ? (
          <Countdown time={item.time} now={now} t={t} />
        ) : null}
        {isLive &&
        !item.liveNow &&
        !confirmedLiveStatus(item.event || item.video) ? (
          <span className="ticket-note">{t("spotlight.started")}</span>
        ) : null}
        {url ? (
          <span className="ticket-link">
            {item.link === "announcement" ? (
              <XLogo aria-hidden="true" />
            ) : item.link === "watch" ? (
              <Play fill="currentColor" aria-hidden="true" />
            ) : null}
            {t(
              item.link === "announcement"
                ? "spotlight.announcement"
                : item.link === "watch"
                  ? "spotlight.watch"
                  : "spotlight.open",
            )}
            <ArrowUpRight aria-hidden="true" />
          </span>
        ) : null}
      </span>
    </div>
  )
}

/** The latest video under a compact ticket. */
function LatestCard({
  companion,
  locale,
  now,
  t,
}: {
  companion: Companion
  locale: Locale
  now: number
  t: Translator
}) {
  const links = videoLinks(companion.video, locale, now)
  const companionForm = videoFormLabel(companion.video, t, now)
  return (
    <div className="latest-card link-card" onClick={followCardLink}>
      <span className="latest-card-art" aria-hidden="true">
        {companion.thumbnailUrl ? (
          <img
            src={companion.thumbnailUrl}
            srcSet={mediaSrcSet(companion.thumbnailUrl)}
            sizes="(max-width: 640px) 40vw, 240px"
            alt=""
            loading="lazy"
          />
        ) : (
          <span className="ticket-art-fallback" />
        )}
        <PlatformBadge platforms={videoPlatforms(companion.video, links)} />
        <span className="latest-card-play">
          <Play fill="currentColor" />
        </span>
      </span>
      <span className="latest-card-copy">
        <span className="ticket-head">
          <span className="kicker">{t("spotlight.latest")}</span>
          {companionForm ? <span className="tag">{companionForm}</span> : null}
          <VideoTagChip video={companion.video} t={t} />
          <VideoAvailabilityChip video={companion.video} t={t} />
        </span>
        <TranslatedTitle
          className="latest-card-title"
          title={companion.title}
          translations={companion.video.translations}
          sourceLanguage={companion.video.sourceLanguage}
          href={links[0]?.url}
          t={t}
        />
        {companion.time ? (
          <small>{formatRelative(companion.time, locale, now)}</small>
        ) : null}
        {links.length > 1 ? (
          <PlatformLinks
            links={links}
            title={companion.title}
            variant="chip"
            t={t}
          />
        ) : null}
      </span>
      <ArrowUpRight className="latest-card-open" aria-hidden="true" />
    </div>
  )
}

/** The single most useful thing to know right now: what airs next. */
function NextTicket({
  summary,
  liveNow,
  isLoading,
  locale,
  now,
  t,
}: {
  summary: Dashboard["summary"]
  liveNow: Dashboard["liveNow"]
  isLoading: boolean
  locale: Locale
  now: number
  t: Translator
}) {
  // The placeholder takes a full ticket's shape, the stage's usual one, so the
  // first snapshot fills it in place instead of resizing the stage.
  if (isLoading) {
    return (
      <div className="ticket is-loading has-art" aria-busy="true">
        <span className="ticket-art" aria-hidden="true" />
        <span className="ticket-stub" aria-hidden="true">
          <span className="ticket-placeholder is-head" />
          <span className="ticket-placeholder is-title" />
          <span className="ticket-placeholder is-when" />
        </span>
        <span className="sr-only">{t("common.loading")}</span>
      </div>
    )
  }
  const item = pickSpotlight(summary, now, liveNow)
  if (!item) {
    return (
      <div className="ticket is-empty">
        <div className="ticket-stub">
          <p className="kicker">{t("spotlight.next")}</p>
          <p className="ticket-title">{t("spotlight.none")}</p>
          <p className="ticket-note">{t("spotlight.noneHint")}</p>
        </div>
      </div>
    )
  }

  const isLive = spotlightIsLive(item, now)
  if (item.layout === "full") {
    return (
      <FullTicket item={item} isLive={isLive} locale={locale} now={now} t={t} />
    )
  }
  const companion = pickCompanion(summary, item)
  return (
    <div className="stage-main">
      <CompactTicket
        item={item}
        isLive={isLive}
        locale={locale}
        now={now}
        t={t}
      />
      {companion ? (
        <LatestCard companion={companion} locale={locale} now={now} t={t} />
      ) : null}
    </div>
  )
}

function ProfileCard({
  profile,
  resources,
  counts,
  postWindowDays,
  videoWindowDays,
  syncState,
  isRefreshing,
  onRefresh,
  t,
}: {
  profile: Profile
  resources: Dashboard["resources"]
  counts: { posts: number; upcomingEvents: number; videos: number }
  postWindowDays: number
  videoWindowDays: number
  syncState: SyncState
  isRefreshing: boolean
  onRefresh: () => void
  t: Translator
}) {
  // Every X account in the link directory, main first; the profile's own
  // account stands in when the directory has none.
  const xUrls = resources
    .map((resource) => resource.url || "")
    .filter((url) => /^https:\/\/(?:www\.)?(?:x|twitter)\.com\/\w+$/u.test(url))
  if (!xUrls.length) xUrls.push(profile.xUrl)
  const bilibili = resources.find((resource) =>
    /bilibili\.com/u.test(resource.url || ""),
  )
  return (
    <aside className="profile-card" aria-labelledby="profile-name">
      <div className="profile-banner">
        {profile.bannerUrl ? (
          <img src={profile.bannerUrl} alt={t("header.bannerAlt")} />
        ) : null}
      </div>
      <div className="profile-body">
        <img
          className="profile-avatar"
          src={profile.avatarUrl ?? undefined}
          alt=""
        />
        <p className="kicker">{t("hero.kicker")}</p>
        <h1 id="profile-name" className="profile-name">
          {profile.displayName}
          <span>{profile.romanizedName}</span>
        </h1>
        <p className="profile-catchphrase">{t("hero.catchphrase")}</p>
        <div className="profile-links">
          {xUrls.map((url) => (
            <ExternalLink key={url} className="pill-button" href={url}>
              <XLogo aria-hidden="true" />
              <span>@{url.split("/").pop()}</span>
            </ExternalLink>
          ))}
          <ExternalLink
            className="round-button"
            href={profile.youtubeUrl}
            aria-label="YouTube"
            title="YouTube"
          >
            <YouTubeLogo aria-hidden="true" />
          </ExternalLink>
          {bilibili ? (
            <ExternalLink
              className="round-button"
              href={bilibili.url}
              aria-label="bilibili"
              title="bilibili"
            >
              <BilibiliLogo aria-hidden="true" />
            </ExternalLink>
          ) : null}
        </div>
        {/* Each count opens the board section it counts. */}
        <ul className="profile-stats">
          <li>
            <a href="#schedule">
              <span>{t("stats.upcoming")}</span>
              <strong>{counts.upcomingEvents}</strong>
            </a>
          </li>
          <li>
            <a href="#feed">
              <span>{t("stats.posts", { days: postWindowDays })}</span>
              <strong>{counts.posts}</strong>
            </a>
          </li>
          <li>
            <a href="#videos">
              <span>{t("stats.videos", { days: videoWindowDays })}</span>
              <strong>{counts.videos}</strong>
            </a>
          </li>
        </ul>
        {/* The run's state and the reload above each source's last read; on
            phones they share one row, the source times first. */}
        <div className="profile-status">
          <div
            className={`profile-sync tone-${syncState.tone}`}
            title={syncState.label}
          >
            <span className="sync-dot" aria-hidden="true" />
            <span className="profile-sync-copy">
              <strong>{syncState.label}</strong>
              {syncState.runAt ? (
                <small>
                  {t("sync.latestRun")}{" "}
                  <time dateTime={syncState.runAt}>
                    {formatStamp(syncState.runAt)}
                  </time>{" "}
                  {t("common.timezoneShort")}
                </small>
              ) : null}
            </span>
            <button
              type="button"
              className={`round-button${isRefreshing ? " is-spinning" : ""}`}
              onClick={onRefresh}
              disabled={isRefreshing}
              aria-label={t(
                isRefreshing ? "common.refreshing" : "common.refresh",
              )}
              title={t(isRefreshing ? "common.refreshing" : "common.refresh")}
            >
              <RefreshCw aria-hidden="true" />
            </button>
          </div>
          {syncState.sources.length ? (
            <dl className="profile-sources" aria-label={t("sync.sourceTimes")}>
              {syncState.sources.map(({ source, timestamp, state }) => {
                const { name: brand, Icon } = SOURCES[source]
                const name = brand ?? t(`sync.source.${source}`)
                const note =
                  state === "fresh"
                    ? null
                    : t(state === "stale" ? "sync.stale" : "sync.waiting")
                return (
                  <div
                    key={source}
                    className={state === "stale" ? "is-stale" : undefined}
                    title={note ? `${name} · ${note}` : name}
                  >
                    <dt>
                      <Icon aria-hidden="true" />
                      <span className="sr-only">{name}</span>
                    </dt>
                    <dd>
                      {timestamp ? (
                        <time dateTime={timestamp}>
                          {formatStamp(timestamp)}
                        </time>
                      ) : (
                        <span aria-hidden="true">—</span>
                      )}
                      {note ? <span className="sr-only">{note}</span> : null}
                    </dd>
                  </div>
                )
              })}
            </dl>
          ) : null}
        </div>
      </div>
    </aside>
  )
}

export function NowStage(props: {
  profile: Profile
  summary: Dashboard["summary"]
  /** An unannounced live broadcast no schedule entry shows. */
  liveNow?: Dashboard["liveNow"]
  resources: Dashboard["resources"]
  counts: { posts: number; upcomingEvents: number; videos: number }
  postWindowDays: number
  videoWindowDays: number
  syncState: SyncState
  isLoading: boolean
  isRefreshing: boolean
  onRefresh: () => void
  locale: Locale
  now: number
  t: Translator
}) {
  const { summary, liveNow, isLoading, locale, now, t } = props
  return (
    <section className="stage" id="now" aria-label={t("nav.now")}>
      <NextTicket
        summary={summary}
        liveNow={liveNow ?? null}
        isLoading={isLoading}
        locale={locale}
        now={now}
        t={t}
      />
      <ProfileCard {...props} />
    </section>
  )
}
