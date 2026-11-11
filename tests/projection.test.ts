import assert from "node:assert/strict"
import test from "node:test"
import type { Dashboard, ScheduleEvent, Video } from "../src/types.ts"
import {
  eventKinds,
  eventStatus,
  scheduleEntries,
  videoFormLabel,
  videoTagLabel,
} from "../src/dashboard/format.ts"
import { pickSpotlight, spotlightIsLive } from "../src/dashboard/spotlight.ts"
import { groupVideos, videoPhase } from "../src/dashboard/video-groups.ts"

const now = Date.parse("2026-11-02T04:00:00Z")
const translate = (key: string) => key

function event(fields: Partial<ScheduleEvent> = {}): ScheduleEvent {
  return {
    id: "event",
    source: "x",
    title: "Synthetic event",
    detail: null,
    startsOn: "2026-11-02",
    startsAt: "2026-11-02T05:00:00Z",
    endsAt: null,
    timezone: "Asia/Tokyo",
    timePrecision: "exact",
    status: "scheduled",
    eventType: "stream",
    url: "https://x.example.test/event",
    provenance: "automatic",
    manualLocked: false,
    isUpcoming: true,
    cancellationStatus: "none",
    ...fields,
  }
}

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
    kind: "upload",
    isUpcoming: false,
    ...fields,
  }
}

const projection: NonNullable<Dashboard["projection"]> = {
  version: 1,
  nextTransitionAt: null,
  focus: { eventIds: ["event"], videoIds: ["reserved"] },
  spotlight: {
    mode: "next",
    eventId: "event",
    videoId: null,
    liveNow: false,
    isLive: true,
    membersChip: false,
    companionVideoId: null,
  },
  sales: [],
  shelf: {
    leadId: "lead",
    leadIsFeatured: false,
    reservations: ["reserved"],
    works: ["work"],
    archives: ["archive"],
    shorts: [],
  },
}

test("event and video presentation rules use the server projection", () => {
  const projectedEvent = event({
    status: "cancelled",
    view: {
      state: "live",
      typeCode: "member",
      activityCode: "talk",
      activityContext: "stream",
      isWork: false,
      calendarEntries: [
        {
          saleMark: null,
          startsOn: "2026-11-03",
          startsAt: null,
          timePrecision: "unknown",
        },
      ],
    },
  })
  assert.equal(eventStatus(projectedEvent, now), "live")
  assert.deepEqual(
    scheduleEntries(projectedEvent).map((item) => item.startsOn),
    ["2026-11-03"],
  )
  assert.deepEqual(eventKinds(projectedEvent), {
    typeKey: "event.type.member",
    tag: "talk",
    work: false,
  })

  const projectedSale = event({
    id: "sale",
    eventType: "sale",
    startsOn: "2026-11-02",
    startsAt: "2026-11-02T03:00:00Z",
    endsOn: "2026-11-22",
    endsAt: "2026-11-22T14:59:00Z",
    view: {
      state: "open",
      typeCode: "sale",
      activityCode: null,
      activityContext: "",
      isWork: false,
      calendarEntries: [
        {
          saleMark: "opens",
          startsOn: "2026-11-02",
          startsAt: "2026-11-02T03:00:00Z",
          timePrecision: "exact",
        },
        {
          saleMark: "closes",
          startsOn: "2026-11-22",
          startsAt: "2026-11-22T14:59:00Z",
          timePrecision: "exact",
        },
      ],
    },
  })
  assert.equal(
    scheduleEntries(projectedSale)[1].saleWindow?.startsOn,
    "2026-11-02",
  )

  const projectedVideo = video("video", {
    kind: "upload",
    view: {
      phase: "live",
      formCode: "stream",
      shelfGroup: "archive",
      latestEligible: false,
      focusEligible: true,
      tagContext: "stream",
    },
  })
  assert.equal(videoPhase(projectedVideo, now), "live")
  assert.equal(
    videoFormLabel(projectedVideo, translate, now),
    "event.type.stream",
  )
  assert.equal(
    videoTagLabel({ ...projectedVideo, tag: "song" }, translate),
    "event.tag.song",
  )
})

test("shelf and spotlight consume projected ordering and identity", () => {
  const lead = video("lead", {
    tag: "song",
    publishedAt: "2026-11-01T00:00:00Z",
    view: {
      phase: "past",
      formCode: "video",
      shelfGroup: "work",
      latestEligible: true,
      focusEligible: false,
      tagContext: "work",
    },
  })
  const work = video("work", {
    publishedAt: "2026-10-01T00:00:00Z",
    view: {
      phase: "past",
      formCode: "video",
      shelfGroup: "work",
      latestEligible: true,
      focusEligible: false,
      tagContext: "work",
    },
  })
  const archive = video("archive", {
    kind: "stream",
    publishedAt: "2026-09-01T00:00:00Z",
    view: {
      phase: "past",
      formCode: "archive",
      shelfGroup: "archive",
      latestEligible: false,
      focusEligible: false,
      tagContext: "stream",
    },
  })
  const reserved = video("reserved", {
    kind: "stream",
    scheduledAt: "2026-11-03T00:00:00Z",
    isUpcoming: true,
    view: {
      phase: "upcoming",
      formCode: "upcoming",
      shelfGroup: "reservation",
      latestEligible: false,
      focusEligible: true,
      tagContext: "stream",
    },
  })
  const groups = groupVideos(
    [work, archive, reserved, lead],
    null,
    now,
    projection,
  )
  assert.equal(groups.lead?.id, "lead")
  assert.deepEqual(
    groups.works.map((item) => item.id),
    ["work"],
  )
  assert.deepEqual(
    groups.archives.map((item) => item.id),
    ["archive"],
  )
  assert.deepEqual(
    groups.reservations.map((item) => item.id),
    ["reserved"],
  )

  const item = pickSpotlight(
    { nextEvent: event(), nextStream: null, latestVideo: null },
    now,
    null,
    projection,
  )
  assert.equal(item?.event?.id, "event")
  assert.equal(spotlightIsLive(item, now), true)
})

test("the shelf leads with the projected lead over an older cover", () => {
  const past = {
    phase: "past",
    formCode: "video",
    shelfGroup: "work",
    latestEligible: true,
    focusEligible: false,
    tagContext: "work",
  } as const
  const lead = video("BV1aaaaaaaaa", {
    source: "bilibili",
    tag: "song",
    publishedAt: "2026-11-01T00:00:00Z",
    url: "https://www.bilibili.com/video/BV1aaaaaaaaa",
    view: past,
  })
  const cover = video("cover", {
    kind: "premiere",
    tag: "song",
    publishedAt: "2026-10-20T00:00:00Z",
    view: past,
  })
  const older = video("older", {
    tag: "song",
    publishedAt: "2026-10-01T00:00:00Z",
    view: past,
  })
  const groups = groupVideos([cover, lead, older], null, now, {
    ...projection,
    shelf: {
      leadId: lead.id,
      leadIsFeatured: false,
      reservations: [],
      works: ["cover", "older"],
      archives: [],
      shorts: [],
    },
  })
  assert.equal(groups.lead?.id, lead.id)
  assert.deepEqual(
    groups.works.map((item) => item.id),
    ["cover", "older"],
  )
})
