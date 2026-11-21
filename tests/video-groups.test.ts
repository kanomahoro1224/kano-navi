import assert from "node:assert/strict"
import test from "node:test"
import type { DashboardProjection, Video, VideoView } from "../src/types.ts"
import {
  groupVideos,
  sideSlots,
  splitVideos,
  videoIsShort,
  videoIsStream,
  videoLinks,
  videoMatchesFilter,
  videoPhase,
  videoPlatforms,
} from "../src/dashboard/video-groups.ts"
import {
  durationAttribute,
  formatDuration,
  videoFormLabel,
  videoTagLabel,
} from "../src/dashboard/format.ts"

const past: VideoView = {
  phase: "past",
  formCode: "video",
  shelfGroup: "work",
  latestEligible: true,
  focusEligible: false,
  tagContext: "work",
}

function video(
  id: string,
  fields: Partial<Video> = {},
  view: Partial<VideoView> = {},
): Video {
  return {
    id,
    source: "youtube",
    title: `Synthetic ${id}`,
    publishedAt: null,
    scheduledAt: null,
    url: `https://www.youtube.com/watch?v=${id}`,
    thumbnailUrl: `/media/${id}`,
    thumbnailStatus: "ready",
    kind: "stream",
    isUpcoming: false,
    ...fields,
    view: { ...past, ...view },
  }
}

function shelf(
  lists: Partial<DashboardProjection["shelf"]>,
): DashboardProjection {
  return {
    version: 1,
    nextTransitionAt: null,
    focus: { eventIds: [], videoIds: [] },
    spotlight: null,
    sales: [],
    shelf: {
      leadId: null,
      leadIsFeatured: false,
      reservations: [],
      works: [],
      archives: [],
      shorts: [],
      ...lists,
    },
  }
}

const ids = (videos: Video[]) => videos.map((item) => item.id)

test("broadcasts are streams and other forms are works", () => {
  assert.equal(videoIsStream(video("s", { kind: "stream" })), true)
  for (const kind of ["premiere", "upload", "unknown", ""])
    assert.equal(videoIsStream(video("w", { kind })), false, kind)
  // What a video is about never makes it a stream.
  assert.equal(
    videoIsStream(video("g", { kind: "upload", tag: "game" })),
    false,
  )
  assert.equal(videoMatchesFilter(video("s"), "streams"), true)
  assert.equal(videoMatchesFilter(video("s"), "works"), false)
  assert.equal(videoMatchesFilter(video("w", { kind: "upload" }), "all"), true)
})

test("a video's phase is the server's", () => {
  for (const phase of [
    "upcoming",
    "live",
    "started",
    "past",
    "cancelled",
  ] as const)
    // The stored fields say otherwise; the view alone decides.
    assert.equal(
      videoPhase(video("v", { isUpcoming: phase !== "upcoming" }, { phase })),
      phase,
    )
})

test("the shelf follows the projection's lead and order, listing every video once", () => {
  const cover = video("cover", { kind: "premiere", tag: "song" })
  const clip = video("clip", { kind: "upload" })
  const older = video("older", { kind: "upload", tag: "song" })
  const archive = video("archive", {}, { shelfGroup: "archive" })
  const short = video("short", { isShort: true }, { shelfGroup: "short" })
  const sooner = video(
    "sooner",
    { isUpcoming: true },
    { phase: "upcoming", shelfGroup: "reservation" },
  )
  const later = video(
    "later",
    { isUpcoming: true },
    { phase: "upcoming", shelfGroup: "reservation" },
  )
  // A video the projection leaves off the shelf, such as a started
  // cancellation, has no group.
  const dropped = video("dropped", {}, { phase: "cancelled", shelfGroup: "" })
  const videos = [older, later, archive, clip, dropped, short, cover, sooner]
  const projection = shelf({
    leadId: "cover",
    reservations: ["sooner", "later"],
    works: ["clip", "older"],
    archives: ["archive"],
    shorts: ["short"],
  })

  const groups = groupVideos(videos, projection)
  assert.equal(groups.lead?.id, "cover")
  assert.equal(groups.leadIsFeatured, false)
  assert.deepEqual(ids(groups.reservations), ["sooner", "later"])
  assert.deepEqual(ids(groups.works), ["clip", "older"])
  assert.deepEqual(ids(groups.archives), ["archive"])
  assert.deepEqual(ids(groups.shorts), ["short"])

  // The operator's featured video leads, marked as featured.
  const featured = groupVideos(
    videos,
    shelf({ ...projection.shelf, leadId: "archive", leadIsFeatured: true }),
  )
  assert.equal(featured.lead?.id, "archive")
  assert.equal(featured.leadIsFeatured, true)

  // Without a projected lead there is none.
  assert.equal(groupVideos(videos, shelf({})).lead, null)
})

