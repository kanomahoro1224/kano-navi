import type { CSSProperties } from "react"
import type { Locale, Translator } from "@/types"
import { useMemo, useState } from "react"
import { Disc3, Tv } from "lucide-react"

import {
  BilibiliLogo,
  ExternalLink,
  YouTubeLogo,
  YouTubeMembersMark,
} from "@/site/links"

import {
  byDate,
  dayLabel,
  discsOf,
  firstEdition,
  fullDate,
  groupByYear,
  personalReleases,
  uploadLinks,
} from "./content"
import type {
  DiscImage,
  PersonalEdition,
  PersonalFormat,
  PersonalRelease,
  StoreBonus,
  WorksCover,
  WorksTrack,
} from "./types"
import { FilterButtons, OrderToggle, type Order } from "./works-controls"
import { CreditLine, WORKS_COVER_SIZES, WorksCoverImage } from "./works-media"
import { WorksSheet } from "./works-sheet"

export type OpenCovers = (
  covers: WorksCover[],
  index: number,
  title: string,
) => void

type PersonalFilter = "all" | PersonalFormat | "unit"

const FILTERS: PersonalFilter[] = ["all", "album", "single", "doujin", "unit"]

function keep(release: PersonalRelease, filter: PersonalFilter) {
  if (filter === "all") return true
  if (filter === "unit") return release.unit !== null
  if (filter === "single") return ["single", "ep"].includes(release.format)
  return release.format === filter
}

/** Format first, then digital: the small line above a title. */
export function releaseKind(release: PersonalRelease, t: Translator) {
  return [
    t(`works.format.${release.format}`),
    release.digital ? t("works.digital") : "",
  ]
    .filter(Boolean)
    .join(" · ")
}

/**
 * A cover in its sleeve with the record behind it, which slides out when the
 * card is pointed at. The record is the disc photo when there is one (also
 * for a release whose only cover is its disc). A download has no disc, so its
 * cover is printed on one; otherwise the record's label shows the cover again.
 */
export function Sleeve({
  cover,
  disc = null,
  printed = false,
  t,
  sizes,
  eager = false,
  className = "",
}: {
  cover: WorksCover | null
  /** A photo of the real disc, shown as the record when there is one. */
  disc?: WorksCover | null
  /** Print the cover on a disc: for a download, which has none. */
  printed?: boolean
  t: Translator
  sizes: string
  eager?: boolean
  className?: string
}) {
  const face = disc ?? (printed ? cover : null)
  return (
    <span className={`works-sleeve ${className}`}>
      <span
        className={`works-record${face ? " has-disc" : ""}`}
        aria-hidden="true"
      >
        {face ? (
          <span className={`works-record-disc${disc ? "" : " is-printed"}`}>
            <WorksCoverImage
              cover={face}
              alt=""
              t={t}
              sizes={WORKS_COVER_SIZES.card}
            />
          </span>
        ) : (
          <span className="works-record-label">
            {cover ? (
              <WorksCoverImage
                cover={cover}
                alt=""
                t={t}
                sizes={WORKS_COVER_SIZES.label}
              />
            ) : null}
          </span>
        )}
      </span>
      <span className="works-cover">
        <WorksCoverImage
          cover={cover}
          alt=""
          t={t}
          sizes={sizes}
          eager={eager}
        />
      </span>
    </span>
  )
}

/**
 * The front of the sleeve: the first cover that is not the disc photo, or
 * the disc photo itself for a release that has no cover.
 */
export function frontCover(release: PersonalRelease) {
  return (
    release.covers.find((cover) => cover.media !== release.disc?.media) ??
    release.covers[0] ??
    null
  )
}

