import { useEffect, useState, type SubmitEvent } from "react"
import { KeyRound, Plus, QrCode, ShieldAlert } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type {
  AdminBiliLoginPoll,
  AdminBiliLoginQR,
  AdminBiliLoginStatus,
  AdminVideo,
  Sources,
} from "@/admin/types"
import { Badge, Button, Facts, Field, Panel } from "@/admin/ui"
import { useAdmin } from "@/admin/use-admin-data"
import { BilibiliIcon } from "@/admin/views/run-parts"

/**
 * The Bilibili account on the Sources page, beside its timer (which shows the
 * last read and backoff), and an operator's way to add one upload by BV ID or
 * link. Uploads stay in the console; one merged into its YouTube copy is
 * listed on that video.
 */
export function BilibiliSource({
  status,
  onAdded,
}: {
  status: Sources["bilibili"] | undefined
  onAdded: (video: AdminVideo) => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [input, setInput] = useState("")
  const [busy, setBusy] = useState(false)
  const usable = Boolean(status?.available && !status.skipped)
  const blocked =
    status?.blockedUntil && Date.parse(status.blockedUntil) > Date.now()
  const add = async (submit: SubmitEvent<HTMLFormElement>) => {
    submit.preventDefault()
    if (!input.trim()) return
    setBusy(true)
    const result = await act(
      () => api.addBilibiliVideo(input.trim()),
      "admin.notice.bilibiliAdded",
    )
    setBusy(false)
    if (!result) return
    setInput("")
    onAdded(result.video)
  }
  return (
    <Panel
      icon={BilibiliIcon}
      title={t("admin.bilibili.title")}
      description={t("admin.bilibili.description")}
      actions={
        status ? (
          <span className="adm-chip-row">
            {!usable ? <Badge>{t("admin.overview.skipped")}</Badge> : null}
            {status.signedIn ? (
              <Badge tone="leaf" icon={KeyRound}>
                {t("admin.bilibili.signedIn")}
              </Badge>
            ) : (
              <Badge icon={KeyRound}>{t("admin.bilibili.anonymous")}</Badge>
            )}
            {blocked ? (
              <Badge tone="danger" icon={ShieldAlert}>
                {t("admin.bilibili.blocked")}
              </Badge>
            ) : null}
          </span>
        ) : null
      }
    >
      {status?.available ? (
        <Facts
          items={[
            [t("admin.bilibili.account"), status.account],
            [t("admin.bilibili.videos"), String(status.videos)],
            [
              t("admin.bilibili.interval"),
              t("admin.bilibili.everyMinutes", {
                count: status.minIntervalMinutes ?? 0,
              }),
            ],
            [t("admin.bilibili.lastError"), status.lastError ?? "—"],
          ]}
        />
      ) : null}
      {!status?.signedIn && status?.available ? (
        <p className="adm-note">{t("admin.bilibili.signInHint")}</p>
      ) : null}
      <form className="adm-bilibili-add" onSubmit={add}>
        <Field
          label={t("admin.bilibili.add")}
          hint={t("admin.bilibili.addHint")}
        >
          <input
            value={input}
            placeholder="BV1xxxxxxxxx"
            maxLength={300}
            disabled={!usable}
            onChange={(e) => setInput(e.target.value)}
          />
        </Field>
        <Button
          type="submit"
          icon={Plus}
          busy={busy}
          disabled={!usable || !input.trim()}
        >
          {t("admin.bilibili.addButton")}
        </Button>
      </form>
      <BilibiliLogin />
    </Panel>
  )
}

/**
 * The collector's scan login, shown beside the account it feeds. The console
 * never calls Bilibili itself: it proxies the collector's loopback endpoints,
 * and that process keeps the cookie jar.
 */
function BilibiliLogin() {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [status, setStatus] = useState<AdminBiliLoginStatus | null>(null)
  const [qr, setQr] = useState<AdminBiliLoginQR | null>(null)
  const [poll, setPoll] = useState<AdminBiliLoginPoll | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let live = true
    void act(() => api.biliLoginStatus()).then((next) => {
      if (live && next) setStatus(next)
    })
    return () => {
      live = false
    }
    // Read once on mount; the button refreshes it after a successful scan.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!qr) return
    let live = true
    const timer = window.setInterval(() => {
      void act(() => api.biliLoginPoll(qr.key)).then((next) => {
        if (!live || !next) return
        setPoll(next)
        if (next.loggedIn) {
          setQr(null)
          setStatus({ available: true, loggedIn: true })
        }
        if (next.code === 86038) setQr(null)
      })
    }, 2000)
    return () => {
      live = false
      window.clearInterval(timer)
    }
    // Only the key matters: a new code restarts the check.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [qr?.key])

  const issue = async () => {
    setBusy(true)
    setPoll(null)
    const next = await act(() => api.biliLoginQR())
    setBusy(false)
    if (next?.image) setQr(next)
  }

  // 86101 (not scanned yet) and 86090 (scanned, waiting) are both "pending";
  // only the second one has seen the phone, so the wording has to differ.
  const note = !status
    ? null
    : !status.available
      ? t("admin.biliLogin.unavailable")
      : poll?.loggedIn
        ? t("admin.biliLogin.success")
        : poll?.scanned
          ? t("admin.biliLogin.scanned")
          : poll?.code === 86038
            ? t("admin.biliLogin.expired")
            : poll?.pending
              ? t("admin.biliLogin.waiting")
              : null

  return (
    <div className="adm-bili-login">
      <div className="adm-bili-login-body">
        <div className="adm-chip-row">
          {status?.loggedIn ? (
            <Badge tone="leaf" icon={KeyRound}>
              {t("admin.bilibili.signedIn")}
            </Badge>
          ) : (
            <Badge icon={KeyRound}>{t("admin.bilibili.anonymous")}</Badge>
          )}
        </div>
        <p className="adm-note">
          {note ?? t("admin.biliLogin.hint")}
        </p>
        <Button
          icon={QrCode}
          variant="primary"
          busy={busy}
          disabled={!status?.available}
          onClick={() => void issue()}
        >
          {qr ? t("admin.biliLogin.refresh") : t("admin.biliLogin.start")}
        </Button>
      </div>
      {qr ? <img src={qr.image} alt={t("admin.biliLogin.title")} /> : null}
    </div>
  )
}
