import type { ComponentType, CSSProperties } from "react"
import type { Translator } from "@/types"
import { useCallback, useEffect, useState } from "react"
import { ArrowUpRight, Info, RotateCw } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { mediaSrcSet } from "@/lib/media"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { ExternalLink } from "@/site/links"
import { REPOSITORY_URL } from "@/site/repository"
import { MediaDialog, stepIndex } from "@/site/media-dialog"
import {
  Placeholder,
  SiteFooter,
  SiteHeader,
  SkipLink,
} from "@/site/site-shell"
import {
  OFFICIAL_DISCOGRAPHY,
  fullDate,
  mediaUrl,
  personalReleases,
  worksSummary,
} from "./content"
import {
  PersonalView,
  ReleaseSheet,
  Sleeve,
  frontCover,
  type OpenCovers,
} from "./personal-view"
import type { PersonalRelease, WorksCover } from "./types"
import { CoverDetails, WORKS_COVER_SIZES, WorksCoverImage } from "./works-media"
import "./works.css"

type Section = "personal" | "participation" | "uploads"
const SECTIONS: Section[] = ["personal", "participation", "uploads"]

type SectionProps = {
  onOpenCovers: OpenCovers
}

/** The two longer sections load their code and data when first shown. */
const sectionLoaders: Record<
  Exclude<Section, "personal">,
  () => Promise<ComponentType<SectionProps>>
> = {
  participation: () =>
    import("./participation-view").then((module) => module.ParticipationView),
  uploads: () => import("./uploads-view").then((module) => module.UploadsView),
}

const loadedSections = new Map<Section, ComponentType<SectionProps>>()

function useSection(section: Exclude<Section, "personal">, wanted: boolean) {
  const [state, setState] = useState<{
    View: ComponentType<SectionProps> | null
    failed: boolean
  }>(() => ({ View: loadedSections.get(section) ?? null, failed: false }))
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    if (!wanted || state.View) return undefined
    let current = true
    sectionLoaders[section]().then(
      (View) => {
        loadedSections.set(section, View)
        if (current) setState({ View, failed: false })
      },
      () => {
        if (current) setState({ View: null, failed: true })
      },
    )
    return () => {
      current = false
    }
  }, [section, wanted, state.View, attempt])
  const retry = useCallback(() => {
    setState({ View: null, failed: false })
    setAttempt((value) => value + 1)
  }, [])
  return { ...state, retry }
}

function sectionFromUrl(): Section {
  const value = new URLSearchParams(window.location.search).get("view")
  return SECTIONS.includes(value as Section) ? (value as Section) : "personal"
}

/** The acoustic cover trilogy, in album order. */
const TRILOGY = ["one", "two", "three"]

/** The order the trilogy leads the page in, left to right. */
const FEATURED_TITLES = ["two", "one", "three"]

const featured = FEATURED_TITLES.map((title) =>
  personalReleases.find((release) => release.title === title),
).filter((release) => release !== undefined)

/** Each album's own colour, taken from its cover: scarf, stripes, dress. */
const FEATURED_TONES: Record<string, string> = {
  one: "#c8432f",
  two: "#e0aa2b",
  three: "#4862bd",
}

/** The trilogy stands as the figures cut from its covers when all have one. */
const figuresReady =
  featured.length === FEATURED_TITLES.length &&
  featured.every((release) => release.cutout)

/**
 * A figure cut from an album's cover, standing before a disc in the album's
 * colour. Without its image it falls back to the album's sleeve.
 */
