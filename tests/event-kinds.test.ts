import assert from "node:assert/strict"
import test from "node:test"
import type { ScheduleEvent } from "../src/types.ts"
import { dashboardMessages } from "../src/dashboard/dashboard-i18n.ts"
import { eventKinds, eventTag, eventTags } from "../src/dashboard/format.ts"

function event(eventType: string, tag?: string | null): ScheduleEvent {
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
    eventType,
    tag,
    url: null,
    provenance: "automatic",
    manualLocked: false,
    isUpcoming: true,
    cancellationStatus: "none",
  }
}

test("an unlabelled or unknown label keeps the type chip alone", () => {
  for (const tag of [undefined, null, "", "karaoke"])
    assert.deepEqual(eventKinds(event("stream", tag)), {
      typeKey: "event.type.stream",
      tag: null,
    })
  assert.equal(eventTag(event("stream", "Song")), "song")
})

test("an activity label replaces the plain stream chip", () => {
  for (const tag of ["game", "talk", "song"])
    assert.deepEqual(eventKinds(event("stream", tag)), { typeKey: null, tag })
})

test("members-only, event and release chips stay beside the label", () => {
  assert.deepEqual(eventKinds(event("member", "song")), {
    typeKey: "event.type.member",
    tag: "song",
  })
  assert.deepEqual(eventKinds(event("event", "live")), {
    typeKey: "event.type.event",
    tag: "live",
  })
  assert.deepEqual(eventKinds(event("release", "video")), {
    typeKey: "event.type.release",
    tag: "video",
  })
})

test("a label that repeats its type is dropped", () => {
  for (const [type, tag] of [
    ["stream", "live"],
    ["member", "live"],
    ["video", "video"],
  ])
    assert.deepEqual(eventKinds(event(type, tag)), {
      typeKey: `event.type.${type}`,
      tag: null,
    })
})

test("every label has a display name in each locale", () => {
  for (const locale of ["zh-CN", "ja", "en"] as const)
    for (const tag of eventTags) {
      const messages: Record<string, unknown> = dashboardMessages[locale]
      assert.equal(typeof messages[`event.tag.${tag}`], "string", locale + tag)
    }
})
