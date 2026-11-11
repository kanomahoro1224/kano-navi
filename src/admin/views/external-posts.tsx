import { useState } from "react"
import {
  ArrowUpRight,
  Eye,
  EyeOff,
  Image as ImageIcon,
  Lock,
  Newspaper,
  Quote,
  Repeat2,
  Search,
  Settings2,
  Undo2,
  UserRound,
  Video,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { japanStamp } from "@/admin/format"
import type {
  AdminBilibiliStatus,
  AdminXStatus,
  XStatusDecision,
  XStatusReason,
  XStatusState,
} from "@/admin/types"
import {
  Badge,
  Button,
  Empty,
  Field,
  FilterReset,
  FilterSelect,
  IconButton,
  Loading,
  Modal,
  PageHeader,
  Pager,
  Panel,
  type Tone,
} from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"
import { stepIcons } from "@/admin/views/run-parts"
import { PlatformTabs } from "@/admin/views/platform-tabs"
import { TranslationCell } from "@/admin/views/translation-parts"
import { cn } from "@/lib/utils"

const reasons: XStatusReason[] = [
  "keyword",
  "manual",
  "quoted",
  "reposted",
  "none",
]
const states: XStatusState[] = ["promoted", "demoted", "stored"]

const reasonTone: Record<XStatusReason, Tone> = {
  keyword: "leaf",
  manual: "berry",
  quoted: "sky",
  reposted: "lavender",
  none: "neutral",
}

const stateTone: Record<XStatusState, Tone> = {
  promoted: "success",
  demoted: "warning",
  stored: "neutral",
}

const reasonLimit = 500

/**
 * Asks for the reason of a promote or demote. The decision is a lock the
 * keyword rules never override.
 */
function DecisionDialog({
  status,
  decision,
  onClose,
  onDone,
}: {
  status: AdminXStatus
  decision: "promote" | "demote"
  onClose: () => void
  onDone: () => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [reason, setReason] = useState("")
  const [busy, setBusy] = useState(false)
  const submit = async () => {
    setBusy(true)
    const result = await act(
      () => api.decideXStatus(status.id, decision, reason),
      `admin.external.notice.${decision}`,
    )
    setBusy(false)
    if (result) {
      onClose()
      onDone()
    }
  }
  return (
    <Modal
      open
      title={t(`admin.external.${decision}.title`)}
      onClose={onClose}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("admin.action.cancel")}
          </Button>
          <Button
            variant="primary"
            busy={busy}
            disabled={!reason.trim()}
            onClick={() => void submit()}
          >
            {t(`admin.external.${decision}`)}
          </Button>
        </>
      }
    >
      <p className="adm-muted">{t(`admin.external.${decision}.hint`)}</p>
      <blockquote className="adm-xstatus-quote">{status.text}</blockquote>
      <Field label={t("admin.external.reason")}>
        <textarea
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          maxLength={reasonLimit}
          rows={3}
          required
        />
      </Field>
    </Modal>
  )
}

