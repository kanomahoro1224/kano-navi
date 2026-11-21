import { useEffect, useState } from "react"
import {
  Activity,
  BellRing,
  HeartPulse,
  History,
  KeyRound,
  ListChecks,
  Pencil,
  Plus,
  Save,
  Send,
  Siren,
  TriangleAlert,
  X,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { formatRelative, formatTime } from "@/admin/format"
import type {
  AlertChannel,
  AlertChannelInput,
  AlertChannelType,
  AlertDelivery,
  AlertHeartbeat,
  AlertIncident,
  AlertRule,
  AlertRuleSetting,
  AlertSettings,
  AlertSeverity,
  AlertTestResult,
} from "@/admin/types"
import {
  Badge,
  Button,
  ConfirmButton,
  Empty,
  Field,
  IconButton,
  Loading,
  Modal,
  PageHeader,
  Panel,
  Switch,
  type Tone,
} from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"
import { cn } from "@/lib/utils"

const severities: AlertSeverity[] = ["info", "warning", "critical"]

const severityTone: Record<AlertSeverity, Tone> = {
  info: "sky",
  warning: "warning",
  critical: "danger",
}

const stateTone: Record<AlertSettings["state"], Tone> = {
  on: "success",
  development: "honey",
  off: "neutral",
}

const deliveryTone: Record<AlertDelivery["status"], Tone> = {
  pending: "running",
  sent: "success",
  failed: "danger",
  dropped: "neutral",
}

/** A save keeps every stored secret unless the channel is the one edited. */
function inputOf(channel: AlertChannel): AlertChannelInput {
  return {
    id: channel.id,
    name: channel.name,
    type: channel.type,
    enabled: channel.enabled,
    server: channel.server,
    topic: channel.topic,
    chatId: channel.chatId,
    link: channel.link,
  }
}

function destination(channel: AlertChannel) {
  switch (channel.type) {
    case "ntfy":
      return `${channel.server.replace(/^https:\/\//u, "")}/${channel.topic}`
    case "telegram":
      return channel.chatId
    default:
      return channel.urlHost
  }
}

interface ChannelDraft extends Omit<AlertChannelInput, "secret" | "url"> {
  url: string
  secret: string
  clearSecret: boolean
}

const emptyDraft: ChannelDraft = {
  name: "",
  type: "ntfy",
  enabled: true,
  server: "",
  topic: "",
  chatId: "",
  url: "",
  link: true,
  secret: "",
  clearSecret: false,
}

function ChannelEditor({
  channel,
  types,
  open,
  onClose,
  onSave,
}: {
  channel: AlertChannel | null
  types: AlertChannelType[]
  open: boolean
  onClose: () => void
  onSave: (input: AlertChannelInput) => Promise<boolean>
}) {
  const { t } = useAppSettings()
  const [draft, setDraft] = useState<ChannelDraft>(emptyDraft)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (!open) return
    setDraft(
      channel
        ? { ...emptyDraft, ...inputOf(channel), secret: "", clearSecret: false }
        : emptyDraft,
    )
  }, [open, channel])
  const set = (patch: Partial<ChannelDraft>) =>
    setDraft((current) => ({ ...current, ...patch }))
  const stored = Boolean(channel?.hasSecret) && channel?.type === draft.type
  const storedUrl =
    channel?.type === "webhook" && draft.type === "webhook" && !!channel.urlHost
  const required = draft.type === "telegram" && !stored
  const ready =
    draft.type === "ntfy"
      ? Boolean(draft.topic.trim())
      : draft.type === "telegram"
        ? Boolean(draft.chatId.trim()) && (stored || Boolean(draft.secret))
        : storedUrl || Boolean(draft.url.trim())
  const submit = async () => {
    const { secret, clearSecret, url, ...rest } = draft
    const input: AlertChannelInput = {
      ...rest,
      id: channel?.id,
      name: rest.name.trim(),
      server: rest.server.trim(),
      topic: rest.topic.trim(),
      chatId: rest.chatId.trim(),
    }
    if (draft.type === "webhook" && url.trim()) input.url = url.trim()
    if (secret) input.secret = secret
    else if (
      clearSecret ||
      // A new address cannot keep a secret that no longer decrypts.
      (channel?.secretUnreadable && input.url !== undefined) ||
      (channel && channel.type !== draft.type)
    )
      input.secret = ""
    setBusy(true)
    const saved = await onSave(input)
    setBusy(false)
    if (saved) onClose()
  }
  const secretLabel = t(`admin.alerts.secret.${draft.type}`)
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t(
        channel ? "admin.alerts.editChannel" : "admin.alerts.addChannel",
      )}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("admin.action.cancel")}
          </Button>
          <Button
            type="submit"
            form="adm-alert-channel-form"
            variant="primary"
            icon={Save}
            busy={busy}
            disabled={!ready}
          >
            {t("admin.action.save")}
          </Button>
        </>
      }
    >
      <form
        id="adm-alert-channel-form"
        className="adm-form"
        onSubmit={(event) => {
          event.preventDefault()
          void submit()
        }}
      >
        <div className="adm-form-row">
          <Field label={t("admin.alerts.type")}>
            <select
              value={draft.type}
              onChange={(event) =>
                set({
                  type: event.target.value as AlertChannelType,
                  secret: "",
                  clearSecret: false,
                })
              }
            >
              {types.map((type) => (
                <option key={type} value={type}>
                  {t(`admin.alerts.channelType.${type}`)}
                </option>
              ))}
            </select>
          </Field>
          <Field label={t("admin.alerts.name")}>
            <input
              value={draft.name}
              maxLength={64}
              onChange={(event) => set({ name: event.target.value })}
            />
          </Field>
        </div>
        {draft.type === "ntfy" ? (
          <div className="adm-form-row">
            <Field label={t("admin.alerts.server")}>
              <input
                value={draft.server}
                type="url"
                autoComplete="off"
                spellCheck={false}
                placeholder="https://ntfy.sh"
                onChange={(event) => set({ server: event.target.value })}
              />
            </Field>
            <Field label={t("admin.alerts.topic")}>
              <input
                value={draft.topic}
                required
                maxLength={64}
                autoComplete="off"
                spellCheck={false}
                onChange={(event) => set({ topic: event.target.value })}
              />
            </Field>
          </div>
        ) : null}
        {draft.type === "telegram" ? (
          <Field label={t("admin.alerts.chatId")}>
            <input
              value={draft.chatId}
              required
              autoComplete="off"
              spellCheck={false}
              placeholder="-1001234567890"
              onChange={(event) => set({ chatId: event.target.value })}
            />
          </Field>
        ) : null}
        {draft.type === "webhook" ? (
          <Field
            label={t("admin.alerts.url")}
            hint={storedUrl ? t("admin.alerts.secretKeepHint") : undefined}
          >
            <input
              value={draft.url}
              required={!storedUrl}
              type="url"
              autoComplete="off"
              spellCheck={false}
              placeholder={
                storedUrl ? `https://${channel?.urlHost}/••••` : "https://"
              }
              onChange={(event) => set({ url: event.target.value })}
            />
          </Field>
        ) : null}
        <Field
          label={secretLabel}
          hint={
            stored && !draft.clearSecret
              ? t(
                  channel?.secretUnreadable
                    ? "admin.alerts.secretUnreadableHint"
                    : "admin.alerts.secretKeepHint",
                )
              : undefined
          }
        >
          <input
            type="password"
            value={draft.secret}
            required={required}
            autoComplete="new-password"
            placeholder={stored && !draft.clearSecret ? "••••••••" : undefined}
            onChange={(event) =>
              set({ secret: event.target.value, clearSecret: false })
            }
          />
        </Field>
        {stored && draft.type !== "telegram" ? (
          <Switch
            checked={draft.clearSecret}
            onChange={(clearSecret) => set({ clearSecret, secret: "" })}
            label={t("admin.alerts.clearSecret")}
          />
        ) : null}
        <Switch
          checked={draft.link}
          onChange={(link) => set({ link })}
          label={t("admin.alerts.link")}
        />
        <Switch
          checked={draft.enabled}
          onChange={(enabled) => set({ enabled })}
          label={t("admin.alerts.enabled")}
        />
      </form>
    </Modal>
  )
}

