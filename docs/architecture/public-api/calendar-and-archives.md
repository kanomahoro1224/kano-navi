# Bounded Calendar and Archives

The bounded public calendar and archive reads.

## Bounded Public Calendar

`GET /api/calendar?from=2026-10-05&to=2026-10-11&page=1&pageSize=100`
reads an inclusive range of at most 42 Japan dates (six Monday-first weeks,
enough for a month grid), with 1–100 events per page in stable chronological
order. It returns sanitized `items` / `events`, `total`, `totalPages`,
`hasNext`, `dayCounts`, `revision`, matching approved schedule images and
`adjacent.previous` / `adjacent.next` event dates. Invalid ranges return 400.
Every read uses the same public event projection as the board, including the
trusted `liveStatus` of an event's linked video; it does not fetch platforms
or models. MCP `schedule_list` is the bounded, paginated history reader;
`dashboard_read` shares the bounded snapshot. Events merged into another event,
and automatic events whose every source post is hidden, are excluded from
every one of these reads, including totals, day counts, adjacent dates and
summary candidates (see [Duplicate merging](../data-model/duplicate-merging.md)
and [Events of hidden posts](../data-model/posts.md#events-of-hidden-posts)).

Every public event carries `endsOn`, the Japan close date of a
[sale period](../data-model/events.md#sale-periods) or null. A range lists a sale when
it contains the sale's opening or close date, and `dayCounts` and the adjacent
dates count the close date too.

Every public event carries `tag`: `live`, `game`, `talk`, `song` or `video`,
or null when the entry is unlabelled. Every read derives it the same way from
the event and its YouTube videos, so the board, `/api/calendar`, MCP
`dashboard_read` and `schedule_list` report the same value, and an entry
without a video link can carry one. Readers translate the code for display
and fall back to the title when it is null; see
[Shown label](../data-model/events.md#shown-label) for the order.

Dashboard SQL selects up to 100 recent post previews plus the latest-post
fallback, up to 100 current-week event previews plus 30 upcoming and 30 live
focus candidates, up to 10 [sale periods](../data-model/events.md#sale-periods) on sale
or opening within seven days (closing soonest first), and at most one linked
event per returned video, and bounded
video candidates plus Featured selection and the latest completed video.
Live event eligibility is applied before the 30-row limit: cancelled and
suspected cancellations, tombstones, non-stream activities and expired streams
do not consume the shortlist. Eligible live events use ascending start instants
and stable ID ties, matching the frontend focus ordering, including streams
spanning earlier weeks. Known end times remove finished streams before limiting
focus candidates and retain streams with explicit longer durations; an absent
end gives timed streams and memberships the strictly less-than-three-hour grace
period, while other YouTube activities such as premieres need an explicit
future end. Video/event association uses indexed primary, source-evidence and
normalized URL identities, in one query for all returned videos rather than
scanning events for each video. Media is resolved only for returned owners:
their media rows and post links are read in batched queries, and each
directory of the media cache is resolved once per read. Event and media growth therefore does
not enlarge a homepage response. Full event totals and upcoming counts are
separate SQL aggregates; calendar pagination preserves access to every
activity instead of dropping later rows.

JSON parser failures return `invalid_json` (400), excessive bodies return
`request_too_large` (413), and unsupported encodings return 415. The global
handler logs only a fixed event/code/status object, including unexpected
errors; it never serializes error messages, stacks, raw bodies, headers or
keys.

## Bounded Public Archives

`GET /api/posts?from=2026-10-01&to=2026-10-31&page=1&pageSize=30&account=kano_2525`
and `GET /api/videos?from=2026-10-01&to=2026-10-31&page=1&pageSize=30` read
the stored posts or videos of an inclusive range of at most 42 Japan dates,
newest first, with 1–50 items per page (30 by default) and stable ID ties.
A post's time is its publication; a video's is its scheduled start, or its
publication without one. Undated videos are not listed. Both return sanitized
`items` in the dashboard's post and video projections (resolved ready media,
no raw JSON), `page`, `pageSize`, `total`, `totalPages`, `hasNext`,
`revision` and `adjacent.previous` / `adjacent.next`, the nearest Japan
dates outside the range that hold an item. The optional post `account`
filter is one X handle, compared without case, and also narrows the totals and
adjacent dates; `accountCounts` always counts the range's posts per lowercase
handle. The optional video `category` filter is `game`, `talk`, `song` or
`other`, narrows the totals and adjacent dates the same way, and
`categoryCounts` always counts the range's videos per category. A video's
category is its shown [tag](../data-model/video-form.md#video-form-and-tags) (`game`,
`talk` or `song`: the operator's, else its schedule entry's label, else the
title inference's), or `other` without one; each read derives it, and every
video projection carries the tag as `tag`, `null` without one, and who can
watch it as `availability` (`public`, `unlisted`, `members`, `private`,
`removed`, or `null` until a check could tell; see
[Video availability](../sources/youtube-videos.md#video-availability)). An invalid range returns 400
`invalid_range`, an invalid handle 400 `invalid_account` and an unknown
category 400 `invalid_category`. Reads revalidate through their
[ETags](snapshot.md), negotiate gzip like the other public JSON
reads and never fetch platforms. The posts page and the board's video dialog
read the timelines below.

Without `from` and `to`,
`GET /api/posts?until=2026-09-30&page=1&pageSize=30&account=kano_2525` and
`GET /api/videos?until=2026-09-30&page=1&pageSize=30&category=game` are the
post and video timelines: the stored posts or videos dated on or before the
inclusive Japan date `until`, or every dated one without it, newest first,
paged, projected and filtered like a range. Their `accountCounts` or
`categoryCounts` count every post or video they page through,
`adjacent.previous` is always `null`, and `adjacent.next` is the nearest later
Japan date holding a post or video of the filter, so a timeline that starts at
an earlier month knows that newer records exist. An invalid `until`, a lone
`from` or `to`, or `until` together with a range returns 400
`invalid_range`.

`GET /api/posts/months` and `GET /api/videos/months` are the month indexes of
the post archive and the video timeline: `months` lists every Japan month
(`YYYY-MM`) that holds a shown post or a listed video, oldest first, each with
its `total` and its `accountCounts` per lowercase handle or `categoryCounts`
per category, plus the `revision`. Hidden posts and records without a
parseable time are not counted. They take no parameters, revalidate through
their ETags and negotiate gzip like the archive pages. The post index also
names the accounts the archive filters by, `accounts: {x, bilibili}`: the
board's X accounts (`meta.xAccounts`) followed by the archive-only accounts
(`x_archive_accounts`, see
[Account exports](../sources/x-accounts.md#account-exports)), and the mids of
`meta.bilibiliAccounts`; and their `accountAvatars`, as the dashboard
gives them.
