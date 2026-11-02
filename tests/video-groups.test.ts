import assert from "node:assert/strict"
import test from "node:test"
import type { Video } from "../src/types.ts"
import {
  groupVideos,
  splitVideos,
  videoIsShort,
  videoIsStream,
  videoLinks,
  videoMatchesFilter,
  videoPhase,
} from "../src/dashboard/video-groups.ts"
import {
  durationAttribute,
  formatDuration,
  videoFormLabel,
  videoTagLabel,
} from "../src/dashboard/format.ts"

const now = Date.parse("2026-10-25T12:00:00Z")
const hour = 3600 * 1000

function video(id: string, fields: Partial<Video> = {}): Video {
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
  }
}

const at = (offset: number) => new Date(now + offset).toISOString()

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

test("a reservation past its start is not an archive until it is over", () => {
  const reserved = { kind: "stream", isUpcoming: true }
  assert.equal(
    videoPhase(video("a", { ...reserved, scheduledAt: at(hour) }), now),
    "upcoming",
  )
  // Started without a confirmed state: presumed on air for the grace.
  assert.equal(
    videoPhase(video("b", { ...reserved, scheduledAt: at(-hour) }), now),
    "started",
  )
  assert.equal(
    videoPhase(video("c", { ...reserved, scheduledAt: at(-4 * hour) }), now),
    "past",
  )
  // A confirmed state decides before the start time.
  assert.equal(
    videoPhase(
      video("d", { scheduledAt: at(-4 * hour), liveStatus: "live" }),
      now,
    ),
    "live",
  )
  assert.equal(
    videoPhase(
      video("e", {
        scheduledAt: at(-hour),
        endsAt: at(-1000),
        liveStatus: "live",
      }),
      now,
    ),
    "past",
  )
  assert.equal(
    videoPhase(
      video("f", { scheduledAt: at(-hour), liveStatus: "upcoming" }),
      now,
    ),
    "upcoming",
  )
  assert.equal(
    videoPhase(
      video("g", { scheduledAt: at(-hour), liveStatus: "ended" }),
      now,
    ),
    "past",
  )
  // A premiere without an end is over once it starts.
  assert.equal(
    videoPhase(video("h", { kind: "premiere", scheduledAt: at(-hour) }), now),
    "past",
  )
  assert.equal(
    videoPhase(
      video("i", { ...reserved, scheduledAt: at(hour), isCancelled: true }),
      now,
    ),
    "cancelled",
  )
})

