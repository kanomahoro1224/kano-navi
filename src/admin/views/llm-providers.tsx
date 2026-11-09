import { useEffect, useMemo, useRef, useState } from "react"
import {
  KeyRound,
  ListPlus,
  Pencil,
  Plug,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  TriangleAlert,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { formatRelative } from "@/admin/format"
import { modelTags } from "@/admin/llm"
import type {
  DiscoveredModel,
  LlmCatalog,
  Model,
  Provider,
  QueueRoute,
  RouteTarget,
} from "@/admin/types"
import {
  Badge,
  Button,
  Callout,
  ConfirmButton,
  ConfirmIconButton,
  Empty,
  Field,
  IconButton,
  StatusBadge,
  Switch,
  type Tone,
} from "@/admin/ui"
import { useAdmin } from "@/admin/use-admin-data"
import { cn } from "@/lib/utils"
import {
  queueOf,
  sameTarget,
  routeOrder,
  queueRoutes,
  routeIcons,
  TagChips,
  ProviderMark,
} from "@/admin/views/llm-parts"

/** A provider's health in one tone: off, missing its key, failing or fine. */
function providerState(provider: Provider): { tone: Tone; key: string } {
  if (!provider.enabled) return { tone: "neutral", key: "disabled" }
  if (!provider.apiKeyConfigured) return { tone: "danger", key: "keyMissing" }
  if (provider.lastStatus && provider.lastStatus !== "success")
    return { tone: "danger", key: "failing" }
  if (provider.lastStatus === "success") return { tone: "success", key: "ok" }
  return { tone: "neutral", key: "unchecked" }
}

/** One catalog model: its tags, whether it is on, and which queues hold it. */
function ModelRow({
  provider,
  model,
  catalog,
  onChanged,
  onQueue,
}: {
  provider: Provider
  model: Model
  catalog: LlmCatalog
  onChanged: () => void
  onQueue: (route: QueueRoute, targets: RouteTarget[]) => Promise<boolean>
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [busy, setBusy] = useState("")
  const run = async (
    key: string,
    work: () => Promise<unknown>,
    notice?: string,
  ) => {
    setBusy(key)
    const result = await act(work, notice)
    setBusy("")
    if (result !== undefined) onChanged()
  }
  const queued = (route: QueueRoute) =>
    queueOf(catalog, route).some((target) =>
      sameTarget(target, provider.id, model.id),
    )
  const toggleQueue = (route: QueueRoute) => {
    const targets = queueOf(catalog, route)
    void onQueue(
      route,
      queued(route)
        ? targets.filter((target) => !sameTarget(target, provider.id, model.id))
        : [...targets, { providerId: provider.id, modelId: model.id }],
    )
  }
  return (
    <li className={cn("adm-model-row", !model.enabled && "is-disabled")}>
      <span className="adm-model-name">
        <strong>{model.name || model.id}</strong>
        <small>
          {model.name && model.name !== model.id ? `${model.id} · ` : ""}
          {t(`admin.provider.origin.${model.origin}`)}
        </small>
      </span>
      <span className="adm-tag-toggles" aria-label={t("admin.provider.tags")}>
        {modelTags.map((tag) => {
          const on = model.tags.includes(tag.id)
          return (
            <button
              key={tag.id}
              type="button"
              aria-pressed={on}
              disabled={busy !== ""}
              className={cn(
                "adm-tag-toggle",
                `adm-tone-${tag.tone}`,
                on && "is-on",
              )}
              title={t(`admin.tag.${tag.id}`)}
              onClick={() =>
                run(`tag:${tag.id}`, () =>
                  api.updateModel(provider.id, {
                    id: model.id,
                    tags: on
                      ? model.tags.filter((item) => item !== tag.id)
                      : [...model.tags, tag.id],
                  }),
                )
              }
            >
              <tag.icon aria-hidden />
              <span>{t(`admin.tag.${tag.id}`)}</span>
            </button>
          )
        })}
      </span>
      <span className="adm-model-queues">
        {routeOrder.map((route) => {
          const { needs } = queueRoutes[route]
          const Icon = routeIcons[route]
          return (
            <button
              key={route}
              type="button"
              className="adm-queue-pick"
              aria-pressed={queued(route)}
              disabled={
                !model.enabled ||
                (!!needs &&
                  !model.capabilities.includes(needs) &&
                  !queued(route))
              }
              title={t(`admin.provider.queueHint.${route}`)}
              onClick={() => toggleQueue(route)}
            >
              <Icon aria-hidden />
              <span>{t(`admin.provider.queueName.${route}`)}</span>
            </button>
          )
        })}
      </span>
      <span className="adm-row-actions">
        <Switch
          compact
          checked={model.enabled}
          disabled={busy !== ""}
          onChange={(enabled) =>
            run("enabled", () =>
              api.updateModel(provider.id, { id: model.id, enabled }),
            )
          }
          label={t("admin.provider.modelEnabled")}
        />
        <IconButton
          icon={Plug}
          label={t("admin.provider.testModel")}
          busy={busy === "test"}
          disabled={!provider.apiKeyConfigured || busy !== ""}
          onClick={() =>
            run(
              "test",
              () => api.testProvider(provider.id, model.id),
              "admin.notice.providerOk",
            )
          }
        />
        <ConfirmIconButton
          icon={Trash2}
          label={t("admin.provider.removeModel")}
          busy={busy === "rm"}
          disabled={busy !== ""}
          onConfirm={() =>
            run(
              "rm",
              () => api.removeModels(provider.id, [model.id]),
              "admin.notice.modelsRemoved",
            )
          }
        />
      </span>
    </li>
  )
}

/** The provider's own model list, fetched on demand, and adding by ID. */
function AddModels({
  provider,
  discoverNow,
  onChanged,
}: {
  provider: Provider
  discoverNow: boolean
  onChanged: () => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [remote, setRemote] = useState<DiscoveredModel[] | null>(null)
  const [picked, setPicked] = useState<string[]>([])
  const [filter, setFilter] = useState("")
  const [manual, setManual] = useState("")
  const [busy, setBusy] = useState("")
  const discover = async () => {
    setBusy("discover")
    const result = await act(() => api.discoverModels(provider.id))
    setBusy("")
    if (!result) return
    setRemote(result.models)
    setPicked([])
    onChanged()
  }
  // A provider just created with a key lists its models straight away.
  const started = useRef(false)
  useEffect(() => {
    if (!discoverNow || started.current || !provider.apiKeyConfigured) return
    started.current = true
    void discover()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [discoverNow, provider.apiKeyConfigured])
  const shown = useMemo(
    () =>
      (remote || []).filter(
        (model) =>
          !model.added &&
          model.id.toLowerCase().includes(filter.trim().toLowerCase()),
      ),
    [remote, filter],
  )
  const add = async () => {
    const chosen = (remote || []).filter((model) => picked.includes(model.id))
    setBusy("add")
    const result = await act(
      () =>
        api.addModels(
          provider.id,
          chosen.map((model) => ({
            id: model.id,
            name: model.name,
            ownedBy: model.ownedBy,
            tags: model.suggestedTags,
          })),
          "remote",
        ),
      "admin.notice.modelsAdded",
    )
    setBusy("")
    if (!result) return
    setRemote((current) =>
      (current || []).map((model) =>
        picked.includes(model.id) ? { ...model, added: true } : model,
      ),
    )
    setPicked([])
    onChanged()
  }
  return (
    <section className="adm-add-models">
      <header className="adm-section-header">
        <h4>{t("admin.provider.addModels")}</h4>
        <Button
          size="sm"
          icon={remote ? RefreshCw : Search}
          busy={busy === "discover"}
          disabled={!provider.apiKeyConfigured || busy !== ""}
          onClick={discover}
        >
          {t(remote ? "admin.provider.refetch" : "admin.provider.discover")}
        </Button>
      </header>
      {!provider.apiKeyConfigured ? (
        <p className="adm-muted">{t("admin.provider.discoverNeedsKey")}</p>
      ) : null}
      {remote ? (
        <div className="adm-discovered">
          <span className="adm-input-icon">
            <Search aria-hidden />
            <input
              type="search"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder={t("admin.provider.filterRemote")}
              aria-label={t("admin.provider.filterRemote")}
            />
          </span>
          {shown.length ? (
            <ul className="adm-discovered-list">
              {shown.slice(0, 200).map((model) => (
                <li
                  key={model.id}
                  className={cn(picked.includes(model.id) && "is-picked")}
                >
                  <label className="adm-check">
                    <input
                      type="checkbox"
                      checked={picked.includes(model.id)}
                      onChange={(e) =>
                        setPicked((current) =>
                          e.target.checked
                            ? [...current, model.id]
                            : current.filter((id) => id !== model.id),
                        )
                      }
                    />
                    <span className="adm-model-name">
                      <strong>{model.id}</strong>
                      {model.ownedBy ? <small>{model.ownedBy}</small> : null}
                    </span>
                  </label>
                  <TagChips tags={model.suggestedTags} />
                </li>
              ))}
            </ul>
          ) : (
            <p className="adm-muted">
              {filter
                ? t("admin.provider.noMatch")
                : t("admin.provider.allAdded")}
            </p>
          )}
          <div className="adm-form-actions">
            <span className="adm-muted">
              {t("admin.provider.listed", {
                count: remote.length,
                added: remote.filter((model) => model.added).length,
              })}
            </span>
            <Button
              size="sm"
              variant="primary"
              icon={ListPlus}
              disabled={!picked.length}
              busy={busy === "add"}
              onClick={add}
            >
              {t("admin.provider.addSelected", { count: picked.length })}
            </Button>
          </div>
        </div>
      ) : null}
      <form
        className="adm-inline-form"
        onSubmit={async (event) => {
          event.preventDefault()
          setBusy("manual")
          const result = await act(
            () => api.addModels(provider.id, [{ id: manual.trim() }]),
            "admin.notice.modelsAdded",
          )
          setBusy("")
          if (result) {
            setManual("")
            onChanged()
          }
        }}
      >
        <Field label={t("admin.provider.addManual")}>
          <input
            value={manual}
            onChange={(e) => setManual(e.target.value)}
            maxLength={128}
            placeholder={t("admin.provider.addManualHint")}
            required
          />
        </Field>
        <Button type="submit" size="sm" icon={Plus} busy={busy === "manual"}>
          {t("admin.provider.add")}
        </Button>
      </form>
    </section>
  )
}

export function ProviderDetail({
  provider,
  catalog,
  discoverNow,
  onEdit,
  onChanged,
  onDeleted,
  onQueue,
}: {
  provider: Provider
  catalog: LlmCatalog
  discoverNow: boolean
  onEdit: () => void
  onChanged: () => void
  onDeleted: () => void
  onQueue: (route: QueueRoute, targets: RouteTarget[]) => Promise<boolean>
}) {
  const { t, locale } = useAppSettings()
  const { api, act } = useAdmin()
  const [testing, setTesting] = useState(false)
  const models = catalog.models.filter(
    (model) => model.providerId === provider.id,
  )
  const state = providerState(provider)
  return (
    <div className="adm-provider-detail">
      <header className="adm-provider-head">
        <ProviderMark provider={provider} />
        <div>
          <h3>{provider.name}</h3>
          <small>{provider.baseUrl}</small>
        </div>
        <div className="adm-actions">
          <Button
            size="sm"
            icon={Plug}
            busy={testing}
            disabled={!provider.apiKeyConfigured}
            onClick={async () => {
              setTesting(true)
              await act(
                () => api.testProvider(provider.id),
                "admin.notice.providerOk",
              )
              setTesting(false)
              // The outcome is recorded on the provider either way.
              onChanged()
            }}
          >
            {t("admin.provider.test")}
          </Button>
          <Button size="sm" icon={Pencil} onClick={onEdit}>
            {t("admin.provider.editShort")}
          </Button>
          <ConfirmButton
            size="sm"
            variant="ghost"
            icon={Trash2}
            aria-label={t("admin.provider.delete")}
            onConfirm={async () => {
              const result = await act(
                () => api.deleteProvider(provider.id),
                "admin.notice.providerDeleted",
              )
              if (result !== undefined) onDeleted()
            }}
          />
        </div>
      </header>
      <div className="adm-chip-row">
        <Badge tone={state.tone}>
          {t(`admin.provider.state.${state.key}`)}
        </Badge>
        <Badge
          tone={provider.apiKeyConfigured ? "leaf" : "danger"}
          icon={KeyRound}
        >
          {t(
            provider.apiKeyConfigured
              ? "admin.provider.keySet"
              : "admin.provider.keyMissing",
          )}
        </Badge>
        {provider.lastCheckedAt ? (
          <span className="adm-muted adm-provider-checked">
            {t("admin.provider.checked", {
              time: formatRelative(provider.lastCheckedAt, locale),
            })}
            {provider.lastStatus ? (
              <StatusBadge
                status={
                  provider.lastStatus === "success" ? "completed" : "failed"
                }
              />
            ) : null}
          </span>
        ) : null}
      </div>
      {provider.lastError ? (
        <Callout
          tone="danger"
          icon={TriangleAlert}
          title={t("admin.provider.lastError")}
        >
          <p className="adm-error-text">{provider.lastError}</p>
        </Callout>
      ) : null}
      <section className="adm-provider-models">
        <header className="adm-section-header">
          <h4>{t("admin.provider.catalog", { count: models.length })}</h4>
        </header>
        {models.length ? (
          <ul className="adm-model-rows">
            {models.map((model) => (
              <ModelRow
                key={model.id}
                provider={provider}
                model={model}
                catalog={catalog}
                onChanged={onChanged}
                onQueue={onQueue}
              />
            ))}
          </ul>
        ) : (
          <Empty title={t("admin.provider.noModels")}>
            {t("admin.provider.noModelsHint")}
          </Empty>
        )}
      </section>
      <AddModels
        key={provider.id}
        provider={provider}
        discoverNow={discoverNow}
        onChanged={onChanged}
      />
    </div>
  )
}

export function ProviderList({
  catalog,
  selected,
  onSelect,
}: {
  catalog: LlmCatalog
  selected: string
  onSelect: (id: string) => void
}) {
  const { t } = useAppSettings()
  return (
    <ul className="adm-provider-list">
      {catalog.providers.map((provider) => {
        const state = providerState(provider)
        const count = catalog.models.filter(
          (model) => model.providerId === provider.id,
        ).length
        return (
          <li key={provider.id}>
            <button
              type="button"
              aria-pressed={selected === provider.id}
              className={cn(
                selected === provider.id && "is-selected",
                !provider.enabled && "is-disabled",
              )}
              onClick={() => onSelect(provider.id)}
            >
              <ProviderMark provider={provider} />
              <span className="adm-provider-list-name">
                <strong>{provider.name}</strong>
                <small>
                  {t("admin.provider.modelCount", { count })}
                  {" · "}
                  {t(`admin.provider.state.${state.key}`)}
                </small>
              </span>
              <i
                className={cn("adm-dot", `adm-tone-${state.tone}`)}
                aria-hidden
              />
            </button>
          </li>
        )
      })}
    </ul>
  )
}
