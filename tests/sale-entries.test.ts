import assert from "node:assert/strict"
import test from "node:test"
import type { Dashboard, ScheduleEvent } from "../src/types.ts"
import { dashboardMessages } from "../src/dashboard/dashboard-i18n.ts"
import { deriveDashboardAt } from "../src/dashboard/derive-dashboard.ts"
import {
  currentSales,
  eventKinds,
  eventStatus,
  scheduleEntries,
} from "../src/dashboard/format.ts"

function sale(fields: Partial<ScheduleEvent> = {}): ScheduleEvent {
  return {
    id: "sale-synthetic",
    source: "x",
    title: "Synthetic goods",
    detail: null,
    // Opens at noon on 2 November and closes at 23:59 on 22 November.
    startsOn: "2026-11-02",
    startsAt: "2026-11-02T03:00:00.000Z",
    endsAt: "2026-11-22T14:59:00.000Z",
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
    ...fields,
  }
}

const shown = (entries: ScheduleEvent[]) =>
  entries.map((entry) => [entry.saleMark, entry.startsOn, entry.startsAt])

test("a sale period shows at its opening and at its close", () => {
  assert.deepEqual(shown(scheduleEntries(sale())), [
    ["opens", "2026-11-02", "2026-11-02T03:00:00.000Z"],
    ["closes", "2026-11-22", "2026-11-22T14:59:00.000Z"],
  ])
  // A close without a time is a date-only entry on its date.
  const [, closing] = scheduleEntries(sale({ endsAt: null }))
  assert.equal(closing.startsAt, null)
  assert.equal(closing.timePrecision, "unknown")
  assert.deepEqual(closing.saleWindow?.startsOn, "2026-11-02")
})

test("a deadline shows its close alone, and a sale without one its opening", () => {
  const deadline = sale({ startsOn: "2026-11-22", startsAt: null })
  assert.deepEqual(shown(scheduleEntries(deadline)), [
    ["closes", "2026-11-22", "2026-11-22T14:59:00.000Z"],
  ])
  const opening = sale({ endsOn: null, endsAt: null })
  assert.deepEqual(shown(scheduleEntries(opening)), [
    ["opens", "2026-11-02", "2026-11-02T03:00:00.000Z"],
  ])
  // Every other type is its own single entry.
  const stream = sale({ eventType: "stream" })
  assert.deepEqual(scheduleEntries(stream), [stream])
})

test("a sale is upcoming, open, then done, on both of its entries", () => {
  const [opening, closing] = scheduleEntries(sale())
  for (const [at, want] of [
    ["2026-11-02T02:59:00Z", "upcoming"],
    ["2026-11-02T03:00:00Z", "open"],
    ["2026-11-22T14:58:00Z", "open"],
    ["2026-11-22T14:59:00Z", "done"],
  ] as const) {
    assert.equal(eventStatus(opening, Date.parse(at)), want, at)
    assert.equal(eventStatus(closing, Date.parse(at)), want, at)
  }
  // A deadline is open until it passes; a close date alone ends that day.
  const deadline = sale({
    startsOn: "2026-11-22",
    startsAt: null,
    endsAt: null,
  })
  assert.equal(
    eventStatus(deadline, Date.parse("2026-11-01T00:00:00Z")),
    "open",
  )
  assert.equal(
    eventStatus(deadline, Date.parse("2026-11-22T14:59:59Z")),
    "open",
  )
  assert.equal(
    eventStatus(deadline, Date.parse("2026-11-22T15:00:00Z")),
    "done",
  )
  // A cancellation still wins.
  assert.equal(
    eventStatus(sale({ cancellationStatus: "manual_confirmed" }), Date.now()),
    "cancelled",
  )
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

test("a sale is never the next event", () => {
  const stream = sale({
    id: "stream-synthetic",
    eventType: "stream",
    startsOn: "2026-11-03",
    startsAt: "2026-11-03T11:00:00.000Z",
    endsOn: null,
    endsAt: null,
  })
  const snapshot = {
    summary: {
      nextEvent: null,
      nextStream: null,
      latestVideo: null,
      counts: null,
    },
    events: [sale(), stream],
    videos: [],
    focus: null,
    meta: { featuredVideoId: null },
  } as unknown as Dashboard
  const { summary } = deriveDashboardAt(
    snapshot,
    Date.parse("2026-11-01T00:00:00Z"),
  )
  assert.equal(summary.nextEvent?.id, "stream-synthetic")
})

test("the on-sale list puts open sales first by their close", () => {
  const now = Date.parse("2026-11-07T03:00:00Z")
  const events = [
    // Open, closing on 22 November.
    sale({ id: "late-close" }),
    // A deadline on 9 November: open until it passes.
    sale({
      id: "deadline",
      startsOn: "2026-11-09",
      startsAt: null,
      endsOn: "2026-11-09",
    }),
    // Opens on 10 November, within the week.
    sale({
      id: "soon",
      startsOn: "2026-11-10",
      startsAt: null,
      endsOn: "2026-11-20",
      endsAt: null,
    }),
    // Opens after the week, closed already, cancelled, and not a sale.
    sale({
      id: "later",
      startsOn: "2026-11-20",
      startsAt: null,
      endsOn: "2026-11-30",
      endsAt: null,
    }),
    sale({
      id: "closed",
      startsOn: "2026-10-01",
      startsAt: null,
      endsOn: "2026-11-01",
      endsAt: null,
    }),
    sale({ id: "cancelled", cancellationStatus: "manual_confirmed" }),
    sale({ id: "stream", eventType: "stream" }),
  ]
  assert.deepEqual(
    currentSales(events, now, 5).map((event) => event.id),
    ["deadline", "late-close", "soon"],
  )
  assert.deepEqual(
    currentSales(events, now).map((event) => event.id),
    ["deadline", "late-close", "soon"],
  )
  assert.deepEqual(
    currentSales(events, now, 2).map((event) => event.id),
    ["deadline", "late-close"],
  )
})
