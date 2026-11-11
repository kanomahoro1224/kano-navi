import { useEffect, useState } from "react"
import {
  KeyRound,
  RotateCw,
  Save,
  ShieldAlert,
  ShoppingBag,
  Trash2,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { formatRelative, japanStamp } from "@/admin/format"
import type { Sources, WebBotAuthStatus } from "@/admin/types"
import { Badge, Button, ConfirmButton, Facts, Field, Panel } from "@/admin/ui"
import { useAdmin } from "@/admin/use-admin-data"
import { explainCode } from "@/admin/views/run-parts"

/** Whether a stored instant is still ahead. */
const future = (value: string | null | undefined) =>
  Boolean(value && Date.parse(value) > Date.now())

/**
 * The shop read's state on the Sources page, below its timer: the sale
 * entries it keeps, its last read and error, and the wait a 429 answer's
 * Retry-After set, during which no read requests the shop.
 */
export function ShopStatusPanel({ shop }: { shop: Sources["shop"] }) {
  const { t, locale } = useAppSettings()
  const status = shop.status
  const stamp = (value: string | null) =>
    value ? `${japanStamp(value)} · ${formatRelative(value, locale)}` : null
  return (
    <Panel
      level={3}
      icon={ShoppingBag}
      title={t("admin.shop.title")}
      actions={
        <span className="adm-chip-row">
          {shop.skipped ? <Badge>{t("admin.overview.skipped")}</Badge> : null}
          {future(status.blockedUntil) ? (
            <Badge tone="danger" icon={ShieldAlert}>
              {t("admin.shop.blockedUntil", {
                time: japanStamp(status.blockedUntil ?? ""),
              })}
            </Badge>
          ) : null}
        </span>
      }
    >
      <Facts
        items={[
          [t("admin.shop.sales"), String(status.sales)],
          [t("admin.shop.lastAttempt"), stamp(status.lastAttemptAt)],
          [
            t("admin.shop.lastError"),
            status.lastError ? (
              <span title={status.lastError}>
                {explainCode(status.lastError, t) ?? status.lastError}
              </span>
            ) : null,
          ],
        ]}
      />
    </Panel>
  )
}

const wbaTone = {
  active: "leaf",
  no_key: "neutral",
  no_origin: "honey",
  key_unreadable: "danger",
} as const

/**
 * The Web Bot Auth key that signs the shop's requests: its state, keyid,
 * age and directory, the signing origin, and generate, rotate and remove
 * behind a second press. While WEB_BOT_AUTH_KEY is set the key is the
 * environment's and only the origin can change.
 */
export function WebBotAuthPanel({
  status,
  onChange,
}: {
  status: WebBotAuthStatus
  onChange: (status: WebBotAuthStatus) => void
}) {
  const { t, locale } = useAppSettings()
  const { api, act } = useAdmin()
  const saved = status.originSetting ?? ""
  const [origin, setOrigin] = useState(saved)
  const [busy, setBusy] = useState("")
  useEffect(() => setOrigin(saved), [saved])
  const run = async (
    name: string,
    work: () => Promise<WebBotAuthStatus>,
    notice: string,
  ) => {
    setBusy(name)
    const next = await act(work, notice)
    setBusy("")
    if (next) onChange(next)
  }
  const hasKey = status.source === "stored" && status.keyId !== null
  const stamp = (value: string | null) =>
    value ? `${japanStamp(value)} · ${formatRelative(value, locale)}` : null
  return (
    <Panel
      level={3}
      icon={KeyRound}
      title={t("admin.wba.title")}
      actions={
        <span className="adm-chip-row">
          {status.environmentManaged ? (
            <Badge>{t("admin.wba.environment")}</Badge>
          ) : null}
          <Badge tone={wbaTone[status.state]}>
            {t(`admin.wba.state.${status.state}`)}
          </Badge>
        </span>
      }
    >
      <div className="adm-form">
        <Facts
          items={[
            [
              t("admin.wba.keyId"),
              status.keyId ? <code>{status.keyId}</code> : null,
            ],
            [t("admin.wba.created"), stamp(status.createdAt)],
            [
              t("admin.wba.directory"),
              status.directoryUrl ? (
                <a href={status.directoryUrl} target="_blank" rel="noreferrer">
                  {status.directoryUrl}
                </a>
              ) : null,
            ],
            [
              t("admin.wba.previous"),
              status.previous
                ? t("admin.wba.previousUntil", {
                    keyId: status.previous.keyId,
                    time: japanStamp(status.previous.until),
                  })
                : null,
            ],
          ]}
        />
        <Field label={t("admin.wba.origin")}>
          <input
            type="text"
            inputMode="url"
            value={origin}
            onChange={(e) => setOrigin(e.target.value)}
            placeholder={status.publicOrigin ?? "https://"}
            spellCheck={false}
            autoCapitalize="off"
          />
        </Field>
        <div className="adm-form-actions">
          {status.environmentManaged ? null : (
            <>
              {hasKey || status.state === "key_unreadable" ? (
                <ConfirmButton
                  size="sm"
                  variant="ghost"
                  icon={Trash2}
                  busy={busy === "remove"}
                  disabled={busy !== ""}
                  onConfirm={() =>
                    run(
                      "remove",
                      () => api.removeWebBotAuthKey(),
                      "admin.notice.wbaRemoved",
                    )
                  }
                >
                  {t("admin.wba.remove")}
                </ConfirmButton>
              ) : null}
              <ConfirmButton
                size="sm"
                variant="ghost"
                icon={hasKey ? RotateCw : KeyRound}
                busy={busy === "generate"}
                disabled={busy !== ""}
                onConfirm={() =>
                  run(
                    "generate",
                    () => api.generateWebBotAuthKey(),
                    "admin.notice.wbaGenerated",
                  )
                }
              >
                {t(hasKey ? "admin.wba.rotate" : "admin.wba.generate")}
              </ConfirmButton>
            </>
          )}
          <Button
            size="sm"
            icon={Save}
            busy={busy === "save"}
            disabled={busy !== "" || origin.trim() === saved}
            onClick={() =>
              run(
                "save",
                () => api.updateWebBotAuth({ origin: origin.trim() || null }),
                "admin.notice.wbaSaved",
              )
            }
          >
            {t("admin.action.save")}
          </Button>
        </div>
      </div>
    </Panel>
  )
}
