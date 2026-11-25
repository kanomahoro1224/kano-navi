import type { ArchiveMonth, Post } from "../types.ts"

import { dateKey } from "../dashboard/format.ts"

/** One Japan day of the timeline, newest post first. */
export interface PostDay {
  /** `YYYY-MM-DD` in Japan time. */
  day: string
  posts: Post[]
}

/** One Japan month of the timeline, newest day first. */
export interface PostMonth {
  /** `YYYY-MM` in Japan time. */
  month: string
  days: PostDay[]
}

/**
 * Splits newest-first posts into Japan months and days, keeping their order.
 * A post without a readable time is left out, since it has no place on a
 * dated timeline.
 */
export function groupPostsByDay(posts: readonly Post[]): PostMonth[] {
  const months: PostMonth[] = []
  for (const post of posts) {
    const day = dateKey(post.publishedAt)
    if (!day) continue
    const month = day.slice(0, 7)
    let current = months.at(-1)
    if (current?.month !== month) {
      current = { month, days: [] }
      months.push(current)
    }
    const last = current.days.at(-1)
    if (last?.day === day) last.posts.push(post)
    else current.days.push({ day, posts: [post] })
  }
  return months
}

/** A month's posts for one account (its lowercase handle), or all of them. */
export function monthCount(month: ArchiveMonth, filter: string | null) {
  return filter ? month.counts[filter] || 0 : month.total
}

/**
 * The quartiles of the months that hold posts: the counts at which the
 * activity map's cells step from level 1 up to 2, 3 and 4.
 */
export function activityScale(counts: readonly number[]) {
  const held = counts.filter((count) => count > 0).sort((a, b) => a - b)
  if (!held.length) return []
  return [0.25, 0.5, 0.75].map(
    (share) => held[Math.round(share * (held.length - 1))],
  )
}

/**
 * How dark a month's cell is in the activity map: 0 for none, then 1–4 by the
 * quartile it falls in, so each shade covers about a quarter of the months
 * whether the account posts a few times a month or every day.
 */
export function activityLevel(count: number, scale: readonly number[]) {
  if (count <= 0) return 0
  return 1 + scale.filter((step) => count >= step).length
}

/** One year row of the activity map, January first. */
export interface ActivityYear {
  year: number
  /** Twelve counts; null for a month after the newest one. */
  months: (number | null)[]
  total: number
}

/**
 * The activity map's rows, newest year first, from the first year holding any
 * post to the newest month's year. Every account shares the same years, so
 * the map keeps its shape as the filter changes.
 */
export function activityYears(
  months: readonly ArchiveMonth[],
  filter: string | null,
): ActivityYear[] {
  if (!months.length) return []
  const counts = new Map(
    months.map((month) => [month.month, monthCount(month, filter)]),
  )
  const first = Number(months[0].month.slice(0, 4))
  const newest = months.at(-1)!.month
  const last = Number(newest.slice(0, 4))
  const rows: ActivityYear[] = []
  for (let year = last; year >= first; year -= 1) {
    const row = Array.from({ length: 12 }, (_, index) => {
      const key = `${year}-${String(index + 1).padStart(2, "0")}`
      return key > newest ? null : counts.get(key) || 0
    })
    rows.push({
      year,
      months: row,
      total: row.reduce<number>((sum, count) => sum + (count || 0), 0),
    })
  }
  return rows
}

/** An account's posts in the archive and the years they span. */
export interface AccountSpan {
  total: number
  first: string | null
  last: string | null
}

export function accountSpan(
  months: readonly ArchiveMonth[],
  filter: string | null,
): AccountSpan {
  let total = 0
  let first: string | null = null
  let last: string | null = null
  for (const month of months) {
    const count = monthCount(month, filter)
    if (!count) continue
    total += count
    first ??= month.month.slice(0, 4)
    last = month.month.slice(0, 4)
  }
  return { total, first, last }
}

/**
 * The month a timeline should open on for an account: the given month when it
 * holds the account's posts, else the nearest earlier one that does, else
 * the account's newest month. Null when the account has no posts at all.
 */
export function nearestMonth(
  months: readonly ArchiveMonth[],
  month: string | null,
  filter: string | null,
) {
  const held = months.filter((entry) => monthCount(entry, filter) > 0)
  if (!held.length) return null
  if (!month) return held.at(-1)!.month
  return held.findLast((entry) => entry.month <= month)?.month ?? held[0].month
}
