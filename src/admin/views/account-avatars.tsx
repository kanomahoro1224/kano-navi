import { useRef, useState } from "react"
import {
  Check,
  Download,
  ImageOff,
  Link2,
  Plus,
  RefreshCw,
  Trash2,
  Upload,
  UserCheck,
  UsersRound,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { japanStamp } from "@/admin/format"
import type {
  AccountAvatarCandidate,
  AccountAvatarItem,
  AccountAvatarMode,
  AccountAvatarMutation,
  AccountAvatarRole,
} from "@/admin/types"
import {
  Badge,
  Button,
  ConfirmButton,
  Empty,
  Field,
  Loading,
  Modal,
  PageHeader,
  Panel,
  Segmented,
  Spinner,
  StatusBadge,
} from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"

const UPLOAD_TYPES = ["image/png", "image/jpeg", "image/webp", "image/gif"]
const MODES: AccountAvatarMode[] = ["auto", "manual", "hidden"]

type FilterRole = "all" | AccountAvatarRole

const ROLE_TONES: Record<AccountAvatarRole, "berry" | "sky" | "neutral"> = {
  own: "berry",
  related: "sky",
  other: "neutral",
}

function candidateMeta(candidate: AccountAvatarCandidate): string[] {
  return [
    candidate.width && candidate.height
      ? `${candidate.width}×${candidate.height}`
      : "",
    japanStamp(candidate.updatedAt),
  ].filter(Boolean)
}

function activeReadyCandidate(
  item: AccountAvatarItem,
): AccountAvatarCandidate | null {
  if (!item.activeCandidateId) return null
  const found = item.candidates.find(
    (candidate) => candidate.id === item.activeCandidateId,
  )
  return found && found.status === "ready" ? found : null
}

function AvatarViewer({
  handle,
  candidate,
  onClose,
}: {
  handle: string
  candidate: AccountAvatarCandidate
  onClose: () => void
}) {
  const { t } = useAppSettings()
  return (
    <Modal
      open
      className="adm-image-viewer"
      title={`@${handle} · ${t(`admin.avatars.origin.${candidate.origin}`)}`}
      onClose={onClose}
    >
      <div className="adm-image-viewer-stage">
        <img src={candidate.previewUrl ?? ""} alt="" />
      </div>
      <div className="adm-image-viewer-nav">
        {candidateMeta(candidate).join(" · ")}
      </div>
    </Modal>
  )
}

function AddAvatarDialog({
  item,
  onUpdated,
  onClose,
}: {
  item: AccountAvatarItem
  onUpdated: (items: AccountAvatarItem[]) => void
  onClose: () => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [url, setUrl] = useState("")
  const [over, setOver] = useState(false)
  const [busy, setBusy] = useState("")
  const file = useRef<HTMLInputElement>(null)

  const perform = async (
    key: string,
    work: () => Promise<AccountAvatarMutation>,
    notice: string,
  ) => {
    setBusy(key)
    const next = await act(work, notice)
    setBusy("")
    if (!next) return
    onUpdated(next.items)
    onClose()
  }

  const upload = (chosen: File | undefined) => {
    if (chosen && UPLOAD_TYPES.includes(chosen.type)) {
      void perform(
        "upload",
        () => api.uploadAccountAvatar(item.source, item.accountKey, chosen),
        "admin.notice.avatarUploaded",
      )
    }
  }

  return (
    <Modal
      open
      className="adm-profile-dialog"
      title={`@${item.accountHandle}`}
      onClose={onClose}
    >
      <div className="adm-profile-add">
        <form
          className="adm-inline-form"
          onSubmit={(event) => {
            event.preventDefault()
            void perform(
              "url",
              () =>
                api.addAccountAvatarUrl(
                  item.source,
                  item.accountKey,
                  url.trim(),
                ),
              "admin.notice.avatarAdded",
            )
          }}
        >
          <Field label={t("admin.avatars.addUrl")}>
            <input
              type="url"
              value={url}
              onChange={(event) => setUrl(event.target.value)}
              placeholder="https://pbs.twimg.com/..."
              aria-describedby={`avatar-url-hint-${item.accountKey}`}
              required
            />
          </Field>
          <Button type="submit" size="sm" icon={Link2} busy={busy === "url"}>
            {t("admin.avatars.addUrlAction")}
          </Button>
        </form>
        <small
          id={`avatar-url-hint-${item.accountKey}`}
          className="adm-field-hint"
        >
          {t("admin.avatars.addUrlHint")}
        </small>
        <span className="adm-profile-or">{t("admin.profile.or")}</span>
        <input
          ref={file}
          type="file"
          accept={UPLOAD_TYPES.join(",")}
          hidden
          onChange={(event) => {
            const chosen = event.target.files?.[0]
            event.target.value = ""
            upload(chosen)
          }}
        />
        <button
          type="button"
          className={`adm-profile-drop${over ? " is-over" : ""}`}
          disabled={busy === "upload"}
          onClick={() => file.current?.click()}
          onDragOver={(event) => {
            event.preventDefault()
            event.dataTransfer.dropEffect = "copy"
            setOver(true)
          }}
          onDragLeave={() => setOver(false)}
          onDrop={(event) => {
            event.preventDefault()
            setOver(false)
            upload(event.dataTransfer.files[0])
          }}
        >
          {busy === "upload" ? <Spinner /> : <Upload aria-hidden />}
          <strong>{t("admin.profile.dropTitle")}</strong>
          <small>{t("admin.profile.dropHint")}</small>
        </button>
      </div>
    </Modal>
  )
}

function AvatarCandidateCard({
  item,
  candidate,
  busyKey,
  onAction,
}: {
  item: AccountAvatarItem
  candidate: AccountAvatarCandidate
  busyKey: string
  onAction: (
    key: string,
    work: () => Promise<AccountAvatarMutation>,
    notice: string,
  ) => void
}) {
  const { t } = useAppSettings()
  const { api } = useAdmin()
  const [viewing, setViewing] = useState(false)
  const ready = candidate.status === "ready"
  const active = candidate.isActive

  return (
    <article className={`adm-candidate is-row${active ? " is-active" : ""}`}>
      <div className="adm-candidate-preview">
        {candidate.previewUrl ? (
          <button
            type="button"
            className="media-opener"
            aria-label={t("admin.avatars.view")}
            onClick={() => setViewing(true)}
          >
            <img src={candidate.previewUrl} alt="" loading="lazy" />
          </button>
        ) : (
          <ImageOff aria-hidden />
        )}
      </div>
      <div className="adm-candidate-body">
        <div className="adm-chip-row">
          <Badge>{t(`admin.avatars.origin.${candidate.origin}`)}</Badge>
          {ready ? null : <StatusBadge status={candidate.status} />}
        </div>
        <small className="adm-candidate-meta">
          {candidateMeta(candidate).map((part) => (
            <span key={part}>{part}</span>
          ))}
        </small>
        {candidate.lastError ? (
          <small className="adm-error-text" title={candidate.lastError}>
            {candidate.lastError}
          </small>
        ) : null}
      </div>
      <div className="adm-candidate-actions">
        <Button
          size="sm"
          variant={active ? "ghost" : undefined}
          icon={Check}
          disabled={!ready || (active && item.mode === "manual")}
          busy={busyKey === `select:${candidate.id}`}
          onClick={() =>
            onAction(
              `select:${candidate.id}`,
              () =>
                api.selectAccountAvatarCandidate(
                  item.source,
                  item.accountKey,
                  candidate.id,
                ),
              "admin.notice.avatarSelected",
            )
          }
        >
          {active && item.mode === "manual"
            ? t("admin.avatars.active")
            : t("admin.avatars.use")}
        </Button>
        {candidate.origin !== "upload" && candidate.sourceUrl ? (
          <Button
            size="sm"
            variant="ghost"
            icon={Download}
            busy={busyKey === `download:${candidate.id}`}
            onClick={() =>
              onAction(
                `download:${candidate.id}`,
                () =>
                  api.downloadAccountAvatarCandidate(
                    item.source,
                    item.accountKey,
                    candidate.id,
                  ),
                "admin.notice.avatarDownloaded",
              )
            }
          >
            {ready ? t("admin.avatars.refresh") : t("admin.avatars.download")}
          </Button>
        ) : null}
        <ConfirmButton
          size="sm"
          variant="ghost"
          icon={Trash2}
          confirmLabel={t("admin.avatars.deleteConfirm")}
          busy={busyKey === `delete:${candidate.id}`}
          onConfirm={() =>
            onAction(
              `delete:${candidate.id}`,
              () =>
                api.deleteAccountAvatarCandidate(
                  item.source,
                  item.accountKey,
                  candidate.id,
                ),
              "admin.notice.avatarDeleted",
            )
          }
        >
          {t("admin.avatars.delete")}
        </ConfirmButton>
      </div>
      {viewing ? (
        <AvatarViewer
          handle={item.accountHandle}
          candidate={candidate}
          onClose={() => setViewing(false)}
        />
      ) : null}
    </article>
  )
}

function AccountAvatarCard({
  item,
  onUpdated,
}: {
  item: AccountAvatarItem
  onUpdated: (items: AccountAvatarItem[]) => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [busyKey, setBusyKey] = useState("")
  const [adding, setAdding] = useState(false)
  const activeReady = activeReadyCandidate(item)
  const initial = (item.accountHandle || "X").charAt(0).toUpperCase()

  const run = async (
    key: string,
    work: () => Promise<AccountAvatarMutation>,
    notice: string,
  ) => {
    setBusyKey(key)
    const next = await act(work, notice)
    setBusyKey("")
    if (next) onUpdated(next.items)
  }

  return (
    <Panel className="adm-avatar-account" level={3}>
      <div className="adm-avatar-account-head">
        <div className="adm-avatar-identity">
          <div
            className={`adm-avatar-thumb${item.mode === "hidden" ? " is-hidden" : ""}`}
            aria-hidden
          >
            {activeReady?.previewUrl && item.mode !== "hidden" ? (
              <img src={activeReady.previewUrl} alt="" />
            ) : (
              <span>{initial}</span>
            )}
          </div>
          <div className="adm-avatar-identity-text">
            <div className="adm-avatar-title-row">
              <strong>@{item.accountHandle}</strong>
              <Badge tone={ROLE_TONES[item.role]}>
                {t(`admin.avatars.role.${item.role}`)}
              </Badge>
            </div>
            {item.updatedAt || item.keywords.length ? (
              <div className="adm-avatar-subline">
                {item.updatedAt ? (
                  <span>{japanStamp(item.updatedAt)}</span>
                ) : null}
                {item.keywords.length ? (
                  <span className="adm-chip-row">
                    {item.keywords.map((keyword) => (
                      <Badge key={keyword}>{keyword}</Badge>
                    ))}
                  </span>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>
        <div className="adm-avatar-controls">
          <Segmented
            value={item.mode}
            label={`${t("admin.avatars.mode")} @${item.accountHandle}`}
            options={MODES.map((mode) => ({
              value: mode,
              label: t(`admin.avatars.mode.${mode}`),
            }))}
            onChange={(mode) => {
              if (mode === item.mode || busyKey) return
              void run(
                `mode:${mode}`,
                () =>
                  api.setAccountAvatarMode(item.source, item.accountKey, mode),
                "admin.notice.avatarModeSaved",
              )
            }}
          />
          <Button
            size="sm"
            icon={RefreshCw}
            busy={busyKey === "discover"}
            onClick={() =>
              void run(
                "discover",
                () => api.discoverAccountAvatar(item.source, item.accountKey),
                "admin.notice.avatarDiscovered",
              )
            }
          >
            {t("admin.avatars.discover")}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            icon={Plus}
            onClick={() => setAdding(true)}
          >
            {t("admin.avatars.upload")}
          </Button>
          {item.role === "own" && activeReady ? (
            <Button
              size="sm"
              variant="ghost"
              icon={UserCheck}
              busy={busyKey === "apply-profile"}
              onClick={() =>
                void run(
                  "apply-profile",
                  () =>
                    api.applyAccountAvatarToProfile(
                      item.source,
                      item.accountKey,
                      activeReady.id,
                    ),
                  "admin.notice.avatarAppliedProfile",
                )
              }
            >
              {t("admin.avatars.applyProfile")}
            </Button>
          ) : null}
        </div>
      </div>

      {item.candidates.length ? (
        <div className="adm-avatar-history">
          <div className="adm-candidate-rows">
            {item.candidates.map((candidate) => (
              <AvatarCandidateCard
                key={candidate.id}
                item={item}
                candidate={candidate}
                busyKey={busyKey}
                onAction={(key, work, notice) => void run(key, work, notice)}
              />
            ))}
          </div>
        </div>
      ) : null}

      {adding ? (
        <AddAvatarDialog
          item={item}
          onUpdated={onUpdated}
          onClose={() => setAdding(false)}
        />
      ) : null}
    </Panel>
  )
}

export function AccountAvatarsView(_: ViewProps) {
  const { t } = useAppSettings()
  const { api } = useAdmin()
  const resource = useResource(() => api.accountAvatars(), [])
  const [filter, setFilter] = useState<FilterRole>("all")
  const items = resource.data?.items ?? []

  const counts = {
    all: items.length,
    own: items.filter((item) => item.role === "own").length,
    related: items.filter((item) => item.role === "related").length,
    other: items.filter((item) => item.role === "other").length,
  }
  const visible =
    filter === "all" ? items : items.filter((item) => item.role === filter)

  const filterOptions: { value: FilterRole; label: string; count: number }[] = [
    { value: "all", label: t("admin.avatars.filter.all"), count: counts.all },
    { value: "own", label: t("admin.avatars.filter.own"), count: counts.own },
    {
      value: "related",
      label: t("admin.avatars.filter.related"),
      count: counts.related,
    },
  ]
  if (counts.other > 0) {
    filterOptions.push({
      value: "other",
      label: t("admin.avatars.filter.other"),
      count: counts.other,
    })
  }

  return (
    <div className="adm-view">
      <PageHeader
        title={t("admin.avatars.title")}
        actions={
          resource.data ? (
            <Segmented
              value={filter}
              label={t("admin.avatars.title")}
              options={filterOptions}
              onChange={setFilter}
            />
          ) : undefined
        }
      />
      {!resource.data ? (
        <Loading />
      ) : visible.length ? (
        <div className="adm-grid">
          {visible.map((item) => (
            <AccountAvatarCard
              key={`${item.source}:${item.accountKey}`}
              item={item}
              onUpdated={(nextItems) => resource.setData({ items: nextItems })}
            />
          ))}
        </div>
      ) : (
        <Panel>
          <Empty icon={UsersRound} title={t("admin.avatars.empty")}>
            {t("admin.avatars.emptyHint")}
          </Empty>
        </Panel>
      )}
    </div>
  )
}
