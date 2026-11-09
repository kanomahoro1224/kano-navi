import { useState } from "react"
import {
  ArrowLeft,
  ArrowRight,
  CalendarCheck,
  CalendarRange,
  Check,
  ExternalLink,
  ImageOff,
  ImagePlus,
  X,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { formatDay } from "@/admin/format"
import type { ScheduleImage } from "@/admin/types"
import { Badge, Button } from "@/admin/ui"
import { useAdmin } from "@/admin/use-admin-data"
import { shiftDay } from "@/admin/views/event-parts"
import {
  mediaLabel,
  needsReview,
  SourcePost,
} from "@/admin/views/schedule-images"

/**
 * One schedule image of the week, large enough to read beside the week's
 * entries: its state, its post, its dates, the label decisions and moves of
 * its dates by a whole week, which take the view along so the image stays
 * beside the week it now covers. An image whose dates are not exactly this
 * week can be fitted to it.
 */
export function WeekImage({
  image,
  week,
  onChanged,
  onWeek,
}: {
  image: ScheduleImage
  week: string
  onChanged: (image: ScheduleImage) => void
  onWeek: (week: string) => void
}) {
  const { t, locale } = useAppSettings()
  const { api, act, refreshOverview } = useAdmin()
  const [busy, setBusy] = useState("")
  const start = image.periodStart || week
  const end = image.periodEnd || start
  const sunday = shiftDay(week, 6)
  const fitted = start === week && end === sunday
  const review = async (status: string) => {
    setBusy(status)
    const result = await act(
      () =>
        api.reviewImage(
          image.id,
          status,
          t(`admin.review.defaultReason.${status}`),
        ),
      "admin.notice.imageReviewed",
    )
    setBusy("")
    if (result) {
      onChanged(result.image)
      void refreshOverview().catch(() => {})
    }
  }
  const setPeriod = async (
    key: string,
    from: string,
    to: string,
    reason: string,
    nextWeek?: string,
  ) => {
    setBusy(key)
    const result = await act(
      () => api.setImagePeriod(image.id, from, to, reason),
      "admin.notice.periodSaved",
    )
    setBusy("")
    if (!result) return
    onChanged(result.image)
    if (nextWeek) onWeek(nextWeek)
  }
  const move = (weeks: number) =>
    setPeriod(
      weeks < 0 ? "previous" : "next",
      shiftDay(start, weeks * 7),
      shiftDay(end, weeks * 7),
      t(
        weeks < 0
          ? "admin.schedule.week.movedEarlier"
          : "admin.schedule.week.movedLater",
      ),
      shiftDay(week, weeks * 7),
    )
  const state = needsReview(image)
    ? { tone: "honey" as const, label: t("admin.review.filter.review") }
    : image.approved
      ? { tone: "leaf" as const, label: t("admin.review.approved") }
      : { tone: "neutral" as const, label: t("admin.schedule.week.notOnBoard") }
  return (
    <article className={`adm-week-image adm-tone-${state.tone}`}>
      <a
        className="adm-week-image-media"
        href={image.previewUrl || undefined}
        target="_blank"
        rel="noreferrer"
        title={t("admin.schedule.week.openImage")}
      >
        {image.previewUrl ? (
          <img src={image.previewUrl} alt={image.alt || ""} />
        ) : (
          <span>
            <ImageOff aria-hidden />
            {t(`admin.review.media.${mediaLabel(image)}`)}
          </span>
        )}
      </a>
      <div className="adm-week-image-body">
        <div className="adm-chip-row">
          <Badge tone={state.tone} icon={image.approved ? Check : undefined}>
            {state.label}
          </Badge>
          <span className="adm-week-image-period">
            <CalendarRange aria-hidden />
            {`${formatDay(start, locale)} – ${formatDay(end, locale)}`}
          </span>
          {image.periodBasis === "manual" ? (
            <Badge tone="sky">{t("admin.review.periodManual")}</Badge>
          ) : null}
          {image.sourceUrl ? (
            <a
              className="adm-link"
              href={image.sourceUrl}
              target="_blank"
              rel="noreferrer"
            >
              {image.sourceAccount
                ? `@${image.sourceAccount}`
                : t("admin.review.sourcePost")}
              <ExternalLink aria-hidden />
            </a>
          ) : null}
        </div>
        <SourcePost image={image} />
        <div className="adm-week-image-actions">
          <Button
            size="sm"
            icon={ArrowLeft}
            busy={busy === "previous"}
            onClick={() => move(-1)}
          >
            {t("admin.schedule.week.moveEarlier")}
          </Button>
          <Button
            size="sm"
            icon={ArrowRight}
            busy={busy === "next"}
            onClick={() => move(1)}
          >
            {t("admin.schedule.week.moveLater")}
          </Button>
          {fitted ? null : (
            <Button
              size="sm"
              variant="ghost"
              icon={ImagePlus}
              busy={busy === "fit"}
              onClick={() =>
                setPeriod(
                  "fit",
                  week,
                  sunday,
                  t("admin.schedule.week.fittedReason"),
                )
              }
            >
              {t("admin.schedule.week.fit")}
            </Button>
          )}
          <span className="adm-spacer" />
          {image.manualStatus === "schedule" ? null : (
            <Button
              size="sm"
              variant="primary"
              icon={CalendarCheck}
              busy={busy === "schedule"}
              onClick={() => review("schedule")}
            >
              {t("admin.review.isSchedule")}
            </Button>
          )}
          {image.manualStatus === "not_schedule" ? null : (
            <Button
              size="sm"
              variant="ghost"
              icon={X}
              busy={busy === "not_schedule"}
              onClick={() => review("not_schedule")}
            >
              {t("admin.review.notSchedule")}
            </Button>
          )}
        </div>
      </div>
    </article>
  )
}
