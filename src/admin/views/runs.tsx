import {
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type SubmitEvent,
} from "react"
import {
  Archive,
  ListChecks,
  RefreshCw,
  Save,
  Sparkles,
  Workflow,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { formatRelative } from "@/admin/format"
import {
  Button,
  Empty,
  Field,
  Loading,
  PageHeader,
  Pager,
  Panel,
} from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"
import { RunLine, runTitle } from "@/admin/views/run-parts"
import { cn } from "@/lib/utils"
import { RunDetail, StepRunDialog } from "@/admin/views/run-detail"

// Wide enough for the history list and the open run side by side.
const splitQuery = "(min-width: 1200px)"

function useSplit() {
  return useSyncExternalStore(
    (onChange) => {
      const media = window.matchMedia(splitQuery)
      media.addEventListener("change", onChange)
      return () => media.removeEventListener("change", onChange)
    },
    () => window.matchMedia(splitQuery).matches,
    () => false,
  )
}

/** The run history; `open` is the run named in the address (#runs/<id>). */
function HistoryPanel({
  open,
  onOpen,
}: {
  open: string
  onOpen: (id: string) => void
}) {
  const { t, locale } = useAppSettings()
  const { api, overview } = useAdmin()
  const split = useSplit()
  const [page, setPage] = useState(1)
  const activeId = overview?.activeRun?.id
  const runs = useResource(() => api.runs(page), [page, activeId])
  const latest = overview?.recentRuns?.[0]
  // A linked run that is not on this page, such as one newer runs pushed
  // off it, is read on its own and listed first; a deleted one is ignored.
  const missing = Boolean(
    open && runs.data && !runs.data.items.some((run) => run.id === open),
  )
  const linked = useResource(
    () =>
      missing
        ? api.run(open).then(
            (found) => found.run,
            () => null,
          )
        : Promise.resolve(null),
    [open, missing],
  )
  const listed = runs.data?.items || []
  // The overview poll carries fresher progress for the active run.
  const items = (
    missing && linked.data?.id === open ? [linked.data, ...listed] : listed
  ).map((run) => (run.id === activeId && overview?.activeRun) || run)
  // A run opened from elsewhere, such as the overview, scrolls into view
  // once it is listed; one chosen here stays where it was clicked.
  const clicked = useRef("")
  const scrolled = useRef("")
  const shown = items.some((run) => run.id === open)
  useEffect(() => {
    if (
      !open ||
      !shown ||
      open === clicked.current ||
      open === scrolled.current
    )
      return
    scrolled.current = open
    document.getElementById(`run-${open}`)?.scrollIntoView({ block: "start" })
  }, [open, shown])
  const choose = (id: string) => {
    clicked.current = id
    onOpen(id)
  }
  // Side by side, the newest run is open until another is chosen.
  const selected = split
    ? items.find((run) => run.id === open) || items[0]
    : items.find((run) => run.id === open)
  return (
    <Panel
      icon={Workflow}
      title={t("admin.runs.history")}
      actions={
        latest?.finishedAt ? (
          <span className="adm-muted">
            {t("admin.runs.lastFinished", {
              time: formatRelative(latest.finishedAt, locale),
            })}
          </span>
        ) : null
      }
    >
      {!runs.data ? (
        <Loading />
      ) : items.length ? (
        <div className={cn("adm-runs", split && "is-split")}>
          <div className="adm-runs-list">
            <div className="adm-run-list">
              {items.map((run) => {
                const isOpen = selected?.id === run.id
                return (
                  <div
                    key={run.id}
                    id={`run-${run.id}`}
                    className={cn("adm-run-item", isOpen && "is-open")}
                  >
                    <RunLine
                      run={run}
                      selected={isOpen}
                      onOpen={() => choose(!split && isOpen ? "" : run.id)}
                    />
                    {!split && isOpen ? (
                      <RunDetail
                        run={run}
                        onChanged={() => void runs.reload()}
                      />
                    ) : null}
                  </div>
                )
              })}
            </div>
            <Pager
              page={runs.data.page}
              totalPages={runs.data.totalPages}
              total={runs.data.total}
              onPage={setPage}
            />
          </div>
          {split && selected ? (
            <aside className="adm-runs-pane" aria-label={runTitle(selected, t)}>
              <RunDetail
                key={selected.id}
                run={selected}
                withHeader
                onChanged={() => void runs.reload()}
              />
            </aside>
          ) : null}
        </div>
      ) : (
        <Empty title={t("admin.overview.noRunsYet")}>
          {t("admin.overview.noRunsHint")}
        </Empty>
      )}
    </Panel>
  )
}

/** How long finished runs stay in the history before the daily cleanup. */
function RetentionPanel() {
  const { t, locale } = useAppSettings()
  const { api, act } = useAdmin()
  const retention = useResource(() => api.runRetention(), [])
  const [draft, setDraft] = useState<{ succeeded: number; failed: number }>()
  const [busy, setBusy] = useState(false)
  const data = retention.data
  useEffect(() => {
    if (data)
      setDraft({ succeeded: data.succeededDays, failed: data.failedDays })
  }, [data])
  const save = async (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!draft) return
    setBusy(true)
    const next = await act(
      () =>
        api.updateRunRetention({
          succeededDays: draft.succeeded,
          failedDays: draft.failed,
        }),
      "admin.notice.retentionSaved",
    )
    setBusy(false)
    if (next) retention.setData(next)
  }
  const days = (value: number) =>
    value === 0
      ? t("admin.retention.forever")
      : t("admin.retention.days", { count: value })
  return (
    <Panel icon={Archive} title={t("admin.retention.title")}>
      {!data || !draft ? (
        <Loading />
      ) : (
        <form className="adm-form adm-retention" onSubmit={save}>
          <div className="adm-retention-fields">
            <Field
              label={t("admin.retention.succeeded")}
              hint={days(draft.succeeded)}
            >
              <input
                type="number"
                min={0}
                max={data.maxDays}
                step={1}
                value={draft.succeeded}
                onChange={(e) =>
                  setDraft({ ...draft, succeeded: Number(e.target.value) })
                }
                required
              />
            </Field>
            <Field
              label={t("admin.retention.failed")}
              hint={days(draft.failed)}
            >
              <input
                type="number"
                min={0}
                max={data.maxDays}
                step={1}
                value={draft.failed}
                onChange={(e) =>
                  setDraft({ ...draft, failed: Number(e.target.value) })
                }
                required
              />
            </Field>
          </div>
          <div className="adm-retention-foot">
            <p className="adm-muted">
              {!data.pruner.available
                ? t("admin.retention.unavailable")
                : data.pruner.lastPrunedAt
                  ? t(
                      data.pruner.lastFailed
                        ? "admin.retention.lastFailed"
                        : "admin.retention.last",
                      {
                        time: formatRelative(data.pruner.lastPrunedAt, locale),
                        count: data.pruner.lastDeleted ?? 0,
                      },
                    )
                  : t("admin.retention.pending")}
            </p>
            <Button
              type="submit"
              size="sm"
              icon={Save}
              busy={busy}
              disabled={
                busy ||
                (draft.succeeded === data.succeededDays &&
                  draft.failed === data.failedDays)
              }
            >
              {t("admin.action.save")}
            </Button>
          </div>
        </form>
      )}
    </Panel>
  )
}

export function RunsView({ tab, navigate }: ViewProps) {
  const { t } = useAppSettings()
  const { start, overview } = useAdmin()
  const [picking, setPicking] = useState(false)
  const busy = Boolean(overview?.activeRun)
  return (
    <div className="adm-view">
      <PageHeader
        title={t("admin.runs.title")}
        actions={
          <>
            <Button icon={ListChecks} onClick={() => setPicking(true)}>
              {t("admin.runs.stepRun")}
            </Button>
            <Button
              icon={Sparkles}
              disabled={busy}
              onClick={() => start({ kind: "scan" })}
            >
              {t("admin.action.scan")}
            </Button>
            <Button
              variant="primary"
              icon={RefreshCw}
              disabled={busy}
              onClick={() => start({ kind: "sync" })}
            >
              {t("admin.action.syncAll")}
            </Button>
          </>
        }
      />
      <HistoryPanel open={tab} onOpen={(id) => navigate("runs", id)} />
      <RetentionPanel />
      {picking ? <StepRunDialog onClose={() => setPicking(false)} /> : null}
    </div>
  )
}
