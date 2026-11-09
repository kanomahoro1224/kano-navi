import type { CSSProperties } from "react"
import { useMemo, useState } from "react"
import { Disc3, ListMusic, Tv, type LucideIcon } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { Translator } from "@/types"
import {
  BilibiliLogo,
  ExternalLink,
  YouTubeLogo,
  YouTubeMembersMark,
} from "@/site/links"
import data from "./uploads.json" with { type: "json" }
import { mediaSrcSet } from "@/lib/media"
import {
  byDate,
  dayLabel,
  groupByYear,
  matches,
  uploadLinks,
  uploadTone,
} from "./content"
import { RecentUploads } from "./recent-uploads"
import type { Upload } from "./types"
import {
  durationLabel,
  recentUploads,
  useWorksUploads,
  videoGone,
  type WorksUploadState,
} from "./use-works-uploads"
import {
  FilterButtons,
  OrderToggle,
  SearchBox,
  type Order,
} from "./works-controls"
import { WORKS_COVER_SIZES } from "./works-media"

const uploads = data as Upload[]

type KindFilter = "all" | "cover" | "original" | "mv" | "other"
const FILTERS: KindFilter[] = ["all", "cover", "original", "mv", "other"]

const platformMarks = {
  niconico: Tv,
  youtube: YouTubeLogo,
  bilibili: BilibiliLogo,
}

function keep(upload: Upload, filter: KindFilter) {
  if (filter === "all") return true
  if (filter === "other")
    return !["cover", "original", "mv"].includes(upload.kind)
  return upload.kind === filter
}

const isMembers = (upload: Upload) => upload.youtubeMembers.length > 0

function watchLabel(
  t: Translator,
  upload: Upload,
  link: ReturnType<typeof uploadLinks>[number],
) {
  return t(link.members ? "works.watchOnMembers" : "works.watchOn", {
    platform: t(`works.platform.${link.platform}`),
    title: upload.song,
  })
}

/**
 * The thumbnail this server cached for one of the upload's videos; otherwise
 * a printed sleeve with the song's title, in one of the page's tones. Nothing
 * shows while the server's answer is on its way.
 */
function UploadArt({
  upload,
  stored,
  loading,
}: {
  upload: Upload
  stored: WorksUploadState | undefined
  loading: boolean
}) {
  const { t } = useAppSettings()
  const [failed, setFailed] = useState(false)
  const cover = stored?.cover
  if (cover && !failed)
    return (
      <img
        src={cover.url}
        srcSet={mediaSrcSet(cover.url, cover.width)}
        sizes={WORKS_COVER_SIZES.upload}
        alt=""
        width={cover.width ?? undefined}
        height={cover.height ?? undefined}
        loading="lazy"
        decoding="async"
        onError={() => setFailed(true)}
      />
    )
  if (loading) return <span className="upload-wait" aria-hidden="true" />
  return (
    <span
      className={`upload-print is-tone-${uploadTone(upload.id)}`}
      aria-hidden="true"
    >
      <span className="upload-print-kind">
        {t(`works.kind.${upload.kind}`)}
      </span>
      <span className="upload-print-title">{upload.song}</span>
      {upload.originalArtist ? (
        <span className="upload-print-artist">{upload.originalArtist}</span>
      ) : null}
      <span className="upload-print-year">{upload.date.slice(0, 4)}</span>
    </span>
  )
}

