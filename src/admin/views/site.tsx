import { useEffect, useState } from "react"
import { Globe, ImageDown, Images, RefreshCw, Save } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { formatDateTime } from "@/admin/format"
import type {
  VariantGroupStatus,
  VariantStatus,
  VisitorRegionSettings,
} from "@/admin/types"
import { Button, Field, Loading, PageHeader, Panel, Switch } from "@/admin/ui"
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

/** One group's ready originals against all of them, with what is left. */
function VariantMeter({ group }: { group: VariantGroupStatus }) {
  const { t } = useAppSettings()
  const missing = group.originals - group.ready
  const share = group.originals > 0 ? group.ready / group.originals : 0
  return (
    <li className="adm-meter">
      <span className="adm-meter-label">
        {t(`admin.site.variantsGroup.${group.group}`)}
      </span>
      <span className="adm-meter-value">
        <strong>{group.ready}</strong>
        <small>{t("admin.site.variantsOf", { count: group.originals })}</small>
      </span>
      <span className="adm-meter-bar" aria-hidden>
        <i style={{ width: `${share * 100}%` }} />
      </span>
      <span className="adm-meter-label">
        {missing > 0
          ? t("admin.site.variantsMissing", { count: missing })
          : t("admin.site.variantsComplete")}
      </span>
    </li>
  )
}

/**
 * The smaller image copies: how complete each group is and a run that makes
 * the missing ones. The page reads the status when it opens and after every
 * run, never on a timer; the server answers its last listing until a run
 * finishes, and Check again lists anew.
 */
function VariantsPanel({
  status,
  onChange,
}: {
  status: VariantStatus
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
  const missing = status.groups.reduce(
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
      description={t("admin.site.variantsDescription")}
      actions={
        <>
          <Button
            size="sm"
            variant="ghost"
            icon={RefreshCw}
            busy={checking}
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
      <ul className="adm-meters">
        {status.groups.map((group) => (
          <VariantMeter key={group.group} group={group} />
        ))}
      </ul>
      <p className="adm-field-hint">
        {t("admin.site.variantsChecked", {
          time: formatDateTime(status.checkedAt, locale),
        })}{" "}
        {t("admin.site.variantsBudget")}
      </p>
    </Panel>
  )
}

/** The Site page: settings that change what visitors see on the public
 * pages, and the image copies the pages are served. */
export function SiteView() {
  const { t } = useAppSettings()
  const { api } = useAdmin()
  const region = useResource(() => api.visitorRegion(), [])
  const variants = useResource(() => api.variantStatus(), [])
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
      {variants.data ? (
        <VariantsPanel
          status={variants.data}
          onChange={(next) => variants.setData(next)}
        />
      ) : variants.loading ? (
        <Loading />
      ) : null}
    </div>
  )
}
