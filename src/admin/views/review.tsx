import { useId, useState } from "react"
import {
  Ban,
  CalendarCheck,
  CalendarX,
  CheckCircle2,
  ExternalLink,
  Images,
  Languages,
  Sparkles,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { japanStamp } from "@/admin/format"
import type { AdminEvent, AdminPost } from "@/admin/types"
import {
  Badge,
  Button,
  Empty,
  IconButton,
  Loading,
  PageHeader,
  Panel,
  Segmented,
  StatusBadge,
} from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"
import { eventToInput, EventWhen } from "@/admin/views/event-parts"
import { MergeQueue } from "@/admin/views/merge-parts"
import {
  ConfidenceMeter,
  ScheduleImageList,
} from "@/admin/views/schedule-images"
import { eventDetailText } from "@/lib/event-text"

const tabs = ["images", "cancellations", "merges", "posts"] as const
type Tab = (typeof tabs)[number]

/** Images that need a decision; every stored image is on the Schedule page. */
function ImagesTab({ navigate }: Pick<ViewProps, "navigate">) {
  const { t } = useAppSettings()
  return (
    <ScheduleImageList
      filters={["review"]}
      actions={
        <Button
          size="sm"
          variant="ghost"
          icon={Images}
          onClick={() => navigate("schedule", "images")}
        >
          {t("admin.review.allImages")}
        </Button>
      }
    />
  )
}

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
          </div>
        </footer>
      </div>
    </article>
  )
}

function CancellationsTab() {
  const { t } = useAppSettings()
  const { api, refreshOverview } = useAdmin()
  const events = useResource(
    () =>
      api.events({ cancellation: "llm_suspected", order: "asc", pageSize: 50 }),
    [],
  )
  if (!events.data) return <Loading />
  return events.data.items.length ? (
    <div className="adm-decision-list">
      {events.data.items.map((event) => (
        <CancellationCard
          key={event.id}
          event={event}
          onDone={() => {
            void events.reload()
            void refreshOverview().catch(() => {})
          }}
        />
      ))}
    </div>
  ) : (
    <Empty icon={CheckCircle2} title={t("admin.review.noCancellations")} />
  )
}

/**
 * The extraction state, last error, failure backoff and queue flag of one
 * post. A failure names the schedule extraction, apart from a failed
 * translation. A queued state already says the post waits, so the flag is
 * not repeated.
 */
export function PostStatus({ post }: { post: AdminPost }) {
  const { t } = useAppSettings()
  return (
    <>
      {post.llmStatus === "failed" ? (
        <Badge tone="danger">{t("admin.review.failure.extraction")}</Badge>
      ) : (
        <StatusBadge status={post.llmStatus} />
      )}
      {post.llmLastError ? (
        <Badge tone="danger">
          {t(`admin.posts.error.${post.llmLastError}`)}
        </Badge>
      ) : null}
      {post.llmRetryStopped ? (
        <Badge tone="warning" className="adm-retry-badge">
          {t("admin.posts.retryStopped")}
        </Badge>
      ) : post.llmRetryAt ? (
        <Badge tone="neutral" className="adm-retry-badge">
          {t("admin.posts.retryAt", {
            time: japanStamp(post.llmRetryAt),
          })}
        </Badge>
      ) : null}
      {post.llmReprocessRequested && post.llmStatus !== "queued" ? (
        <Badge tone="sky">{t("admin.posts.queued")}</Badge>
      ) : null}
    </>
  )
}

/**
 * Have the LLM process a post again in the next schedule scan, and open it.
 * Compact actions are icons that fit a row; the caller lays them out.
 */