function UploadCard({
  upload,
  index,
  stored,
  loading,
}: {
  upload: Upload
  index: number
  stored: WorksUploadState | undefined
  loading: boolean
}) {
  const { t } = useAppSettings()
  const links = uploadLinks(upload)
  const youtube = stored?.videos.filter((video) => video.source === "youtube")
  const live = youtube?.find((video) => !videoGone(video))
  const gone = Boolean(youtube?.length) && !live
  const length = stored?.videos.find(
    (video) => video.durationSeconds,
  )?.durationSeconds
  const primary = links.find((link) => !link.members) ?? links[0]
  const people = [
    upload.originalArtist
      ? t("works.originalBy", { artist: upload.originalArtist })
      : "",
    upload.with ? t("works.with", { names: upload.with }) : "",
  ].filter(Boolean)
  return (
    <li className="upload-card" style={{ "--order": index } as CSSProperties}>
      {primary ? (
        <ExternalLink
          className="upload-art"
          href={primary.href}
          aria-label={watchLabel(t, upload, primary)}
          tabIndex={-1}
        >
          <UploadArt upload={upload} stored={stored} loading={loading} />
        </ExternalLink>
      ) : (
        <span className="upload-art">
          <UploadArt upload={upload} stored={stored} loading={loading} />
        </span>
      )}
      <div className="upload-copy">
        <div className="upload-head">
          <p className="works-meta">
            <time dateTime={upload.date}>{dayLabel(upload.date)}</time>
            <span>{t(`works.kind.${upload.kind}`)}</span>
            {length ? <span>{durationLabel(length)}</span> : null}
            {gone ? (
              <span className="works-gone">{t("works.videoGone")}</span>
            ) : null}
            {isMembers(upload) ? (
              <span className="works-members">
                <YouTubeMembersMark aria-hidden="true" />
                {t("works.membersOnly")}
              </span>
            ) : null}
          </p>
          <UploadLinks upload={upload} stored={stored} />
        </div>
        <div className="upload-title-row">
          <h4 className="upload-title">{upload.song}</h4>
          {upload.tracks.length || upload.albums.length ? (
            <span className="upload-title-marks">
              {upload.tracks.length ? (
                <UploadListMenu
                  icon={ListMusic}
                  label={t("works.songsIn")}
                  ariaLabel={t("works.showSongs", {
                    title: upload.song,
                    count: upload.tracks.length,
                  })}
                  items={upload.tracks}
                  numbered
                />
              ) : null}
              {upload.albums.length ? (
                <UploadListMenu
                  icon={Disc3}
                  label={t("works.onAlbums")}
                  ariaLabel={t("works.showAlbums", {
                    title: upload.song,
                    count: upload.albums.length,
                  })}
                  items={upload.albums}
                />
              ) : null}
            </span>
          ) : null}
        </div>
        {people.length ? (
          <p className="upload-people">{people.join(" · ")}</p>
        ) : null}
      </div>
    </li>
  )
}

/**
 * A mark at the end of an upload's title that opens a list: the songs a
 * long upload sings, numbered, or the albums that later included the
 * recording. A count shows when the list has more than one entry.
 */
