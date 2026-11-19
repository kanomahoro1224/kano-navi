import assert from "node:assert/strict"
import test from "node:test"
import type { ArchiveMonth } from "../src/types.ts"
import {
  accountSpan,
  activityLevel,
  activityScale,
  activityYears,
  groupPostsByDay,
  nearestMonth,
} from "../src/posts/timeline.ts"

const post = (id: string, publishedAt: string) =>
  ({ id, publishedAt }) as Parameters<typeof groupPostsByDay>[0][number]

const months: ArchiveMonth[] = [
  { month: "2024-11", total: 3, counts: { kano_example: 3 } },
  { month: "2025-02", total: 5, counts: { kano_example: 1, unit_example: 4 } },
  { month: "2025-04", total: 40, counts: { kano_example: 40 } },
]

test("posts fall under their Japan month and day, newest first", () => {
  const grouped = groupPostsByDay([
    // 00:30 on 1 November in Japan is still 31 October in UTC.
    post("a", "2026-10-31T15:30:00Z"),
    post("b", "2026-10-31T14:59:00Z"),
    post("c", "2026-10-31T03:00:00Z"),
    post("undated", ""),
    post("d", "2026-09-30T20:00:00Z"),
  ])
  assert.deepEqual(
    grouped.map(({ month, days }) => ({
      month,
      days: days.map(({ day, posts }) => [day, posts.map(({ id }) => id)]),
    })),
    [
      { month: "2026-11", days: [["2026-11-01", ["a"]]] },
      {
        month: "2026-10",
        days: [
          ["2026-10-31", ["b", "c"]],
          // 05:00 on 1 October in Japan, still 30 September in UTC.
          ["2026-10-01", ["d"]],
        ],
      },
    ],
  )
})

test("the activity map shades months by quartile of the busy months", () => {
  const scale = activityScale([0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9])
  assert.deepEqual(scale, [3, 5, 7])
  assert.deepEqual(
    [0, 1, 2, 3, 4, 5, 6, 7, 9].map((count) => activityLevel(count, scale)),
    [0, 1, 1, 2, 2, 3, 3, 4, 4],
  )
  // A lone busy month is the darkest; an empty account has no scale.
  assert.equal(activityLevel(12, activityScale([0, 12])), 4)
  assert.deepEqual(activityScale([0, 0]), [])
})

test("the activity map lists every year, newest first, for any account", () => {
  const years = activityYears(months, "unit_example")
  assert.deepEqual(
    years.map(({ year, total }) => [year, total]),
    [
      [2025, 4],
      [2024, 0],
    ],
  )
  // Months after the newest one are left blank rather than empty.
  assert.deepEqual(years[0].months.slice(0, 5), [0, 4, 0, 0, null])
  assert.equal(years[1].months[10], 0)
  assert.equal(activityYears([], null).length, 0)
})

test("an account card counts its posts and the years they span", () => {
  assert.deepEqual(accountSpan(months, null), {
    total: 48,
    first: "2024",
    last: "2025",
  })
  assert.deepEqual(accountSpan(months, "unit_example"), {
    total: 4,
    first: "2025",
    last: "2025",
  })
  assert.deepEqual(accountSpan(months, "missing"), {
    total: 0,
    first: null,
    last: null,
  })
})

test("a chosen month falls back to the account's nearest earlier month", () => {
  assert.equal(nearestMonth(months, "2025-03", null), "2025-02")
  assert.equal(nearestMonth(months, "2025-02", "kano_example"), "2025-02")
  assert.equal(nearestMonth(months, "2025-01", "unit_example"), "2025-02")
  assert.equal(nearestMonth(months, null, "unit_example"), "2025-02")
  assert.equal(nearestMonth(months, null, null), "2025-04")
  assert.equal(nearestMonth(months, "2025-04", "missing"), null)
})
