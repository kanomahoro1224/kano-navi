import { useEffect, useState, type SubmitEvent } from "react"
import { CalendarCheck, ImageOff, Images, PencilLine } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { japanStamp, statusTone } from "@/admin/format"
import type { AdminPost, PostVisibility } from "@/admin/types"
import {
  Badge,
  Button,
  Field,
  FieldGroup,
  IconButton,
  Modal,
  Segmented,
} from "@/admin/ui"
import { useAdmin } from "@/admin/use-admin-data"
import {
  localUrl,
  PostImages,
  PostQuote,
  PostAccount,
  PostStatus,
  PostVisibilityBadges,
  RefreshQueued,
  PostActions,
} from "@/admin/views/post-parts"
import { TranslationCell } from "@/admin/views/translation-parts"

/** The label a post's schedule image record carries: the operator's, else
 * the image model's. */
function scheduleStatus(post: AdminPost) {
  const record = post.scheduleImage
  if (!record) return null
  return record.manualStatus === "unreviewed"
    ? record.llmStatus
    : record.manualStatus
}

/** The post's schedule image record as a chip, when it has one. */
function ScheduleImageBadge({ post }: { post: AdminPost }) {
  const { t } = useAppSettings()
  const status = scheduleStatus(post)
  if (!status) return null
  return (
    <Badge tone={statusTone(status)} icon={CalendarCheck}>
      {t("admin.posts.schedule.label")} · {t(`admin.status.${status}`)}
      {post.scheduleImage?.picked
        ? ` · ${t("admin.posts.schedule.picked")}`
        : ""}
    </Badge>
  )
}

/**
 * Make one of an X post's own images its schedule image, labelled with a
 * reason and, optionally, the dates it covers. The record otherwise comes
 * only from the synchronization keyword rule, which takes the first image.
 */