function UploadListMenu({
  icon: Icon,
  label,
  ariaLabel,
  items,
  numbered = false,
}: {
  icon: LucideIcon
  label: string
  ariaLabel: string
  items: string[]
  numbered?: boolean
}) {
  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="upload-list-button"
          aria-label={ariaLabel}
          title={label}
        >
          <Icon aria-hidden="true" />
          {items.length > 1 ? <span>{items.length}</span> : null}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="upload-list-menu">
        <DropdownMenuLabel>{label}</DropdownMenuLabel>
        {items.map((item, index) => (
          <DropdownMenuItem key={index}>
            {numbered ? (
              <span className="upload-list-number">{index + 1}</span>
            ) : null}
            {item}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** A small mark for each site the recording is on, beside its date. */
function UploadLinks({
  upload,
  stored,
}: {
  upload: Upload
  stored: WorksUploadState | undefined
}) {
  const { t } = useAppSettings()
  const links = uploadLinks(upload)
  const goneIds = new Set(
    (stored?.videos ?? []).filter(videoGone).map((video) => video.id),
  )
  if (!links.length)
    return <span className="works-quiet upload-links">{t("works.noLink")}</span>
  return (
    <p className="upload-links">
      {links.map((link) => {
        const Mark = platformMarks[link.platform]
        return (
          <ExternalLink
            key={link.href}
            className={`upload-link is-${link.platform}${link.members ? " is-members" : ""}${goneIds.has(link.id) ? " is-gone" : ""}`}
            href={link.href}
            aria-label={watchLabel(t, upload, link)}
            title={[
              link.platform === "bilibili" && upload.bilibiliTitle
                ? `${t("works.platform.bilibili")} · ${upload.bilibiliTitle}`
                : link.members
                  ? `${t("works.platform.youtube")} · ${t("works.membersOnly")}`
                  : t(`works.platform.${link.platform}`),
              goneIds.has(link.id) ? t("works.videoGone") : "",
            ]
              .filter(Boolean)
              .join(" · ")}
          >
            <Mark aria-hidden="true" />
            {link.members ? (
              <YouTubeMembersMark
                className="works-members-mark"
                aria-hidden="true"
              />
            ) : null}
          </ExternalLink>
        )
      })}
    </p>
  )
}

export function UploadsView() {
  const { t } = useAppSettings()
  const status = useWorksUploads()
  const overlay = status.state === "ready" ? status.overlay : undefined
  const stored = overlay?.uploads
  const recent = recentUploads(overlay)
  const loading = status.state === "loading"
  const [filter, setFilter] = useState<KindFilter>("all")
  const [onlyLoose, setOnlyLoose] = useState(false)
  const [onlyMembers, setOnlyMembers] = useState(false)
  const [query, setQuery] = useState("")
  const [order, setOrder] = useState<Order>("newest")
  const groups = useMemo(
    () =>
      groupByYear(
        byDate(
          uploads.filter(
            (upload) =>
              keep(upload, filter) &&
              (!onlyLoose || !upload.albums.length) &&
              (!onlyMembers || isMembers(upload)) &&
              matches(
                query,
                upload.song,
                upload.originalArtist,
                upload.with,
                upload.bilibiliTitle,
              ),
          ),
          order,
        ),
      ),
    [filter, onlyLoose, onlyMembers, query, order],
  )
  const shown = groups.reduce((sum, group) => sum + group.items.length, 0)
  return (
    <>
      {overlay ? (
        <RecentUploads
          items={recent}
          total={Math.max(recent.length, overlay.recentTotal ?? 0)}
          updatedAt={overlay.updatedAt}
        />
      ) : null}
      <div className="works-toolbar">
        <FilterButtons
          label={t("works.kindFilter")}
          options={FILTERS}
          value={filter}
          onChange={setFilter}
          name={(item) => `works.kinds.${item}`}
          count={(item) =>
            uploads.filter((upload) => keep(upload, item)).length
          }
          t={t}
        />
        <OrderToggle order={order} onChange={setOrder} t={t} />
        <div className="works-refine">
          <SearchBox
            value={query}
            onChange={setQuery}
            label={t("works.searchUploads")}
          />
          <button
            type="button"
            className="pill-button works-toggle"
            aria-pressed={onlyLoose}
            onClick={() => setOnlyLoose((value) => !value)}
          >
            {t("works.notOnAlbum")}
            <small>
              {uploads.filter((upload) => !upload.albums.length).length}
            </small>
          </button>
          <button
            type="button"
            className="pill-button works-toggle"
            aria-pressed={onlyMembers}
            onClick={() => setOnlyMembers((value) => !value)}
          >
            <YouTubeMembersMark
              className="works-members-mark"
              aria-hidden="true"
            />
            {t("works.membersOnly")}
            <small>{uploads.filter(isMembers).length}</small>
          </button>
        </div>
      </div>
      <p className="works-result-count" role="status">
        {t("works.uploadCount", { count: shown })}
      </p>
      {groups.length ? (
        <ol className="works-years">
          {groups.map((group) => (
            <li key={group.year} className="works-year">
              <h3 className="works-year-label">
                <span>{group.year}</span>
                <small>
                  {t("works.uploadCount", { count: group.items.length })}
                </small>
              </h3>
              <ul className="upload-grid">
                {group.items.map((upload, index) => (
                  <UploadCard
                    key={upload.id}
                    upload={upload}
                    index={index}
                    stored={stored?.[upload.id]}
                    loading={loading}
                  />
                ))}
              </ul>
            </li>
          ))}
        </ol>
      ) : (
        <p className="works-empty">{t("works.empty")}</p>
      )}
      {status.state === "failed" ? (
        <p className="works-notice">{t("works.uploadsOffline")}</p>
      ) : null}
    </>
  )
}