function FeaturedFigure({
  release,
  t,
}: {
  release: PersonalRelease
  t: Translator
}) {
  const [failed, setFailed] = useState(false)
  const cutout = release.cutout
  if (!cutout || failed)
    return (
      <Sleeve
        cover={frontCover(release)}
        disc={release.disc}
        t={t}
        sizes={WORKS_COVER_SIZES.trio}
        eager
        className="is-trio"
      />
    )
  const url = mediaUrl(cutout.media)
  return (
    <span className="works-figure">
      <span className="works-figure-sun" aria-hidden="true" />
      <img
        src={url}
        srcSet={mediaSrcSet(url, cutout.width)}
        sizes={WORKS_COVER_SIZES.figure}
        alt=""
        width={cutout.width}
        height={cutout.height}
        decoding="async"
        onError={() => setFailed(true)}
      />
      <span className="works-figure-label" aria-hidden="true">
        <b>{String(TRILOGY.indexOf(release.title) + 1).padStart(2, "0")}</b>
        {release.title}
      </span>
    </span>
  )
}

type Point = [number, number]
type Curve = { start: Point; segments: [Point, Point, Point][] }

/**
 * The ground the figures stand and sit on: two sits lowest at the left, one
 * stands in the middle, three sits on the rise. Each figure's base in
 * works.css sits on this curve.
 */
const GROUND: Curve = {
  start: [0, 900],
  segments: [
    [
      [50, 922],
      [120, 928],
      [260, 928],
    ],
    [
      [400, 928],
      [470, 924],
      [530, 902],
    ],
    [
      [570, 888],
      [610, 880],
      [700, 878],
    ],
    [
      [790, 876],
      [840, 860],
      [930, 858],
    ],
    [
      [1030, 850],
      [1110, 862],
      [1160, 872],
    ],
  ],
}

const HILLS: Curve[] = [
  {
    start: [0, 600],
    segments: [
      [
        [120, 540],
        [250, 548],
        [360, 590],
      ],
      [
        [470, 630],
        [580, 560],
        [700, 550],
      ],
      [
        [820, 540],
        [930, 610],
        [1040, 580],
      ],
      [
        [1100, 565],
        [1140, 550],
        [1160, 548],
      ],
    ],
  },
  {
    start: [0, 760],
    segments: [
      [
        [120, 700],
        [250, 700],
        [360, 740],
      ],
      [
        [470, 778],
        [580, 720],
        [700, 705],
      ],
      [
        [820, 690],
        [920, 750],
        [1030, 738],
      ],
      [
        [1090, 730],
        [1130, 712],
        [1160, 706],
      ],
    ],
  },
]

const curvePath = ({ start, segments }: Curve) =>
  `M${start.join(" ")}` +
  segments
    .map((points) => ` C${points.map((p) => p.join(" ")).join(" ")}`)
    .join("")

const filled = (curve: Curve) => `${curvePath(curve)} V1000 H0 Z`

/** A point a fraction t along one cubic segment of a curve. */
function pointOn({ start, segments }: Curve, segment: number, t: number) {
  const from = segment ? segments[segment - 1][2] : start
  const [c1, c2, to] = segments[segment]
  const u = 1 - t
  const axis = (i: 0 | 1) =>
    u * u * u * from[i] +
    3 * u * u * t * c1[i] +
    3 * u * t * t * c2[i] +
    t * t * t * to[i]
  return [axis(0), axis(1)] as Point
}

/** Grass along the ground's crest: three blades every few steps. */
const GRASS = GROUND.segments
  .flatMap((_, segment) =>
    [0.08, 0.3, 0.52, 0.74, 0.94].map((t, i) => {
      const [x, y] = pointOn(GROUND, segment, t)
      const lean = (segment + i) % 2 ? 1 : -1
      return `M${x} ${y + 2}l${-5 * lean} -15M${x + 4} ${y + 2}l${2 * lean} -20M${x + 8} ${y + 2}l${7 * lean} -13`
    }),
  )
  .join("")

/** Small flowers in the three albums' colours, along the crest. */
const FLOWERS = (
  [
    [0, 0.15, "one"],
    [0, 0.75, "three"],
    [1, 0.6, "two"],
    [2, 0.5, "three"],
    [3, 0.3, "one"],
    [3, 0.85, "two"],
    [4, 0.55, "one"],
  ] as const
).map(([segment, t, title]) => {
  const [x, y] = pointOn(GROUND, segment, t)
  return { x, y: y - 9, tone: FEATURED_TONES[title] }
})

