import { useId, useState } from "react"
import {
  CalendarCheck,
  CalendarDays,
  CalendarX,
  CheckCircle2,
  ExternalLink,
  FileSearch,
  Sparkles,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { formatDay } from "@/admin/format"
import type { AdminEvent, AttentionWeek, ScheduleImage } from "@/admin/types"
import { Badge, Button, Empty, Loading } from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"
import { EventEvidencePanel } from "@/admin/views/event-evidence"
import { eventToInput, EventWhen, shiftDay } from "@/admin/views/event-parts"
import { weekStart } from "@/admin/views/schedule-calendar"
import { ConfidenceMeter, ImageCard } from "@/admin/views/schedule-images"
import { eventDetailText } from "@/lib/event-text"

/** The calendar address that opens a week with only what needs a look. */
export function lookTab(week: string) {
  return `look-${week}`
}

/** A suspected cancellation as a decision card: the model's evidence and
 * confidence, the reason to save, and the entry's source posts on request. */
function CancellationCard({
  event,
  onDone,
}: {
  event: AdminEvent
  onDone: () => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [reason, setReason] = useState(event.cancellationReason || "")
  const [busy, setBusy] = useState("")
  const [showEvidence, setShowEvidence] = useState(false)
  const reasonId = useId()
  const decide = async (status: "manual_confirmed" | "none") => {
    setBusy(status)
    const result = await act(
      () =>
        api.updateEvent(event.id, {
          ...eventToInput(event),
          cancellationStatus: status,
          cancellationReason: status === "manual_confirmed" ? reason : "",
        }),
      status === "none"
        ? "admin.notice.cancellationDismissed"
        : "admin.notice.cancellationConfirmed",
    )
    setBusy("")
    if (result) onDone()
  }
  return (
    <article className="adm-decision is-event adm-tone-honey">
      <div className="adm-decision-when">
        <EventWhen event={event} />
      </div>
      <div className="adm-decision-body">
        <header className="adm-decision-head">
          <div className="adm-decision-title">
            <h3>{event.title}</h3>
            {event.detail ? (
              <p className="adm-muted">{eventDetailText(event.detail, t)}</p>
            ) : null}
          </div>
          {event.url ? (
            <a
              className="adm-link"
              href={event.url}
              target="_blank"
              rel="noreferrer"
            >
              {t("admin.review.eventLink")}
              <ExternalLink aria-hidden />
            </a>
          ) : null}
        </header>
        <section className="adm-verdict">
          <div className="adm-verdict-head">
            <span className="adm-verdict-label">
              <Sparkles aria-hidden />
              {t("admin.review.cancelSuspected")}
            </span>
            {event.cancellationConfidence != null ? (
              <ConfidenceMeter
                value={event.cancellationConfidence}
                tone="honey"
              />
            ) : null}
          </div>
          {event.cancellationEvidence ? (
            <blockquote className="adm-verdict-reason">
              {event.cancellationEvidence}
            </blockquote>
          ) : null}
        </section>
        {showEvidence ? <EventEvidencePanel eventId={event.id} /> : null}
        <footer className="adm-decision-bar">
          <label className="adm-visually-hidden" htmlFor={reasonId}>
            {t("admin.review.cancelReason")}
          </label>
          <input
            id={reasonId}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder={t("admin.review.cancelReasonPlaceholder")}
            maxLength={500}
          />
          <div className="adm-actions">
            <Button
              size="sm"
              variant="danger"
              icon={CalendarX}
              disabled={!reason.trim()}
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
            <Button
              size="sm"
              variant="ghost"
              icon={FileSearch}
              aria-expanded={showEvidence}
              onClick={() => setShowEvidence((shown) => !shown)}
            >
              {t(showEvidence ? "admin.evidence.hide" : "admin.evidence.show")}
            </Button>
          </div>
        </footer>
      </div>
    </article>
  )
}

/** The week's uncertain additions, settled on the calendar beside the rest
 * of the week. */
function UncertainList({ events }: { events: AdminEvent[] }) {
  const { t } = useAppSettings()
  return (
    <ul className="adm-attention-uncertain">
      {events.map((event) => (
        <li key={event.id}>
          <EventWhen event={event} />
          <strong>{event.title}</strong>
          <span className="adm-chip-row">
            {(event.review?.reasons ?? []).map((reason) => (
              <Badge key={reason} tone="lavender">
                {t(`admin.schedule.review.reason.${reason}`, {
                  percent: Math.round((event.confidence ?? 0) * 100),
                })}
              </Badge>
            ))}
          </span>
        </li>
      ))}
    </ul>
  )
}

/** The cards of pending images; a decided one leaves the queue. */
function ImageCards({
  images,
  onChanged,
}: {
  images: ScheduleImage[]
  onChanged: () => void
}) {
  return images.map((image) => (
    <ImageCard key={image.id} image={image} onChanged={onChanged} />
  ))
}

/**
 * One week's schedule work: what it holds as counts, the button that opens
 * it on the calendar with only what needs a look, the uncertain additions to
 * settle there, and the image and cancellation decisions that need no more
 * than their card.
 */
function WeekGroup({
  week,
  thisWeek,
  navigate,
  onChanged,
}: {
  week: AttentionWeek
  thisWeek: string
  navigate: ViewProps["navigate"]
  onChanged: () => void
}) {
  const { t, locale } = useAppSettings()
  const suspected = week.events.filter(
    (event) => event.review?.kind === "suspected",
  )
  const uncertain = week.events.filter(
    (event) => event.review?.kind === "uncertain",
  )
  const counts = [
    ["images", week.images.length],
    ["cancellations", suspected.length],
    ["uncertain", uncertain.length],
  ] as const
  return (
    <section className="adm-attention-week">
      <header className="adm-attention-week-head">
        <div>
          <h3>
            {`${formatDay(week.weekStart, locale)} – ${formatDay(shiftDay(week.weekStart, 6), locale)}`}
            {week.weekStart === thisWeek ? (
              <Badge tone="sky">{t("admin.review.week.this")}</Badge>
            ) : null}
          </h3>
          <p className="adm-muted">
            {counts
              .filter(([, count]) => count > 0)
              .map(
                ([key, count]) =>
                  `${count} ${t(`admin.overview.attention.${key}`, { count })}`,
              )
              .join(" · ")}
          </p>
        </div>
        <Button
          size="sm"
          icon={CalendarDays}
          onClick={() => navigate("schedule", lookTab(week.weekStart))}
        >
          {t("admin.review.week.open")}
        </Button>
      </header>
      {uncertain.length ? <UncertainList events={uncertain} /> : null}
      <ImageCards images={week.images} onChanged={onChanged} />
      {suspected.map((event) => (
        <CancellationCard key={event.id} event={event} onDone={onChanged} />
      ))}
    </section>
  )
}

/**
 * The Review queue's schedule work, grouped by week, oldest first: pending
 * images, suspected cancellations and uncertain additions. Weeks from last
 * week on are open; earlier ones fold away, and undated images form their
 * own group, whose cards set their dates. Every decision reads the queue
 * and the overview counts again.
 */
export function ScheduleAttentionQueue({
  navigate,
}: Pick<ViewProps, "navigate">) {
  const { t } = useAppSettings()
  const { api, refreshOverview } = useAdmin()
  const attention = useResource(() => api.scheduleAttention(), [])
  const data = attention.data
  if (!data) return <Loading />
  const changed = () => {
    void attention.reload()
    void refreshOverview().catch(() => {})
  }
  const thisWeek = weekStart(data.today)
  const from = shiftDay(thisWeek, -7)
  const recent = data.weeks.filter((week) => week.weekStart >= from)
  const earlier = data.weeks.filter((week) => week.weekStart < from).reverse()
  if (!data.weeks.length && !data.undated.length)
    return (
      <Empty icon={CheckCircle2} title={t("admin.review.schedule.empty")} />
    )
  const group = (week: AttentionWeek) => (
    <WeekGroup
      key={week.weekStart}
      week={week}
      thisWeek={thisWeek}
      navigate={navigate}
      onChanged={changed}
    />
  )
  return (
    <div className="adm-attention-weeks">
      {data.truncated ? (
        <p className="adm-muted">{t("admin.review.schedule.truncated")}</p>
      ) : null}
      {recent.map(group)}
      {data.undated.length ? (
        <section className="adm-attention-week">
          <header className="adm-attention-week-head">
            <div>
              <h3>{t("admin.review.schedule.undated")}</h3>
              <p className="adm-muted">
                {t("admin.review.schedule.undatedHint")}
              </p>
            </div>
          </header>
          <ImageCards images={data.undated} onChanged={changed} />
        </section>
      ) : null}
      {earlier.length ? (
        <details className="adm-disclosure adm-attention-earlier">
          <summary>
            {t("admin.review.schedule.earlier", { count: earlier.length })}
          </summary>
          <div className="adm-attention-weeks">{earlier.map(group)}</div>
        </details>
      ) : null}
    </div>
  )
}
