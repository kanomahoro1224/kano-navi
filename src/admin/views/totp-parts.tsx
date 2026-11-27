import { useEffect, useRef, useState, type SubmitEvent } from "react"
import {
  Check,
  ClipboardCopy,
  KeyRound,
  QrCode,
  RotateCw,
  ShieldCheck,
  ShieldOff,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { ApiError, errorMessage, renderMessage } from "@/admin/api"
import { formatRelative, japanStamp } from "@/admin/format"
import type { TOTPEnrollment, TOTPStatus } from "@/admin/types"
import {
  Badge,
  Button,
  Facts,
  Field,
  Modal,
  Panel,
  type Tone,
} from "@/admin/ui"
import { useAdmin } from "@/admin/use-admin-data"

type CodeState = "off" | "on" | "environment" | "development"

const stateTones: Record<CodeState, Tone> = {
  off: "neutral",
  on: "leaf",
  environment: "sky",
  development: "honey",
}

/** How the panel names the code: development never asks for it. */
function codeState(status: TOTPStatus): CodeState {
  if (status.state === "off") return "off"
  if (!status.asked) return "development"
  return status.state === "environment" ? "environment" : "on"
}

/** A failed change in words; a refused factor names neither. */
function useFailure() {
  const { t } = useAppSettings()
  return (error: unknown) => {
    if (error instanceof ApiError) {
      if (error.code === "invalid_credentials")
        return t("admin.security.totp.refused")
      if (error.code === "totp_code_invalid")
        return t("admin.security.totp.codeInvalid")
      if (error.code === "totp_enrollment_missing")
        return t("admin.security.totp.enrollmentEnded")
    }
    return renderMessage(errorMessage(error), t)
  }
}

/** Six digits, with the space or hyphen some apps show ignored. */
const codeInput = {
  inputMode: "numeric" as const,
  autoComplete: "one-time-code",
  pattern: "[0-9 \\-]{6,8}",
  maxLength: 8,
  spellCheck: false,
}

/**
 * The secret as text for manual entry, in groups of four, with a copy
 * button.
 */
function SecretText({ secret }: { secret: string }) {
  const { t } = useAppSettings()
  const [copied, setCopied] = useState(false)
  const grouped = secret.match(/.{1,4}/gu)?.join(" ") ?? secret
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(secret)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      // The text stays selectable.
    }
  }
  return (
    <div className="adm-totp-secret">
      <code>{grouped}</code>
      <Button
        size="sm"
        variant="ghost"
        icon={copied ? Check : ClipboardCopy}
        onClick={() => void copy()}
      >
        {t(copied ? "admin.security.totp.copied" : "admin.security.totp.copy")}
      </Button>
    </div>
  )
}

/**
 * Sets up or replaces the code: the dialog opens an enrollment, shows its
 * QR code and secret, and confirms it with the password and a code from the
 * new authenticator, plus the current code when replacing. Closing it
 * before the confirm drops the enrollment.
 */
function EnrollDialog({
  replacing,
  onClose,
  onBound,
}: {
  replacing: boolean
  onClose: () => void
  onBound: (status: TOTPStatus) => void
}) {
  const { t } = useAppSettings()
  const { api, notify } = useAdmin()
  const failure = useFailure()
  const [enrollment, setEnrollment] = useState<TOTPEnrollment | null>(null)
  const [password, setPassword] = useState("")
  const [code, setCode] = useState("")
  const [currentCode, setCurrentCode] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [round, setRound] = useState(0)
  const bound = useRef(false)
  useEffect(() => {
    let live = true
    setEnrollment(null)
    setError(null)
    api
      .startTOTP()
      .then((next) => {
        if (live) setEnrollment(next)
      })
      .catch((reason) => {
        if (live) setError(failure(reason))
      })
    return () => {
      live = false
    }
  }, [round])
  const close = () => {
    if (!bound.current) void api.cancelTOTP().catch(() => {})
    onClose()
  }
  const confirm = async (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const next = await api.confirmTOTP({
        password,
        code,
        ...(replacing ? { currentCode } : {}),
      })
      bound.current = true
      notify("admin.notice.totpBound")
      onBound(next)
    } catch (reason) {
      setError(failure(reason))
      setCode("")
      if (
        reason instanceof ApiError &&
        reason.code === "totp_enrollment_missing"
      )
        setEnrollment(null)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      open
      title={t(
        replacing
          ? "admin.security.totp.replaceTitle"
          : "admin.security.totp.setUpTitle",
      )}
      onClose={close}
    >
      <form className="adm-form" onSubmit={confirm}>
        <div className="adm-login-qr">
          {enrollment ? (
            <svg
              className="adm-qr"
              viewBox={`0 0 ${enrollment.qr.size} ${enrollment.qr.size}`}
              role="img"
              aria-label={t("admin.security.totp.qrCode")}
              shapeRendering="crispEdges"
            >
              <path d={enrollment.qr.path} />
            </svg>
          ) : (
            <div className="adm-qr is-empty" aria-hidden>
              <QrCode />
            </div>
          )}
          {enrollment ? <SecretText secret={enrollment.secret} /> : null}
          {!enrollment && error ? (
            <Button
              size="sm"
              icon={RotateCw}
              onClick={() => setRound((value) => value + 1)}
            >
              {t("admin.security.totp.again")}
            </Button>
          ) : null}
        </div>
        <Field label={t("admin.security.totp.password")}>
          <input
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            autoComplete="current-password"
            required
          />
        </Field>
        {replacing ? (
          <Field label={t("admin.security.totp.currentCode")}>
            <input
              {...codeInput}
              value={currentCode}
              onChange={(event) => setCurrentCode(event.target.value)}
              required
            />
          </Field>
        ) : null}
        <Field label={t("admin.security.totp.newCode")}>
          <input
            {...codeInput}
            value={code}
            onChange={(event) => setCode(event.target.value)}
            required
          />
        </Field>
        {error ? (
          <p className="adm-inline-error" role="alert">
            {error}
          </p>
        ) : null}
        <div className="adm-form-actions">
          <Button
            type="submit"
            variant="primary"
            size="sm"
            icon={ShieldCheck}
            busy={busy}
            disabled={!enrollment || busy}
          >
            {t("admin.security.totp.confirm")}
          </Button>
        </div>
      </form>
    </Modal>
  )
}