function testSummary(
  result: AlertTestResult,
  t: (key: string, values?: Record<string, any>) => string,
) {
  return result.ok
    ? t("admin.alerts.testOk", { ms: result.latencyMs })
    : t(`admin.alerts.error.${result.error ?? "failed"}`)
}

function ChannelsPanel({
  settings,
  save,
  onTested,
}: {
  settings: AlertSettings
  save: (channels: AlertChannelInput[], success?: string) => Promise<boolean>
  onTested: () => void
}) {
  const { t } = useAppSettings()
  const { api, notify, fail } = useAdmin()
  const [editing, setEditing] = useState<AlertChannel | null>(null)
  const [adding, setAdding] = useState(false)
  const [testing, setTesting] = useState<string | null>(null)
  const [results, setResults] = useState<
    Record<string, { ok: boolean; text: string }>
  >({})
  const channels = settings.channels
  const inputs = channels.map(inputOf)
  const test = async (channel: AlertChannel) => {
    setTesting(channel.id)
    try {
      const result = await api.testAlertChannel(channel.id)
      setResults((current) => ({
        ...current,
        [channel.id]: { ok: result.ok, text: testSummary(result, t) },
      }))
      if (result.ok)
        notify("admin.notice.alertTestSent", {
          name: channel.name || t(`admin.alerts.channelType.${channel.type}`),
        })
      onTested()
    } catch (error) {
      fail(error)
    } finally {
      setTesting(null)
    }
  }
  const editorChannel = adding ? null : editing
  return (
    <Panel
      icon={Send}
      title={t("admin.alerts.channels")}
      actions={
        <Button
          size="sm"
          variant="primary"
          icon={Plus}
          disabled={channels.length >= 8}
          onClick={() => setAdding(true)}
        >
          {t("admin.alerts.addChannel")}
        </Button>
      }
    >
      {channels.length ? (
        <ul className="adm-alert-list">
          {channels.map((channel, index) => {
            const result = results[channel.id]
            return (
              <li
                key={channel.id}
                className={cn(
                  "adm-alert-row",
                  !channel.enabled && "is-off",
                  channel.secretUnreadable && "has-issue",
                )}
              >
                <span className="adm-alert-main">
                  <strong>
                    {channel.name ||
                      t(`admin.alerts.channelType.${channel.type}`)}
                  </strong>
                  <small className="adm-alert-mono">
                    {destination(channel)}
                  </small>
                  <span className="adm-chip-row">
                    <Badge>
                      {t(`admin.alerts.channelType.${channel.type}`)}
                    </Badge>
                    {!channel.enabled ? (
                      <Badge>{t("admin.alerts.off")}</Badge>
                    ) : null}
                    {channel.secretUnreadable ? (
                      <Badge tone="danger" icon={TriangleAlert}>
                        {t("admin.alerts.secretUnreadable")}
                      </Badge>
                    ) : channel.hasSecret ? (
                      <Badge icon={KeyRound}>
                        {t(`admin.alerts.secret.${channel.type}`)}
                      </Badge>
                    ) : null}
                    {result ? (
                      <Badge tone={result.ok ? "success" : "danger"}>
                        {result.text}
                      </Badge>
                    ) : null}
                  </span>
                </span>
                <span className="adm-row-actions">
                  <IconButton
                    icon={Activity}
                    label={t("admin.alerts.test")}
                    busy={testing === channel.id}
                    disabled={channel.secretUnreadable || testing !== null}
                    onClick={() => void test(channel)}
                  />
                  <IconButton
                    icon={Pencil}
                    label={t("admin.alerts.edit")}
                    onClick={() => setEditing(channel)}
                  />
                  <ConfirmButton
                    size="sm"
                    variant="ghost"
                    icon={X}
                    aria-label={t("admin.alerts.remove")}
                    title={t("admin.alerts.remove")}
                    onConfirm={() =>
                      void save(
                        inputs.filter((_, at) => at !== index),
                        "admin.notice.alertChannelRemoved",
                      )
                    }
                  />
                </span>
              </li>
            )
          })}
        </ul>
      ) : (
        <Empty title={t("admin.alerts.noChannels")} />
      )}
      <ChannelEditor
        open={adding || editing !== null}
        channel={editorChannel}
        types={settings.channelTypes}
        onClose={() => {
          setAdding(false)
          setEditing(null)
        }}
        onSave={(input) =>
          save(
            editorChannel
              ? inputs.map((item) => (item.id === input.id ? input : item))
              : [...inputs, input],
            "admin.notice.alertChannelSaved",
          )
        }
      />
    </Panel>
  )
}

