import { useEffect, useState } from "react"
import {
  Clapperboard,
  Globe,
  ImageDown,
  ImageOff,
  Images,
  ListMusic,
  RefreshCw,
  Save,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { japanStamp } from "@/admin/format"
import type {
  VariantGroupStatus,
  VariantStatus,
  VisitorRegionSettings,
  WorksCoverStatus,
  WorksCoverVideo,
  WorksRecentItem,
  WorksRecentReport,
} from "@/admin/types"
import {
  Badge,
  Button,
  Field,
  Loading,
  PageHeader,
  Panel,
  Switch,
  type Tone,
} from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"

/**
 * The mainland China site suggestion: whether the public pages offer the
 * mirror to visitors the proxy marks as mainland China, and the provider and
 * mirror the suggestion names, saved together.
 */
function VisitorRegionPanel({
  settings,
  onChange,
}: {
  settings: VisitorRegionSettings
  onChange: (settings: VisitorRegionSettings) => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [draft, setDraft] = useState(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => setDraft(settings), [settings])
  const dirty =
    draft.enabled !== settings.enabled ||
    draft.providerName !== settings.providerName ||
    draft.providerUrl !== settings.providerUrl ||
    draft.mirrorUrl !== settings.mirrorUrl
  const set = (patch: Partial<VisitorRegionSettings>) =>
    setDraft((current) => ({ ...current, ...patch }))
  const save = async () => {
    setBusy(true)
    const next = await act(
      () => api.updateVisitorRegion(draft),
      "admin.notice.visitorRegionSaved",
    )
    setBusy(false)
    if (next) onChange(next)
  }
  return (
    <Panel icon={Globe} title={t("admin.site.visitorRegion")}>
      <form
        className="adm-form"
        onSubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <Switch
          checked={draft.enabled}
          disabled={busy}
          onChange={(enabled) => set({ enabled })}
          label={t("admin.site.visitorRegionToggle")}
        />
        <div className="adm-form-row">
          <Field label={t("admin.site.visitorRegionProvider")}>
            <input
              value={draft.providerName}
              onChange={(event) => set({ providerName: event.target.value })}
              maxLength={120}
              placeholder={t("admin.site.visitorRegionProviderPlaceholder")}
            />
          </Field>
          <Field label={t("admin.site.visitorRegionProviderUrl")}>
            <input
              type="url"
              value={draft.providerUrl}
              onChange={(event) => set({ providerUrl: event.target.value })}
              placeholder="https://"
            />
          </Field>
        </div>
        <Field label={t("admin.site.visitorRegionMirrorUrl")}>
          <input
            type="url"
            value={draft.mirrorUrl}
            onChange={(event) => set({ mirrorUrl: event.target.value })}
            placeholder="https://"
          />
        </Field>
        <div className="adm-form-actions">
          <Button
            type="submit"
            size="sm"
            variant="primary"
            icon={Save}
            busy={busy}
            disabled={!dirty || busy}
          >
            {t("admin.action.save")}
          </Button>
        </div>
      </form>
    </Panel>
  )
}

/** The groups in the order the server lists them, shown before it answers. */
const variantGroups: VariantGroupStatus["group"][] = [
  "lineblog",
  "works",
  "journey",
  "media",
]

/**
 * One group's ready originals against all of them, with what is left; a
 * group not read yet keeps its place with dashes.
 */
function VariantMeter({
  name,
  group,
}: {
  name: VariantGroupStatus["group"]
  group: VariantGroupStatus | undefined
}) {
  const { t } = useAppSettings()
  const missing = group ? group.originals - group.ready : 0
  const share = group && group.originals > 0 ? group.ready / group.originals : 0
  return (
    <li className={`adm-meter${group ? "" : " is-pending"}`}>
      <span className="adm-meter-label">
        {t(`admin.site.variantsGroup.${name}`)}
      </span>
      <span className="adm-meter-value">
        <strong>{group ? group.ready : "—"}</strong>
        {group ? (
          <small>
            {t("admin.site.variantsOf", { count: group.originals })}
          </small>
        ) : null}
      </span>
      <span className="adm-meter-bar" aria-hidden>
        <i style={{ width: `${share * 100}%` }} />
      </span>
      <span className="adm-meter-label">
        {!group
          ? " "
          : missing > 0
            ? t("admin.site.variantsMissing", { count: missing })
            : t("admin.site.variantsComplete")}
      </span>
    </li>
  )
}

/**
 * The smaller image copies: how complete each group is and a run that makes
 * the missing ones. The panel renders at once and fills in when the status
 * arrives. The page reads the status when it opens and after every run,
 * never on a timer; the server answers its last listing until a run
 * finishes, and Check again lists anew.
 */
function VariantsPanel({
  status,
  loading,
  onChange,
}: {
  status: VariantStatus | undefined
  loading: boolean
  onChange: (status: VariantStatus) => void
}) {
  const { t, locale } = useAppSettings()
  const { api, act, start, overview } = useAdmin()
  const [busy, setBusy] = useState(false)
  const [checking, setChecking] = useState(false)
  const check = async () => {
    setChecking(true)
    const next = await act(() => api.variantStatus(true))
    setChecking(false)
    if (next) onChange(next)
  }
  const active = overview?.activeRun
  const running = active?.kind === "variants"
  const missing = (status?.groups ?? []).reduce(
    (sum, group) => sum + group.originals - group.ready,
    0,
  )
  const make = async () => {
    setBusy(true)
    await start({ kind: "variants" })
    setBusy(false)
  }
  return (
    <Panel
      icon={Images}
      title={t("admin.site.variants")}
      actions={
        <>
          <Button
            size="sm"
            variant="ghost"
            icon={RefreshCw}
            busy={checking || (loading && !status)}
            onClick={() => void check()}
          >
            {t("admin.site.variantsRefresh")}
          </Button>
          <Button
            size="sm"
            variant="primary"
            icon={ImageDown}
            busy={busy || running}
            disabled={missing === 0 || Boolean(active)}
            onClick={() => void make()}
          >
            {running
              ? t("admin.site.variantsRunning")
              : t("admin.site.variantsMake")}
          </Button>
        </>
      }
    >
      <ul className="adm-meters" aria-busy={!status}>
        {variantGroups.map((name) => (
          <VariantMeter
            key={name}
            name={name}
            group={status?.groups.find((group) => group.group === name)}
          />
        ))}
      </ul>
      <p className="adm-field-hint">
        {status
          ? t("admin.site.variantsChecked", {
              time: japanStamp(status.checkedAt),
            })
          : loading
            ? t("admin.loading")
            : null}{" "}
        {t("admin.site.variantsBudget")}
      </p>
    </Panel>
  )
}

/** The counts of the covers card, in the order they are shown. */
const coverCounts = ["covered", "due", "waiting", "uncovered"] as const

const coverTones: Record<string, Tone> = {
  due: "sky",
  waiting: "honey",
  uncovered: "danger",
  gone: "neutral",
}

/** Where an operator opens a video to check it by hand. */
function videoPage(video: WorksCoverVideo) {
  switch (video.platform) {
    case "youtube":
      return `https://www.youtube.com/watch?v=${video.id}`
    case "niconico":
      return `https://www.nicovideo.jp/watch/${video.id}`
    case "bilibili":
      return `https://www.bilibili.com/video/${video.id}`
  }
}

/**
 * The song titles of the packaged uploads, read the first time the list
 * opens; until then an upload is named by its ID.
 */
function useUploadSongs(open: boolean) {
  const [songs, setSongs] = useState<Map<string, string>>()
  useEffect(() => {
    if (!open || songs) return
    let live = true
    import("@/works/uploads.json")
      .then((module) => {
        if (live)
          setSongs(
            new Map(module.default.map((upload) => [upload.id, upload.song])),
          )
      })
      .catch(() => {})
    return () => {
      live = false
    }
  }, [open, songs])
  return songs
}

/**
 * The Works page's upload covers: how many uploads have one, which still
 * lack one and why, and a media step that looks the due ones up and
 * downloads them. The status is read from the snapshot when the page opens
 * and after every change; only the media step reads the platforms.
 */
function WorksCoversPanel({
  status,
  loading,
  onCheck,
}: {
  status: WorksCoverStatus | undefined
  loading: boolean
  onCheck: () => Promise<unknown>
}) {
  const { t } = useAppSettings()
  const { start, overview } = useAdmin()
  const [busy, setBusy] = useState(false)
  const [checking, setChecking] = useState(false)
  const [open, setOpen] = useState(false)
  const songs = useUploadSongs(open)
  const active = overview?.activeRun
  const running =
    active?.kind === "workflow" &&
    active.steps.length === 1 &&
    active.steps[0] === "media"
  const uploads = status?.uploads ?? []
  const check = async () => {
    setChecking(true)
    await onCheck()
    setChecking(false)
  }
  const fill = async () => {
    setBusy(true)
    await start({ kind: "workflow", steps: ["media"] })
    setBusy(false)
  }
  return (
    <Panel
      icon={Clapperboard}
      title={t("admin.site.worksCovers")}
      actions={
        <>
          <Button
            size="sm"
            variant="ghost"
            icon={RefreshCw}
            busy={checking || (loading && !status)}
            onClick={() => void check()}
          >
            {t("admin.site.worksCoversRefresh")}
          </Button>
          <Button
            size="sm"
            variant="primary"
            icon={ImageDown}
            busy={busy || running}
            disabled={
              !status || status.skipped || !status.due || Boolean(active)
            }
            onClick={() => void fill()}
          >
            {running
              ? t("admin.site.worksCoversRunning")
              : t("admin.site.worksCoversFill")}
          </Button>
        </>
      }
    >
      <ul className="adm-meters" aria-busy={!status}>
        {coverCounts.map((name) => {
          const value = status?.[name]
          const share = status?.total ? (value ?? 0) / status.total : 0
          return (
            <li
              key={name}
              className={`adm-meter${status ? "" : " is-pending"}`}
            >
              <span className="adm-meter-label">
                {t(`admin.site.worksCoversCount.${name}`)}
              </span>
              <span className="adm-meter-value">
                <strong>{value ?? "—"}</strong>
                {status ? (
                  <small>
                    {t("admin.site.worksCoversOf", { count: status.total })}
                  </small>
                ) : null}
              </span>
              <span className="adm-meter-bar" aria-hidden>
                <i style={{ width: `${share * 100}%` }} />
              </span>
            </li>
          )
        })}
      </ul>
      {uploads.length ? (
        <details
          className="adm-disclosure"
          onToggle={(event) => setOpen(event.currentTarget.open)}
        >
          <summary>
            {t("admin.site.worksCoversList", { count: uploads.length })}
          </summary>
          <ol className="adm-works-covers">
            {uploads.map((upload) => (
              <li key={upload.id}>
                <div className="adm-chip-row">
                  <time dateTime={upload.date}>{upload.date}</time>
                  <strong>{songs?.get(upload.id) ?? upload.id}</strong>
                  <Badge tone={coverTones[upload.state]}>
                    {t(`admin.site.worksCoversState.${upload.state}`)}
                  </Badge>
                </div>
                <ul>
                  {upload.videos.map((video) => (
                    <li
                      key={`${video.platform}:${video.id}`}
                      className="adm-chip-row"
                    >
                      <span className="adm-muted">
                        {t(
                          `admin.site.worksCoversPlatform.${video.members ? "members" : video.platform}`,
                        )}
                      </span>
                      <a
                        href={videoPage(video)}
                        target="_blank"
                        rel="noreferrer"
                      >
                        <code>{video.id}</code>
                      </a>
                      <Badge tone={coverTones[video.state]}>
                        {t(`admin.site.worksCoversVideo.${video.state}`)}
                      </Badge>
                      {video.code ? <code>{video.code}</code> : null}
                      {video.retryAt ? (
                        <span className="adm-muted">
                          {t("admin.site.worksCoversRetry", {
                            time: japanStamp(video.retryAt),
                          })}
                        </span>
                      ) : null}
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ol>
        </details>
      ) : null}
      <p className="adm-field-hint">
        {status
          ? t("admin.site.worksCoversChecked", {
              time: japanStamp(status.checkedAt),
            })
          : loading
            ? t("admin.loading")
            : null}{" "}
        {status?.skipped
          ? t("admin.site.worksCoversSkipped")
          : t("admin.site.worksCoversBudget")}
      </p>
    </Panel>
  )
}

const recentTones: Record<string, Tone> = {
  auto: "sky",
  operator: "leaf",
  excluded: "neutral",
}

/**
 * The Works page's uploads after the collection date: the videos it lists,
 * by their tag, form and format (auto) or by the operator's decision, and
 * those the operator keeps off. Exclude takes an automatic one off; Follow
 * the rules clears a decision. A video the rules miss is included from its
 * video editor.
 */
function WorksRecentPanel({
  report,
  loading,
  onChange,
}: {
  report: WorksRecentReport | undefined
  loading: boolean
  onChange: () => Promise<unknown>
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [busy, setBusy] = useState("")
  const decide = async (item: WorksRecentItem, work: boolean | null) => {
    setBusy(item.id)
    const result = await act(
      () => api.correctVideo(item.id, { work }),
      "admin.notice.worksDecisionSaved",
    )
    if (result) await onChange()
    setBusy("")
  }
  const items = report?.items ?? []
  return (
    <Panel
      icon={ListMusic}
      title={t("admin.site.worksRecent", {
        date: report?.updatedAt ?? "—",
      })}
    >
      {items.length ? (
        <ol className="adm-works-covers adm-works-recent">
          {items.map((item) => {
            const state = item.origin ?? "excluded"
            return (
              <li key={item.id}>
                <span className="adm-merge-video">
                  <span className="adm-video-thumb">
                    {item.thumbnailUrl ? (
                      <img src={item.thumbnailUrl} alt="" loading="lazy" />
                    ) : (
                      <ImageOff aria-hidden />
                    )}
                  </span>
                  <span className="adm-merge-video-body">
                    <a
                      className="adm-link"
                      href={item.url}
                      target="_blank"
                      rel="noreferrer"
                    >
                      <strong>{item.title}</strong>
                    </a>
                    <small>
                      {[
                        japanStamp(item.scheduledAt || item.publishedAt),
                        t(`admin.site.worksCoversPlatform.${item.source}`),
                        item.tag ?? t("admin.videos.tag.none"),
                        item.kind ?? "—",
                      ].join(" · ")}
                    </small>
                  </span>
                </span>
                <div className="adm-chip-row">
                  <Badge tone={recentTones[state]}>
                    {t(`admin.site.worksRecentOrigin.${state}`)}
                  </Badge>
                  {item.work === null ? (
                    <Button
                      size="sm"
                      variant="ghost"
                      busy={busy === item.id}
                      disabled={Boolean(busy)}
                      onClick={() => void decide(item, false)}
                    >
                      {t("admin.site.worksRecentExclude")}
                    </Button>
                  ) : (
                    <Button
                      size="sm"
                      variant="ghost"
                      busy={busy === item.id}
                      disabled={Boolean(busy)}
                      onClick={() => void decide(item, null)}
                    >
                      {t("admin.site.worksRecentClear")}
                    </Button>
                  )}
                </div>
              </li>
            )
          })}
        </ol>
      ) : (
        <p className="adm-field-hint">
          {report
            ? t("admin.site.worksRecentEmpty")
            : loading
              ? t("admin.loading")
              : null}
        </p>
      )}
      {report && report.total > items.length ? (
        <p className="adm-field-hint">
          {t("admin.site.worksRecentLimit", {
            count: items.length,
            total: report.total,
          })}
        </p>
      ) : null}
      <p className="adm-field-hint">{t("admin.site.worksRecentHint")}</p>
    </Panel>
  )
}

/** The Site page: settings that change what visitors see on the public
 * pages, the image copies the pages are served, and the Works upload
 * covers and recent uploads. */
export function SiteView() {
  const { t } = useAppSettings()
  const { api } = useAdmin()
  const region = useResource(() => api.visitorRegion(), [])
  const variants = useResource(() => api.variantStatus(), [])
  const covers = useResource(() => api.worksCovers(), [])
  const recent = useResource(() => api.worksRecent(), [])
  return (
    <div className="adm-view">
      <PageHeader title={t("admin.site.title")} />
      {!region.data ? (
        <Loading />
      ) : (
        <VisitorRegionPanel
          settings={region.data}
          onChange={(next) => region.setData(next)}
        />
      )}
      <VariantsPanel
        status={variants.data}
        loading={variants.loading}
        onChange={(next) => variants.setData(next)}
      />
      <WorksCoversPanel
        status={covers.data}
        loading={covers.loading}
        onCheck={covers.reload}
      />
      <WorksRecentPanel
        report={recent.data}
        loading={recent.loading}
        onChange={recent.reload}
      />
    </div>
  )
}