function ReleaseCard({
  release,
  index,
  t,
  onOpen,
}: {
  release: PersonalRelease
  index: number
  t: Translator
  onOpen: () => void
}) {
  const aside = [
    release.editions.length > 1
      ? t("works.editionCount", { count: release.editions.length })
      : "",
    release.cancelled ? t("works.cancelled") : "",
  ].filter(Boolean)
  return (
    <li className="release-card" style={{ "--order": index } as CSSProperties}>
      <button type="button" aria-haspopup="dialog" onClick={onOpen}>
        <Sleeve
          cover={frontCover(release)}
          disc={release.disc}
          printed={release.printedDisc}
          t={t}
          sizes={WORKS_COVER_SIZES.card}
        />
        <span className="release-copy">
          <span className="works-meta">
            <time dateTime={release.date}>{dayLabel(release.date)}</time>
            <span>{releaseKind(release, t)}</span>
          </span>
          <strong>{release.title}</strong>
          {release.unit ? (
            <span className="release-unit">{release.unit}</span>
          ) : null}
        </span>
        {aside.length ? (
          <span className="release-editions">{aside.join(" · ")}</span>
        ) : null}
      </button>
    </li>
  )
}

const listenMarks = {
  niconico: Tv,
  youtube: YouTubeLogo,
  bilibili: BilibiliLogo,
}