function RuleRow({
  rule,
  save,
}: {
  rule: AlertRule
  save: (setting: AlertRuleSetting) => Promise<boolean>
}) {
  const { t } = useAppSettings()
  const setting = rule.setting
  const [threshold, setThreshold] = useState(String(setting.threshold))
  useEffect(() => setThreshold(String(setting.threshold)), [setting.threshold])
  const commitThreshold = () => {
    const value = Number(threshold)
    // An empty box is not 0: the server would read 0 as the default.
    if (
      !threshold.trim() ||
      !Number.isInteger(value) ||
      value === setting.threshold
    ) {
      setThreshold(String(setting.threshold))
      return
    }
    void save({ ...setting, threshold: value }).then((saved) => {
      if (!saved) setThreshold(String(setting.threshold))
    })
  }
  return (
    <li className={cn("adm-alert-rule", !setting.enabled && "is-off")}>
      <Switch
        checked={setting.enabled}
        onChange={(enabled) => void save({ ...setting, enabled })}
        label={t(`admin.alerts.rule.${rule.code}`)}
      />
      {setting.enabled ? (
        <div className="adm-alert-rule-fields">
          {rule.param ? (
            <label className="adm-alert-inline">
              <input
                type="number"
                inputMode="numeric"
                min={rule.min}
                max={rule.max}
                value={threshold}
                onChange={(event) => setThreshold(event.target.value)}
                onBlur={commitThreshold}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault()
                    commitThreshold()
                  }
                }}
              />
              <span>{t(`admin.alerts.param.${rule.param}`)}</span>
            </label>
          ) : null}
          <label className="adm-alert-inline">
            <select
              value={setting.severity}
              aria-label={t("admin.alerts.severity")}
              onChange={(event) =>
                void save({
                  ...setting,
                  severity: event.target.value as AlertSeverity,
                })
              }
            >
              {severities.map((severity) => (
                <option key={severity} value={severity}>
                  {t(`admin.alerts.severity.${severity}`)}
                </option>
              ))}
            </select>
          </label>
          <Switch
            checked={setting.notifyResolved}
            onChange={(notifyResolved) =>
              void save({ ...setting, notifyResolved })
            }
            label={t("admin.alerts.notifyResolved")}
          />
        </div>
      ) : null}
    </li>
  )
}

