import { useRef, useState, type ReactNode } from "react"
import {
  Check,
  Download,
  Eye,
  ImageOff,
  Images,
  Link2,
  Plus,
  RectangleHorizontal,
  RotateCcw,
  Search,
  Trash2,
  Upload,
  UserRound,
  UsersRound,
} from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { japanStamp } from "@/admin/format"
import type {
  AccountAvatarCandidate,
  AccountAvatarItem,
  AccountAvatarsState,
  ProfileMedia,
  ProfileMediaState,
  ProfileSlot,
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
  Spinner,
  StatusBadge,
} from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"

const SLOTS: ProfileSlot[] = ["avatar", "banner"]

const SLOT_ICONS = { avatar: UserRound, banner: RectangleHorizontal }

/** What the board shows while no candidate is selected for a slot. */
const BUILT_IN: Record<ProfileSlot, string> = {
  avatar: "/assets/kano-avatar.jpg",
  banner: "/assets/kano-banner.jpg",
}

/** Size and time of a candidate, the muted line under its image. */
function useMeta() {
  useAppSettings()
  return (item: {
    width: number | null
    height: number | null
    updatedAt: string
  }) =>
    [
      item.width && item.height ? `${item.width}×${item.height}` : "",
      japanStamp(item.updatedAt),
    ].filter(Boolean)
}

/** Sort slot candidates with the active item first, YouTube avatars before custom ones, then newest. */
function orderedForSlot(items: ProfileMedia[], slot: ProfileSlot) {
  return [...items].sort((a, b) => {
    if (a.isActive !== b.isActive) return a.isActive ? -1 : 1
    if (slot === "avatar" && a.source !== b.source) {
      if (a.source === "youtube") return -1
      if (b.source === "youtube") return 1
    }
    return b.updatedAt.localeCompare(a.updatedAt)
  })
}

function Viewer({
  title,
  previewUrl,
  metaParts,
  onClose,
}: {
  title: string
  previewUrl: string
  metaParts: string[]
  onClose: () => void
}) {
  return (
    <Modal open className="adm-image-viewer" title={title} onClose={onClose}>
      <div className="adm-image-viewer-stage">
        <img src={previewUrl} alt="" />
      </div>
      {metaParts.length ? (
        <div className="adm-image-viewer-nav">{metaParts.join(" · ")}</div>
      ) : null}
    </Modal>
  )
}