test("a filtered view places the lead in its server-assigned group", () => {
  const cover = video("cover", { kind: "premiere", tag: "song" })
  const clip = video("clip", { kind: "upload" })
  const projection = shelf({ leadId: "cover", works: ["clip"] })
  // The lead is not in the shelf lists, so it joins the end of its group.
  assert.deepEqual(ids(splitVideos([cover, clip], projection).works), [
    "clip",
    "cover",
  ])
  // A video the projection does not know and that has no group stays out.
  const unknown = video("unknown", {}, { shelfGroup: "" })
  assert.deepEqual(ids(splitVideos([unknown, clip], projection).works), [
    "clip",
  ])
})

test("the side group shows four works alone or five items beside reservations", () => {
  for (const [reservations, works, expected] of [
    [0, 10, [0, 4, 0]],
    [0, 2, [0, 2, 0]],
    [1, 10, [1, 4, 0]],
    [3, 10, [3, 2, 0]],
    [4, 10, [4, 1, 0]],
    // Tickets stop at four, so an earlier work keeps its slot.
    [5, 10, [4, 1, 1]],
    [8, 10, [4, 1, 4]],
    [2, 1, [2, 1, 0]],
    // Without a work to show, the ticket cap and the overflow stay the same.
    [6, 0, [4, 0, 2]],
  ] as const) {
    const [tickets, shown, hidden] = expected
    assert.deepEqual(
      sideSlots(reservations, works),
      { tickets, works: shown, hidden },
      `${reservations}/${works}`,
    )
  }
})

test("lengths read like the platforms show them", () => {
  assert.equal(formatDuration(289), "4:49")
  assert.equal(formatDuration(33), "0:33")
  assert.equal(formatDuration(25000), "6:56:40")
  assert.equal(formatDuration(3600), "1:00:00")
  for (const unknown of [null, undefined, 0, -5, Number.NaN])
    assert.equal(formatDuration(unknown), "")
  assert.equal(durationAttribute(25000), "PT6H56M40S")
  assert.equal(durationAttribute(33), "PT33S")
})

test("the form chip names the server's form code, never the tag", () => {
  const t = (key: string) => key
  for (const [formCode, key] of [
    ["short", "media.kind.short"],
    ["upcoming", "media.kind.upcoming"],
    ["stream", "event.type.stream"],
    ["archive", "media.kind.liveArchive"],
    ["premiere", "media.kind.premiere"],
    ["video", "media.kind.video"],
    // An aired premiere is an ordinary video without a form chip.
    [null, null],
  ] as const)
    assert.equal(
      videoFormLabel(video("v", { tag: "game" }, { formCode }), t),
      key,
      String(formCode),
    )
})

test("the tag chip reads a stream as its activity and a work as what it is", () => {
  const t = (key: string) => key
  const tag = (fields: Partial<Video>, tagContext: VideoView["tagContext"]) =>
    videoTagLabel(video("v", fields, { tagContext }), t)
  assert.equal(tag({ tag: "song" }, "stream"), "event.tag.song")
  assert.equal(tag({ tag: "game" }, "stream"), "event.tag.game")
  assert.equal(tag({ tag: "song" }, "work"), "media.kind.music")
  assert.equal(tag({ tag: "talk" }, "work"), "media.kind.talk")
  // A schedule label that names a form is not a tag.
  for (const label of ["live", "video", null])
    assert.equal(tag({ tag: label }, "stream"), null, String(label))
})

