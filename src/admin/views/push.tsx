import { useState } from "react"
import { BellRing, Send, Trash2 } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { formatDateTime } from "@/admin/format"
import type { AdminPushDevice, AdminPushState } from "@/admin/types"
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
} from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"

function DeviceRow({
  device,
  busy,
  locale,
  onTest,
  onRemove,
}: {
  device: AdminPushDevice
  busy: boolean
  locale: string
  onTest: () => void
  onRemove: () => void
}) {
  const { t } = useAppSettings()
  const categories = [
    ["live", device.notifyLive],
    ["video", device.notifyVideo],
    ["post", device.notifyPost],
  ] as const
  return (
    <li className="adm-push-device">
      <div className="adm-push-device-main">
        <div className="adm-push-device-heading">
          <strong>{device.deviceId}</strong>
          {device.enabled ? (
            <Badge tone="leaf">{t("admin.pushKit.enabled")}</Badge>
          ) : (
            <Badge>{t("admin.pushKit.disabled")}</Badge>
          )}
        </div>
        <code className="adm-push-token">{device.token}</code>
        <div className="adm-chip-row">
          <Badge tone="sky">{device.platform}</Badge>
          {device.locale ? <Badge>{device.locale}</Badge> : null}
          {device.appVersion ? <Badge>v{device.appVersion}</Badge> : null}
          {categories.map(([category, on]) => (
            <Badge key={category} tone={on ? "leaf" : "neutral"}>
              {t(`admin.pushKit.category.${category}`)}
            </Badge>
          ))}
        </div>
        <small>
          {t("admin.pushKit.lastSeen")} ·{" "}
          {formatDateTime(device.lastSeenAt, locale)}
        </small>
      </div>
      <div className="adm-actions">
        <Button
          size="sm"
          variant="ghost"
          icon={Send}
          busy={busy}
          disabled={!device.enabled}
          onClick={onTest}
        >
          {t("admin.pushKit.test")}
        </Button>
        <ConfirmButton
          size="sm"
          variant="ghost"
          icon={Trash2}
          confirmLabel={t("admin.pushKit.removeConfirm")}
          onConfirm={onRemove}
        >
          {t("admin.pushKit.remove")}
        </ConfirmButton>
      </div>
    </li>
  )
}

export function PushView(_: ViewProps) {
  const { t, locale } = useAppSettings()
  const { api, act } = useAdmin()
  const resource = useResource(() => api.push(), [])
  const [busy, setBusy] = useState("")
  const [outcome, setOutcome] = useState<{ ok: boolean; text: string } | null>(
    null,
  )
  const state: AdminPushState | undefined = resource.data
  const enabledDevices = state?.devices.filter((item) => item.enabled) ?? []

  const test = async (deviceId?: string) => {
    setBusy(deviceId || "all")
    const result = await act(() => api.testPush(deviceId))
    setBusy("")
    if (!result) return
    setOutcome({
      ok: result.ok,
      text: result.ok
        ? t("admin.pushKit.testSent", {
            count: result.devices,
            stale: result.stale,
          })
        : result.error || t("admin.pushKit.testFailed"),
    })
    void resource.reload()
  }

  const remove = async (deviceId: string) => {
    setBusy(deviceId)
    const done = await act(
      () => api.deletePushDevice(deviceId),
      "admin.notice.pushDeviceRemoved",
    )
    setBusy("")
    if (done !== undefined) void resource.reload()
  }

  return (
    <div className="adm-view">
      <PageHeader
        title={t("admin.pushKit.title")}
        description={t("admin.pushKit.description")}
        actions={
          <Button
            icon={Send}
            variant="primary"
            busy={busy === "all"}
            disabled={!enabledDevices.length}
            onClick={() => void test()}
          >
            {t("admin.pushKit.testAll")}
          </Button>
        }
      />
      {!state ? (
        <Loading />
      ) : (
        <>
          <Panel
            icon={BellRing}
            title={t("admin.pushKit.service")}
            description={t("admin.pushKit.serviceHint")}
          >
            <Facts
              items={[
                [
                  t("admin.pushKit.status"),
                  state.configured ? (
                    <Badge tone="leaf">{t("admin.pushKit.configured")}</Badge>
                  ) : (
                    <Badge tone="danger">
                      {t("admin.pushKit.unconfigured")}
                    </Badge>
                  ),
                ],
                [t("admin.pushKit.projectId"), state.projectId || "—"],
                [t("admin.pushKit.keyId"), state.keyId || "—"],
                [t("admin.pushKit.subAccount"), state.subAccount || "—"],
                [t("admin.pushKit.category"), state.category || "—"],
                [
                  t("admin.pushKit.testMode"),
                  state.testMode ? (
                    <Badge tone="honey">{t("admin.pushKit.on")}</Badge>
                  ) : (
                    t("admin.pushKit.off")
                  ),
                ],
              ]}
            />
          </Panel>
          <Panel
            icon={BellRing}
            title={t("admin.pushKit.devices", { count: state.devices.length })}
            description={t("admin.pushKit.devicesHint")}
          >
            {outcome ? (
              <Callout
                tone={outcome.ok ? "leaf" : "danger"}
                title={
                  outcome.ok
                    ? t("admin.pushKit.testSucceeded")
                    : t("admin.pushKit.testFailed")
                }
              >
                {outcome.text}
              </Callout>
            ) : null}
            {state.devices.length ? (
              <ul className="adm-push-devices">
                {state.devices.map((device) => (
                  <DeviceRow
                    key={device.deviceId}
                    device={device}
                    busy={busy === device.deviceId}
                    locale={locale}
                    onTest={() => void test(device.deviceId)}
                    onRemove={() => void remove(device.deviceId)}
                  />
                ))}
              </ul>
            ) : (
              <Empty icon={BellRing} title={t("admin.pushKit.noDevices")}>
                {t("admin.pushKit.noDevicesHint")}
              </Empty>
            )}
          </Panel>
        </>
      )}
    </div>
  )
}