/** The board's profile card drawn with the images in use, and what each is. */
function Preview({
  state,
  onState,
  onManageAvatars,
}: {
  state: ProfileMediaState
  onState: (state: ProfileMediaState) => void
  onManageAvatars: () => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const meta = useMeta()
  const [clearing, setClearing] = useState<ProfileSlot | null>(null)
  const url = (slot: ProfileSlot) =>
    state.active[slot]?.previewUrl || BUILT_IN[slot]
  const clearSlot = async (slot: ProfileSlot) => {
    setClearing(slot)
    const next = await act(
      () => api.clearProfileMedia(slot),
      "admin.notice.mediaCleared",
    )
    setClearing(null)
    if (next) onState({ items: next.items, active: next.active })
  }
  return (
    <Panel
      icon={Eye}
      title={t("admin.profile.preview")}
      actions={
        <Button
          size="sm"
          variant="ghost"
          icon={UsersRound}
          onClick={onManageAvatars}
        >
          {t("admin.profile.manageAvatars")}
        </Button>
      }
      className="adm-profile-preview-panel"
    >
      <div className="adm-profile-preview">
        <div className="adm-profile-card" aria-hidden>
          <div className="adm-profile-card-banner">
            <img src={url("banner")} alt="" />
          </div>
          <img className="adm-profile-card-avatar" src={url("avatar")} alt="" />
          <span className="adm-profile-card-line" />
          <span className="adm-profile-card-line is-short" />
        </div>
        <dl className="adm-profile-slots">
          {SLOTS.map((slot) => {
            const active = state.active[slot]
            const SlotIcon = SLOT_ICONS[slot]
            return (
              <div key={slot}>
                <dt>
                  <SlotIcon aria-hidden />
                  <strong>{t(`admin.profile.${slot}`)}</strong>
                </dt>
                <dd>
                  {active?.previewUrl ? (
                    <>
                      <Badge tone="leaf" icon={Check}>
                        {t("admin.profile.active")}
                      </Badge>
                      <Badge>
                        {t(`admin.profile.source.${active.source}`)}
                      </Badge>
                      <small>{meta(active).join(" · ")}</small>
                      <Button
                        size="sm"
                        variant="ghost"
                        icon={RotateCcw}
                        busy={clearing === slot}
                        onClick={() => void clearSlot(slot)}
                      >
                        {t("admin.profile.useBuiltIn")}
                      </Button>
                    </>
                  ) : (
                    <Badge>{t("admin.profile.builtIn")}</Badge>
                  )}
                </dd>
              </div>
            )
          })}
        </dl>
      </div>
    </Panel>
  )
}

function Candidate({
  item,
  onState,
}: {
  item: ProfileMedia
  onState: (state: ProfileMediaState) => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const meta = useMeta()
  const [busy, setBusy] = useState("")
  const [viewing, setViewing] = useState(false)
  const run = async (
    key: string,
    work: () => Promise<ProfileMediaState>,
    notice: string,
  ) => {
    setBusy(key)
    const state = await act(work, notice)
    setBusy("")
    if (state) onState({ items: state.items, active: state.active })
  }
  const ready = item.status === "ready"
  return (
    <article
      className={`adm-candidate is-row${item.isActive ? " is-active" : ""}`}
    >
      <div className="adm-candidate-preview">
        {item.previewUrl ? (
          <button
            type="button"
            className="media-opener"
            aria-label={t("admin.profile.view")}
            onClick={() => setViewing(true)}
          >
            <img src={item.previewUrl} alt="" loading="lazy" />
          </button>
        ) : (
          <ImageOff aria-hidden />
        )}
      </div>
      <div className="adm-candidate-body">
        <span className="adm-chip-row">
          <Badge>{t(`admin.profile.source.${item.source}`)}</Badge>
          {ready ? null : (
            <StatusBadge
              status={item.status === "failed" ? "failed" : "pending"}
            />
          )}
        </span>
        <small className="adm-candidate-meta">
          {meta(item).map((part) => (
            <span key={part}>{part}</span>
          ))}
        </small>
        {item.lastError ? (
          <small className="adm-error-text" title={item.lastError}>
            {item.lastError}
          </small>
        ) : null}
      </div>
      <div className="adm-candidate-actions">
        <Button
          size="sm"
          variant={item.isActive ? "ghost" : undefined}
          icon={item.isActive ? Check : SLOT_ICONS[item.slot]}
          disabled={!ready || item.isActive}
          aria-pressed={item.isActive}
          busy={busy === item.slot}
          onClick={() =>
            run(
              item.slot,
              () => api.selectProfileMedia(item.id, item.slot),
              "admin.notice.mediaSelected",
            )
          }
        >
          {item.isActive
            ? t("admin.profile.active")
            : t(`admin.profile.useSlot.${item.slot}`)}
        </Button>
        {item.source !== "upload" ? (
          <Button
            size="sm"
            variant="ghost"
            icon={Download}
            busy={busy === "download"}
            onClick={() =>
              run(
                "download",
                () => api.downloadProfileMedia(item.id),
                "admin.notice.mediaDownloaded",
              )
            }
          >
            {ready ? t("admin.profile.refresh") : t("admin.profile.download")}
          </Button>
        ) : null}
        <ConfirmButton
          size="sm"
          variant="ghost"
          icon={Trash2}
          confirmLabel={t("admin.profile.deleteConfirm")}
          busy={busy === "delete"}
          onConfirm={() =>
            run(
              "delete",
              () => api.deleteProfileMedia(item.id),
              "admin.notice.mediaDeleted",
            )
          }
        >
          {t("admin.profile.delete")}
        </ConfirmButton>
      </div>
      {viewing && item.previewUrl ? (
        <Viewer
          title={t(`admin.profile.source.${item.source}`)}
          previewUrl={item.previewUrl}
          metaParts={meta(item)}
          onClose={() => setViewing(false)}
        />
      ) : null}
    </article>
  )
}

function pickAccountCandidate(account: AccountAvatarItem): {
  ready: AccountAvatarCandidate | null
  pending: AccountAvatarCandidate | null
} {
  const active = account.activeCandidateId
    ? account.candidates.find((c) => c.id === account.activeCandidateId)
    : undefined
  const ready =
    (active?.status === "ready" ? active : undefined) ??
    account.candidates.find((c) => c.status === "ready") ??
    null
  const newest = account.candidates[0] ?? null
  const pending =
    newest && newest.status !== "ready" && newest.id !== ready?.id
      ? newest
      : null
  return { ready, pending }
}

export function isOwnAccountActiveSiteAvatar(
  account: Pick<AccountAvatarItem, "accountHandle">,
  ready: AccountAvatarCandidate | null,
  activeSiteAvatar: ProfileMedia | null,
): boolean {
  if (!activeSiteAvatar || !ready) return false
  const expectedRef = `account-avatar:${account.accountHandle.toLowerCase()}`
  const activeRef = activeSiteAvatar.sourceRef?.toLowerCase() ?? null
  if (activeRef?.startsWith("account-avatar:") && activeRef !== expectedRef) {
    return false
  }
  const activeSha = activeSiteAvatar.activeSha256 ?? activeSiteAvatar.sha256
  if (ready.sha256 && activeSha) {
    return ready.sha256 === activeSha
  }
  return Boolean(
    ready.sourceUrl && activeSiteAvatar.sourceUrl === ready.sourceUrl,
  )
}

function OwnAccountAvatarCard({
  account,
  activeSiteAvatar,
  onAccountsState,
  onProfileState,
}: {
  account: AccountAvatarItem
  activeSiteAvatar: ProfileMedia | null
  onAccountsState: (state: AccountAvatarsState) => void
  onProfileState: (state: ProfileMediaState) => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const meta = useMeta()
  const [busy, setBusy] = useState("")
  const [viewing, setViewing] = useState(false)
  const { ready, pending } = pickAccountCandidate(account)
  const isSiteAvatar = isOwnAccountActiveSiteAvatar(
    account,
    ready,
    activeSiteAvatar,
  )
  const display = ready ?? pending
  const handle = `@${account.accountHandle}`

  const runAvatarMutation = async (
    key: string,
    work: () => Promise<{
      items: AccountAvatarItem[]
      profile?: ProfileMediaState
    }>,
    notice: string,
  ) => {
    setBusy(key)
    const result = await act(work, notice)
    setBusy("")
    if (!result) return
    onAccountsState({ items: result.items })
    if (result.profile) {
      onProfileState(result.profile)
    }
  }

  return (
    <article
      className={`adm-candidate is-row${isSiteAvatar ? " is-active" : ""}`}
    >
      <div className="adm-candidate-preview">
        {ready?.previewUrl ? (
          <button
            type="button"
            className="media-opener"
            aria-label={t("admin.profile.view")}
            onClick={() => setViewing(true)}
          >
            <img src={ready.previewUrl} alt={handle} loading="lazy" />
          </button>
        ) : (
          <ImageOff aria-hidden />
        )}
      </div>
      <div className="adm-candidate-body">
        <span className="adm-chip-row">
          <Badge>{handle}</Badge>
          {account.mode === "hidden" ? (
            <Badge>{t(`admin.avatars.mode.${account.mode}`)}</Badge>
          ) : null}
          {pending ? (
            <StatusBadge
              status={pending.status === "failed" ? "failed" : "pending"}
            />
          ) : null}
        </span>
        {display ? (
          <small className="adm-candidate-meta">
            {meta(display).map((part) => (
              <span key={part}>{part}</span>
            ))}
          </small>
        ) : (
          <small className="adm-candidate-meta">
            <span>{t("admin.avatars.noCandidates")}</span>
          </small>
        )}
        {pending?.lastError ? (
          <small className="adm-error-text" title={pending.lastError}>
            {pending.lastError}
          </small>
        ) : null}
      </div>
      <div className="adm-candidate-actions">
        <Button
          size="sm"
          variant={isSiteAvatar ? "ghost" : undefined}
          icon={isSiteAvatar ? Check : UserRound}
          disabled={!ready || isSiteAvatar}
          busy={busy === "apply"}
          onClick={() =>
            ready &&
            void runAvatarMutation(
              "apply",
              () =>
                api.applyAccountAvatarToProfile(
                  "x",
                  account.accountKey,
                  ready.id,
                ),
              "admin.notice.avatarAppliedProfile",
            )
          }
        >
          {isSiteAvatar
            ? t("admin.profile.siteAvatarInUse")
            : t("admin.profile.useSiteAvatar")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          icon={Search}
          busy={busy === "discover"}
          onClick={() =>
            void runAvatarMutation(
              "discover",
              () => api.discoverAccountAvatar("x", account.accountKey),
              "admin.notice.avatarDiscovered",
            )
          }
        >
          {t("admin.avatars.discover")}
        </Button>
        {pending ? (
          <Button
            size="sm"
            variant="ghost"
            icon={Download}
            busy={busy === "download"}
            onClick={() =>
              void runAvatarMutation(
                "download",
                () =>
                  api.downloadAccountAvatarCandidate(
                    "x",
                    account.accountKey,
                    pending.id,
                  ),
                "admin.notice.avatarDownloaded",
              )
            }
          >
            {pending.status === "failed"
              ? t("admin.avatars.refresh")
              : t("admin.avatars.download")}
          </Button>
        ) : null}
      </div>
      {viewing && ready?.previewUrl ? (
        <Viewer
          title={handle}
          previewUrl={ready.previewUrl}
          metaParts={meta(ready)}
          onClose={() => setViewing(false)}
        />
      ) : null}
    </article>
  )
}

function CandidateSection({
  title,
  count,
  actions,
  children,
}: {
  title: string
  count: number
  actions?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="adm-profile-section">
      <div className="adm-profile-section-head">
        <h3>
          {title}
          <em className="adm-profile-count">{count}</em>
        </h3>
        {actions ? (
          <div className="adm-profile-section-actions">{actions}</div>
        ) : null}
      </div>
      {children}
    </section>
  )
}

const UPLOAD_TYPES = ["image/png", "image/jpeg", "image/webp", "image/gif"]

/** The source a pasted image URL belongs to, read from its host. */
function sourceOf(url: string): "x" | "youtube" {
  try {
    return /(?:^|\.)twimg\.com$/u.test(new URL(url).hostname) ? "x" : "youtube"
  } catch {
    return "x"
  }
}

type Perform = (
  key: string,
  work: () => Promise<ProfileMediaState>,
  notice: string,
) => Promise<void>

function UploadTab({
  slot,
  busy,
  perform,
}: {
  slot: ProfileSlot
  busy: string
  perform: Perform
}) {
  const { t } = useAppSettings()
  const { api } = useAdmin()
  const [url, setUrl] = useState("")
  const [over, setOver] = useState(false)
  const file = useRef<HTMLInputElement>(null)
  const upload = (chosen: File | undefined) => {
    if (chosen && UPLOAD_TYPES.includes(chosen.type))
      void perform(
        "upload",
        () => api.uploadProfileMedia(chosen, slot),
        "admin.notice.mediaUploaded",
      )
  }
  return (
    <div className="adm-profile-add">
      <form
        className="adm-inline-form"
        onSubmit={(event) => {
          event.preventDefault()
          void perform(
            "add",
            () => api.addProfileMedia(sourceOf(url), url, slot),
            "admin.notice.mediaAdded",
          )
        }}
      >
        <Field label={t("admin.profile.addUrl")}>
          <input
            type="url"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://"
            aria-describedby="profile-url-hint"
            required
          />
        </Field>
        <Button type="submit" size="sm" icon={Link2} busy={busy === "add"}>
          {t("admin.profile.add")}
        </Button>
      </form>
      <small id="profile-url-hint" className="adm-field-hint">
        {t("admin.profile.addUrlHint")}
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
  )
}

/** Upload takes an image URL or a file for the site's avatar or banner. */
function AddDialog({
  slot,
  onState,
  onClose,
}: {
  slot: ProfileSlot
  onState: (state: ProfileMediaState) => void
  onClose: () => void
}) {
  const { t } = useAppSettings()
  const { act } = useAdmin()
  const [busy, setBusy] = useState("")
  const perform: Perform = async (key, work, notice) => {
    setBusy(key)
    const next = await act(work, notice)
    setBusy("")
    if (!next) return
    onState({ items: next.items, active: next.active })
    onClose()
  }
  return (
    <Modal
      open
      className="adm-profile-dialog"
      title={
        slot === "banner"
          ? t("admin.profile.addBannerTitle")
          : t("admin.profile.addAvatarTitle")
      }
      onClose={onClose}
    >
      <UploadTab slot={slot} busy={busy} perform={perform} />
    </Modal>
  )
}

function FetchBannerDialog({
  ownHandles,
  onState,
  onClose,
}: {
  ownHandles: string[]
  onState: (state: ProfileMediaState) => void
  onClose: () => void
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [source, setSource] = useState<"x" | "youtube">("x")
  const [handle, setHandle] = useState(ownHandles[0] ?? "")
  const [busy, setBusy] = useState(false)
  const selectedHandle =
    source === "x" ? handle || ownHandles[0] || undefined : undefined

  const submit = async () => {
    setBusy(true)
    const next = await act(
      () => api.discoverProfileMedia(source, "banner", selectedHandle),
      "admin.notice.mediaDiscovered",
    )
    setBusy(false)
    if (!next) return
    onState({ items: next.items, active: next.active })
    onClose()
  }

  return (
    <Modal
      open
      className="adm-profile-dialog"
      title={t("admin.profile.fetchBannerTitle")}
      onClose={onClose}
    >
      <div className="adm-profile-add">
        <div
          className="adm-chip-row"
          role="group"
          aria-label={t("admin.profile.discoverFrom")}
        >
          {(["x", "youtube"] as const).map((choice) => (
            <Button
              key={choice}
              size="sm"
              variant={source === choice ? undefined : "ghost"}
              onClick={() => setSource(choice)}
            >
              {t(`admin.profile.source.${choice}`)}
            </Button>
          ))}
        </div>
        {source === "x" && ownHandles.length ? (
          <Field label={t("admin.profile.xAccount")}>
            <div className="adm-chip-row">
              {ownHandles.map((itemHandle) => {
                const chosen = (selectedHandle ?? "") === itemHandle
                return (
                  <Button
                    key={itemHandle}
                    size="sm"
                    variant={chosen ? undefined : "ghost"}
                    onClick={() => setHandle(itemHandle)}
                  >
                    @{itemHandle}
                  </Button>
                )
              })}
            </div>
          </Field>
        ) : null}
        <div className="adm-profile-dialog-actions">
          <Button icon={Search} busy={busy} onClick={() => void submit()}>
            {t("admin.profile.discover")}
          </Button>
        </div>
      </div>
    </Modal>
  )
}

export function ProfileView({ navigate }: ViewProps) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const media = useResource(() => api.profileMedia(), [])
  const accounts = useResource(() => api.accountAvatars(), [])
  const [addingSlot, setAddingSlot] = useState<ProfileSlot | null>(null)
  const [fetchingBanner, setFetchingBanner] = useState(false)
  const [fetchingYoutube, setFetchingYoutube] = useState(false)

  const state = media.data
  const ownAccounts = (accounts.data?.items ?? []).filter(
    (item) => item.role === "own",
  )
  const ownHandles = ownAccounts.map((item) => item.accountHandle)
  const ownAvatarUrls = new Set(
    ownAccounts.flatMap((account) =>
      account.candidates
        .map((candidate) => candidate.sourceUrl)
        .filter((value): value is string => Boolean(value)),
    ),
  )

  const avatarItems = state
    ? orderedForSlot(
        state.items.filter(
          (item) =>
            item.slot === "avatar" &&
            !item.sourceRef?.startsWith("account-avatar:") &&
            !(
              item.source === "x" &&
              item.sourceUrl &&
              ownAvatarUrls.has(item.sourceUrl)
            ),
        ),
        "avatar",
      )
    : []
  const bannerItems = state
    ? orderedForSlot(
        state.items.filter((item) => item.slot === "banner"),
        "banner",
      )
    : []

  const fetchYoutubeAvatar = async () => {
    setFetchingYoutube(true)
    const next = await act(
      () => api.discoverProfileMedia("youtube", "avatar"),
      "admin.notice.mediaDiscovered",
    )
    setFetchingYoutube(false)
    if (next) media.setData({ items: next.items, active: next.active })
  }

  return (
    <div className="adm-view">
      <PageHeader title={t("admin.profile.title")} />
      {!state ? (
        <Loading />
      ) : (
        <div className="adm-grid">
          <Preview
            state={state}
            onState={media.setData}
            onManageAvatars={() => navigate("avatars")}
          />
          <Panel icon={Images} title={t("admin.profile.candidates")}>
            <div className="adm-profile-sections">
              <CandidateSection
                title={t("admin.profile.section.xAvatars")}
                count={ownAccounts.length}
                actions={
                  <Button
                    size="sm"
                    variant="ghost"
                    icon={UsersRound}
                    onClick={() => navigate("avatars")}
                  >
                    {t("admin.profile.manageAllAvatars")}
                  </Button>
                }
              >
                {ownAccounts.length ? (
                  <div className="adm-candidate-rows">
                    {ownAccounts.map((account) => (
                      <OwnAccountAvatarCard
                        key={`${account.source}:${account.accountKey}`}
                        account={account}
                        activeSiteAvatar={state.active.avatar}
                        onAccountsState={accounts.setData}
                        onProfileState={media.setData}
                      />
                    ))}
                  </div>
                ) : (
                  <Empty title={t("admin.profile.noAccountAvatars")} />
                )}
              </CandidateSection>

              <CandidateSection
                title={t("admin.profile.section.youtubeAvatars")}
                count={avatarItems.length}
                actions={
                  <>
                    <Button
                      size="sm"
                      variant="ghost"
                      icon={Search}
                      busy={fetchingYoutube}
                      onClick={() => void fetchYoutubeAvatar()}
                    >
                      {t("admin.profile.fetchYoutubeAvatar")}
                    </Button>
                    <Button
                      size="sm"
                      icon={Plus}
                      onClick={() => setAddingSlot("avatar")}
                    >
                      {t("admin.profile.addButton")}
                    </Button>
                  </>
                }
              >
                {avatarItems.length ? (
                  <div className="adm-candidate-rows">
                    {avatarItems.map((item) => (
                      <Candidate
                        key={item.id}
                        item={item}
                        onState={media.setData}
                      />
                    ))}
                  </div>
                ) : (
                  <Empty title={t("admin.profile.noYoutubeAvatars")} />
                )}
              </CandidateSection>

              <CandidateSection
                title={t("admin.profile.section.banners")}
                count={bannerItems.length}
                actions={
                  <>
                    <Button
                      size="sm"
                      variant="ghost"
                      icon={Search}
                      onClick={() => setFetchingBanner(true)}
                    >
                      {t("admin.profile.fetchBanner")}
                    </Button>
                    <Button
                      size="sm"
                      icon={Plus}
                      onClick={() => setAddingSlot("banner")}
                    >
                      {t("admin.profile.addButton")}
                    </Button>
                  </>
                }
              >
                {bannerItems.length ? (
                  <div className="adm-candidate-rows">
                    {bannerItems.map((item) => (
                      <Candidate
                        key={item.id}
                        item={item}
                        onState={media.setData}
                      />
                    ))}
                  </div>
                ) : (
                  <Empty title={t("admin.profile.noBanners")} />
                )}
              </CandidateSection>
            </div>
          </Panel>
          {addingSlot ? (
            <AddDialog
              slot={addingSlot}
              onState={media.setData}
              onClose={() => setAddingSlot(null)}
            />
          ) : null}
          {fetchingBanner ? (
            <FetchBannerDialog
              ownHandles={ownHandles}
              onState={media.setData}
              onClose={() => setFetchingBanner(false)}
            />
          ) : null}
        </div>
      )}
    </div>
  )
}