export function PostActions({
  post,
  onDone,
  compact = false,
  reprocess: withReprocess = true,
}: {
  post: AdminPost
  onDone: () => void
  compact?: boolean
  /** False leaves only the link, for a post that failed only translation. */
  reprocess?: boolean
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [busy, setBusy] = useState(false)
  const reprocess = async () => {
    setBusy(true)
    const result = await act(
      () => api.reprocessPost(post.id),
      "admin.notice.postQueued",
    )
    setBusy(false)
    if (result) onDone()
  }
  if (compact)
    return (
      <>
        <IconButton
          icon={Sparkles}
          label={t("admin.posts.reprocess")}
          busy={busy}
          onClick={reprocess}
        />
        <a
          className="adm-icon-button"
          href={post.url}
          target="_blank"
          rel="noreferrer"
          aria-label={t("admin.posts.open")}
          title={t("admin.posts.open")}
        >
          <ExternalLink aria-hidden />
        </a>
      </>
    )
  return (
    <>
      {withReprocess ? (
        <Button size="sm" icon={Sparkles} busy={busy} onClick={reprocess}>
          {t("admin.posts.reprocess")}
        </Button>
      ) : null}
      <a className="adm-link" href={post.url} target="_blank" rel="noreferrer">
        {t("admin.posts.open")}
        <ExternalLink aria-hidden />
      </a>
    </>
  )
}

/** A failed machine translation of the post's current wording, with its error. */
function TranslationFailure({ post }: { post: AdminPost }) {
  const { t } = useAppSettings()
  const translation = post.translation
  if (translation?.machineStatus !== "failed") return null
  return (
    <>
      <Badge tone="danger">{t("admin.review.failure.translation")}</Badge>
      {translation.lastError ? (
        <Badge tone="danger">
          {t(`admin.posts.error.${translation.lastError}`)}
        </Badge>
      ) : null}
      {translation.requested ? (
        <Badge tone="sky">{t("admin.posts.queued")}</Badge>
      ) : null}
    </>
  )
}

type PostFilter = "extraction" | "translation" | "uncertain" | "queued"

const postFilters: Record<PostFilter, Record<string, unknown>> = {
  extraction: { llm: "failed" },
  translation: { translation: "failed" },
  uncertain: { llm: "uncertain" },
  queued: { reprocess: true },
}

/**
 * One post of the Failed posts tab: which step failed and why, and the
 * actions for the tab's step. A failed extraction can be dismissed, which
 * stops its retries; a failed translation can be queued again.
 */
export function PostRow({
  post,
  filter,
  onDone,
}: {
  post: AdminPost
  filter: PostFilter
  onDone: () => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [busy, setBusy] = useState("")
  const run = async (
    kind: string,
    call: () => Promise<unknown>,
    notice: string,
  ) => {
    setBusy(kind)
    const result = await act(call, notice)
    setBusy("")
    if (result) onDone()
  }
  const translation = filter === "translation"
  return (
    <article className="adm-post-row">
      <div className="adm-post-meta">
        <strong>{post.accountHandle || t(`admin.step.${post.source}`)}</strong>
        <time dateTime={post.publishedAt}>{japanStamp(post.publishedAt)}</time>
        {translation && post.llmStatus !== "failed" ? null : (
          <PostStatus post={post} />
        )}
        <TranslationFailure post={post} />
      </div>
      <p className="adm-post-text">{post.text}</p>
      <div className="adm-actions">
        {filter === "extraction" && post.llmStatus === "failed" ? (
          <Button
            size="sm"
            icon={Ban}
            title={t("admin.posts.dismissTitle")}
            busy={busy === "dismiss"}
            onClick={() =>
              run(
                "dismiss",
                () => api.dismissPost(post.id),
                "admin.notice.postDismissed",
              )
            }
          >
            {t("admin.posts.dismiss")}
          </Button>
        ) : null}
        {translation ? (
          <Button
            size="sm"
            icon={Languages}
            busy={busy === "translate"}
            disabled={Boolean(post.translation?.requested)}
            onClick={() =>
              run(
                "translate",
                () =>
                  api.requestTranslations({
                    ownerType: "post",
                    ids: [post.id],
                  }),
                "admin.notice.translationQueued",
              )
            }
          >
            {t("admin.review.retryTranslation")}
          </Button>
        ) : null}
        <PostActions post={post} onDone={onDone} reprocess={!translation} />
      </div>
    </article>
  )
}

function PostsTab() {
  const { t } = useAppSettings()
  const { api, overview, refreshOverview } = useAdmin()
  const [filter, setFilter] = useState<PostFilter>("extraction")
  const posts = useResource(
    () => api.posts({ ...postFilters[filter], pageSize: 50 }),
    [filter],
  )
  const counts: Partial<Record<PostFilter, number>> = {
    extraction: overview?.review.failedExtractions,
    translation: overview?.review.failedTranslations,
  }
  return (
    <>
      <div className="adm-toolbar">
        <Segmented
          label={t("admin.review.filter")}
          value={filter}
          onChange={setFilter}
          options={[
            { value: "extraction", label: t("admin.review.posts.extraction") },
            {
              value: "translation",
              label: t("admin.review.posts.translation"),
            },
            { value: "uncertain", label: t("admin.status.uncertain") },
            { value: "queued", label: t("admin.posts.queued") },
          ].map((option) => ({
            ...option,
            value: option.value as PostFilter,
            count: counts[option.value as PostFilter],
          }))}
        />
      </div>
      {!posts.data ? (
        <Loading />
      ) : posts.data.items.length ? (
        <div className="adm-post-list">
          {posts.data.items.map((post) => (
            <PostRow
              key={post.id}
              post={post}
              filter={filter}
              onDone={() => {
                void posts.reload()
                void refreshOverview().catch(() => {})
              }}
            />
          ))}
        </div>
      ) : (
        <Empty icon={CheckCircle2} title={t("admin.review.noPosts")} />
      )}
    </>
  )
}

export function ReviewView({ tab, navigate }: ViewProps) {
  const { t } = useAppSettings()
  const { overview } = useAdmin()
  const active: Tab = (tabs as readonly string[]).includes(tab)
    ? (tab as Tab)
    : "images"
  const counts = {
    images: overview?.review.pendingImages,
    cancellations: overview?.review.suspectedCancellations,
    merges: overview?.review.videoMerges,
    posts: overview?.review.failedPosts,
  }
  return (
    <div className="adm-view">
      <PageHeader title={t("admin.review.title")} />
      <Panel>
        <div
          className="adm-tabs"
          role="tablist"
          aria-label={t("admin.review.title")}
        >
          {tabs.map((id) => (
            <button
              key={id}
              type="button"
              role="tab"
              aria-selected={active === id}
              className={active === id ? "is-active" : undefined}
              onClick={() => navigate("review", id)}
            >
              {t(`admin.review.tab.${id}`)}
              {counts[id] ? <em>{counts[id]}</em> : null}
            </button>
          ))}
        </div>
        <div role="tabpanel" className="adm-tab-panel">
          {active === "images" ? <ImagesTab navigate={navigate} /> : null}
          {active === "cancellations" ? <CancellationsTab /> : null}
          {active === "merges" ? <MergeQueue /> : null}
          {active === "posts" ? <PostsTab /> : null}
        </div>
      </Panel>
    </div>
  )
}
