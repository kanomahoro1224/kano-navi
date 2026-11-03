import { useEffect, useId, useRef, useState } from "react"
import {
  ArrowRight,
  Ban,
  CalendarCheck,
  CalendarX,
  CheckCircle2,
  ExternalLink,
  Images,
  Languages,
  Sparkles,
  X,
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
  Pager,
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
import { cn } from "@/lib/utils"

const tabs = ["images", "cancellations", "merges", "posts"] as const
type Tab = (typeof tabs)[number]

/** Images that need a decision; every stored image is on the Schedule page. */
function ImagesTab({ navigate }: Pick<ViewProps, "navigate">) {
  const { t } = useAppSettings()
  return (
    <ScheduleImageList
      filters={["review"]}
      navigate={navigate}
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
        {withReprocess ? (
          <IconButton
            icon={Sparkles}
            label={t("admin.posts.reprocess")}
            busy={busy}
            onClick={reprocess}
          />
        ) : null}
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

/**
 * The machine translation of the post's current wording: a failure with its
 * error, and the queue flag of a requested pass. `queued` false leaves the
 * flag out, for a row that only reports a failure.
 */
function TranslationStatus({
  post,
  queued = true,
}: {
  post: AdminPost
  queued?: boolean
}) {
  const { t } = useAppSettings()
  const translation = post.translation
  const failed = translation?.machineStatus === "failed"
  if (!translation || (!failed && !(queued && translation.requested)))
    return null
  return (
    <>
      {failed ? (
        <Badge tone="danger">{t("admin.review.failure.translation")}</Badge>
      ) : null}
      {failed && translation.lastError ? (
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

/** The processing step a post list covers. */
type PostStep = "extraction" | "translation"
/**
 * The step's results that wait on the operator; translation has no
 * uncertain result. Posts queued for a run wait on the run instead and are
 * listed from the overview's backlog.
 */
type PostState = "failed" | "uncertain"

const postStates: Record<PostStep, PostState[]> = {
  extraction: ["failed", "uncertain"],
  translation: ["failed"],
}

const postFilters: Record<
  PostStep,
  Partial<Record<PostState, Record<string, unknown>>>
> = {
  extraction: {
    failed: { llm: "failed" },
    uncertain: { llm: "uncertain" },
  },
  translation: {
    failed: { translation: "failed" },
  },
}

type BulkAction = "dismiss" | "reprocess" | "translate"

/** What a step's selected posts in one state can have done to them at once. */
const bulkActions: Record<
  PostStep,
  Partial<Record<PostState, BulkAction[]>>
> = {
  extraction: { failed: ["dismiss", "reprocess"], uncertain: ["reprocess"] },
  translation: { failed: ["translate"] },
}

const postPageSize = 50

/**
 * One post of the Posts tab: where the list's step stands for it and why,
 * and that step's actions. A failed extraction can be dismissed, which stops
 * its retries; a translation can be queued again. A post in a list with bulk
 * actions has a selection box on its left edge.
 */
function PostRow({
  post,
  step,
  onDone,
  selected,
  onSelect,
}: {
  post: AdminPost
  step: PostStep
  onDone: () => void
  selected?: boolean
  /** Toggle the selection; `range` extends it from the last toggled post. */
  onSelect?: (range: boolean) => void
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
  const translation = step === "translation"
  return (
    <article
      className={cn(
        "adm-post-row",
        onSelect && "is-selectable",
        selected && "is-selected",
      )}
    >
      {onSelect ? (
        <label className="adm-post-pick">
          <input
            type="checkbox"
            checked={Boolean(selected)}
            readOnly
            aria-label={t("admin.review.selectPost", {
              time: japanStamp(post.publishedAt),
            })}
            onClick={(event) => onSelect(event.shiftKey)}
          />
        </label>
      ) : null}
      <div className="adm-post-meta">
        <strong>{post.accountHandle || t(`admin.step.${post.source}`)}</strong>
        <time dateTime={post.publishedAt}>{japanStamp(post.publishedAt)}</time>
        {translation && post.llmStatus !== "failed" ? null : (
          <PostStatus post={post} />
        )}
        <TranslationStatus post={post} queued={translation} />
      </div>
      <p className="adm-post-text">{post.text}</p>
      <div className="adm-actions">
        {!translation && post.llmStatus === "failed" ? (
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

/**
 * The selection bar over a selectable post list: a page-wide box that reads
 * checked, mixed or empty, the count, and the filter's bulk actions. It
 * stays under the top bar while the list scrolls.
 */
function PostSelectionBar({
  actions,
  selected,
  pageCount,
  busy,
  onSelectPage,
  onClear,
  onAction,
}: {
  actions: BulkAction[]
  selected: number
  pageCount: number
  busy: BulkAction | ""
  onSelectPage: (all: boolean) => void
  onClear: () => void
  onAction: (action: BulkAction) => void
}) {
  const { t } = useAppSettings()
  const all = selected > 0 && selected === pageCount
  const mixed = selected > 0 && !all
  const icons = { dismiss: Ban, reprocess: Sparkles, translate: Languages }
  const labels = {
    dismiss: t("admin.posts.dismiss"),
    reprocess: t("admin.review.bulkReprocess"),
    translate: t("admin.review.retryTranslation"),
  }
  const titles: Partial<Record<BulkAction, string>> = {
    dismiss: t("admin.posts.dismissTitle"),
    reprocess: t("admin.posts.reprocess"),
  }
  return (
    <div className={cn("adm-select-bar", selected > 0 && "is-active")}>
      <label className="adm-check">
        <input
          type="checkbox"
          checked={all}
          ref={(input) => {
            if (input) input.indeterminate = mixed
          }}
          onChange={() => onSelectPage(!all)}
        />
        {selected ? (
          <strong aria-live="polite">
            {t("admin.review.selected", { count: selected })}
          </strong>
        ) : (
          t("admin.review.selectPage")
        )}
      </label>
      {selected ? (
        <IconButton
          icon={X}
          label={t("admin.review.clearSelection")}
          onClick={onClear}
        />
      ) : (
        <span className="adm-select-hint">{t("admin.review.selectHint")}</span>
      )}
      <div className="adm-actions">
        {actions.map((action) => (
          <Button
            key={action}
            size="sm"
            variant={action === "dismiss" ? "ghost" : undefined}
            icon={icons[action]}
            title={titles[action]}
            disabled={!selected || (Boolean(busy) && busy !== action)}
            busy={busy === action}
            onClick={() => onAction(action)}
          >
            {labels[action]}
          </Button>
        ))}
      </div>
    </div>
  )
}

/**
 * Posts that wait on a processing step, picked by two filters: the step
 * (schedule extraction or translation, each with its failure count) and the
 * state within it. A new step keeps the state when the step has it. The
 * step's posts queued for a run are counted under the filters, with a link
 * to their list.
 */
function PostsTab({ navigate }: Pick<ViewProps, "navigate">) {
  const { t } = useAppSettings()
  const { api, act, notify, overview } = useAdmin()
  const [step, setStep] = useState<PostStep>("extraction")
  const [state, setState] = useState<PostState>("failed")
  const [page, setPage] = useState(1)
  const [selection, setSelection] = useState<ReadonlySet<string>>(new Set())
  const [busy, setBusy] = useState<BulkAction | "">("")
  // The last toggled post, where a shift-click range starts.
  const anchor = useRef<string | null>(null)
  const posts = useResource(
    () =>
      api.posts({
        ...postFilters[step][state],
        page,
        pageSize: postPageSize,
      }),
    [step, state, page],
  )
  const items = posts.data?.items ?? []
  const actions = bulkActions[step][state] ?? []

  // A new filter or page starts with nothing selected; a reload keeps only
  // the selected posts that are still listed.
  useEffect(() => {
    setSelection(new Set())
    anchor.current = null
  }, [step, state, page])
  useEffect(() => {
    setSelection((current) => {
      const listed = new Set(posts.data?.items.map((post) => post.id))
      const kept = [...current].filter((id) => listed.has(id))
      return kept.length === current.size ? current : new Set(kept)
    })
  }, [posts.data])

  const reload = async () => {
    const next = await posts.reload()
    // A page emptied by a decision moves to the last page that has posts.
    if (next && !next.items.length && page > 1)
      setPage(Math.max(1, next.totalPages))
  }

  const toggle = (id: string, range: boolean) => {
    const ids = items.map((post) => post.id)
    const on = !selection.has(id)
    const from = range && anchor.current ? ids.indexOf(anchor.current) : -1
    const to = ids.indexOf(id)
    const span =
      from < 0 ? [id] : ids.slice(Math.min(from, to), Math.max(from, to) + 1)
    setSelection((current) => {
      const next = new Set(current)
      for (const each of span) {
        if (on) next.add(each)
        else next.delete(each)
      }
      return next
    })
    anchor.current = id
  }

  const runBulk = async (action: BulkAction) => {
    const ids = items
      .filter((post) => selection.has(post.id))
      .map((post) => post.id)
    if (!ids.length) return
    setBusy(action)
    const count = await act(async () => {
      if (action === "dismiss") return (await api.dismissPosts(ids)).dismissed
      if (action === "reprocess") return (await api.reprocessPosts(ids)).queued
      return (await api.requestTranslations({ ownerType: "post", ids })).queued
    })
    setBusy("")
    if (count === undefined) return
    notify(
      {
        dismiss: "admin.notice.postsDismissed",
        reprocess: "admin.notice.postsQueued",
        translate: "admin.notice.translationsQueued",
      }[action],
      { count },
    )
    setSelection(new Set())
    anchor.current = null
    await reload()
  }

  const failures: Record<PostStep, number | undefined> = {
    extraction: overview?.review.failedExtractions,
    translation: overview?.review.failedTranslations,
  }
  const stepIcons = { extraction: Sparkles, translation: Languages }
  const stateLabels: Record<PostState, string> = {
    failed: t("admin.status.failed"),
    uncertain: t("admin.status.uncertain"),
  }
  const waiting = {
    extraction: overview?.backlog?.extraction ?? 0,
    translation: overview?.backlog?.postTranslation ?? 0,
  }[step]
  return (
    <>
      <div className="adm-toolbar adm-post-filters">
        <div className="adm-filter-group">
          <span className="adm-filter-label" aria-hidden>
            {t("admin.review.posts.step")}
          </span>
          <Segmented
            label={t("admin.review.posts.step")}
            value={step}
            onChange={(next) => {
              setStep(next)
              if (!postStates[next].includes(state)) setState("failed")
              setPage(1)
            }}
            options={(["extraction", "translation"] as const).map((id) => {
              const StepIcon = stepIcons[id]
              return {
                value: id,
                label: (
                  <>
                    <StepIcon aria-hidden />
                    {t(`admin.review.posts.${id}`)}
                  </>
                ),
                count: failures[id],
              }
            })}
          />
        </div>
        {postStates[step].length > 1 ? (
          <div className="adm-filter-group">
            <span className="adm-filter-label" aria-hidden>
              {t("admin.review.posts.state")}
            </span>
            <Segmented
              label={t("admin.review.posts.state")}
              value={state}
              onChange={(next) => {
                setState(next)
                setPage(1)
              }}
              options={postStates[step].map((id) => ({
                value: id,
                label: stateLabels[id],
              }))}
            />
          </div>
        ) : null}
      </div>
      {waiting ? (
        <p className="adm-note adm-review-waiting">
          {t(`admin.review.posts.waiting.${step}`, { count: waiting })}
          <Button
            size="sm"
            variant="ghost"
            icon={ArrowRight}
            onClick={() => navigate("posts", `queued-${step}`)}
          >
            {t("admin.review.posts.showWaiting")}
          </Button>
        </p>
      ) : null}
      {!posts.data ? (
        <Loading />
      ) : items.length ? (
        <>
          {actions.length ? (
            <PostSelectionBar
              actions={actions}
              selected={selection.size}
              pageCount={items.length}
              busy={busy}
              onSelectPage={(all) =>
                setSelection(
                  all ? new Set(items.map((post) => post.id)) : new Set(),
                )
              }
              onClear={() => {
                setSelection(new Set())
                anchor.current = null
              }}
              onAction={runBulk}
            />
          ) : null}
          <div className="adm-post-list">
            {items.map((post) => (
              <PostRow
                key={post.id}
                post={post}
                step={step}
                onDone={() => void reload()}
                selected={selection.has(post.id)}
                onSelect={
                  actions.length ? (range) => toggle(post.id, range) : undefined
                }
              />
            ))}
          </div>
          <Pager
            page={posts.data.page}
            totalPages={posts.data.totalPages}
            total={posts.data.total}
            onPage={setPage}
          />
        </>
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
          {active === "posts" ? <PostsTab navigate={navigate} /> : null}
        </div>
      </Panel>
    </div>
  )
}
