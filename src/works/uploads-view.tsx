import type { CSSProperties } from "react"
import { useMemo, useState } from "react"
import { Disc3, Tv } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import {
  BilibiliLogo,
  ExternalLink,
  YouTubeLogo,
  YouTubeMembersMark,
} from "@/site/links"
import data from "./uploads.json" with { type: "json" }
import { byDate, dayLabel, groupByYear, matches, uploadLinks } from "./content"
import type { Upload } from "./types"
import {
  FilterButtons,
  OrderToggle,
  SearchBox,
  type Order,
} from "./works-controls"
import { WORKS_COVER_SIZES, WorksCoverImage } from "./works-media"

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

/** One of the page's tones, the same for an upload on every visit. */
export function uploadTone(id: string) {
  let hash = 0
  for (const char of id) hash = (hash * 31 + char.charCodeAt(0)) >>> 0
  return hash % 5
}

/**
 * A video thumbnail when the collection has one; otherwise a printed
 * sleeve with the song's title, in one of the page's tones.
 */
function UploadArt({ upload }: { upload: Upload }) {
  const { t } = useAppSettings()
  if (upload.cover)
    return (
      <WorksCoverImage
        cover={upload.cover}
        alt=""
        t={t}
        sizes={WORKS_COVER_SIZES.upload}
      />
    )
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

function UploadCard({ upload, index }: { upload: Upload; index: number }) {
  const { t } = useAppSettings()
  const links = uploadLinks(upload)
  const primary = links.find((link) => !link.members) ?? links[0]
  const people = [
    upload.originalArtist
      ? t("works.originalBy", { artist: upload.originalArtist })
      : "",
    upload.with ? t("works.with", { names: upload.with }) : "",
  ].filter(Boolean)
  const watchLabel = (link: (typeof links)[number]) =>
    t(link.members ? "works.watchOnMembers" : "works.watchOn", {
      platform: t(`works.platform.${link.platform}`),
      title: upload.song,
    })
  return (
    <li className="upload-card" style={{ "--order": index } as CSSProperties}>
      {primary ? (
        <ExternalLink
          className="upload-art"
          href={primary.href}
          aria-label={watchLabel(primary)}
          tabIndex={-1}
        >
          <UploadArt upload={upload} />
        </ExternalLink>
      ) : (
        <span className="upload-art">
          <UploadArt upload={upload} />
        </span>
      )}
      <div className="upload-copy">
        <p className="works-meta">
          <time dateTime={upload.date}>{dayLabel(upload.date)}</time>
          <span>{t(`works.kind.${upload.kind}`)}</span>
          {isMembers(upload) ? (
            <span className="works-members">
              <YouTubeMembersMark aria-hidden="true" />
              {t("works.membersOnly")}
            </span>
          ) : null}
        </p>
        <h4 className="upload-title">{upload.song}</h4>
        {people.length ? (
          <p className="upload-people">{people.join(" · ")}</p>
        ) : null}
        {upload.albums.length ? (
          <p className="upload-albums">
            <Disc3 aria-hidden="true" />
            <span>
              {t("works.onAlbums")} {upload.albums.join(" / ")}
            </span>
          </p>
        ) : null}
        {upload.tracks.length ? (
          <details className="credit-details">
            <summary>{t("works.more")}</summary>
            <ol className="upload-songs">
              {upload.tracks.map((song, index) => (
                <li key={index}>{song}</li>
              ))}
            </ol>
          </details>
        ) : null}
        <p className="upload-links">
          {links.length ? (
            links.map((link) => {
              const Mark = platformMarks[link.platform]
              return (
                <ExternalLink
                  key={link.href}
                  className={`upload-link is-${link.platform}${link.members ? " is-members" : ""}`}
                  href={link.href}
                  aria-label={watchLabel(link)}
                  title={
                    link.platform === "bilibili" && upload.bilibiliTitle
                      ? `${t("works.platform.bilibili")} · ${upload.bilibiliTitle}`
                      : link.members
                        ? `${t("works.platform.youtube")} · ${t("works.membersOnly")}`
                        : t(`works.platform.${link.platform}`)
                  }
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
            })
          ) : (
            <span className="works-quiet">{t("works.noLink")}</span>
          )}
        </p>
      </div>
    </li>
  )
}

export function UploadsView() {
  const { t } = useAppSettings()
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
        <SearchBox
          value={query}
          onChange={setQuery}
          label={t("works.searchUploads")}
        />
        <OrderToggle order={order} onChange={setOrder} t={t} />
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
                  <UploadCard key={upload.id} upload={upload} index={index} />
                ))}
              </ul>
            </li>
          ))}
        </ol>
      ) : (
        <p className="works-empty">{t("works.empty")}</p>
      )}
    </>
  )
}