function RulesPanel({
  rules,
  save,
}: {
  rules: AlertRule[]
  save: (code: string, setting: AlertRuleSetting) => Promise<boolean>
}) {
  const { t } = useAppSettings()
  return (
    <Panel icon={ListChecks} title={t("admin.alerts.rules")}>
      <ul className="adm-alert-rules">
        {rules.map((rule) => (
          <RuleRow
            key={rule.code}
            rule={rule}
            save={(setting) => save(rule.code, setting)}
          />
        ))}
      </ul>
    </Panel>
  )
}

function HeartbeatPanel({
  heartbeat,
  save,
}: {
  heartbeat: AlertHeartbeat
  save: (value: {
    enabled: boolean
    intervalMinutes: number
    url?: string
  }) => Promise<boolean>
}) {
  const { t, locale } = useAppSettings()
  const { api, notify, fail } = useAdmin()
  const [url, setUrl] = useState("")
  const [interval, setMinutes] = useState(String(heartbeat.intervalMinutes))
  const [busy, setBusy] = useState(false)
  const [testing, setTesting] = useState(false)
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(
    null,
  )
  useEffect(
    () => setMinutes(String(heartbeat.intervalMinutes)),
    [heartbeat.intervalMinutes],
  )
  const minutes = Number(interval)
  // An empty box is not 0: the server would read 0 as the default.
  const minutesValid = Boolean(interval.trim()) && Number.isInteger(minutes)
  const changed =
    (Boolean(url) && minutesValid) ||
    (minutesValid && minutes !== heartbeat.intervalMinutes)
  const submit = async () => {
    setBusy(true)
    const saved = await save({
      enabled: heartbeat.enabled,
      intervalMinutes: minutes,
      ...(url ? { url } : {}),
    })
    setBusy(false)
    if (saved) setUrl("")
  }
  const test = async () => {
    setTesting(true)
    try {
      const next = await api.testAlertHeartbeat()
      setResult({ ok: next.ok, text: testSummary(next, t) })
      if (next.ok) notify("admin.notice.alertHeartbeatSent")
    } catch (error) {
      fail(error)
    } finally {
      setTesting(false)
    }
  }
  return (
    <Panel
      icon={HeartPulse}
      title={t("admin.alerts.heartbeat")}
      actions={
        <Button
          size="sm"
          variant="ghost"
          icon={Activity}
          busy={testing}
          disabled={!heartbeat.hasUrl || heartbeat.urlUnreadable}
          onClick={() => void test()}
        >
          {t("admin.alerts.test")}
        </Button>
      }
    >
      <div className="adm-form">
        <Switch
          checked={heartbeat.enabled}
          disabled={!heartbeat.hasUrl && !heartbeat.enabled}
          onChange={(enabled) =>
            void save({ enabled, intervalMinutes: heartbeat.intervalMinutes })
          }
          label={t("admin.alerts.heartbeatEnabled")}
        />
        <div className="adm-form-row">
          <Field
            label={t("admin.alerts.heartbeatUrl")}
            hint={
              heartbeat.urlUnreadable
                ? t("admin.alerts.secretUnreadableHint")
                : heartbeat.hasUrl
                  ? t("admin.alerts.secretKeepHint")
                  : undefined
            }
          >
            <input
              type="password"
              value={url}
              autoComplete="off"
              placeholder={heartbeat.hasUrl ? "••••••••" : "https://"}
              onChange={(event) => setUrl(event.target.value)}
            />
          </Field>
          <Field label={t("admin.alerts.heartbeatInterval")}>
            <input
              type="number"
              inputMode="numeric"
              min={1}
              max={60}
              value={interval}
              onChange={(event) => setMinutes(event.target.value)}
            />
          </Field>
        </div>
        <div className="adm-alert-foot">
          <span className="adm-chip-row">
            {heartbeat.lastOkAt ? (
              <Badge tone="success">
                {t("admin.alerts.lastOk", {
                  time: formatRelative(heartbeat.lastOkAt, locale),
                })}
              </Badge>
            ) : null}
            {heartbeat.lastFailureAt ? (
              <Badge tone="danger">
                {t("admin.alerts.lastFailure", {
                  time: formatRelative(heartbeat.lastFailureAt, locale),
                })}
                {heartbeat.lastError
                  ? ` · ${t(`admin.alerts.error.${heartbeat.lastError}`)}`
                  : ""}
              </Badge>
            ) : null}
            {result ? (
              <Badge tone={result.ok ? "success" : "danger"}>
                {result.text}
              </Badge>
            ) : null}
          </span>
          <Button
            size="sm"
            variant="primary"
            icon={Save}
            busy={busy}
            disabled={!changed}
            onClick={() => void submit()}
          >
            {t("admin.action.save")}
          </Button>
        </div>
      </div>
    </Panel>
  )
}

