import assert from "node:assert/strict"
import test from "node:test"
import type { EventView, ScheduleEvent } from "../src/types.ts"
import { dashboardMessages } from "../src/dashboard/dashboard-i18n.ts"
import { eventKinds, eventTagKey, eventTags } from "../src/dashboard/format.ts"

function event(view: Partial<EventView>): ScheduleEvent {
  return {
    id: "evt-synthetic",
    source: "x",
    title: "Synthetic entry",
    detail: null,
    startsOn: "2026-11-02",
    startsAt: "2026-11-02T12:00:00Z",
    endsAt: null,
    timezone: "Asia/Tokyo",
    timePrecision: "exact",
    status: null,
    // The stored fields disagree with the view, which alone decides.
    eventType: "stream",
    tag: "talk",
    url: null,
    provenance: "automatic",
    manualLocked: false,
    isUpcoming: true,
    cancellationStatus: "none",
    view: {
      state: "upcoming",
      typeCode: null,
      activityCode: null,
      activityContext: "",
      isWork: false,
      calendarEntries: [],
      ...view,
    },
  }
}

test("the kind chips follow the server's type and activity codes", () => {
  assert.deepEqual(eventKinds(event({ typeCode: "member" })), {
    typeKey: "event.type.member",
    tag: null,
    work: false,
  })
  assert.deepEqual(
    eventKinds(event({ activityCode: "song", activityContext: "stream" })),
    { typeKey: null, tag: "song", work: false },
  )
  assert.deepEqual(
    eventKinds(
      event({
        typeCode: "release",
        activityCode: "video",
        activityContext: "work",
        isWork: true,
      }),
    ),
    { typeKey: "event.type.release", tag: "video", work: true },
  )
})

test("a song reads as music on a work, else as singing", () => {
  const key = (isWork: boolean) => {
    const { tag, work } = eventKinds(event({ activityCode: "song", isWork }))
    return tag ? eventTagKey(tag, work) : null
  }
  assert.equal(key(true), "media.kind.music")
  assert.equal(key(false), "event.tag.song")
  // Only a song changes its words; a work's game stays gameplay.
  assert.equal(eventTagKey("game", true), "event.tag.game")
})

test("every label and type code has a display name in each locale", () => {
  const typeCodes = ["stream", "member", "event", "release", "video", "sale"]
  for (const locale of ["zh-CN", "ja", "en"] as const) {
    const messages: Record<string, unknown> = dashboardMessages[locale]
    for (const tag of eventTags)
      assert.equal(typeof messages[`event.tag.${tag}`], "string", locale + tag)
    for (const type of typeCodes)
      assert.equal(
        typeof messages[`event.type.${type}`],
        "string",
        locale + type,
      )
  }
})