/**
 * Rolling hills behind the figures, after the night hills of a fan-made MV:
 * two far ridges the discs sink behind, then the grassy ground in front.
 */
function FeaturedLandscape() {
  return (
    <svg
      className="works-landscape"
      viewBox="0 0 1160 1000"
      preserveAspectRatio="none"
      aria-hidden="true"
      focusable="false"
    >
      <path className="is-far" d={filled(HILLS[0])} />
      <path className="is-mid" d={filled(HILLS[1])} />
      <path className="is-near" d={filled(GROUND)} />
      <path className="is-crest" d={curvePath(GROUND)} />
      <path className="is-grass" d={GRASS} />
      {FLOWERS.map((flower, index) => (
        <g key={index} className="is-flower">
          <circle cx={flower.x} cy={flower.y} r="6.5" fill={flower.tone} />
          <circle cx={flower.x} cy={flower.y} r="2.2" />
        </g>
      ))}
    </svg>
  )
}

function LazySection({
  section,
  active,
  onOpenCovers,
}: {
  section: Exclude<Section, "personal">
  active: boolean
  onOpenCovers: OpenCovers
}) {
  const { t } = useAppSettings()
  const { View, failed, retry } = useSection(section, active)
  if (View) return <View onOpenCovers={onOpenCovers} />
  if (failed)
    return (
      <div className="works-load-failed" role="alert">
        <p>{t("works.loadFailed")}</p>
        <button type="button" className="works-chip" onClick={retry}>
          <RotateCw aria-hidden="true" />
          {t("works.retry")}
        </button>
      </div>
    )
  return (
    <div className="works-loading" role="status">
      <span className="sr-only">{t("works.loading")}</span>
      <Placeholder lines={4} />
    </div>
  )
}

