import assert from "node:assert/strict"
import test from "node:test"
import type { CalendarEntryView, ScheduleEvent } from "../src/types.ts"
import { dashboardMessages } from "../src/dashboard/dashboard-i18n.ts"
import {
  eventKinds,
  eventStatus,
  saleClosesAt,
  saleOpensAt,
  scheduleEntries,
} from "../src/dashboard/format.ts"

const opens: CalendarEntryView = {
  saleMark: "opens",
  startsOn: "2026-11-02",
  startsAt: "2026-11-02T03:00:00.000Z",
  timePrecision: "exact",
}
const closes: CalendarEntryView = {
  saleMark: "closes",
  startsOn: "2026-11-22",
  startsAt: null,
  timePrecision: "unknown",
}

function sale(fields: Partial<ScheduleEvent> = {}): ScheduleEvent {
  return {
    id: "sale-synthetic",
    source: "x",
    title: "Synthetic goods",
    detail: null,
    // Opens at noon on 2 November and closes on 22 November.
    startsOn: "2026-11-02",
    startsAt: "2026-11-02T03:00:00.000Z",
    endsAt: null,
    endsOn: "2026-11-22",
    timezone: "Asia/Tokyo",
    timePrecision: "exact",
    status: "scheduled",
    eventType: "sale",
    url: "https://shop.example.test/synthetic",
    provenance: "automatic",
    manualLocked: false,
    isUpcoming: true,
    cancellationStatus: "none",
    view: {
      state: "open",
      typeCode: "sale",
      activityCode: null,
      activityContext: "",
      isWork: false,
      calendarEntries: [opens, closes],
    },
    ...fields,
  }
}

const shown = (entries: ScheduleEvent[]) =>
  entries.map((entry) => [
    entry.saleMark,
    entry.startsOn,
    entry.startsAt,
    entry.timePrecision,
  ])

test("a sale shows the calendar entries the server lists, each with its period", () => {
  const entries = scheduleEntries(sale())
  assert.deepEqual(shown(entries), [
    ["opens", "2026-11-02", "2026-11-02T03:00:00.000Z", "exact"],
    ["closes", "2026-11-22", null, "unknown"],
  ])
  for (const entry of entries)
    assert.deepEqual(entry.saleWindow, {
      startsOn: "2026-11-02",
      startsAt: "2026-11-02T03:00:00.000Z",
      endsOn: "2026-11-22",
      endsAt: null,
    })
  // A deadline is its closing entry alone.
  const deadline = sale({
    view: { ...sale().view, calendarEntries: [closes] },
  })
  assert.deepEqual(shown(scheduleEntries(deadline)), [
    ["closes", "2026-11-22", null, "unknown"],
  ])
})

test("any other entry is its single calendar entry, without a sale period", () => {
  const stream = sale({
    eventType: "stream",
    view: {
      ...sale().view,
      typeCode: "stream",
      calendarEntries: [{ ...opens, saleMark: null }],
    },
  })
  const [entry, ...rest] = scheduleEntries(stream)
  assert.equal(rest.length, 0)
  assert.equal(entry.saleMark, undefined)
  assert.equal(entry.saleWindow, undefined)
})

test("both entries of a sale read the server's state", () => {
  for (const state of ["upcoming", "open", "done", "cancelled"] as const) {
    const event = sale({ view: { ...sale().view, state } })
    for (const entry of scheduleEntries(event))
      assert.equal(eventStatus(entry), state)
  }
})

test("a sale card times its period from the stored window", () => {
  const event = sale()
  assert.equal(saleOpensAt(event), Date.parse("2026-11-02T03:00:00.000Z"))
  // A close date without a time ends that Japan day.
  assert.equal(saleClosesAt(event), Date.parse("2026-11-22T23:59:59.999+09:00"))
  // A deadline's opening is unknown.
  const deadline = sale({ startsOn: "2026-11-22", startsAt: null })
  assert.equal(saleOpensAt(deadline), -Infinity)
})

test("a sale has its own type chip and wording in every locale", () => {
  assert.equal(eventKinds(sale()).typeKey, "event.type.sale")
  for (const messages of Object.values(dashboardMessages))
    for (const key of [
      "event.type.sale",
      "schedule.sale.opens",
      "schedule.sale.closes",
      "schedule.status.open",
      "sales.title",
      "sales.soon",
      "sales.closes",
      "sales.opens",
    ])
      assert.ok(messages[key as keyof typeof messages], key)
})
