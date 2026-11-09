import { useState } from "react"
import {
  CalendarCheck,
  CalendarPlus,
  CalendarX,
  CheckCheck,
  CircleHelp,
  Combine,
  ExternalLink,
  FileSearch,
  Pencil,
  Split,
  Trash2,
  TriangleAlert,
  Undo2,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { eventStatusLabel, formatDay, formatTime } from "@/admin/format"
import type { AdminEvent } from "@/admin/types"
import { eventDetailText } from "@/lib/event-text"
import { Badge, Button, ConfirmButton, IconButton } from "@/admin/ui"
import { useAdmin } from "@/admin/use-admin-data"
import { EventEvidencePanel } from "@/admin/views/event-evidence"
import {
  CancellationBadge,
  eventToInput,
  HiddenByPostsBadge,
  MergeBadge,
  PlatformBadges,
  ProvenanceBadge,
  shiftDay,
} from "@/admin/views/event-parts"

// The schedule's entry rows and day headings, shared by its list and its
// calendar.

/** Consecutive entries on the same Japan date, in list order. */
export function groupByDay(events: AdminEvent[]) {
  const days: { day: string; events: AdminEvent[] }[] = []
  for (const event of events) {
    const last = days[days.length - 1]
    if (last?.day === event.startsOn) last.events.push(event)
    else days.push({ day: event.startsOn, events: [event] })
  }
  return days
}

/** Below this model confidence an automatic entry is an uncertain addition. */
/**
 * What an entry asks of the operator, as the server's review rule decides:
 * a suspected cancellation to decide, or an uncertain addition to confirm or
 * delete, with the reasons; null when it asks nothing.
 */
export function reviewOf(event: AdminEvent) {
  return event.review ?? null
}

/**
 * The entry's review in place: the model's suspicion and its evidence with
 * the cancellation decision, or why an addition is uncertain; either opens
 * the source posts below. Confirming or deleting an uncertain addition uses
 * the row's own actions.
 */
function ReviewNote({
  event,
  onChanged,
}: {
  event: AdminEvent
  onChanged: () => void
}) {
  const { t } = useAppSettings()
  const { api, act, refreshOverview } = useAdmin()
  const [busy, setBusy] = useState("")
  const [open, setOpen] = useState(false)
  const review = reviewOf(event)
  if (!review) return null
  const decide = async (status: "manual_confirmed" | "none") => {
    setBusy(status)
    const result = await act(
      () =>
        api.updateEvent(event.id, {
          ...eventToInput(event),
          cancellationStatus: status,
          cancellationReason:
            status === "manual_confirmed"
              ? event.cancellationEvidence ||
                t("admin.schedule.review.cancelReason")
              : "",
        }),
      status === "none"
        ? "admin.notice.cancellationDismissed"
        : "admin.notice.cancellationConfirmed",
    )
    setBusy("")
    if (result) {
      onChanged()
      void refreshOverview().catch(() => {})
    }
  }
  const suspected = review.kind === "suspected"
  const Icon = suspected ? TriangleAlert : CircleHelp
  const detail = suspected
    ? event.cancellationConfidence != null
      ? `${Math.round(event.cancellationConfidence * 100)}%`
      : ""
    : review.reasons
        .map((reason) =>
          t(`admin.schedule.review.reason.${reason}`, {
            percent: Math.round((event.confidence ?? 0) * 100),
          }),
        )
        .join(" · ")
  return (
    <div className={`adm-review-note is-${review.kind}`}>
      <div className="adm-review-note-head">
        <Icon aria-hidden />
        <strong>{t(`admin.schedule.review.${review.kind}`)}</strong>
        {detail ? <span>{detail}</span> : null}
      </div>
      {suspected && event.cancellationEvidence ? (
        <p>{event.cancellationEvidence}</p>
      ) : null}
      <div className="adm-review-note-actions">
        {suspected ? (
          <>
            <Button
              size="sm"
              variant="danger"
              icon={CalendarX}
              busy={busy === "manual_confirmed"}
              onClick={() => decide("manual_confirmed")}
            >
              {t("admin.review.confirmCancelled")}
            </Button>
            <Button
              size="sm"
              icon={CalendarCheck}
              busy={busy === "none"}
              onClick={() => decide("none")}
            >
              {t("admin.review.notCancelled")}
            </Button>
          </>
        ) : null}
        <Button
          size="sm"
          variant="ghost"
          icon={FileSearch}
          aria-expanded={open}
          onClick={() => setOpen((shown) => !shown)}
        >
          {t(open ? "admin.evidence.hide" : "admin.evidence.show")}
        </Button>
      </div>
      {open ? <EventEvidencePanel eventId={event.id} /> : null}
    </div>
  )
}

/** The duplicates merging hid behind an entry, each with its undo. */
function MergedDuplicates({
  merged,
  onChanged,
}: {
  merged: AdminEvent[]
  onChanged: () => void
}) {
  const { t, locale } = useAppSettings()
  const { api, act } = useAdmin()
  if (!merged.length) return null
  return (
    <ul className="adm-merged-list">
      {merged.map((item) => (
        <li key={item.id}>
          <Combine aria-hidden />
          <span>
            {t("admin.schedule.review.mergedHere", { title: item.title })}
            {item.startsAt ? ` · ${formatTime(item.startsAt, locale)}` : ""}
          </span>
          <IconButton
            icon={Split}
            label={t("admin.schedule.unmerge")}
            onClick={async () => {
              if (
                await act(
                  () => api.unmergeEvent(item.id),
                  "admin.notice.eventUnmerged",
                )
              )
                onChanged()
            }}
          />
        </li>
      ))}
    </ul>
  )
}

/** The entry's time in Japan, or All day; date-only entries never gain one. */
export function EventTime({ event }: { event: AdminEvent }) {
  const { t, locale } = useAppSettings()
  if (!event.startsAt)
    return (
      <span className="adm-event-time is-all-day">
        <strong>{t("admin.schedule.allDay")}</strong>
      </span>
    )
  return (
    <span className="adm-event-time">
      <strong>
        {event.timePrecision === "approximate" ? "~" : ""}
        {formatTime(event.startsAt, locale)}
      </strong>
      <small>
        {event.endsAt ? `–${formatTime(event.endsAt, locale)} ` : ""}JST
      </small>
    </span>
  )
}

export function EventRow({
  event,
  merged = [],
  onEdit,
  onMerge,
  onChanged,
}: {
  event: AdminEvent
  /** The duplicates merged into this entry, shown under it. */
  merged?: AdminEvent[]
  onEdit: (event: AdminEvent) => void
  onMerge: (event: AdminEvent) => void
  onChanged: () => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [confirming, setConfirming] = useState(false)
  const deleted = Boolean(event.deletedAt)
  const review = reviewOf(event)
  return (
    <li
      className={`adm-event-row${deleted ? " is-deleted" : ""}${event.mergedInto ? " is-merged" : ""}${event.hiddenByPosts ? " is-hidden" : ""}${review ? ` needs-review is-${review.kind}` : ""}`}
    >
      <EventTime event={event} />
      <div className="adm-event-main">
        <button
          type="button"
          className="adm-event-title"
          onClick={() => onEdit(event)}
          title={t("admin.schedule.editNamed", { title: event.title })}
        >
          {event.title}
        </button>
        {event.detail ? (
          <small>{eventDetailText(event.detail, t)}</small>
        ) : null}
        <span className="adm-chip-row">
          <Badge>{t(`admin.event.type.${event.eventType || "event"}`)}</Badge>
          {event.tag ? (
            <Badge tone="lavender">{t(`admin.event.tag.${event.tag}`)}</Badge>
          ) : null}
          <PlatformBadges event={event} />
          <ProvenanceBadge event={event} />
          {review?.kind === "suspected" ? null : (
            <CancellationBadge event={event} />
          )}
          <MergeBadge event={event} />
          <HiddenByPostsBadge event={event} />
          {event.status ? (
            <Badge tone="sky">{eventStatusLabel(event.status, t)}</Badge>
          ) : null}
          {deleted ? (
            <Badge tone="danger">{t("admin.schedule.deleted")}</Badge>
          ) : null}
        </span>
        <ReviewNote event={event} onChanged={onChanged} />
        <MergedDuplicates merged={merged} onChanged={onChanged} />
      </div>
      <div className="adm-row-actions adm-event-actions">
        {deleted ? (
          <Button size="sm" icon={Undo2} onClick={() => onEdit(event)}>
            {t("admin.schedule.restore")}
          </Button>
        ) : !event.manualLocked ? (
          <Button
            size="sm"
            icon={CheckCheck}
            busy={confirming}
            title={t("admin.schedule.confirm")}
            onClick={async () => {
              setConfirming(true)
              const result = await act(
                () => api.confirmEvent(event.id),
                "admin.notice.eventConfirmed",
              )
              setConfirming(false)
              if (result) onChanged()
            }}
          >
            {t("admin.schedule.confirmShort")}
          </Button>
        ) : null}
        {event.url ? (
          <a
            className="adm-icon-button"
            href={event.url}
            target="_blank"
            rel="noreferrer"
            aria-label={t("admin.review.eventLink")}
            title={t("admin.review.eventLink")}
          >
            <ExternalLink aria-hidden />
          </a>
        ) : null}
        {deleted ? null : event.mergedInto ? (
          <IconButton
            icon={Split}
            label={t("admin.schedule.unmerge")}
            onClick={async () => {
              if (
                await act(
                  () => api.unmergeEvent(event.id),
                  "admin.notice.eventUnmerged",
                )
              )
                onChanged()
            }}
          />
        ) : (
          <IconButton
            icon={Combine}
            label={t("admin.schedule.merge.open")}
            onClick={() => onMerge(event)}
          />
        )}
        {deleted ? null : (
          <>
            <IconButton
              icon={Pencil}
              label={t("admin.schedule.edit")}
              onClick={() => onEdit(event)}
            />
            <ConfirmButton
              size="sm"
              variant="ghost"
              icon={Trash2}
              aria-label={t("admin.schedule.delete")}
              title={t("admin.schedule.delete")}
              onConfirm={async () => {
                // A delete answers null; a failure is undefined.
                const result = await act(
                  () => api.deleteEvent(event.id),
                  "admin.notice.eventDeleted",
                )
                if (result !== undefined) onChanged()
              }}
            />
          </>
        )}
      </div>
    </li>
  )
}

/** A day's heading: its date, how near it is, its count and a quick add. */
export function DayHeading({
  day,
  today,
  count,
  onCreate,
}: {
  day: string
  today: string
  count: number
  onCreate: (day: string) => void
}) {
  const { t, locale } = useAppSettings()
  const near =
    day === today
      ? "today"
      : day === shiftDay(today, 1)
        ? "tomorrow"
        : day === shiftDay(today, -1)
          ? "yesterday"
          : ""
  const label = formatDay(day, locale)
  return (
    <header className={`adm-day-head${day === today ? " is-today" : ""}`}>
      <h3>
        {label}
        {near ? <em>{t(`admin.schedule.day.${near}`)}</em> : null}
      </h3>
      <small>{t("admin.schedule.day.count", { count })}</small>
      <IconButton
        icon={CalendarPlus}
        label={t("admin.schedule.day.create", { day: label })}
        onClick={() => onCreate(day)}
      />
    </header>
  )
}
