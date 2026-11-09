import { useEffect, useRef, useState } from "react"
import {
  ArrowRight,
  Ban,
  CheckCircle2,
  CircleAlert,
  CircleHelp,
  ExternalLink,
  Languages,
  Sparkles,
  X,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { japanStamp } from "@/admin/format"
import type { AdminPost } from "@/admin/types"
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
import { MergeQueue } from "@/admin/views/merge-parts"
import { PostImages, PostQuote } from "@/admin/views/posts"
import { ScheduleAttentionQueue } from "@/admin/views/review-schedule"
import { cn } from "@/lib/utils"

const tabs = ["schedule", "merges", "posts"] as const
type Tab = (typeof tabs)[number]

/** The tab of an address; the schedule's former images and cancellations
 * tabs open its one queue. */
function tabOf(tab: string): Tab {
  if (tab === "images" || tab === "cancellations") return "schedule"
  return (tabs as readonly string[]).includes(tab) ? (tab as Tab) : "schedule"
}

/**
 * Who posted: an X post's @handle, or the source's name for another source
 * (a Bilibili dynamic's handle is the account's numeric mid).
 */
export function PostAccount({ post }: { post: AdminPost }) {
  const { t } = useAppSettings()
  return post.source === "x" && post.accountHandle
    ? `@${post.accountHandle.replace(/^@/, "")}`
    : t(`admin.step.${post.source}`)
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
 * Where the list's step stands for a post, as one line under it: the state
 * with its icon in the row's colour, the error, when the step last tried,
 * the automatic retry and whether a run waits for it.
 */
function PostVerdict({ post, step }: { post: AdminPost; step: PostStep }) {
  const { t } = useAppSettings()
  if (step === "translation") {
    const translation = post.translation
    return (
      <p className="adm-post-verdict">
        <Languages aria-hidden />
        <strong>{t("admin.review.failure.translation")}</strong>
        {translation?.lastError ? (
          <span>{t(`admin.posts.error.${translation.lastError}`)}</span>
        ) : null}
        {translation?.lastAttemptAt ? (
          <span className="adm-muted">
            {t("admin.review.posts.lastTried", {
              time: japanStamp(translation.lastAttemptAt),
            })}
          </span>
        ) : null}
        {translation?.requested ? (
          <Badge tone="sky">{t("admin.posts.queued")}</Badge>
        ) : null}
      </p>
    )
  }
  const failed = post.llmStatus === "failed"
  const Icon = failed ? CircleAlert : CircleHelp
  return (
    <p className="adm-post-verdict">
      <Icon aria-hidden />
      <strong>
        {t(
          failed
            ? "admin.review.failure.extraction"
            : "admin.review.posts.uncertain",
        )}
      </strong>
      {post.llmLastError ? (
        <span>{t(`admin.posts.error.${post.llmLastError}`)}</span>
      ) : null}
      {post.llmLastAttemptAt ? (
        <span className="adm-muted">
          {t("admin.review.posts.lastTried", {
            time: japanStamp(post.llmLastAttemptAt),
          })}
        </span>
      ) : null}
      {post.llmRetryStopped ? (
        <Badge tone="warning" className="adm-retry-badge">
          {t("admin.posts.retryStopped")}
        </Badge>
      ) : post.llmRetryAt ? (
        <span className="adm-muted">
          {t("admin.posts.retryAt", { time: japanStamp(post.llmRetryAt) })}
        </span>
      ) : null}
      {post.llmReprocessRequested ? (
        <Badge tone="sky">{t("admin.posts.queued")}</Badge>
      ) : null}
    </p>
  )
}

/**
 * One post of the Posts tab as a card whose left edge takes its state's
 * colour (danger for a failure, honey for an uncertain extraction): who and
 * when with the link to the post, the wording beside its images and under it
 * the quote, then the step's verdict and its actions along the bottom. A
 * failed extraction can be dismissed, which stops its retries; a
 * translation can be queued again. A post in a list with bulk actions has a
 * selection box on its left edge.
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
  const tone = translation || post.llmStatus === "failed" ? "danger" : "honey"
  return (
    <article
      className={cn(
        "adm-post-row",
        `adm-tone-${tone}`,
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
      <div className="adm-post-row-main">
        <header className="adm-post-row-head">
          <strong>
            <PostAccount post={post} />
          </strong>
          <time dateTime={post.publishedAt}>
            {japanStamp(post.publishedAt)}
          </time>
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
        </header>
        <div className="adm-post-row-content">
          <div className="adm-post-row-words">
            <p className="adm-post-text">{post.text}</p>
            <PostQuote post={post} onDone={onDone} />
          </div>
          <PostImages post={post} limit={3} />
        </div>
        <footer className="adm-post-row-foot">
          <PostVerdict post={post} step={step} />
          <div className="adm-actions">
            {!translation && post.llmStatus === "failed" ? (
              <Button
                size="sm"
                variant="ghost"
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
            ) : (
              <Button
                size="sm"
                icon={Sparkles}
                title={t("admin.posts.reprocess")}
                busy={busy === "reprocess"}
                disabled={post.llmReprocessRequested}
                onClick={() =>
                  run(
                    "reprocess",
                    () => api.reprocessPost(post.id),
                    "admin.notice.postQueued",
                  )
                }
              >
                {t("admin.review.bulkReprocess")}
              </Button>
            )}
          </div>
        </footer>
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
  const active = tabOf(tab)
  const review = overview?.review
  const counts = {
    schedule: review
      ? review.pendingImages +
        review.suspectedCancellations +
        review.uncertainEvents
      : undefined,
    merges: review?.videoMerges,
    posts: review?.failedPosts,
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
          {active === "schedule" ? (
            <ScheduleAttentionQueue navigate={navigate} />
          ) : null}
          {active === "merges" ? <MergeQueue /> : null}
          {active === "posts" ? <PostsTab navigate={navigate} /> : null}
        </div>
      </Panel>
    </div>
  )
}