function subjectLabel(
  subject: string | null,
  t: (key: string, values?: Record<string, any>) => string,
) {
  if (!subject) return ""
  const key = `admin.alerts.subject.${subject}`
  const label = t(key)
  return label === key ? subject : label
}

function IncidentsPanel({ incidents }: { incidents: AlertIncident[] }) {
  const { t, locale } = useAppSettings()
  return (
    <Panel icon={Siren} title={t("admin.alerts.open")}>
      <ul className="adm-alert-list">
        {incidents.map((incident) => (
          <li key={incident.id} className="adm-alert-row">
            <span className="adm-alert-main">
              <strong>
                {t(`admin.alerts.rule.${incident.rule}`)} ·{" "}
                {subjectLabel(incident.subject, t)}
              </strong>
              <small>
                {t("admin.alerts.openedAt", {
                  time: formatRelative(incident.openedAt, locale),
                })}
              </small>
            </span>
            <Badge tone={severityTone[incident.severity]}>
              {t(`admin.alerts.severity.${incident.severity}`)}
            </Badge>
          </li>
        ))}
      </ul>
    </Panel>
  )
}

function DeliveriesPanel({
  deliveries,
  channels,
}: {
  deliveries: AlertDelivery[] | undefined
  channels: AlertChannel[]
}) {
  const { t, locale } = useAppSettings()
  const names = new Map(
    channels.map((channel) => [
      channel.id,
      channel.name || t(`admin.alerts.channelType.${channel.type}`),
    ]),
  )
  return (
    <Panel icon={History} title={t("admin.alerts.deliveries")}>
      {!deliveries ? (
        <Loading />
      ) : deliveries.length ? (
        <ul className="adm-alert-list is-compact">
          {deliveries.map((delivery) => (
            <li key={delivery.id} className="adm-alert-row">
              <span className="adm-alert-main">
                <strong>
                  {delivery.rule
                    ? `${t(`admin.alerts.rule.${delivery.rule}`)} · ${subjectLabel(delivery.subject, t)}`
                    : t("admin.alerts.kind.test")}
                </strong>
                <small>
                  {formatTime(delivery.createdAt, locale)} ·{" "}
                  {names.get(delivery.channelId) ??
                    t("admin.alerts.removedChannel")}
                  {delivery.rule
                    ? ` · ${t(`admin.alerts.kind.${delivery.kind}`)}`
                    : ""}
                  {delivery.attempts > 1
                    ? ` · ${t("admin.alerts.attempts", { count: delivery.attempts })}`
                    : ""}
                </small>
              </span>
              <Badge tone={deliveryTone[delivery.status]}>
                {t(`admin.alerts.status.${delivery.status}`)}
                {delivery.lastError
                  ? ` · ${t(`admin.alerts.error.${delivery.lastError}`)}`
                  : ""}
              </Badge>
            </li>
          ))}
        </ul>
      ) : (
        <Empty title={t("admin.alerts.noDeliveries")} />
      )}
    </Panel>
  )
}

