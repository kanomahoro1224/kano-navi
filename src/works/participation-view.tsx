import { useMemo, useState } from "react"
import { ArrowUpRight, ListMusic, Users } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { ExternalLink } from "@/site/links"
import data from "./participation.json" with { type: "json" }
import { byDate, dayLabel, discsOf, groupByYear, matches } from "./content"
import type { OpenCovers } from "./personal-view"
import type { ParticipationRelease, ParticipationTrack } from "./types"
import {
  FilterButtons,
  OrderToggle,
  SearchBox,
  type Order,
} from "./works-controls"
import { WORKS_COVER_SIZES, WorksCoverImage } from "./works-media"

const releases = data as ParticipationRelease[]

type RoleFilter = "all" | "vocal" | "lyrics" | "other"
const FILTERS: RoleFilter[] = ["all", "vocal", "lyrics", "other"]

function keep(release: ParticipationRelease, filter: RoleFilter) {
  if (filter === "all") return true
  if (filter === "other")
    return release.roles.some((role) => role !== "vocal" && role !== "lyrics")
  return release.roles.includes(filter)
}

function TrackLine({
  track,
  withDisc = false,
}: {
  track: ParticipationTrack
  withDisc?: boolean
}) {
  const { t } = useAppSettings()
  return (
    <>
      <span className="works-track-no">
        {withDisc ? `${track.disc}-${track.no}` : track.no}
      </span>
      <span className="works-track-copy">
        <strong>{track.title}</strong>
        {track.credit || track.kanoCredits.length ? (
          <small>
            {track.credit}
            {track.credit && track.kanoCredits.length ? " · " : null}
            {track.kanoCredits.length ? (
              <span className="works-kano-credit">
                {t("works.kanoCredit", {
                  roles: track.kanoCredits
                    .map((credit) => t(`works.credit.${credit}`))
                    .join(" / "),
                })}
              </span>
            ) : null}
          </small>
        ) : null}
      </span>
    </>
  )
}

function ParticipationRow({
  release,
  onOpenCovers,
}: {
  release: ParticipationRelease
  onOpenCovers: OpenCovers
}) {
  const { t } = useAppSettings()
  const kanoTracks = release.tracks.filter((track) => track.kano)
  const discs = discsOf(release.tracks)
  const many = discs.length > 1
  return (
    <li className="credit-row">
      {release.cover ? (
        <button
          type="button"
          className="media-opener credit-cover"
          onClick={() =>
            release.cover && onOpenCovers([release.cover], 0, release.title)
          }
          aria-label={t("works.openCover", { title: release.title })}
        >
          <WorksCoverImage
            cover={release.cover}
            alt=""
            t={t}
            sizes={WORKS_COVER_SIZES.row}
          />
        </button>
      ) : (
        <span className="credit-cover">
          <WorksCoverImage cover={null} alt="" t={t} />
        </span>
      )}
      <div className="credit-copy">
        <p className="credit-meta">
          <time dateTime={release.date}>{dayLabel(release.date)}</time>
          {[release.label, release.event, release.catalog]
            .filter(Boolean)
            .map((value) => (
              <span key={value}>{value}</span>
            ))}
        </p>
        <h3>
          {release.source ? (
            <ExternalLink href={release.source}>
              {release.title}
              <ArrowUpRight aria-hidden="true" />
            </ExternalLink>
          ) : (
            release.title
          )}
        </h3>
        <p className="works-tags">
          {release.roles.map((role) => (
            <span key={role} className="tag is-accent">
              {t(`works.role.${role}`)}
            </span>
          ))}
          {release.forms.map((form) => (
            <span key={form} className="tag">
              {t(`works.form.${form}`)}
            </span>
          ))}
          {release.unit ? (
            <span className="tag">
              <Users aria-hidden="true" />
              {t("works.memberOf", { unit: release.unit })}
            </span>
          ) : null}
        </p>
        {kanoTracks.length ? (
          <ol className="credit-tracks" aria-label={t("works.kanoTracks")}>
            {kanoTracks.map((track) => (
              <li key={`${track.disc}-${track.no}`}>
                <TrackLine track={track} withDisc={many} />
              </li>
            ))}
          </ol>
        ) : null}
        {release.tracks.length > kanoTracks.length ? (
          <details className="credit-details">
            <summary>
              <ListMusic aria-hidden="true" />
              {t("works.fullTracklist", { count: release.tracks.length })}
            </summary>
            <div className="works-tracklist">
              {discs.map(({ disc, tracks }) => (
                <div key={disc}>
                  {many ? (
                    <p className="works-disc">{t("works.disc", { disc })}</p>
                  ) : null}
                  <ol>
                    {tracks.map((track) => (
                      <li
                        key={`${disc}-${track.no}`}
                        className={track.kano ? "is-kano" : undefined}
                      >
                        <TrackLine track={track} />
                      </li>
                    ))}
                  </ol>
                </div>
              ))}
            </div>
          </details>
        ) : null}
      </div>
    </li>
  )
}

export function ParticipationView({
  onOpenCovers,
}: {
  onOpenCovers: OpenCovers
}) {
  const { t } = useAppSettings()
  const [filter, setFilter] = useState<RoleFilter>("all")
  const [query, setQuery] = useState("")
  const [order, setOrder] = useState<Order>("newest")
  const groups = useMemo(
    () =>
      groupByYear(
        byDate(
          releases.filter(
            (release) =>
              keep(release, filter) &&
              matches(
                query,
                release.title,
                release.label,
                release.unit ?? "",
                ...release.tracks
                  .filter((track) => track.kano)
                  .map((track) => `${track.title} ${track.credit}`),
              ),
          ),
          order,
        ),
      ),
    [filter, query, order],
  )
  const shown = groups.reduce((sum, group) => sum + group.items.length, 0)
  return (
    <>
      <div className="works-toolbar">
        <FilterButtons
          label={t("works.roleFilter")}
          options={FILTERS}
          value={filter}
          onChange={setFilter}
          name={(item) => `works.roles.${item}`}
          count={(item) =>
            releases.filter((release) => keep(release, item)).length
          }
          t={t}
        />
        <SearchBox
          value={query}
          onChange={setQuery}
          label={t("works.searchParticipation")}
        />
        <OrderToggle order={order} onChange={setOrder} t={t} />
      </div>
      <p className="works-result-count" role="status">
        {t("works.releaseCount", { count: shown })}
      </p>
      {groups.length ? (
        <ol className="works-years">
          {groups.map((group) => (
            <li key={group.year} className="works-year">
              <h3 className="works-year-label">
                <span>{group.year}</span>
                <small>
                  {t("works.releaseCount", { count: group.items.length })}
                </small>
              </h3>
              <ul className="credit-list">
                {group.items.map((release) => (
                  <ParticipationRow
                    key={release.id}
                    release={release}
                    onOpenCovers={onOpenCovers}
                  />
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
