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
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { japanStamp } from "@/admin/format"
import type {
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
import { TranslationCell } from "@/admin/views/translation-parts"

const reasons: XStatusReason[] = [
  "keyword",
  "manual",
  "quoted",
  "reposted",
  "none",
]
const states: XStatusState[] = ["promoted", "demoted", "stored", "post"]

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
  post: "sky",
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
  const promotable =
    status.related && status.state !== "promoted" && status.state !== "post"
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
 * The X status pool: every status of a related account and every status an
 * own account quotes or reposts. Promote and demote are durable locks;
 * related accounts and their keyword rules are set on the Sources page.
 */
export function ExternalPostsView({ navigate }: ViewProps) {
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
    <div className="adm-view">
      <PageHeader
        title={t("admin.external.title")}
        description={t("admin.external.description")}
        actions={
          <Button
            size="sm"
            variant="ghost"
            icon={Settings2}
            onClick={() => navigate("sources", "x")}
          >
            {t("admin.external.configure")}
          </Button>
        }
      />
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
            <ul
              className="adm-post-feed"
              aria-label={t("admin.external.title")}
            >
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
    </div>
  )
}
