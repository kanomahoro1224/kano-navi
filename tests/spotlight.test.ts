import assert from "node:assert/strict"
import test from "node:test"
import type {
  DashboardProjection,
  ScheduleEvent,
  Video,
  VideoView,
} from "../src/types.ts"
import {
  pickCompanion,
  pickSpotlight,
  spotlightIsLive,
  spotlightIsOngoing,
  spotlightLink,
} from "../src/dashboard/spotlight.ts"
import { eventLink } from "../src/dashboard/format.ts"

const archiveView: VideoView = {
  phase: "past",
  formCode: "archive",
  shelfGroup: "archive",
  latestEligible: true,
  focusEligible: false,
  tagContext: "stream",
}

const latestVideo: Video = {
  id: "latest",
  source: "youtube",
  title: "Synthetic archive",
  publishedAt: "2026-10-20T11:00:00Z",
  scheduledAt: null,
  url: "https://www.youtube.com/watch?v=synthetic01",
  thumbnailUrl: "/media/synthetic-thumb",
  thumbnailStatus: "ready",
  kind: "stream",
  isUpcoming: false,
  view: archiveView,
}

const reserved: Video = {
  ...latestVideo,
  id: "reserved",
  url: "https://www.youtube.com/watch?v=synthetic02",
  scheduledAt: "2026-10-22T11:00:00Z",
  publishedAt: null,
  isUpcoming: true,
  view: {
    ...archiveView,
    phase: "upcoming",
    formCode: "upcoming",
    shelfGroup: "reservation",
  },
}

const dateOnlyEvent: ScheduleEvent = {
  id: "event-date-only",
  source: "x",
  title: "Synthetic collab",
  detail: null,
  startsOn: "2026-10-24",
  startsAt: null,
  endsAt: null,
  timezone: "Asia/Tokyo",
  timePrecision: "unknown",
  status: null,
  eventType: "stream",
  url: "https://x.com/example/status/1",
  provenance: "automatic",
  manualLocked: false,
  isUpcoming: true,
  cancellationStatus: "none",
  view: {
    state: "upcoming",
    typeCode: "stream",
    activityCode: null,
    activityContext: "",
    isWork: false,
    calendarEntries: [],
  },
}

type Spotlight = NonNullable<DashboardProjection["spotlight"]>

/** A projection whose stage is the given spotlight. */
function staged(spotlight: Partial<Spotlight> | null): DashboardProjection {
  return {
    version: 1,
    nextTransitionAt: null,
    focus: { eventIds: [], videoIds: [] },
    spotlight: spotlight && {
      mode: "next",
      eventId: null,
      videoId: null,
      liveNow: false,
      isLive: false,
      membersChip: false,
      companionVideoId: null,
      ...spotlight,
    },
    sales: [],
    shelf: {
      leadId: null,
      leadIsFeatured: false,
      reservations: [],
      works: [],
      archives: [],
      shorts: [],
    },
  }
}

test("a schedule entry without artwork gets the compact ticket and the projected companion", () => {
  const summary = { nextEvent: dateOnlyEvent, nextStream: null, latestVideo }
  const projection = staged({
    eventId: dateOnlyEvent.id,
    companionVideoId: latestVideo.id,
  })
  const item = pickSpotlight(summary, null, projection)
  assert.ok(item)
  assert.equal(item.layout, "compact")
  assert.equal(item.link, "announcement")
  assert.equal(item.dateOnly, "2026-10-24")
  assert.equal(item.time, null)
  const companion = pickCompanion(summary, item, projection)
  assert.equal(companion?.url, latestVideo.url)
  assert.equal(companion?.thumbnailUrl, latestVideo.thumbnailUrl)
  // Without a projected companion the compact ticket stands alone.
  const alone = staged({ eventId: dateOnlyEvent.id })
  assert.equal(pickCompanion(summary, item, alone), null)
})

test("a reserved stream with artwork keeps the full ticket without a companion", () => {
  const summary = { nextEvent: null, nextStream: reserved, latestVideo }
  const projection = staged({
    videoId: reserved.id,
    companionVideoId: latestVideo.id,
  })
  const item = pickSpotlight(summary, null, projection)
  assert.ok(item)
  assert.equal(item.layout, "full")
  assert.equal(item.link, "watch")
  assert.equal(item.url, reserved.url)
  assert.equal(pickCompanion(summary, item, projection), null)
})

test("the latest video is a full ticket that is never on air", () => {
  const summary = { nextEvent: null, nextStream: null, latestVideo }
  const item = pickSpotlight(
    summary,
    null,
    staged({ mode: "latest", videoId: latestVideo.id }),
  )
  assert.equal(item?.mode, "latest")
  assert.equal(item?.layout, "full")
  assert.equal(item?.video?.id, latestVideo.id)
  assert.equal(spotlightIsLive(item), false)
})

test("the stage is empty when the projection names nothing it can show", () => {
  const summary = {
    nextEvent: dateOnlyEvent,
    nextStream: reserved,
    latestVideo,
  }
  assert.equal(pickSpotlight(summary, null, staged(null)), null)
  // A named record the summary does not carry is not replaced by another.
  assert.equal(
    pickSpotlight(summary, null, staged({ eventId: "elsewhere" })),
    null,
  )
  assert.equal(
    pickSpotlight(summary, null, staged({ liveNow: true, isLive: true })),
    null,
  )
})