function PostScheduleImage({
  post,
  onDone,
  navigate,
}: {
  post: AdminPost
  onDone: () => void
  navigate: ViewProps["navigate"]
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [index, setIndex] = useState(post.scheduleImage?.index ?? 0)
  const [reason, setReason] = useState("")
  const [start, setStart] = useState("")
  const [end, setEnd] = useState("")
  const [busy, setBusy] = useState(false)
  if (post.source !== "x" || !post.media.length) return null
  const save = async (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault()
    setBusy(true)
    const result = await act(
      () =>
        api.pickScheduleImage(post.id, {
          index,
          reason,
          ...(start ? { start, end } : {}),
        }),
      "admin.notice.schedulePicked",
    )
    setBusy(false)
    if (!result) return
    setReason("")
    onDone()
  }
  const dated = Boolean(start || end)
  return (
    <form
      className="adm-post-editor-section adm-form adm-schedule-pick-form"
      onSubmit={save}
    >
      <h3>{t("admin.posts.schedule.label")}</h3>
      {post.scheduleImage ? (
        <div className="adm-chip-row">
          <ScheduleImageBadge post={post} />
          <Button
            size="sm"
            variant="ghost"
            icon={Images}
            onClick={() => navigate("schedule", "images")}
          >
            {t("admin.posts.schedule.open")}
          </Button>
        </div>
      ) : null}
      {post.media.length > 1 ? (
        <FieldGroup label={t("admin.posts.schedule.image")}>
          <div className="adm-thumb-choices adm-schedule-pick">
            {post.media.map((image, position) => {
              const url = localUrl(image.url)
              return (
                <label
                  key={position}
                  className={`adm-thumb-choice${index === position ? " is-selected" : ""}`}
                >
                  <input
                    type="radio"
                    name={`schedule-image-${post.id}`}
                    checked={index === position}
                    onChange={() => setIndex(position)}
                  />
                  <span className="adm-schedule-pick-image">
                    {url ? (
                      <img src={url} alt="" loading="lazy" />
                    ) : (
                      <ImageOff aria-hidden />
                    )}
                  </span>
                  <small>
                    {t("admin.posts.schedule.imageOption", {
                      index: position + 1,
                    })}
                  </small>
                </label>
              )
            })}
          </div>
        </FieldGroup>
      ) : null}
      <Field label={t("admin.review.reason")}>
        <input
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          placeholder={t("admin.posts.schedule.reasonPlaceholder")}
          maxLength={500}
          required
        />
      </Field>
      <div className="adm-form-row">
        <Field label={t("admin.review.periodStart")}>
          <input
            type="date"
            value={start}
            onChange={(event) => setStart(event.target.value)}
            required={dated}
          />
        </Field>
        <Field label={t("admin.review.periodEnd")}>
          <input
            type="date"
            value={end}
            min={start}
            onChange={(event) => setEnd(event.target.value)}
            required={dated}
          />
        </Field>
      </div>
      <Button type="submit" size="sm" icon={CalendarCheck} busy={busy}>
        {t("admin.posts.schedule.pick")}
      </Button>
    </form>
  )
}

/** The operator's decision on a post, or auto when the rules decide. */
function postVisibility(
  shownAt: string | null,
  hiddenAt: string | null,
): PostVisibility {
  return shownAt ? "shown" : hiddenAt ? "hidden" : "auto"
}

const visibilityChoices: PostVisibility[] = ["auto", "shown", "hidden"]

const visibilityNotices = {
  auto: "admin.notice.postAuto",
  shown: "admin.notice.postShown",
  hidden: "admin.notice.postHidden",
} as const

/**
 * The post in full: its wording, images, translations and quote, its
 * extraction state, its schedule image and the operator's decisions. The
 * public pages follow what X says about the post unless the operator shows
 * or hides it.
 */
function PostEditor({
  post,
  onClose,
  onDone,
  navigate,
}: {
  post: AdminPost
  onClose: () => void
  onDone: () => void
  navigate: ViewProps["navigate"]
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [visibility, setVisibility] = useState(
    postVisibility(post.shownAt, post.hiddenAt),
  )
  const [busy, setBusy] = useState(false)
  // A refetch may bring a decision made elsewhere, such as External posts
  // withdrawing the operator's hide or show.
  useEffect(
    () => setVisibility(postVisibility(post.shownAt, post.hiddenAt)),
    [post.shownAt, post.hiddenAt],
  )
  const choose = async (next: PostVisibility) => {
    if (busy || next === visibility) return
    setBusy(true)
    const result = await act(
      () => api.setPostVisibility(post.id, next),
      visibilityNotices[next],
    )
    setBusy(false)
    if (!result) return
    setVisibility(next)
    onDone()
  }
  return (
    <Modal
      open
      wide
      className="adm-post-editor"
      title={
        <>
          <PostAccount post={post} /> · {japanStamp(post.publishedAt)}
        </>
      }
      onClose={onClose}
      footer={<PostActions post={post} onDone={onDone} />}
    >
      <div className="adm-chip-row">
        <PostVisibilityBadges post={post} />
        <PostStatus post={post} />
        <RefreshQueued post={post} />
      </div>
      <p className="adm-post-editor-text">{post.text}</p>
      <PostImages post={post} limit={post.media.length} />
      {post.translation ? (
        <section className="adm-post-editor-section">
          <h3>{t("admin.posts.translations")}</h3>
          <TranslationCell
            owner="post"
            id={post.id}
            original={post.text}
            translation={post.translation}
            onChanged={onDone}
          />
        </section>
      ) : null}
      <PostQuote post={post} onDone={onDone} full />
      <PostScheduleImage post={post} onDone={onDone} navigate={navigate} />
      <section className="adm-post-editor-section">
        <h3>{t("admin.posts.visibility")}</h3>
        <Segmented
          label={t("admin.posts.visibility")}
          value={visibility}
          onChange={(next) => void choose(next)}
          options={visibilityChoices.map((choice) => ({
            value: choice,
            label: t(`admin.posts.visibility.set.${choice}`),
          }))}
        />
      </section>
    </Modal>
  )
}

/**
 * One post in the feed, one row of columns: who and when, the wording and
 * quote, the post's own images, its translations, its extraction state and
 * its actions. A narrower feed stacks them (see the stylesheet). A post the
 * list's bulk action applies to has a selection box before its account.
 */
export function PostItem({
  post,
  onDone,
  navigate,
  selected,
  onSelect,
}: {
  post: AdminPost
  onDone: () => void
  navigate: ViewProps["navigate"]
  selected?: boolean
  /** Toggle the selection; `range` extends it from the last toggled post. */
  onSelect?: (range: boolean) => void
}) {
  const { t } = useAppSettings()
  const [editing, setEditing] = useState(false)
  const account = (
    <strong>
      <PostAccount post={post} />
    </strong>
  )
  return (
    <li
      className={`adm-post${post.visible ? "" : " is-hidden"}${selected ? " is-selected" : ""}`}
    >
      <span className="adm-post-origin">
        {onSelect ? (
          <label className="adm-post-select">
            <input
              type="checkbox"
              checked={Boolean(selected)}
              readOnly
              aria-label={t("admin.review.selectPost", {
                time: japanStamp(post.publishedAt),
              })}
              onClick={(event) => onSelect(event.shiftKey)}
            />
            {account}
          </label>
        ) : (
          account
        )}
        <time dateTime={post.publishedAt}>{japanStamp(post.publishedAt)}</time>
      </span>
      <div className="adm-post-body">
        <p className="adm-post-text">{post.text}</p>
        <PostQuote post={post} onDone={onDone} />
      </div>
      <PostImages post={post} />
      <div className="adm-post-translation">
        <TranslationCell
          owner="post"
          id={post.id}
          original={post.text}
          translation={post.translation}
          onChanged={onDone}
        />
      </div>
      <span className="adm-chip-row adm-post-state">
        <PostVisibilityBadges post={post} />
        {post.promotedAt ? (
          <button
            type="button"
            className="adm-badge-button"
            onClick={() => navigate("external")}
            title={t("admin.posts.promotedHint")}
          >
            <Badge tone="lavender">{t("admin.posts.promoted")}</Badge>
          </button>
        ) : null}
        <PostStatus post={post} />
        <RefreshQueued post={post} />
        <ScheduleImageBadge post={post} />
      </span>
      <span className="adm-post-controls">
        <IconButton
          icon={PencilLine}
          label={t("admin.posts.edit")}
          onClick={() => setEditing(true)}
        />
        <PostActions post={post} onDone={onDone} compact />
      </span>
      {editing ? (
        <PostEditor
          post={post}
          onClose={() => setEditing(false)}
          onDone={onDone}
          navigate={navigate}
        />
      ) : null}
    </li>
  )
}