export function WorksApp() {
  const { locale, t } = useAppSettings()
  const [section, setSection] = useState<Section>(sectionFromUrl)
  const [openId, setOpenId] = useState<string | null>(null)
  const [pointerTabs, setPointerTabs] = useState(false)
  const [viewer, setViewer] = useState<{
    items: WorksCover[]
    index: number
    title: string
  } | null>(null)

  useEffect(() => {
    document.title = t("works.metaTitle")
    document
      .querySelector('meta[name="description"]')
      ?.setAttribute("content", t("works.metaDescription"))
  }, [t])

  useEffect(() => {
    const onPop = () => setSection(sectionFromUrl())
    window.addEventListener("popstate", onPop)
    return () => window.removeEventListener("popstate", onPop)
  }, [])

  const choose = (next: string) => {
    const value = next as Section
    setSection(value)
    const url = new URL(window.location.href)
    if (value === "personal") url.searchParams.delete("view")
    else url.searchParams.set("view", value)
    window.history.replaceState(window.history.state, "", url.href)
  }

  const openCovers = useCallback<OpenCovers>((items, index, title) => {
    setViewer({ items, index, title })
  }, [])
  const closeCovers = useCallback(() => setViewer(null), [])
  const stepCovers = useCallback((offset: number) => {
    setViewer((current) =>
      current
        ? {
            ...current,
            index: stepIndex(current.index, offset, current.items.length),
          }
        : current,
    )
  }, [])

  const counts: Record<Section, number> = {
    personal: worksSummary.personal,
    participation: worksSummary.participation,
    uploads: worksSummary.uploads,
  }
  const open = personalReleases.find((release) => release.id === openId)

  return (
    <div className="site works" id="top">
      <SkipLink href="#works-content">{t("works.skip")}</SkipLink>
      <SiteHeader page="works" />

      <main className="site-main" id="works-content">
        <section className="works-hero" aria-labelledby="works-title">
          <div className="works-hero-copy">
            <p className="works-eyebrow">
              <span>{t("works.kicker")}</span>
              <span>
                {worksSummary.firstYear}–{worksSummary.lastYear}
              </span>
            </p>
            <h1 id="works-title">{t("works.title")}</h1>
            <p className="works-lead">{t("works.lead")}</p>
            <p className="works-caveat">
              <Info aria-hidden="true" />
              <span>
                {t("works.caveat")}{" "}
                <ExternalLink href={`${REPOSITORY_URL}/issues`}>
                  {t("works.reportIssue")}
                </ExternalLink>
              </span>
            </p>
            <ExternalLink className="works-link" href={OFFICIAL_DISCOGRAPHY}>
              {t("works.officialDiscography")}
              <ArrowUpRight aria-hidden="true" />
            </ExternalLink>
          </div>
          {featured.length ? (
            <div className="works-trio">
              <div className="works-trio-scene">
                {figuresReady ? <FeaturedLandscape /> : null}
                <ul
                  className={`works-trio-stage${figuresReady ? " is-figures" : ""}`}
                  aria-label={t("works.featured")}
                >
                  {featured.map((release, index) => (
                    <li
                      key={release.id}
                      className={`is-${index} is-${release.title}`}
                      style={
                        {
                          "--tone": FEATURED_TONES[release.title],
                          "--order": index,
                        } as CSSProperties
                      }
                    >
                      <button
                        type="button"
                        className="works-trio-item"
                        aria-haspopup="dialog"
                        aria-label={`${release.title} · ${fullDate(release.date, locale)}`}
                        onClick={() => setOpenId(release.id)}
                      >
                        {figuresReady ? (
                          <FeaturedFigure release={release} t={t} />
                        ) : (
                          <Sleeve
                            cover={frontCover(release)}
                            disc={release.disc}
                            t={t}
                            sizes={WORKS_COVER_SIZES.trio}
                            eager
                            className="is-trio"
                          />
                        )}
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          ) : null}
        </section>

        <Tabs value={section} className="works-sections" onValueChange={choose}>
          {/*
           * The tabs focus themselves when chosen, which the browser counts
           * as keyboard focus; a pointer choice hides that outline until the
           * next key press brings it back.
           */}
          <TabsList
            className="works-tabs"
            aria-label={t("works.sections")}
            data-pointer={pointerTabs || undefined}
            onPointerDown={() => setPointerTabs(true)}
            onKeyDown={() => setPointerTabs(false)}
          >
            {SECTIONS.map((item) => (
              <TabsTrigger key={item} value={item}>
                {t(`works.section.${item}`)}
                <sup>{counts[item]}</sup>
              </TabsTrigger>
            ))}
          </TabsList>

          <TabsContent value="personal" className="works-panel">
            <PersonalView t={t} onOpenRelease={setOpenId} />
          </TabsContent>
          <TabsContent value="participation" className="works-panel">
            <LazySection
              section="participation"
              active={section === "participation"}
              onOpenCovers={openCovers}
            />
          </TabsContent>
          <TabsContent value="uploads" className="works-panel">
            <LazySection
              section="uploads"
              active={section === "uploads"}
              onOpenCovers={openCovers}
            />
          </TabsContent>
        </Tabs>
      </main>

      <SiteFooter disclaimer={t("works.disclaimer")} />
      {open ? (
        <ReleaseSheet
          key={open.id}
          release={open}
          locale={locale}
          t={t}
          onClose={() => setOpenId(null)}
          onOpenCovers={openCovers}
        />
      ) : null}
      <MediaDialog
        items={viewer?.items}
        index={viewer?.index || 0}
        label={viewer?.title || ""}
        onClose={closeCovers}
        onStep={stepCovers}
        renderImage={(item) => (
          <WorksCoverImage
            key={item.media}
            cover={item}
            alt={viewer?.title || ""}
            t={t}
            eager
          />
        )}
        renderDetails={(item) => (
          <CoverDetails cover={item} title={viewer?.title || ""} t={t} />
        )}
      />
    </div>
  )
}