test("On air and the members chip are the projection's", () => {
  const summary = { nextEvent: null, nextStream: reserved, latestVideo: null }
  for (const isLive of [true, false])
    for (const membersChip of [true, false]) {
      const item = pickSpotlight(
        summary,
        null,
        staged({ videoId: reserved.id, isLive, membersChip }),
      )
      assert.equal(spotlightIsLive(item), isLive)
      assert.equal(item?.membersChip, membersChip)
    }
  // A date-only entry has no time to be on air at.
  const dateOnly = pickSpotlight(
    { nextEvent: dateOnlyEvent, nextStream: null, latestVideo: null },
    null,
    staged({ eventId: dateOnlyEvent.id, isLive: true }),
  )
  assert.equal(spotlightIsLive(dateOnly), false)
})

test("an entry under way is neither on air nor counting down", () => {
  const fair: ScheduleEvent = {
    ...dateOnlyEvent,
    id: "fair",
    eventType: "event",
    startsAt: "2026-10-24T03:00:00Z",
    endsAt: "2026-10-24T09:00:00Z",
    timePrecision: "exact",
    view: { ...dateOnlyEvent.view, state: "ongoing", typeCode: "event" },
  }
  const summary = { nextEvent: fair, nextStream: null, latestVideo: null }
  const item = pickSpotlight(summary, null, staged({ eventId: fair.id }))
  assert.equal(spotlightIsLive(item), false)
  assert.equal(spotlightIsOngoing(item), true)
  // An upcoming entry, or one the projection reads on air, is not.
  const next = pickSpotlight(
    { ...summary, nextEvent: dateOnlyEvent },
    null,
    staged({ eventId: dateOnlyEvent.id }),
  )
  assert.equal(spotlightIsOngoing(next), false)
  const onAir = pickSpotlight(
    summary,
    null,
    staged({ eventId: fair.id, isLive: true }),
  )
  assert.equal(spotlightIsOngoing(onAir), false)
})

test("links name what they open", () => {
  assert.equal(spotlightLink("https://x.com/example/status/1"), "announcement")
  assert.equal(
    spotlightLink("https://m.youtube.com/watch?v=synthetic01"),
    "watch",
  )
  assert.equal(spotlightLink("https://youtu.be/synthetic01"), "watch")
  assert.equal(spotlightLink("https://live.bilibili.com/100000001"), "watch")
  assert.equal(spotlightLink("https://x.com.example.test/status/1"), "open")
  assert.equal(spotlightLink("https://media.example.test/page"), "open")
  assert.equal(spotlightLink("not a url"), "open")
  assert.equal(spotlightLink(null), null)
})

const room = "https://live.bilibili.com/100000001"
const bilibiliStream: ScheduleEvent = {
  ...dateOnlyEvent,
  id: "bilibili-stream",
  platform: "bilibili",
  startsOn: "2026-10-21",
  startsAt: "2026-10-21T11:30:00Z",
  timePrecision: "exact",
  url: "https://www.bilibili.com/opus/1000000000000000001",
  liveStatus: "live",
  liveUrl: room,
  view: { ...dateOnlyEvent.view, state: "live" },
}

test("a Bilibili stream or membership entry links to its live room once known", () => {
  assert.equal(eventLink(bilibiliStream), room)
  assert.equal(eventLink({ ...bilibiliStream, liveStatus: "ended" }), room)
  assert.equal(eventLink({ ...bilibiliStream, eventType: "member" }), room)
  // Other entries, and streams whose room is unknown, keep their own link.
  assert.equal(
    eventLink({ ...bilibiliStream, eventType: "event" }),
    bilibiliStream.url,
  )
  assert.equal(
    eventLink({ ...bilibiliStream, liveUrl: null }),
    bilibiliStream.url,
  )
  const item = pickSpotlight(
    { nextEvent: bilibiliStream, nextStream: null, latestVideo: null },
    null,
    staged({ eventId: bilibiliStream.id, isLive: true }),
  )
  assert.equal(item?.url, room)
  assert.equal(item?.link, "watch")
  assert.equal(spotlightIsLive(item), true)
})

test("an unannounced live broadcast leads the stage when the projection names it", () => {
  const liveNow = {
    platform: "bilibili" as const,
    url: room,
    title: "Synthetic unannounced stream",
    startedAt: "2026-10-21T11:40:00Z",
  }
  const summary = { nextEvent: dateOnlyEvent, nextStream: null, latestVideo }
  const projection = staged({
    liveNow: true,
    isLive: true,
    companionVideoId: latestVideo.id,
  })
  const item = pickSpotlight(summary, liveNow, projection)
  assert.ok(item?.liveNow)
  assert.equal(item.url, room)
  assert.equal(item.link, "watch")
  assert.equal(item.layout, "compact")
  assert.ok(spotlightIsLive(item))
  assert.equal(pickCompanion(summary, item, projection)?.url, latestVideo.url)
})
