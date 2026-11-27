import assert from "node:assert/strict"
import test from "node:test"
import {
  dateKey,
  formatJapanDay,
  formatJapanTime,
  formatStamp,
} from "../src/dashboard/format.ts"
import { postDate, postMonth } from "../src/lineblog/content.ts"
import {
  TOKYO_TIME_ZONE,
  dateFormatter,
  intlLocale,
  tokyoClock,
  tokyoDay,
  tokyoFormatter,
  tokyoParts,
} from "../src/lib/tokyo-time.ts"
import { japanDay } from "../src/works/use-works-uploads.ts"

// 15:30 UTC on 31 March is already 00:30 on 1 April in Japan.
const acrossMidnight = "2026-03-31T15:30:00.000Z"

test("Japan dates and clocks follow Asia/Tokyo, not the process zone", () => {
  assert.equal(TOKYO_TIME_ZONE, "Asia/Tokyo")
  assert.equal(tokyoDay(acrossMidnight), "2026-04-01")
  assert.equal(tokyoClock(acrossMidnight), "00:30")
  assert.equal(tokyoDay(new Date(acrossMidnight)), "2026-04-01")
  assert.equal(tokyoClock(Date.parse(acrossMidnight)), "00:30")
  assert.deepEqual(
    tokyoParts(acrossMidnight, { year: "numeric", month: "2-digit" }),
    { year: "2026", literal: "/", month: "04" },
  )
})

test("an empty or invalid instant has no Japan date", () => {
  for (const value of [null, undefined, "", 0, "not a date"]) {
    assert.equal(tokyoDay(value), "")
    assert.equal(tokyoClock(value), "")
    assert.equal(tokyoParts(value, { year: "numeric" }), null)
  }
})

test("formatters map site locales and are reused", () => {
  assert.equal(intlLocale("ja"), "ja-JP")
  assert.equal(intlLocale("zh-CN"), "zh-CN")
  assert.equal(intlLocale("en"), "en-US")
  assert.equal(intlLocale("fr"), "en-US")
  const options = { month: "short", day: "numeric" } as const
  assert.equal(tokyoFormatter("ja", options), tokyoFormatter("ja", options))
  assert.equal(
    tokyoFormatter("ja", options).resolvedOptions().timeZone,
    TOKYO_TIME_ZONE,
  )
  assert.equal(
    tokyoFormatter("en", { ...options, timeZone: "UTC" }).format(
      new Date(acrossMidnight),
    ),
    "Mar 31",
  )
  assert.equal(
    dateFormatter("en", options).resolvedOptions().timeZone,
    Intl.DateTimeFormat().resolvedOptions().timeZone,
  )
})

test("page formats read the shared Japan time", () => {
  assert.equal(dateKey(acrossMidnight), "2026-04-01")
  assert.equal(formatJapanTime(acrossMidnight), "00:30")
  assert.equal(formatStamp(acrossMidnight), "04.01 00:30")
  assert.equal(formatStamp(null), "—")
  assert.equal(formatJapanDay(acrossMidnight, "en"), "Wed, Apr 1")
  assert.equal(japanDay(acrossMidnight), "2026-04-01")
  assert.equal(postDate(acrossMidnight), "2026/04/01 00:30")
  assert.equal(postMonth(acrossMidnight), "2026-04")
})
