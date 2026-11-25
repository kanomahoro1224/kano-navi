import assert from "node:assert/strict"
import test from "node:test"
import type {
  Dashboard,
  DashboardProjection,
  ScheduleEvent,
  Video,
  VideoView,
} from "../src/types.ts"
import {
  eventAnnouncement,
  eventLink,
  eventLinks,
  eventShowsPlatformLinks,
} from "../src/dashboard/format.ts"
import { pickSpotlight, spotlightIsLive } from "../src/dashboard/spotlight.ts"

const watch = "https://www.youtube.com/watch?v=synthetic01"
const room = "https://live.example.invalid/100000001"

const simulcast: ScheduleEvent = {
  id: "youtube-synthetic01",
  source: "youtube",
  platform: "youtube",
  platforms: ["youtube", "bilibili"],
  links: [
    { platform: "youtube", url: watch, liveStatus: null },
    { platform: "bilibili", url: room, liveStatus: null },
  ],
  title: "Synthetic simulcast",
  detail: null,
  startsOn: "2026-10-21",
  startsAt: "2026-10-21T11:00:00Z",
  endsAt: null,
  timezone: "Asia/Tokyo",
  timePrecision: "exact",
  status: null,
  eventType: "stream",
  url: watch,
  liveUrl: room,
  provenance: "automatic",
  manualLocked: false,
  isUpcoming: false,
  cancellationStatus: "none",
  view: {
    state: "live",
    typeCode: "stream",
    activityCode: null,
    activityContext: "",
    isWork: false,
    calendarEntries: [],
  },
}

const streamView: VideoView = {
  phase: "live",
  formCode: "stream",
  shelfGroup: "reservation",
  latestEligible: false,
  focusEligible: true,
  tagContext: "stream",
}

/** A projection whose stage names the simulcast's video and entry. */
function spotlightOn(videoId: string, isLive: boolean): DashboardProjection {
  return {
    version: 1,
    nextTransitionAt: null,
    focus: { eventIds: [simulcast.id], videoIds: [videoId] },
    spotlight: {
      mode: "next",
      eventId: simulcast.id,
      videoId,
      liveNow: false,
      isLive,
      membersChip: false,
      companionVideoId: null,
    },
    sales: [],
    shelf: {
      leadId: null,
      leadIsFeatured: false,
      reservations: [videoId],
      works: [],
      archives: [],
      shorts: [],
    },
  }
}

test("an entry links every platform, its own first", () => {
  assert.deepEqual(eventLinks(simulcast), [
    { source: "youtube", url: watch },
    { source: "bilibili", url: room },
  ])
  // The YouTube watch link stays the main link, although a liveUrl exists.
  assert.equal(eventLink(simulcast), watch)
})

test("a live platform leads and is the main link", () => {
  const live: ScheduleEvent = {
    ...simulcast,
    links: [
      { platform: "youtube", url: watch, liveStatus: "ended" },
      { platform: "bilibili", url: room, liveStatus: "live" },
    ],
  }
  assert.deepEqual(eventLinks(live), [
    { source: "bilibili", url: room, live: true },
    { source: "youtube", url: watch },
  ])
  assert.equal(eventLink(live), room)
})

test("zh-CN readers get Bilibili first once the entry is over", () => {
  const past: ScheduleEvent = {
    ...simulcast,
    view: { ...simulcast.view, state: "done" },
  }
  assert.equal(eventLinks(past, "zh-CN")[0].source, "bilibili")
  assert.equal(eventLinks(past, "ja")[0].source, "youtube")
  assert.equal(eventLinks(simulcast, "zh-CN")[0].source, "youtube")
})

test("a platform without a link is left out, and a snapshot without links keeps the old link", () => {
  const unlinked: ScheduleEvent = {
    ...simulcast,
    links: [
      { platform: "youtube", url: watch },
      { platform: "bilibili", url: null },
    ],
  }
  assert.equal(eventLinks(unlinked).length, 1)
  const legacy: ScheduleEvent = {
    ...simulcast,
    links: undefined,
    platforms: undefined,
    platform: "bilibili",
    url: "https://x.com/example/status/1",
  }
  assert.equal(eventLink(legacy), room)
})

test("the video ticket of a simulcast keeps the entry's other platform", () => {
  const video: Video = {
    id: "synthetic01",
    source: "youtube",
    title: "Synthetic simulcast",
    publishedAt: null,
    scheduledAt: "2026-10-21T11:00:00Z",
    url: watch,
    thumbnailUrl: "/media/synthetic-thumb",
    thumbnailStatus: "ready",
    kind: "stream",
    isUpcoming: false,
    liveStatus: "live",
    view: streamView,
  }
  const summary = {
    nextEvent: simulcast,
    nextStream: video,
    latestVideo: null,
  } as unknown as Dashboard["summary"]
  const item = pickSpotlight(summary, null, spotlightOn(video.id, true))
  assert.equal(item?.video?.id, "synthetic01")
  assert.deepEqual(item?.extraLinks, [{ source: "bilibili", url: room }])
})

test("an announcement stays the entry's own link, never a platform's", () => {
  // An X notice of a simulcast before its reservation: only the room links
  // a platform, and the post keeps the open arrow.
  const notice: ScheduleEvent = {
    ...simulcast,
    url: "https://x.com/example/status/1",
    links: [
      { platform: "youtube", url: null },
      { platform: "bilibili", url: room },
    ],
  }
  const links = eventLinks(notice)
  assert.deepEqual(links, [{ source: "bilibili", url: room }])
  assert.equal(eventShowsPlatformLinks(notice, links), true)
  assert.equal(
    eventAnnouncement(notice, links),
    "https://x.com/example/status/1",
  )
  // A YouTube event's watch link is its YouTube link: no separate arrow.
  assert.equal(eventAnnouncement(simulcast, eventLinks(simulcast)), null)
  // One platform link that is the entry's own url shows no chips.
  const single: ScheduleEvent = {
    ...simulcast,
    platforms: ["youtube"],
    links: [{ platform: "youtube", url: watch }],
  }
  assert.equal(eventShowsPlatformLinks(single, eventLinks(single)), false)
})

test("a reserved video reads live when the projection says its simulcast is", () => {
  const video: Video = {
    id: "synthetic02",
    source: "youtube",
    title: "Synthetic simulcast",
    publishedAt: null,
    scheduledAt: "2026-10-21T11:00:00Z",
    url: watch,
    thumbnailUrl: "/media/synthetic-thumb",
    thumbnailStatus: "ready",
    kind: "stream",
    isUpcoming: true,
    liveStatus: "upcoming",
    view: { ...streamView, phase: "upcoming", formCode: "upcoming" },
  }
  const event: ScheduleEvent = {
    ...simulcast,
    liveStatus: "live",
    links: [
      { platform: "youtube", url: watch, liveStatus: "upcoming" },
      { platform: "bilibili", url: room, liveStatus: "live" },
    ],
  }
  const summary = {
    nextEvent: event,
    nextStream: video,
    latestVideo: null,
  } as unknown as Dashboard["summary"]
  const item = pickSpotlight(summary, null, spotlightOn(video.id, true))
  assert.equal(item?.video?.id, "synthetic02")
  assert.equal(spotlightIsLive(item), true)
  assert.deepEqual(item?.extraLinks, [
    { source: "bilibili", url: room, live: true },
  ])
  const waiting = pickSpotlight(summary, null, spotlightOn(video.id, false))
  assert.equal(spotlightIsLive(waiting), false)
})
