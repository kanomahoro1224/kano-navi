import { useState } from "react"
import { BellRing, Globe, RefreshCw, Send, Trash2 } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { japanStamp } from "@/admin/format"
import type { AdminWebPushState, AdminWebPushSubscription } from "@/admin/types"
import {
  Badge,
  Button,
  Callout,
  ConfirmButton,
  Empty,
  Facts,
  Loading,
  PageHeader,
  Panel,
  Switch,
} from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"

function BrowserRow({
  subscription,
  busy,
  onTest,
  onRemove,
}: {
  subscription: AdminWebPushSubscription
  busy: boolean
  onTest: () => void
  onRemove: () => void
}) {
  const { t } = useAppSettings()
  const categories = [
    ["live", subscription.notifyLive],
    ["video", subscription.notifyVideo],
    ["post", subscription.notifyPost],
  ] as const
  return (
    <li className="adm-push-device">
      <div className="adm-push-device-main">
        <div className="adm-push-device-heading">
          <strong>{subscription.locale || "—"}</strong>
          {subscription.enabled ? (
            <Badge tone="leaf">{t("admin.webPushKit.enabled")}</Badge>
          ) : (
            <Badge>{t("admin.webPushKit.disabled")}</Badge>
          )}
          {subscription.failureCount > 0 ? (
            <Badge tone="danger">{subscription.failureCount}</Badge>
          ) : null}
        </div>
        <code className="adm-push-token">{subscription.endpointLabel}</code>
        <div className="adm-chip-row">
          {categories.map(([category, on]) => (
            <Badge key={category} tone={on ? "leaf" : "neutral"}>
              {t(`admin.pushKit.category.${category}`)}
            </Badge>
          ))}
        </div>
        <small>
          {subscription.userAgent || "—"} · {t("admin.pushKit.lastSeen")} ·{" "}
          {japanStamp(subscription.lastSeenAt)}
        </small>
      </div>
      <div className="adm-actions">
        <Button
          size="sm"
          variant="ghost"
          icon={Send}
          busy={busy}
          disabled={!subscription.enabled}
          onClick={onTest}
        >
          {t("admin.pushKit.test")}
        </Button>
        <ConfirmButton
          size="sm"
          variant="ghost"
          icon={Trash2}
          confirmLabel={t("admin.webPushKit.removeConfirm")}
          onConfirm={onRemove}
        >
          {t("admin.webPushKit.remove")}
        </ConfirmButton>
      </div>
    </li>
  )
}

export function WebPushView(_: ViewProps) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const resource = useResource(() => api.webPush(), [])
  const [busy, setBusy] = useState("")
  const [outcome, setOutcome] = useState<{ ok: boolean; text: string } | null>(
    null,
  )
  const state: AdminWebPushState | undefined = resource.data

  const test = async (endpoint?: string) => {
    setBusy(endpoint || "all")
    setOutcome(null)
    const result = await act(() => api.testWebPush(endpoint))
    setBusy("")
    if (!result) return
    setOutcome({
      ok: result.ok,
      text: result.ok
        ? t(
            result.testMode
              ? "admin.webPushKit.testSentImportant"
              : "admin.webPushKit.testSentNormal",
            { count: result.subscriptions, stale: result.stale },
          )
        : result.error || t("admin.webPushKit.testFailed"),
    })
    void resource.reload()
  }

  const toggleTestMode = async (value: boolean) => {
    setBusy("testMode")
    const updated = await act(() =>
      api.updateWebPushSettings({ testMode: value }),
    )
    setBusy("")
    if (updated !== undefined) void resource.reload()
  }

  const remove = async (endpoint: string) => {
    setBusy(endpoint)
    const done = await act(
      () => api.deleteWebPush(endpoint),
      "admin.notice.webPushRemoved",
    )
    setBusy("")
    if (done !== undefined) void resource.reload()
  }

  const refresh = async () => {
    setBusy("refresh")
    await resource.reload()
    setBusy("")
  }

  return (
    <div className="adm-view">
      <PageHeader
        title={t("admin.webPushKit.title")}
        description={t("admin.webPushKit.description")}
        actions={
          <>
            <Button
              icon={RefreshCw}
              variant="ghost"
              busy={busy === "refresh"}
              onClick={() => void refresh()}
            >
              {t("admin.webPushKit.refresh")}
            </Button>
            <Button
              icon={Send}
              variant="primary"
              busy={busy === "all"}
              disabled={!state?.enabled}
              onClick={() => void test()}
            >
              {t("admin.webPushKit.testAll")}
            </Button>
          </>
        }
      />
      {!state ? (
        <Loading />
      ) : (
        <>
          <Panel
            icon={Globe}
            title={t("admin.webPushKit.service")}
            description={t("admin.webPushKit.serviceHint")}
          >
            <Facts
              items={[
                [
                  t("admin.pushKit.status"),
                  state.configured ? (
                    <Badge tone="leaf">
                      {t("admin.webPushKit.configured")}
                    </Badge>
                  ) : (
                    <Badge tone="danger">
                      {t("admin.webPushKit.unconfigured")}
                    </Badge>
                  ),
                ],
                [
                  t("admin.webPushKit.total", { count: state.total }),
                  t("admin.webPushKit.enabledCount", { count: state.enabled }),
                ],
              ]}
            />
            <Callout
              tone="sky"
              icon={BellRing}
              title={t("admin.webPushKit.priority")}
            >
              {t("admin.webPushKit.priorityHint")}
            </Callout>
            <Switch
              checked={state.testMode}
              disabled={!state.configured || busy === "testMode"}
              onChange={(value) => void toggleTestMode(value)}
              label={t("admin.webPushKit.testMode")}
              description={t("admin.webPushKit.testModeHint")}
            />
          </Panel>
          <Panel
            icon={Globe}
            title={t("admin.webPushKit.browsers", { count: state.total })}
            description={t("admin.webPushKit.browsersHint")}
          >
            {outcome ? (
              <Callout
                tone={outcome.ok ? "leaf" : "danger"}
                title={
                  outcome.ok
                    ? t("admin.webPushKit.testSucceeded")
                    : t("admin.webPushKit.testFailed")
                }
              >
                {outcome.text}
              </Callout>
            ) : null}
            {state.subscriptions.length ? (
              <ul className="adm-push-devices">
                {state.subscriptions.map((subscription) => (
                  <BrowserRow
                    key={subscription.endpoint}
                    subscription={subscription}
                    busy={busy === subscription.endpoint}
                    onTest={() => void test(subscription.endpoint)}
                    onRemove={() => void remove(subscription.endpoint)}
                  />
                ))}
              </ul>
            ) : (
              <Empty icon={Globe} title={t("admin.webPushKit.noBrowsers")}>
                {t("admin.webPushKit.noBrowsersHint")}
              </Empty>
            )}
          </Panel>
        </>
      )}
    </div>
  )
}
