import { useState, type ReactNode, type SubmitEvent } from "react"
import {
  KeyRound,
  MessageSquareText,
  Plus,
  RadioTower,
  ShieldAlert,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { ApiError } from "@/admin/api"
import { formatRelative, japanStamp } from "@/admin/format"
import type { BilibiliLiveRoom, BilibiliLogin, Sources } from "@/admin/types"
import { Badge, Button, Facts, Field, Panel, type Tone } from "@/admin/ui"
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
      title={t("admin.bilibili.forceHint", {
        time: pending.until ? japanStamp(pending.until) : "—",
      })}
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
  hint: string
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

/** The badges both Bilibili panels lead with: skipped and the backoff. */
function StateBadges({
  status,
  children,
}: {
  status: Sources["bilibili"]
  children?: ReactNode
}) {
  const { t } = useAppSettings()
  return (
    <span className="adm-chip-row">
      {!status.available || status.skipped ? (
        <Badge>{t("admin.overview.skipped")}</Badge>
      ) : null}
      {children}
      {future(status.blockedUntil) ? (
        <Badge tone="danger" icon={ShieldAlert}>
          {t("admin.bilibili.blocked")}
        </Badge>
      ) : null}
    </span>
  )
}

/** Whether the operator's single reads can run: the source is on here. */
const usableOf = (status: Sources["bilibili"] | undefined) =>
  Boolean(status?.available && !status.skipped)

/**
 * The upload read's state on the Sources page, below its timer: the
 * account, what is stored, the read interval, the last read error and the
 * login cookie the read sends, with whether the source runs here and the
 * captcha backoff. Uploads stay in the console; one merged into its YouTube
 * copy is listed on that video.
 */
export function BilibiliUploadsStatus({
  status,
}: {
  status: Sources["bilibili"] | undefined
}) {
  const { t } = useAppSettings()
  const login = loginOf(status)
  return (
    <Panel
      level={3}
      icon={BilibiliIcon}
      title={t("admin.bilibili.uploadsTitle")}
      description={t("admin.bilibili.description")}
      actions={
        status ? (
          <StateBadges status={status}>
            <LoginBadge login={login} />
          </StateBadges>
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
            [
              t("admin.bilibili.lastError"),
              status.lastError ? <ErrorText code={status.lastError} /> : null,
            ],
          ]}
        />
      ) : null}
      {status?.available && login === "not_configured" ? (
        <p className="adm-note">{t("admin.bilibili.signInHint")}</p>
      ) : null}
      {status?.available && login === "expired" ? (
        <p className="adm-note">{t("admin.bilibili.loginExpiredHint")}</p>
      ) : null}
    </Panel>
  )
}

/**
 * The dynamics listing's state on the Sources page, below its timer: the
 * account, the login cookie and its last check, and the listing's last
 * read, with whether the source runs here and the captcha backoff it shares
 * with the upload read. Dynamics show as posts beside X posts on the board.
 */
export function BilibiliDynamicsStatus({
  status,
}: {
  status: Sources["bilibili"] | undefined
}) {
  const { t } = useAppSettings()
  const stamp = useStamp()
  const login = loginOf(status)
  const dynamics = status?.dynamics
  const facts: [ReactNode, ReactNode][] = status?.available
    ? [
        [t("admin.bilibili.account"), status.account],
        [
          t("admin.bilibili.loginChecked"),
          stamp(status.loginCheckedAt ?? null),
        ],
      ]
    : []
  if (status?.available && dynamics)
    facts.push(
      [t("admin.bilibili.dynamics.posts"), String(dynamics.posts)],
      [t("admin.bilibili.dynamics.lastAttempt"), stamp(dynamics.lastAttemptAt)],
      [t("admin.bilibili.dynamics.lastSuccess"), stamp(dynamics.lastSuccessAt)],
      [
        t("admin.bilibili.dynamics.skipped"),
        dynamics.skipped
          ? t(`admin.bilibili.dynamics.skipped.${dynamics.skipped}`)
          : null,
      ],
      [
        t("admin.bilibili.lastError"),
        dynamics.lastError ? <ErrorText code={dynamics.lastError} /> : null,
      ],
    )
  return (
    <Panel
      level={3}
      icon={MessageSquareText}
      title={t("admin.bilibili.dynamics.title")}
      description={t("admin.bilibili.dynamics.description")}
      actions={
        status ? (
          <StateBadges status={status}>
            <LoginBadge login={login} />
          </StateBadges>
        ) : null
      }
    >
      {facts.length ? <Facts items={facts} /> : null}
      {status?.available && login === "not_configured" ? (
        <p className="adm-note">{t("admin.bilibili.dynamics.loginHint")}</p>
      ) : null}
      {status?.available && login === "expired" ? (
        <p className="adm-note">{t("admin.bilibili.loginExpiredHint")}</p>
      ) : null}
    </Panel>
  )
}

/**
 * The operator's single read beside a Bilibili timer, as the manual fetch
 * sits beside X's and YouTube's: one form, disabled while the source does
 * not run here. It respects the shared captcha backoff and offers one
 * forced read after a refusal.
 */
function AddPanel({
  status,
  title,
  ...form
}: {
  status: Sources["bilibili"] | undefined
  title: string
  label: string
  hint: string
  placeholder: string
  notice: string
  send: (input: string, force: boolean) => Promise<unknown>
  onAdded: () => void
}) {
  const { t } = useAppSettings()
  const usable = usableOf(status)
  return (
    <Panel
      icon={Plus}
      title={title}
      actions={
        status && !usable ? <Badge>{t("admin.overview.skipped")}</Badge> : null
      }
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
      status={status}
      title={t("admin.bilibili.add")}
      label={t("admin.bilibili.addField")}
      hint={t("admin.bilibili.addHint")}
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
      status={status}
      title={t("admin.bilibili.addPost")}
      label={t("admin.bilibili.addPostField")}
      hint={t("admin.bilibili.addPostHint")}
      placeholder="https://www.bilibili.com/opus/…"
      notice="admin.notice.bilibiliPostAdded"
      send={(input, force) => api.addBilibiliPost(input, force)}
      onAdded={onAdded}
    />
  )
}