function XStatusItem({
  status,
  onDone,
  navigate,
}: {
  status: AdminXStatus
  onDone: () => void
  navigate: ViewProps["navigate"]
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [deciding, setDeciding] = useState<"promote" | "demote" | null>(null)
  const [busy, setBusy] = useState(false)
  const automatic = async () => {
    setBusy(true)
    const decision: XStatusDecision = "automatic"
    const result = await act(
      () => api.decideXStatus(status.id, decision),
      "admin.external.notice.automatic",
    )
    setBusy(false)
    if (result) onDone()
  }
  const promotable = status.related && status.state !== "promoted"
  const demotable = status.state === "promoted"
  return (
    <li
      className={`adm-post adm-xstatus${status.state === "demoted" ? " is-hidden" : ""}`}
    >
      <span className="adm-post-origin">
        <strong>@{status.authorHandle}</strong>
        {status.authorName ? (
          <span className="adm-muted">{status.authorName}</span>
        ) : null}
        {status.publishedAt ? (
          <time dateTime={status.publishedAt}>
            {japanStamp(status.publishedAt)}
          </time>
        ) : null}
      </span>
      <div className="adm-post-body">
        <p className="adm-post-text">{status.text}</p>
        <span className="adm-chip-row">
          {status.images ? (
            <Badge icon={ImageIcon}>
              {t("admin.external.images", { count: status.images })}
            </Badge>
          ) : null}
          {status.quotedBy.map((id) => (
            <Button
              key={id}
              size="sm"
              variant="ghost"
              icon={Quote}
              onClick={() => navigate("posts", `find-${id}`)}
            >
              {t("admin.external.quotedBy")}
            </Button>
          ))}
          {status.repostedBy.map((account) => (
            <Badge key={account} icon={Repeat2}>
              {t("admin.external.repostedBy", { account })}
            </Badge>
          ))}
          {status.state === "promoted" || status.state === "demoted" ? (
            <Button
              size="sm"
              variant="ghost"
              icon={Newspaper}
              onClick={() => navigate("posts", `find-${status.id}`)}
            >
              {t("admin.external.openPost")}
            </Button>
          ) : null}
        </span>
        {status.decision && status.decisionReason ? (
          <p className="adm-xstatus-reason">
            <Lock aria-hidden />
            {status.decisionReason}
            {status.decidedAt ? (
              <span className="adm-muted">
                {" "}
                · {japanStamp(status.decidedAt)}
              </span>
            ) : null}
          </p>
        ) : null}
      </div>
      <div className="adm-post-translation">
        {status.translation ? (
          <TranslationCell
            owner="x_status"
            id={status.id}
            original={status.text}
            translation={status.translation}
            onChanged={onDone}
          />
        ) : status.state !== "stored" ? (
          <span className="adm-muted">
            {t("admin.external.postTranslation")}
          </span>
        ) : null}
      </div>
      <span className="adm-chip-row adm-post-state">
        <Badge tone={stateTone[status.state]}>
          {t(`admin.external.state.${status.state}`)}
        </Badge>
        {status.decision ? (
          <Badge tone="berry" icon={Lock}>
            {t(`admin.external.locked.${status.decision}`)}
          </Badge>
        ) : null}
        {status.reasons.map((reason) => (
          <Badge key={reason} tone={reasonTone[reason]}>
            {t(`admin.external.reason.${reason}`)}
          </Badge>
        ))}
      </span>
      <span className="adm-post-controls">
        {promotable ? (
          <IconButton
            icon={Eye}
            label={t("admin.external.promote")}
            onClick={() => setDeciding("promote")}
          />
        ) : null}
        {demotable ? (
          <IconButton
            icon={EyeOff}
            label={t("admin.external.demote")}
            onClick={() => setDeciding("demote")}
          />
        ) : null}
        {status.decision ? (
          <IconButton
            icon={Undo2}
            busy={busy}
            label={t("admin.external.automatic")}
            onClick={() => void automatic()}
          />
        ) : null}
        <a
          className="adm-icon-button"
          href={status.url}
          target="_blank"
          rel="noreferrer"
          aria-label={t("admin.external.open")}
          title={t("admin.external.open")}
        >
          <ArrowUpRight aria-hidden />
        </a>
      </span>
      {deciding ? (
        <DecisionDialog
          status={status}
          decision={deciding}
          onClose={() => setDeciding(null)}
          onDone={onDone}
        />
      ) : null}
    </li>
  )
}

const blankFilters = { search: "", author: "", reason: "", state: "" }

/**
 * The X status pool: every status of a related account and every other
 * account's status an own account quotes or reposts; the own accounts'
 * statuses belong to Posts. Promote and demote are durable locks;
 * related accounts and their keyword rules are set on the Sources page.
 */
function XStatusPool({ navigate }: Pick<ViewProps, "navigate">) {
  const { t } = useAppSettings()
  const { api } = useAdmin()
  const [filters, setFilters] = useState(blankFilters)
  const [search, setSearch] = useState("")
  const [page, setPage] = useState(1)
  const related = useResource(() => api.relatedAccounts(), [])
  const statuses = useResource(
    () => api.xStatuses({ ...filters, page, pageSize: 30 }),
    [filters, page],
  )
  const update = (patch: Partial<typeof filters>) => {
    setFilters((current) => ({ ...current, ...patch }))
    setPage(1)
  }
  const active = (
    Object.keys(blankFilters) as (keyof typeof blankFilters)[]
  ).filter((key) => filters[key] !== blankFilters[key]).length
  const any = { value: "", label: t("admin.filter.any") }
  return (
    <Panel>
      <form
        className="adm-filters"
        onSubmit={(event) => {
          event.preventDefault()
          update({ search })
        }}
      >
        <label className="adm-search">
          <Search aria-hidden />
          <input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onBlur={() => search !== filters.search && update({ search })}
            placeholder={t("admin.external.search")}
            aria-label={t("admin.external.search")}
          />
        </label>
        <FilterSelect
          label={t("admin.external.author")}
          value={filters.author}
          onChange={(author) => update({ author })}
          options={[
            any,
            ...(related.data?.accounts || []).map(({ handle }) => ({
              value: handle,
              label: `@${handle}`,
            })),
          ]}
        />
        <FilterSelect
          label={t("admin.external.reasonFilter")}
          value={filters.reason}
          onChange={(reason) => update({ reason })}
          options={[
            any,
            ...reasons.map((reason) => ({
              value: reason,
              label: t(`admin.external.reason.${reason}`),
            })),
          ]}
        />
        <FilterSelect
          label={t("admin.external.stateFilter")}
          value={filters.state}
          onChange={(state) => update({ state })}
          options={[
            any,
            ...states.map((state) => ({
              value: state,
              label: t(`admin.external.state.${state}`),
            })),
          ]}
        />
        <FilterReset
          count={active}
          onReset={() => {
            setSearch("")
            update(blankFilters)
          }}
        />
      </form>
      <div className="adm-list-bar">
        <span className="adm-muted">
          {statuses.data
            ? t("admin.pager.total", { total: statuses.data.total })
            : ""}
        </span>
      </div>
      {!statuses.data ? (
        <Loading />
      ) : statuses.data.items.length ? (
        <>
          <ul className="adm-post-feed" aria-label={t("admin.external.title")}>
            {statuses.data.items.map((status) => (
              <XStatusItem
                key={status.id}
                status={status}
                onDone={() => void statuses.reload()}
                navigate={navigate}
              />
            ))}
          </ul>
          <Pager
            page={statuses.data.page}
            totalPages={statuses.data.totalPages}
            total={statuses.data.total}
            onPage={setPage}
          />
        </>
      ) : (
        <Empty icon={Newspaper} title={t("admin.external.empty")}>
          {t("admin.external.emptyHint")}
        </Empty>
      )}
    </Panel>
  )
}

const shownTone: Record<AdminBilibiliStatus["shown"], Tone> = {
  public: "success",
  hidden: "warning",
  gone: "neutral",
}

const BilibiliMark = stepIcons.bilibili

/** Why a forward does not show the dynamic, as its chip's title. */
function forwardNote(
  forward: AdminBilibiliStatus["forwardedBy"][number],
  t: ReturnType<typeof useAppSettings>["t"],
) {
  if (forward.hidden) return t("admin.external.forward.hidden")
  if (forward.quoteHidden) return t("admin.external.forward.quoteHidden")
  if (!forward.quoteAvailable) return t("admin.external.forward.unavailable")
  return undefined
}

/**
 * A pooled Bilibili dynamic. It is never promoted: its forward's quote card
 * shows it, so the console edits only its translations.
 */
function BilibiliStatusItem({
  status,
  onDone,
  navigate,
}: {
  status: AdminBilibiliStatus
  onDone: () => void
  navigate: ViewProps["navigate"]
}) {
  const { t } = useAppSettings()
  return (
    <li
      className={cn(
        "adm-post adm-xstatus",
        status.shown !== "public" && "is-hidden",
      )}
    >
      <span className="adm-post-origin">
        <strong className="adm-post-mark">
          <BilibiliMark aria-hidden />
          {status.authorName || status.authorMid}
        </strong>
        {status.authorName ? (
          <span className="adm-muted">{status.authorMid}</span>
        ) : null}
        {status.publishedAt ? (
          <time dateTime={status.publishedAt}>
            {japanStamp(status.publishedAt)}
          </time>
        ) : null}
      </span>
      <div className="adm-post-body">
        <p className="adm-post-text">{status.text}</p>
        <span className="adm-chip-row">
          {status.images ? (
            <Badge icon={ImageIcon}>
              {t("admin.external.images", { count: status.images })}
            </Badge>
          ) : null}
          {status.forwardedBy.map((forward) => {
            const note = forwardNote(forward, t)
            return (
              <Button
                key={forward.postId}
                size="sm"
                variant="ghost"
                icon={note ? EyeOff : Repeat2}
                title={note}
                onClick={() => navigate("posts", `find-${forward.postId}`)}
              >
                {t("admin.external.forwardedBy")}
              </Button>
            )
          })}
        </span>
      </div>
      <div className="adm-post-translation">
        {status.translation ? (
          <TranslationCell
            owner="bilibili_status"
            id={status.id}
            original={status.text}
            translation={status.translation}
            onChanged={onDone}
          />
        ) : null}
      </div>
      <span className="adm-chip-row adm-post-state">
        <Badge tone={shownTone[status.shown]}>
          {t(`admin.external.shown.${status.shown}`)}
        </Badge>
        {status.kind === "video" ? (
          <Badge tone="sky" icon={Video}>
            {t("admin.external.kind.video")}
          </Badge>
        ) : null}
        {status.reasons.map((reason) => (
          <Badge key={reason} tone="lavender">
            {t(`admin.external.reason.${reason}`)}
          </Badge>
        ))}
      </span>
      <span className="adm-post-controls">
        <a
          className="adm-icon-button"
          href={status.url}
          target="_blank"
          rel="noreferrer"
          aria-label={t("admin.external.openBilibili")}
          title={t("admin.external.openBilibili")}
        >
          <ArrowUpRight aria-hidden />
        </a>
      </span>
    </li>
  )
}

const blankBilibiliFilters = { search: "", author: "", kind: "" }

/**
 * The Bilibili status pool: the dynamics own posts forward, apart from own
 * dynamics, which are stored posts.
 */
function BilibiliStatusPool({ navigate }: Pick<ViewProps, "navigate">) {
  const { t } = useAppSettings()
  const { api } = useAdmin()
  const [filters, setFilters] = useState(blankBilibiliFilters)
  const [search, setSearch] = useState("")
  const [author, setAuthor] = useState("")
  const [page, setPage] = useState(1)
  const statuses = useResource(
    () => api.bilibiliStatuses({ ...filters, page, pageSize: 30 }),
    [filters, page],
  )
  const update = (patch: Partial<typeof filters>) => {
    setFilters((current) => ({ ...current, ...patch }))
    setPage(1)
  }
  const active = (
    Object.keys(blankBilibiliFilters) as (keyof typeof blankBilibiliFilters)[]
  ).filter((key) => filters[key] !== blankBilibiliFilters[key]).length
  return (
    <Panel>
      <form
        className="adm-filters"
        onSubmit={(event) => {
          event.preventDefault()
          update({ search, author })
        }}
      >
        <label className="adm-search">
          <Search aria-hidden />
          <input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onBlur={() => search !== filters.search && update({ search })}
            placeholder={t("admin.external.search")}
            aria-label={t("admin.external.search")}
          />
        </label>
        <label className="adm-search">
          <UserRound aria-hidden />
          <input
            type="search"
            value={author}
            onChange={(e) => setAuthor(e.target.value)}
            onBlur={() => author !== filters.author && update({ author })}
            placeholder={t("admin.external.authorSearch")}
            aria-label={t("admin.external.authorSearch")}
          />
        </label>
        <FilterSelect
          label={t("admin.external.kindFilter")}
          value={filters.kind}
          onChange={(kind) => update({ kind })}
          options={[
            { value: "", label: t("admin.filter.any") },
            { value: "dynamic", label: t("admin.external.kind.dynamic") },
            { value: "video", label: t("admin.external.kind.video") },
          ]}
        />
        <FilterReset
          count={active}
          onReset={() => {
            setSearch("")
            setAuthor("")
            update(blankBilibiliFilters)
          }}
        />
      </form>
      <div className="adm-list-bar">
        <span className="adm-muted">
          {statuses.data
            ? t("admin.pager.total", { total: statuses.data.total })
            : ""}
        </span>
      </div>
      {!statuses.data ? (
        <Loading />
      ) : statuses.data.items.length ? (
        <>
          <ul
            className="adm-post-feed"
            aria-label={t("admin.platform.bilibili")}
          >
            {statuses.data.items.map((status) => (
              <BilibiliStatusItem
                key={status.id}
                status={status}
                onDone={() => void statuses.reload()}
                navigate={navigate}
              />
            ))}
          </ul>
          <Pager
            page={statuses.data.page}
            totalPages={statuses.data.totalPages}
            total={statuses.data.total}
            onPage={setPage}
          />
        </>
      ) : (
        <Empty icon={Repeat2} title={t("admin.external.bilibiliEmpty")}>
          {t("admin.external.bilibiliEmptyHint")}
        </Empty>
      )}
    </Panel>
  )
}

const pools = ["x", "bilibili"] as const

/**
 * External posts: a tab per status pool. The X pool takes promote and
 * demote decisions; the Bilibili pool is read-only apart from translations.
 * The selected pool is the view's tab in the URL.
 */
export function ExternalPostsView({ tab, navigate }: ViewProps) {
  const { t } = useAppSettings()
  const pool = tab === "bilibili" ? "bilibili" : "x"
  return (
    <div className="adm-view">
      <PageHeader
        title={t("admin.external.title")}
        actions={
          pool === "x" ? (
            <Button
              size="sm"
              variant="ghost"
              icon={Settings2}
              onClick={() => navigate("sources", "x")}
            >
              {t("admin.external.configure")}
            </Button>
          ) : undefined
        }
      />
      <PlatformTabs
        platforms={pools}
        value={pool}
        onSelect={(id) => navigate("external", id)}
      >
        {pool === "x" ? (
          <XStatusPool navigate={navigate} />
        ) : (
          <BilibiliStatusPool navigate={navigate} />
        )}
      </PlatformTabs>
    </div>
  )
}