test("a Short belongs to the Shorts filter alone", () => {
  const streamShort = video("short-stream", { kind: "stream", isShort: true })
  const songShort = video("short-song", {
    kind: "upload",
    tag: "song",
    isShort: true,
  })
  // An unknown format is an ordinary upload, however brief.
  const brief = video("brief", {
    kind: "upload",
    isShort: null,
    durationSeconds: 30,
  })
  const archive = video("archive", { isShort: false })
  assert.equal(videoIsShort(streamShort), true)
  assert.equal(videoIsShort(brief), false)
  assert.equal(videoIsShort(video("unset")), false)
  for (const [item, filter, expected] of [
    [streamShort, "shorts", true],
    [streamShort, "streams", false],
    [songShort, "works", false],
    [streamShort, "all", true],
    [archive, "shorts", false],
    [archive, "streams", true],
    [brief, "works", true],
    [brief, "shorts", false],
  ] as const)
    assert.equal(
      videoMatchesFilter(item, filter),
      expected,
      `${item.id}/${filter}`,
    )
})

test("Chinese readers open a past video's bilibili copy first", () => {
  const bilibili = {
    source: "bilibili",
    url: "https://www.bilibili.com/video/BV1synthetic1",
  }
  const merged = video("m", { kind: "upload", alternates: [bilibili] })
  const youtube = { source: "youtube", url: merged.url }

  assert.deepEqual(videoLinks(merged, "zh-CN"), [bilibili, youtube])
  for (const locale of ["en", "ja"] as const)
    assert.deepEqual(videoLinks(merged, locale), [youtube, bilibili])

  // Without a bilibili copy every reader gets the video's own link.
  assert.deepEqual(videoLinks(video("y"), "zh-CN"), [
    { source: "youtube", url: "https://www.youtube.com/watch?v=y" },
  ])

  // A reservation or a broadcast on air stays on its own platform.
  for (const phase of ["upcoming", "live", "started"] as const) {
    const item = video("r", { alternates: [bilibili] }, { phase })
    assert.equal(videoLinks(item, "zh-CN")[0].source, "youtube", phase)
  }
})

test("a private or removed video keeps only its other platforms", () => {
  const bilibili = {
    source: "bilibili",
    url: "https://www.bilibili.com/video/BV1synthetic2",
  }
  const gone = video("g", {
    kind: "upload",
    availability: "private",
    alternates: [bilibili],
  })
  assert.deepEqual(videoLinks(gone, "en"), [bilibili])
  assert.deepEqual(
    videoLinks(video("x", { availability: "removed" }), "en"),
    [],
  )
  // Members-only and unchecked videos keep their own link.
  for (const availability of ["members", null] as const)
    assert.equal(
      videoLinks(video("k", { availability }), "en")[0]?.source,
      "youtube",
    )
})

test("a cover marks every platform a work is on, a gone one included", () => {
  const bilibili = {
    source: "bilibili",
    url: "https://www.bilibili.com/video/BV1synthetic3",
  }
  const marks = (item: Video, locale: "en" | "zh-CN" = "en") =>
    videoPlatforms(item, videoLinks(item, locale))
  assert.deepEqual(marks(video("y")), ["youtube"])
  assert.deepEqual(marks(video("m", { availability: "members" })), ["youtube"])
  const merged = video("b", { kind: "upload", alternates: [bilibili] })
  assert.deepEqual(marks(merged), ["youtube", "bilibili"])
  assert.deepEqual(marks(merged, "zh-CN"), ["bilibili", "youtube"])
  // A private or removed video still names its own platform, link or not.
  assert.deepEqual(marks(video("x", { availability: "removed" })), ["youtube"])
  assert.deepEqual(
    marks(video("p", { availability: "private", alternates: [bilibili] })),
    ["youtube", "bilibili"],
  )
  const upload = video("BV1synthetic4", {
    source: "bilibili",
    url: "https://www.bilibili.com/video/BV1synthetic4",
  })
  assert.deepEqual(marks(upload), ["bilibili"])
})