/** Small buttons to the same recording's uploads. */
function ListenLinks({ track, t }: { track: WorksTrack; t: Translator }) {
  if (!track.listen) return null
  const links = uploadLinks(track.listen)
  if (!links.length) return null
  return (
    <span className="works-listen">
      {links.map((link) => {
        const Mark = listenMarks[link.platform]
        const platform = t(`works.platform.${link.platform}`)
        return (
          <ExternalLink
            key={link.href}
            className={`works-listen-link is-${link.platform}`}
            href={link.href}
            aria-label={t(
              link.members ? "works.watchOnMembers" : "works.watchOn",
              { platform, title: track.title },
            )}
            title={
              link.members
                ? `${platform} · ${t("works.membersOnly")}`
                : platform
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
      })}
    </span>
  )
}

/**
 * The tracklist by disc. A disc with a photo shows it beside its heading;
 * selecting it opens the edition's disc photos in the viewer.
 */
function Tracklist({
  tracks,
  photos,
  title,
  t,
  onOpenCovers,
}: {
  tracks: WorksTrack[]
  photos: DiscImage[]
  title: string
  t: Translator
  onOpenCovers: OpenCovers
}) {
  if (!tracks.length)
    return <p className="works-quiet">{t("works.noTracks")}</p>
  const discs = discsOf(tracks)
  const images = photos.map((item) => item.image)
  return (
    <div className="works-tracklist">
      {discs.map(({ disc, tracks: discTracks }) => {
        const label = discTracks[0]?.discLabel
        const photo = photos.findIndex((item) => item.disc === disc)
        const heading = [
          discs.length > 1 ? t("works.disc", { disc }) : "",
          label,
        ].filter(Boolean)
        return (
          <div key={disc}>
            {heading.length || photo >= 0 ? (
              <div className="works-disc-head">
                {photo >= 0 ? (
                  <button
                    type="button"
                    className="media-opener works-disc-photo"
                    onClick={() => onOpenCovers(images, photo, title)}
                    aria-label={t("works.openDisc", { disc, title })}
                  >
                    <WorksCoverImage
                      cover={images[photo]}
                      alt=""
                      t={t}
                      sizes={WORKS_COVER_SIZES.discPhoto}
                    />
                  </button>
                ) : null}
                {heading.length ? (
                  <p className="works-disc">{heading.join(" · ")}</p>
                ) : null}
              </div>
            ) : null}
            <ol>
              {discTracks.map((track) => (
                <li key={`${disc}-${track.no}`}>
                  <span className="works-track-no">
                    {String(track.no).padStart(2, "0")}
                  </span>
                  <span className="works-track-copy">
                    <strong>{track.title}</strong>
                    {track.credit || track.note ? (
                      <small>
                        {[track.credit, track.note].filter(Boolean).join(" · ")}
                      </small>
                    ) : null}
                  </span>
                  <ListenLinks track={track} t={t} />
                </li>
              ))}
            </ol>
          </div>
        )
      })}
    </div>
  )
}

/** Images that open in the viewer: package contents and store bonuses. */
function ImageStrip({
  images,
  title,
  t,
  onOpenCovers,
}: {
  images: WorksCover[]
  title: string
  t: Translator
  onOpenCovers: OpenCovers
}) {
  if (!images.length) return null
  return (
    <ul className="works-thumbs">
      {images.map((image, index) => (
        <li key={image.media}>
          <button
            type="button"
            className="media-opener"
            onClick={() => onOpenCovers(images, index, title)}
            aria-label={t("works.openImage", {
              title: image.caption || title,
            })}
            title={image.caption}
          >
            <WorksCoverImage
              cover={image}
              alt=""
              t={t}
              sizes={WORKS_COVER_SIZES.edition}
            />
          </button>
        </li>
      ))}
    </ul>
  )
}

function uniqueImages(groups: WorksCover[][]) {
  const seen = new Map<string, WorksCover>()
  for (const image of groups.flat())
    if (!seen.has(image.media)) seen.set(image.media, image)
  return [...seen.values()]
}

function BonusList({ items }: { items: StoreBonus[] }) {
  return (
    <ul className="works-bonuses">
      {items.map((bonus, index) => (
        <li key={index}>
          <span className="works-bonus-store">{bonus.store}</span>
          <span>{bonus.item}</span>
        </li>
      ))}
    </ul>
  )
}

function Facts({ facts, t }: { facts: [string, string][]; t: Translator }) {
  const shown = facts.filter(([, value]) => value)
  if (!shown.length) return null
  return (
    <dl className="works-facts">
      {shown.map(([key, value]) => (
        <div key={key}>
          <dt>{t(key)}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  )
}

function EditionPicker({
  editions,
  value,
  onChange,
  t,
}: {
  editions: PersonalEdition[]
  value: number
  onChange: (index: number) => void
  t: Translator
}) {
  return (
    <div
      className="works-edition-picker"
      role="group"
      aria-label={t("works.editionPicker")}
    >
      {editions.map((edition, index) => {
        const cover = edition.covers[0] ?? null
        const detail = [
          edition.format,
          edition.cancelled ? t("works.cancelled") : "",
        ]
          .filter(Boolean)
          .join(" · ")
        return (
          <button
            key={edition.name}
            type="button"
            aria-pressed={index === value}
            onClick={() => onChange(index)}
          >
            <span className="works-edition-art" aria-hidden="true">
              {cover ? (
                <WorksCoverImage
                  cover={cover}
                  alt=""
                  t={t}
                  sizes={WORKS_COVER_SIZES.edition}
                />
              ) : (
                <Disc3 />
              )}
            </span>
            <span className="works-edition-text">
              <span className="works-edition-name">{edition.name}</span>
              {detail ? <small>{detail}</small> : null}
            </span>
          </button>
        )
      })}
    </div>
  )
}

export function ReleaseSheet({
  release,
  locale,
  t,
  onClose,
  onOpenCovers,
}: {
  release: PersonalRelease
  locale: Locale
  t: Translator
  onClose: () => void
  onOpenCovers: OpenCovers
}) {
  const [editionIndex, setEditionIndex] = useState(() =>
    firstEdition(release.editions),
  )
  const [shown, setShown] = useState(0)
  const edition = release.editions[editionIndex] ?? null
  const covers = edition?.covers.length ? edition.covers : release.covers
  const cover = covers[shown] ?? null
  const caption = cover?.caption ?? ""
  const tracks = edition ? edition.tracks : release.tracks
  const discPhotos = edition ? edition.discs : release.discs
  // The record in the header is the chosen edition's first disc, drawn when
  // that edition's disc has no photo (another edition's may look different).
  const recordDisc = edition
    ? (discPhotos.find((item) => item.disc === 1)?.image ?? null)
    : release.disc
  const storeBonuses = edition?.storeBonuses ?? []
  const bonusImages = uniqueImages([
    ...storeBonuses.map((bonus) => bonus.images),
    ...release.storeBonuses.map((bonus) => bonus.images),
  ])
  const listened = new Set(
    tracks.filter((track) => track.listen).map((track) => track.title),
  ).size
  const chooseEdition = (index: number) => {
    setEditionIndex(index)
    setShown(0)
  }
  return (
    <WorksSheet label={release.title} onClose={onClose}>
      <article className="release-sheet">
        <header className="release-head">
          {cover ? (
            <span className="release-head-backdrop" aria-hidden="true">
              <WorksCoverImage
                key={cover.media}
                cover={cover}
                alt=""
                t={t}
                sizes="320px"
              />
            </span>
          ) : null}
          <div className="release-head-inner">
            <div className="release-head-art">
              {cover ? (
                <button
                  type="button"
                  className="media-opener release-head-cover"
                  onClick={() => onOpenCovers(covers, shown, release.title)}
                  aria-label={t("works.openCover", { title: release.title })}
                >
                  <Sleeve
                    key={cover.media}
                    cover={cover}
                    disc={recordDisc}
                    printed={(edition ?? release).printedDisc}
                    t={t}
                    sizes={WORKS_COVER_SIZES.sheet}
                    eager
                    className="is-sheet"
                  />
                </button>
              ) : (
                <Sleeve
                  cover={null}
                  t={t}
                  sizes={WORKS_COVER_SIZES.sheet}
                  className="is-sheet"
                />
              )}
              {covers.length > 1 ? (
                <div
                  className="release-sheet-editions"
                  role="group"
                  aria-label={t("works.coverPicker")}
                >
                  {covers.map((item, index) => (
                    <button
                      key={item.media}
                      type="button"
                      aria-pressed={index === shown}
                      aria-label={t("works.editionNumber", {
                        number: index + 1,
                        count: covers.length,
                      })}
                      title={item.caption}
                      onClick={() => setShown(index)}
                    >
                      <WorksCoverImage
                        cover={item}
                        alt=""
                        t={t}
                        sizes={WORKS_COVER_SIZES.edition}
                      />
                    </button>
                  ))}
                </div>
              ) : null}
              {caption || cover?.thirdParty || cover?.credit ? (
                <p className="release-sheet-caption">
                  {caption ? <span lang="zh-CN">{caption}</span> : null}
                  {cover?.credit ? (
                    <small>
                      <CreditLine credit={cover.credit} t={t} />
                    </small>
                  ) : cover?.thirdParty ? (
                    <small>{t("works.thirdPartyFile")}</small>
                  ) : null}
                </p>
              ) : null}
            </div>
            <div className="release-head-copy">
              <p className="works-eyebrow">
                <time dateTime={release.date}>
                  {fullDate(release.date, locale)}
                </time>
                <span>{releaseKind(release, t)}</span>
                {release.cancelled ? <span>{t("works.cancelled")}</span> : null}
              </p>
              <h2>{release.title}</h2>
              {release.unit ? (
                <p className="release-sheet-unit">{release.unit}</p>
              ) : null}
              <p className="release-head-label">{release.label}</p>
              <p className="release-head-figures">
                <span>
                  <strong>{tracks.length}</strong>
                  {t("works.figureTracks")}
                </span>
                {release.editions.length > 1 ? (
                  <span>
                    <strong>{release.editions.length}</strong>
                    {t("works.editions")}
                  </span>
                ) : null}
                {listened ? (
                  <span>
                    <strong>{listened}</strong>
                    {t("works.figureListen")}
                  </span>
                ) : null}
              </p>
              {release.editions.length > 1 ? (
                <EditionPicker
                  editions={release.editions}
                  value={editionIndex}
                  onChange={chooseEdition}
                  t={t}
                />
              ) : null}
            </div>
          </div>
        </header>
        <div className="release-body">
          <section className="release-tracks" aria-label={t("works.tracklist")}>
            <p className="works-field-label">
              {t("works.tracklist")}
              {edition && release.editions.length > 1 ? (
                <small>{edition.name}</small>
              ) : null}
            </p>
            <Tracklist
              tracks={tracks}
              photos={discPhotos}
              title={release.title}
              t={t}
              onOpenCovers={onOpenCovers}
            />
          </section>
          <aside className="release-aside">
            {edition ? (
              <section className="release-edition">
                <h3 className="release-edition-name">
                  {edition.name}
                  {edition.cancelled ? (
                    <span className="tag is-accent">
                      {t("works.cancelled")}
                    </span>
                  ) : null}
                </h3>
                <Facts
                  facts={[
                    [
                      "works.editionDate",
                      edition.date ? fullDate(edition.date, locale) : "",
                    ],
                    ["works.catalog", edition.catalog],
                    ["works.jan", edition.jan],
                    ["works.price", edition.price],
                    ["works.discFormat", edition.format],
                    ["works.packaging", edition.packaging],
                  ]}
                  t={t}
                />
                {edition.bonuses.length || edition.bonusImages.length ? (
                  <>
                    <p className="works-field-label">{t("works.inPackage")}</p>
                    {edition.bonuses.length ? (
                      <ul className="works-package">
                        {edition.bonuses.map((item) => (
                          <li key={item}>{item}</li>
                        ))}
                      </ul>
                    ) : null}
                    <ImageStrip
                      images={edition.bonusImages}
                      title={release.title}
                      t={t}
                      onOpenCovers={onOpenCovers}
                    />
                  </>
                ) : null}
              </section>
            ) : (
              // A release sold in one version shows its own facts here.
              <section className="release-edition">
                <h3 className="release-edition-name">
                  {releaseKind(release, t)}
                </h3>
                <Facts
                  facts={[
                    ["works.editionDate", fullDate(release.date, locale)],
                    ["works.labelName", release.label],
                    ["works.catalog", release.catalog],
                  ]}
                  t={t}
                />
              </section>
            )}
            {storeBonuses.length || release.storeBonuses.length ? (
              <section className="release-bonuses">
                <p className="works-field-label">{t("works.storeBonuses")}</p>
                {storeBonuses.length ? (
                  <BonusList items={storeBonuses} />
                ) : null}
                {release.storeBonuses.length ? (
                  <>
                    {edition ? (
                      <p className="works-bonus-scope">
                        {t("works.anyEdition")}
                      </p>
                    ) : null}
                    <BonusList items={release.storeBonuses} />
                  </>
                ) : null}
                <ImageStrip
                  images={bonusImages}
                  title={release.title}
                  t={t}
                  onOpenCovers={onOpenCovers}
                />
              </section>
            ) : null}
          </aside>
        </div>
      </article>
    </WorksSheet>
  )
}

export function PersonalView({
  t,
  onOpenRelease,
}: {
  t: Translator
  onOpenRelease: (id: string) => void
}) {
  const [filter, setFilter] = useState<PersonalFilter>("all")
  const [order, setOrder] = useState<Order>("newest")
  const groups = useMemo(
    () =>
      groupByYear(
        byDate(
          personalReleases.filter((release) => keep(release, filter)),
          order,
        ),
      ),
    [filter, order],
  )
  return (
    <>
      <div className="works-toolbar">
        <FilterButtons
          label={t("works.personalFilter")}
          options={FILTERS}
          value={filter}
          onChange={setFilter}
          name={(item) =>
            item === "unit" ? "works.units" : `works.formats.${item}`
          }
          count={(item) =>
            personalReleases.filter((release) => keep(release, item)).length
          }
          t={t}
        />
        <OrderToggle order={order} onChange={setOrder} t={t} />
      </div>
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
              <ul className="release-grid">
                {group.items.map((release, index) => (
                  <ReleaseCard
                    key={release.id}
                    release={release}
                    index={index}
                    t={t}
                    onOpen={() => onOpenRelease(release.id)}
                  />
                ))}
              </ul>
            </li>
          ))}
        </ol>
      ) : (
        <p className="works-empty">
          <Disc3 aria-hidden="true" />
          {t("works.empty")}
        </p>
      )}
    </>
  )
}