/** The Alerts page: where problems are delivered, which rules raise them,
 * the heartbeat, open incidents and the delivery history. */
export function AlertsView() {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const resource = useResource(() => api.alertSettings(), [])
  const history = useResource(() => api.alertDeliveries(), [])
  const settings = resource.data
  const apply = async (
    body: Parameters<typeof api.updateAlertSettings>[0],
    success?: string,
  ) => {
    const next = await act(() => api.updateAlertSettings(body), success)
    if (next) resource.setData(next)
    return Boolean(next)
  }
  return (
    <div className="adm-view">
      <PageHeader
        title={t("admin.alerts.title")}
        actions={
          settings ? (
            <Badge tone={stateTone[settings.state]} icon={BellRing}>
              {t(`admin.alerts.state.${settings.state}`)}
            </Badge>
          ) : null
        }
      />
      {!settings ? (
        <Loading />
      ) : (
        <div className="adm-alerts-grid">
          <div className="adm-alerts-column">
            {settings.incidents.length ? (
              <IncidentsPanel incidents={settings.incidents} />
            ) : null}
            <ChannelsPanel
              settings={settings}
              save={(channels, success) => apply({ channels }, success)}
              onTested={() => void history.reload()}
            />
            <HeartbeatPanel
              heartbeat={settings.heartbeat}
              save={(heartbeat) =>
                apply({ heartbeat }, "admin.notice.alertHeartbeatSaved")
              }
            />
          </div>
          <div className="adm-alerts-column">
            <RulesPanel
              rules={settings.rules}
              save={(code, setting) =>
                apply(
                  { rules: { [code]: setting } },
                  "admin.notice.alertRuleSaved",
                )
              }
            />
            <DeliveriesPanel
              deliveries={history.data?.deliveries}
              channels={settings.channels}
            />
          </div>
        </div>
      )}
    </div>
  )
}