test("the newest song or premiere leads and every video is listed once", () => {
  const cover = video("cover", {
    kind: "premiere",
    tag: "song",
    scheduledAt: at(-30 * hour),
    durationSeconds: 289,
  })
  const clip = video("clip", {
    kind: "upload",
    tag: "talk",
    publishedAt: at(-10 * hour),
  })
  const older = video("older", {
    kind: "upload",
    tag: "song",
    publishedAt: at(-90 * hour),
  })
  const archiveNew = video("archive-new", { scheduledAt: at(-20 * hour) })
  const archiveOld = video("archive-old", { scheduledAt: at(-60 * hour) })
  const later = video("later", {
    kind: "stream",
    isUpcoming: true,
    scheduledAt: at(48 * hour),
  })
  const sooner = video("sooner", {
    kind: "stream",
    isUpcoming: true,
    scheduledAt: at(2 * hour),
  })
  const onAir = video("on-air", {
    scheduledAt: at(-hour),
    liveStatus: "live",
  })
  const cancelledPast = video("cancelled-past", {
    scheduledAt: at(-2 * hour),
    isCancelled: true,
  })
  const cancelledNext = video("cancelled-next", {
    kind: "stream",
    isUpcoming: true,
    scheduledAt: at(5 * hour),
    isCancelled: true,
  })
  const videos = [
    archiveOld,
    clip,
    later,
    cover,
    cancelledPast,
    older,
    onAir,
    archiveNew,
    sooner,
    cancelledNext,
  ]

  const groups = groupVideos(videos, null, now)
  assert.equal(groups.lead?.id, "cover")
  assert.equal(groups.leadIsFeatured, false)
  assert.deepEqual(
    groups.reservations.map((item) => item.id),
    ["on-air", "sooner", "cancelled-next", "later"],
  )
  assert.deepEqual(
    groups.works.map((item) => item.id),
    ["clip", "older"],
  )
  // A cancelled reservation stays listed until its start, then drops out.
  assert.deepEqual(
    groups.archives.map((item) => item.id),
    ["archive-new", "archive-old"],
  )

  // The operator's featured video leads instead and the cover stays a work.
  const featured = groupVideos(videos, "archive-old", now)
  assert.equal(featured.lead?.id, "archive-old")
  assert.equal(featured.leadIsFeatured, true)
  assert.deepEqual(
    featured.works.map((item) => item.id),
    ["clip", "cover", "older"],
  )
  assert.deepEqual(
    featured.archives.map((item) => item.id),
    ["archive-new"],
  )

  // Without a cover or premiere the newest upload leads; without works none.
  assert.equal(groupVideos([clip, archiveNew], null, now).lead?.id, "clip")
  assert.equal(groupVideos([archiveNew, sooner], null, now).lead, null)
  assert.deepEqual(
    splitVideos([cover, clip], now).works.map((item) => item.id),
    ["clip", "cover"],
  )
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

test("the form chip names the waiting room or the form, never the tag", () => {
  const t = (key: string) => key
  const form = (fields: Partial<Video>) => videoFormLabel(video("v", fields), t)
  assert.equal(form({ isUpcoming: true, tag: "game" }), "media.kind.upcoming")
  assert.equal(form({ tag: "game" }), "media.kind.liveArchive")
  assert.equal(form({ kind: "premiere", tag: "song" }), "media.kind.premiere")
  for (const kind of ["upload", "unknown"])
    assert.equal(form({ kind, tag: "talk" }), "media.kind.video", kind)
})

test("the tag chip reads a stream as its activity and a work as what it is", () => {
  const t = (key: string) => key
  const tag = (fields: Partial<Video>) => videoTagLabel(video("v", fields), t)
  assert.equal(tag({ tag: "song" }), "event.tag.song")
  assert.equal(tag({ isUpcoming: true, tag: "game" }), "event.tag.game")
  assert.equal(tag({ kind: "premiere", tag: "song" }), "media.kind.music")
  assert.equal(tag({ kind: "upload", tag: "talk" }), "media.kind.talk")
  // A schedule label that names a form is not a tag.
  for (const label of ["live", "video", null])
    assert.equal(tag({ tag: label }), null, String(label))
})

test("Shorts are their own group, never a work, an archive or the lead", () => {
  // A Short broadcast is still a Short; its format decides.
  const streamShort = video("short-stream", {
    kind: "stream",
    isShort: true,
    publishedAt: at(-2 * hour),
  })
  const songShort = video("short-song", {
    kind: "upload",
    tag: "song",
    isShort: true,
    publishedAt: at(-hour),
  })
  // An unknown format is an ordinary upload, however brief.
  const brief = video("brief", {
    kind: "upload",
    isShort: null,
    publishedAt: at(-3 * hour),
    durationSeconds: 30,
  })
  const regular = video("regular", {
    kind: "upload",
    isShort: false,
    publishedAt: at(-5 * hour),
  })
  const archive = video("archive", {
    isShort: false,
    scheduledAt: at(-20 * hour),
  })
  // A Short that is still coming up stays with the reservations.
  const reserved = video("short-reserved", {
    kind: "stream",
    isShort: true,
    isUpcoming: true,
    scheduledAt: at(hour),
  })
  const videos = [streamShort, regular, songShort, archive, brief, reserved]

  const groups = groupVideos(videos, null, now)
  // The newest cover is a Short, so the newest ordinary upload leads.
  assert.equal(groups.lead?.id, "brief")
  assert.deepEqual(
    groups.shorts.map((item) => item.id),
    ["short-song", "short-stream"],
  )
  assert.deepEqual(
    groups.works.map((item) => item.id),
    ["regular"],
  )
  assert.deepEqual(
    groups.archives.map((item) => item.id),
    ["archive"],
  )
  assert.deepEqual(
    groups.reservations.map((item) => item.id),
    ["short-reserved"],
  )
  assert.equal(groupVideos([songShort, streamShort], null, now).lead, null)

  // The operator's featured Short still leads.
  const featured = groupVideos(videos, "short-song", now)
  assert.equal(featured.lead?.id, "short-song")
  assert.deepEqual(
    featured.shorts.map((item) => item.id),
    ["short-stream"],
  )

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

  const t = (key: string) => key
  assert.equal(videoFormLabel(streamShort, t), "media.kind.short")
  assert.equal(videoFormLabel(brief, t), "media.kind.video")
})

test("Chinese readers open a past video's bilibili copy first", () => {
  const bilibili = {
    source: "bilibili",
    url: "https://www.bilibili.com/video/BV1synthetic1",
  }
  const merged = video("m", {
    kind: "upload",
    publishedAt: at(-hour),
    alternates: [bilibili],
  })
  const youtube = { source: "youtube", url: merged.url }

  assert.deepEqual(videoLinks(merged, "zh-CN", now), [bilibili, youtube])
  for (const locale of ["en", "ja"] as const)
    assert.deepEqual(videoLinks(merged, locale, now), [youtube, bilibili])

  // Without a bilibili copy every reader gets the video's own link.
  assert.deepEqual(videoLinks(video("y"), "zh-CN", now), [
    { source: "youtube", url: "https://www.youtube.com/watch?v=y" },
  ])

  // A reservation or a broadcast on air stays on its own platform.
  const reserved = video("r", {
    kind: "stream",
    isUpcoming: true,
    scheduledAt: at(hour),
    alternates: [bilibili],
  })
  const onAir = video("l", {
    scheduledAt: at(-hour),
    liveStatus: "live",
    alternates: [bilibili],
  })
  for (const item of [reserved, onAir])
    assert.equal(videoLinks(item, "zh-CN", now)[0].source, "youtube", item.id)
})