/** Turns the console's code off with the password and a current code. */
function TurnOffDialog({
  onClose,
  onOff,
}: {
  onClose: () => void
  onOff: (status: TOTPStatus) => void
}) {
  const { t } = useAppSettings()
  const { api, notify } = useAdmin()
  const failure = useFailure()
  const [password, setPassword] = useState("")
  const [code, setCode] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const submit = async (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const next = await api.turnOffTOTP({ password, code })
      notify("admin.notice.totpOff")
      onOff(next)
    } catch (reason) {
      setError(failure(reason))
      setCode("")
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open title={t("admin.security.totp.turnOffTitle")} onClose={onClose}>
      <form className="adm-form" onSubmit={submit}>
        <Field label={t("admin.security.totp.password")}>
          <input
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            autoComplete="current-password"
            required
          />
        </Field>
        <Field label={t("admin.security.totp.currentCode")}>
          <input
            {...codeInput}
            value={code}
            onChange={(event) => setCode(event.target.value)}
            required
          />
        </Field>
        {error ? (
          <p className="adm-inline-error" role="alert">
            {error}
          </p>
        ) : null}
        <div className="adm-form-actions">
          <Button
            type="submit"
            variant="danger"
            size="sm"
            icon={ShieldOff}
            busy={busy}
            disabled={busy}
          >
            {t("admin.security.totp.turnOff")}
          </Button>
        </div>
      </form>
    </Modal>
  )
}

/** The login one-time code: its state and the set-up, replace and turn-off
 * dialogs. The environment's code is shown, never changed. */
export function TOTPPanel({
  status,
  onChange,
}: {
  status: TOTPStatus
  onChange: (status: TOTPStatus) => void
}) {
  const { t, locale } = useAppSettings()
  const [dialog, setDialog] = useState<"" | "enroll" | "off">("")
  const state = codeState(status)
  const consoleBound = status.state === "console"
  const done = (next: TOTPStatus) => {
    setDialog("")
    onChange(next)
  }
  return (
    <Panel
      icon={KeyRound}
      title={t("admin.security.totp.title")}
      actions={
        <Badge tone={stateTones[state]}>
          {t(`admin.security.totp.state.${state}`)}
        </Badge>
      }
    >
      <div className="adm-form">
        {consoleBound && status.boundAt ? (
          <Facts
            items={[
              [
                t("admin.security.totp.boundAt"),
                `${japanStamp(status.boundAt)} · ${formatRelative(status.boundAt, locale)}`,
              ],
            ]}
          />
        ) : null}
        {status.storedUnused ? (
          <Badge tone="neutral">{t("admin.security.totp.storedUnused")}</Badge>
        ) : null}
        {status.state === "environment" ? null : (
          <div className="adm-form-actions">
            {consoleBound ? (
              <Button
                size="sm"
                variant="ghost"
                icon={ShieldOff}
                onClick={() => setDialog("off")}
              >
                {t("admin.security.totp.turnOff")}
              </Button>
            ) : null}
            <Button
              size="sm"
              variant={consoleBound ? "outline" : "primary"}
              icon={QrCode}
              onClick={() => setDialog("enroll")}
            >
              {t(
                consoleBound
                  ? "admin.security.totp.replace"
                  : "admin.security.totp.setUp",
              )}
            </Button>
          </div>
        )}
      </div>
      {dialog === "enroll" ? (
        <EnrollDialog
          replacing={consoleBound}
          onClose={() => setDialog("")}
          onBound={done}
        />
      ) : null}
      {dialog === "off" ? (
        <TurnOffDialog onClose={() => setDialog("")} onOff={done} />
      ) : null}
    </Panel>
  )
}
