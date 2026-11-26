import {
  useEffect,
  useRef,
  useState,
  type ReactNode,
  type SubmitEvent,
} from "react"
import {
  ClipboardPaste,
  KeyRound,
  LogOut,
  Plus,
  QrCode,
  RadioTower,
  RotateCw,
  ShieldAlert,
  Trash2,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { ApiError } from "@/admin/api"
import { formatRelative, japanStamp } from "@/admin/format"
import type {
  AdminBiliLoginPoll,
  AdminBiliLoginQR,
  AdminBiliLoginStatus,
  AdminVideo,
  BilibiliLiveRoom,
  BilibiliLogin,
  BilibiliQR,
  BilibiliQRState,
  Sources,
} from "@/admin/types"
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
import { BilibiliIcon, explainCode } from "@/admin/views/run-parts"

const roomStates: Record<number, { key: string; tone: Tone }> = {
  0: { key: "admin.bilibili.live.off", tone: "neutral" },
  1: { key: "admin.bilibili.live.on", tone: "leaf" },
  2: { key: "admin.bilibili.live.rotation", tone: "neutral" },
}

const loginStates: Record<BilibiliLogin, { key: string; tone: Tone }> = {
  signed_in: { key: "admin.bilibili.signedIn", tone: "leaf" },
  expired: { key: "admin.bilibili.loginExpired", tone: "danger" },
  unchecked: { key: "admin.bilibili.loginUnchecked", tone: "honey" },
  not_configured: { key: "admin.bilibili.anonymous", tone: "neutral" },
}

/** A refusal by the local backoff that the server lets one read override. */
function forceable(error: unknown): error is ApiError {
  return (
    error instanceof ApiError &&
    error.code === "bilibili_blocked" &&
    error.force === true
  )
}

/**
 * A single-item Bilibili read (adding an upload or a dynamic, reading an
 * upload again) that the captcha backoff refused can be sent once anyway:
 * `read` runs `send(false)` like any action, and after such a refusal
 * `offer` is a "Try once anyway" button that sends `send(true)` once.
 * Timers and Run now never get this override.
 */
export function useForcedRead() {
  const { t } = useAppSettings()
  const { act } = useAdmin()
  const [pending, setPending] = useState<{
    until?: string
    send: () => Promise<unknown>
  } | null>(null)
  const [busy, setBusy] = useState(false)
  const read = async <T,>(
    send: (force: boolean) => Promise<T>,
    notice: string,
    done: (result: T) => void,
  ) => {
    setPending(null)
    const result = await act(async () => {
      try {
        return await send(false)
      } catch (error) {
        if (forceable(error))
          setPending({
            until: error.blockedUntil,
            send: async () => {
              const forced = await act(() => send(true), notice)
              if (forced) done(forced)
            },
          })
        throw error
      }
    }, notice)
    if (result) done(result)
    return Boolean(result)
  }
  const force = async () => {
    if (!pending) return
    setBusy(true)
    await pending.send()
    setBusy(false)
    setPending(null)
  }
  const offer: ReactNode = pending ? (
    <Button
      size="sm"
      variant="ghost"
      icon={ShieldAlert}
      busy={busy}
      onClick={force}
      title={t("admin.bilibili.forceHint")}
    >
      {t("admin.bilibili.force")}
    </Button>
  ) : null
  return { read, offer }
}

/** One add form of the panel: a link or ID field and its button. */
function AddForm({
  label,
  hint,
  placeholder,
  disabled,
  notice,
  send,
  onAdded,
}: {
  label: string
  hint?: string
  placeholder: string
  disabled: boolean
  notice: string
  send: (input: string, force: boolean) => Promise<unknown>
  onAdded: () => void
}) {
  const { t } = useAppSettings()
  const [input, setInput] = useState("")
  const [busy, setBusy] = useState(false)
  const { read, offer } = useForcedRead()
  const submit = async (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault()
    const value = input.trim()
    if (!value) return
    setBusy(true)
    await read(
      (force) => send(value, force),
      notice,
      () => {
        setInput("")
        onAdded()
      },
    )
    setBusy(false)
  }
  return (
    <form className="adm-bilibili-add" onSubmit={submit}>
      <Field label={label} hint={hint}>
        <input
          value={input}
          placeholder={placeholder}
          maxLength={300}
          disabled={disabled}
          onChange={(e) => setInput(e.target.value)}
        />
      </Field>
      <Button
        type="submit"
        icon={Plus}
        busy={busy}
        disabled={disabled || !input.trim()}
      >
        {t("admin.bilibili.addButton")}
      </Button>
      {offer}
    </form>
  )
}

/** The live room's last check: its state, title and when it went live. */
export function LiveRoomState({ live }: { live: BilibiliLiveRoom }) {
  const { t } = useAppSettings()
  const state = live.liveStatus == null ? null : roomStates[live.liveStatus]
  return state ? (
    <Badge tone={state.tone} icon={RadioTower}>
      {t(state.key)}
    </Badge>
  ) : null
}

const future = (value: string | null | undefined) =>
  Boolean(value && Date.parse(value) > Date.now())

/** A Japan time with how long ago or until it is, or null. */
function useStamp() {
  const { locale } = useAppSettings()
  return (value: string | null | undefined) =>
    value ? `${japanStamp(value)} · ${formatRelative(value, locale)}` : null
}

/** An error code explained, with the code as its tooltip, or nothing. */
function ErrorText({ code }: { code: string | null | undefined }) {
  const { t } = useAppSettings()
  return code ? <span title={code}>{explainCode(code, t) ?? code}</span> : null
}

/** The live room's last check, as the live-check panel lists it. */
export function LiveRoomFacts({ live }: { live: BilibiliLiveRoom }) {
  const { t } = useAppSettings()
  const stamp = useStamp()
  return (
    <Facts
      items={[
        [
          t("admin.bilibili.live.room"),
          live.roomId ? (
            <a
              className="adm-link"
              href={`https://live.bilibili.com/${encodeURIComponent(live.roomId)}`}
              target="_blank"
              rel="noreferrer"
            >
              {live.roomId}
            </a>
          ) : null,
        ],
        [t("admin.bilibili.live.streamTitle"), live.title],
        [
          t("admin.bilibili.live.liveTime"),
          live.liveTime
            ? stamp(live.liveTime)
            : live.liveStatus === 1
              ? t("admin.bilibili.live.startUnknown")
              : null,
        ],
        [t("admin.bilibili.live.checkedAt"), stamp(live.checkedAt)],
        [t("admin.bilibili.live.lastLiveAt"), stamp(live.lastLiveAt)],
        [
          t("admin.bilibili.live.blockedUntil"),
          future(live.blockedUntil) ? stamp(live.blockedUntil) : null,
        ],
        [
          t("admin.bilibili.lastError"),
          live.lastError ? <ErrorText code={live.lastError} /> : null,
        ],
      ]}
    />
  )
}

/**
 * The login cookie as both reads send it: `signedIn` only says a cookie is
 * configured, `login` is its last check.
 */
function loginOf(status: Sources["bilibili"] | undefined): BilibiliLogin {
  return status?.login ?? (status?.signedIn ? "signed_in" : "not_configured")
}

function LoginBadge({ login }: { login: BilibiliLogin }) {
  const { t } = useAppSettings()
  const state = loginStates[login] ?? loginStates.not_configured
  return (
    <Badge tone={state.tone} icon={KeyRound}>
      {t(state.key)}
    </Badge>
  )
}

/** Whether the operator's single reads can run: the source is on here. */
const usableOf = (status: Sources["bilibili"] | undefined) =>
  Boolean(status?.available && !status.skipped)

/** The login cookie both Bilibili reads send, as a chip of the summary. */
export function BilibiliLoginBadge({
  status,
}: {
  status: Sources["bilibili"] | undefined
}) {
  return status?.available ? <LoginBadge login={loginOf(status)} /> : null
}

/**
 * What a Bilibili read needs of its login cookie, under the summary: how to
 * set one while none is configured (the uploads read anonymously, the
 * dynamics listing not at all) and that an expired one needs replacing.
 */
export function BilibiliLoginNote({
  status,
  dynamics,
}: {
  status: Sources["bilibili"] | undefined
  dynamics: boolean
}) {
  const { t } = useAppSettings()
  if (!status?.available) return null
  const login = loginOf(status)
  const key =
    login === "expired"
      ? "admin.bilibili.loginExpiredHint"
      : login === "not_configured" && dynamics
        ? "admin.bilibili.dynamics.loginHint"
        : null
  return key ? (
    <p className="adm-source-note">
      <KeyRound aria-hidden />
      {t(key)}
    </p>
  ) : null
}

/**
 * The operator's single read beside a source's manual fetch: one form,
 * disabled while the source does not run here. A Bilibili read respects the
 * shared captcha backoff and offers one forced read after a refusal.
 */
export function AddPanel({
  usable,
  title,
  ...form
}: {
  /** The source runs here; otherwise the panel is marked skipped. */
  usable: boolean
  title: string
  label: string
  hint?: string
  placeholder: string
  notice: string
  send: (input: string, force: boolean) => Promise<unknown>
  onAdded: () => void
}) {
  const { t } = useAppSettings()
  return (
    <Panel
      icon={Plus}
      title={title}
      actions={usable ? null : <Badge>{t("admin.overview.skipped")}</Badge>}
    >
      <AddForm {...form} disabled={!usable} />
    </Panel>
  )
}

/** Adds or re-reads one upload of the account by BV ID or video link. */
export function BilibiliAddUpload({
  status,
  onAdded,
}: {
  status: Sources["bilibili"] | undefined
  onAdded: () => void
}) {
  const { t } = useAppSettings()
  const { api } = useAdmin()
  return (
    <AddPanel
      usable={usableOf(status)}
      title={t("admin.bilibili.add")}
      label={t("admin.bilibili.addField")}
      placeholder="BV1xxxxxxxxx"
      notice="admin.notice.bilibiliAdded"
      send={(input, force) => api.addBilibiliVideo(input, force)}
      onAdded={onAdded}
    />
  )
}

/** Adds or re-reads one dynamic of the account by its link; needs no login. */
export function BilibiliAddDynamic({
  status,
  onAdded,
}: {
  status: Sources["bilibili"] | undefined
  onAdded: () => void
}) {
  const { t } = useAppSettings()
  const { api } = useAdmin()
  return (
    <AddPanel
      usable={usableOf(status)}
      title={t("admin.bilibili.addPost")}
      label={t("admin.bilibili.addPostField")}
      placeholder="https://www.bilibili.com/opus/…"
      notice="admin.notice.bilibiliPostAdded"
      send={(input, force) => api.addBilibiliPost(input, force)}
      onAdded={onAdded}
    />
  )
}

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
        <p className="adm-note">{note ?? t("admin.biliLogin.hint")}</p>
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

const qrTones: Record<BilibiliQRState, Tone> = {
  waiting: "neutral",
  scanned: "honey",
  signed_in: "leaf",
  expired: "neutral",
  failed: "danger",
}

/**
 * The login both Bilibili reads send: where it comes from and its checks,
 * and for the login the server keeps, its import, QR sign-in and forget.
 */
export function BilibiliLoginPanel({
  status,
  onChange,
}: {
  status: Sources["bilibili"]
  onChange: (status: Sources["bilibili"]) => void
}) {
  const { t } = useAppSettings()
  const stamp = useStamp()
  const [dialog, setDialog] = useState<"" | "import" | "qr" | "forget">("")
  if (!status.available) return null
  const stored = status.loginSource === "stored"
  const unreadable = status.loginUnreadable === true
  const source = status.loginSource
    ? [
        t(`admin.bilibili.login.source.${status.loginSource}`),
        status.loginVia ? t(`admin.bilibili.login.via.${status.loginVia}`) : "",
      ]
        .filter(Boolean)
        .join(" · ")
    : null
  const items: [ReactNode, ReactNode][] = [
    [t("admin.bilibili.login.source"), source],
    [t("admin.bilibili.login.name"), status.loginName ?? null],
    [t("admin.bilibili.login.checked"), stamp(status.loginCheckedAt)],
  ]
  if (stored)
    items.push(
      [t("admin.bilibili.login.refreshed"), stamp(status.loginRefreshedAt)],
      [
        t("admin.bilibili.login.refreshChecked"),
        stamp(status.loginRefreshCheckedAt),
      ],
    )
  if (unreadable)
    items.push([
      t("admin.bilibili.login.stored"),
      <Badge tone="danger">{t("admin.bilibili.login.unreadable")}</Badge>,
    ])
  if (stored && status.loginRefreshError)
    items.push([
      t("admin.bilibili.login.refreshError"),
      <ErrorText code={status.loginRefreshError} />,
    ])
  const close = () => setDialog("")
  const changed = (next: Sources["bilibili"]) => {
    setDialog("")
    onChange(next)
  }
  return (
    <Panel
      level={3}
      icon={KeyRound}
      title={t("admin.bilibili.login.title")}
      actions={<LoginBadge login={loginOf(status)} />}
    >
      <div className="adm-form">
        <Facts items={items} />
        {status.loginStorable ? (
          <div className="adm-form-actions">
            {stored || unreadable ? (
              <Button
                size="sm"
                variant="ghost"
                icon={Trash2}
                onClick={() => setDialog("forget")}
              >
                {t("admin.bilibili.login.forget")}
              </Button>
            ) : null}
            <Button
              size="sm"
              variant="ghost"
              icon={ClipboardPaste}
              onClick={() => setDialog("import")}
            >
              {t("admin.bilibili.login.import")}
            </Button>
            <Button size="sm" icon={QrCode} onClick={() => setDialog("qr")}>
              {t("admin.bilibili.login.qr")}
            </Button>
          </div>
        ) : null}
      </div>
      {dialog === "import" ? (
        <ImportLoginDialog onClose={close} onSaved={changed} />
      ) : null}
      {dialog === "forget" ? (
        <ForgetLoginDialog
          signOut={!unreadable}
          onClose={close}
          onForgotten={changed}
        />
      ) : null}
      {dialog === "qr" ? (
        <QRSignInDialog onClose={close} onSignedIn={changed} />
      ) : null}
    </Panel>
  )
}

/** Stores a private window's Cookie header and its ac_time_value. */
function ImportLoginDialog({
  onClose,
  onSaved,
}: {
  onClose: () => void
  onSaved: (status: Sources["bilibili"]) => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [cookie, setCookie] = useState("")
  const [refreshToken, setRefreshToken] = useState("")
  const [busy, setBusy] = useState(false)
  const save = async (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault()
    setBusy(true)
    const next = await act(
      () =>
        api.importBilibiliLogin({
          cookie: cookie.trim(),
          refreshToken: refreshToken.trim(),
        }),
      "admin.notice.bilibiliLoginImported",
    )
    setBusy(false)
    if (next) onSaved(next)
  }
  return (
    <Modal open title={t("admin.bilibili.login.importTitle")} onClose={onClose}>
      <form className="adm-form" onSubmit={save}>
        <Field label={t("admin.bilibili.login.cookie")}>
          <textarea
            className="adm-login-secret"
            rows={4}
            value={cookie}
            onChange={(event) => setCookie(event.target.value)}
            spellCheck={false}
            autoCapitalize="off"
            autoComplete="off"
            required
          />
        </Field>
        <Field label={t("admin.bilibili.login.refreshToken")}>
          <input
            className="adm-login-secret"
            type="password"
            value={refreshToken}
            onChange={(event) => setRefreshToken(event.target.value)}
            spellCheck={false}
            autoCapitalize="off"
            autoComplete="off"
            required
          />
        </Field>
        <div className="adm-form-actions">
          <Button
            type="submit"
            variant="primary"
            size="sm"
            icon={ClipboardPaste}
            busy={busy}
            disabled={!cookie.trim() || !refreshToken.trim()}
          >
            {t("admin.bilibili.login.save")}
          </Button>
        </div>
      </form>
    </Modal>
  )
}

/**
 * Drops the stored login, or signs it out at Bilibili first; a login that
 * no longer decrypts can only be dropped.
 */
function ForgetLoginDialog({
  signOut: canSignOut,
  onClose,
  onForgotten,
}: {
  signOut: boolean
  onClose: () => void
  onForgotten: (status: Sources["bilibili"]) => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [busy, setBusy] = useState<"" | "forget" | "signOut">("")
  const forget = async (signOut: boolean) => {
    setBusy(signOut ? "signOut" : "forget")
    const next = await act(
      () => api.forgetBilibiliLogin(signOut),
      "admin.notice.bilibiliLoginForgotten",
    )
    setBusy("")
    if (next) onForgotten(next)
  }
  return (
    <Modal open title={t("admin.bilibili.login.forgetTitle")} onClose={onClose}>
      <div className="adm-form-actions adm-login-choice">
        <Button
          size="sm"
          variant="outline"
          icon={Trash2}
          busy={busy === "forget"}
          disabled={busy !== ""}
          onClick={() => void forget(false)}
        >
          {t("admin.bilibili.login.forgetOnly")}
        </Button>
        {canSignOut ? (
          <Button
            size="sm"
            variant="danger"
            icon={LogOut}
            busy={busy === "signOut"}
            disabled={busy !== ""}
            onClick={() => void forget(true)}
          >
            {t("admin.bilibili.login.signOut")}
          </Button>
        ) : null}
      </div>
    </Modal>
  )
}

/** How often the console reads the server's QR sign-in. */
const qrReadInterval = 2000

/**
 * The server's QR sign-in: the dialog starts one, reads its local state
 * until it ends, and cancels it when closed while it waits.
 */
function QRSignInDialog({
  onClose,
  onSignedIn,
}: {
  onClose: () => void
  onSignedIn: (status: Sources["bilibili"]) => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [qr, setQR] = useState<BilibiliQR | null>(null)
  const [round, setRound] = useState(0)
  // Each round starts one sign-in, and its success is handed on once.
  const started = useRef(-1)
  const finished = useRef(false)
  const open = qr?.state === "waiting" || qr?.state === "scanned"
  const apply = (next: BilibiliQR | undefined) => {
    if (!next) return
    setQR(next)
    if (next.state !== "signed_in" || finished.current) return
    finished.current = true
    void act(() => api.sources(), "admin.notice.bilibiliSignedIn").then(
      (sources) => {
        if (sources) onSignedIn(sources.bilibili)
      },
    )
  }
  useEffect(() => {
    if (started.current === round) return
    started.current = round
    void act(() => api.startBilibiliQR()).then(apply)
  })
  useEffect(() => {
    if (!open) return
    const timer = window.setInterval(() => {
      void api
        .bilibiliQR()
        .then(apply)
        .catch(() => {})
    }, qrReadInterval)
    return () => window.clearInterval(timer)
  })
  const close = () => {
    if (open) void api.cancelBilibiliQR().catch(() => {})
    onClose()
  }
  const state = qr?.state ?? null
  return (
    <Modal open title={t("admin.bilibili.login.qrTitle")} onClose={close}>
      <div className="adm-login-qr">
        {qr?.qr ? (
          <svg
            className="adm-qr"
            viewBox={`0 0 ${qr.qr.size} ${qr.qr.size}`}
            role="img"
            aria-label={t("admin.bilibili.login.qrCode")}
            shapeRendering="crispEdges"
          >
            <path d={qr.qr.path} />
          </svg>
        ) : (
          <div className="adm-qr is-empty" aria-hidden>
            <QrCode />
          </div>
        )}
        {state ? (
          <Badge tone={qrTones[state]}>
            {t(`admin.bilibili.login.qrState.${state}`)}
          </Badge>
        ) : null}
        {qr?.error ? <ErrorText code={qr.error} /> : null}
        {state === "expired" || state === "failed" ? (
          <Button
            size="sm"
            icon={RotateCw}
            onClick={() => {
              setQR(null)
              setRound((value) => value + 1)
            }}
          >
            {t("admin.bilibili.login.qrAgain")}
          </Button>
        ) : null}
      </div>
    </Modal>
  )
}
