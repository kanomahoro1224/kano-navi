# Public API

Public reads, feeds, media and MCP read a prepared SQLite snapshot. None of
them fetches X, YouTube or a model provider. The service that hosts them is
described in [Backend](backend.md).

Media addresses below are written as this site's root-relative routes
(`/media/...`, and the dashboard's `/assets/...` fallback images). With
[`CDN_PUBLIC_BASE`](../operations/cdn.md) set, every public HTTP read and MCP
tool returns those same routes with the CDN base in front instead, such as
`https://cdn.example.invalid/media/<id>?v=<sha256>`; nothing else in a body
changes, and its ETag is computed over the body as sent. Without it they stay
root-relative.

## Public Snapshot Contract

`GET /api/health` returns service status, the current ISO timestamp, and a
whitelisted synchronization summary; it does not return a database path or
label. `lastSync.message` is fixed English base text, not localized copy: it
is `Sync completed`, `Some data sources are unavailable or reported warnings;
the existing snapshot remains`, `Sync failed; the existing snapshot remains`,
or `Sync in progress` for the corresponding `success`, `partial`, `failed`,
and `running` statuses. It is `null` for any other status. `GET /api/dashboard?days=3`
clamps days to 1 through 30 and returns
profile, summary, posts, events, videos, focus, timeline, resources, schedule
assets, media status, and metadata. Posts are the newest 100 published within
the window plus, however old, the eight newest of each own account; MCP
`dashboard_read` and `posts_list` read the same list. A post with
`promoted: true` is another account's status promoted onto the board, a
related account's or a reposted one (see
[Related accounts](sources.md#related-accounts) and
[Reposts](sources.md#reposts)): it is never `latestPost`
or one of an account's eight. It appears in the list and in the post archive
like any other post, except while a listed own post quotes it with its quote
shown: the quote card already shows it, and it returns once that post or its
quote is hidden. Its `accountHandle` is its
author. `repostedBy` lists, as handles without `@` and first sighting first,
the own accounts that reposted a promoted post; it is `[]` for every other
post. `counts.posts` counts
the window only. `counts.videos` counts public videos whose publication time is
within the most recent seven days; it is independent of the bounded homepage
video list. Public videos, on the board, the archive, the single-video read,
MCP and the Atom feed, are every YouTube video and each Bilibili upload that
is not merged into another video and whose form is settled (see
[Video merging](data-model.md#video-merging)); a video's `source` is
`youtube` or `bilibili` and its `url` its own platform's page, and a YouTube
video lists its merged Bilibili uploads in `alternates` as `{source, url}`.
A Bilibili upload is never upcoming or live; its `kind` is `upload` or
`stream` (a live replay, shown as a stream archive).
The summary contains `nextEvent`, `nextStream`, `latestVideo`, `latestPost`,
and counts; `latestPost` survives an empty requested window. `nextEvent` is
never a sale period. `latestVideo` is
the stage's fallback: the newest non-upcoming video that is not a Short,
unless the operator featured that Short, and that is neither private nor
removed; a video of unknown format or unknown availability stays
eligible. Metadata includes configured X accounts and the Featured video ID.

A post's `media` and its quote's `media` list images as `{id, url, status,
sourceUrl, alt, position, width, height, kind}`. `kind` is `photo`, `video`
or `gif`: a video's or GIF's entry is its thumbnail, and the board links it
to the post or the quoted status on X. Every image that is not an X video or
GIF, a Bilibili one included, is a `photo`.

A post's `source` is `x` or `bilibili`. A Bilibili dynamic has the ID
`bilibili-<dynamic ID>`, its opus page as `url`, the account's mid as
`accountHandle` and its space as `accountUrl` and zero engagement counts; its
text may be empty when the dynamic held only emoji. A forward's `quote` is
the dynamic it forwards (see [Quoted posts](data-model.md#quoted-posts)),
with that dynamic's post ID as `id` (`null` when the listing did not name
it), the author's mid as `authorHandle`, and its Bilibili page as `url`; a
forward's own text may be empty. A dynamic with stickers adds `rich`, and an
available quote with stickers `quote.rich`: the original wording as
`{text}` and `{emoji, url}` segments whose `text` joined is `text`, where
`url` is a `/media/` address of this site (on `CDN_PUBLIC_BASE` when set) or
`null` and never a third-party one (see
[Bilibili stickers](data-model.md#bilibili-stickers)).

Every public event reports its `platform` (`youtube`, `bilibili`, `other`,
`unknown`, or null for an entry stored before platforms were recorded) and
`platforms`, the platforms it is shown on with its own first (an empty array
when none is known; see
[Event platforms](data-model.md#event-platforms)). Full event reads (the
board, the calendar) add `links`, one `{platform, url, liveStatus}` per
shown platform in that order, where `url` is a page on that platform or
null (an announcement post stays the event's `url`) and `liveStatus` is
that platform's broadcast state, and `liveUrl`: the
account's Bilibili live room for an entry of type `stream` or `member` shown
on Bilibili once a live check learned the room, null otherwise. `url` stays
the announcement's link, except that a stored live-room link reads as null.
An entry's `liveStatus` follows its linked video, a YouTube event an
operator merged into it, or the room when the room's broadcast matched it
(see [Live room](sources.md#live-room)), and is `live` while any of them
is. The
dashboard adds `liveNow`, `{platform: "bilibili", url, title, startedAt}`,
while the room is live within two Bilibili check intervals and the
broadcast matches no stored entry, and null otherwise; it is never an event,
and readers match it against entries by platform and time, not by link. `meta.bilibiliAccounts` lists the accounts (mids) of the
stored Bilibili posts; the post archive's `account` filter and its
`accountCounts` take a mid like an X handle.

`meta.sourceFetchedAt` contains only `x`, `youtube`, `bilibili` and `shop`
ISO timestamps or null, derived from successful incremental source reads, the
head pages of manual timeline fetches, the Bilibili account's last successful
read and the shop's. Other manual fetches do not refresh it; a stored recent-window
checkpoint counts up to its window's end time. Timeline `count` and `all`
fetches retain their successful head-page read time, so resuming old pages
does not refresh the source; a timeline checkpoint without that time leaves
the source time unknown. Media-only runs,
cursor cleanup and API reads do not advance these source timestamps. Internal
account IDs, checkpoints and fetch errors remain protected.

`GET /api/visitor-region` returns `{ "enabled": false, "mainlandChina": false }`
by default. When the operator enables the setting with a provider name,
provider website and mirror site, it also returns `providerName`, `providerUrl`
and `mirrorUrl`. It returns `mainlandChina: true` when the configured edge or
reverse proxy supplies a `CN` country code, and `false` otherwise. It reads `CF-IPCountry` first, then `X-Country-Code`,
`X-Geo-Country` and `X-Visitor-Country`; the proxy must overwrite these
headers rather than pass visitor-supplied values through. The response is
`no-store` and contains no address or raw header. The public pages link the
site in the Simplified Chinese footer whenever `enabled` is true and use
`mainlandChina` only to decide whether to offer it in a notice. The operator console controls
`enabled` through its `visitor_region_enabled` setting, which defaults to off.

`backend/internal/publicview` owns shared public event, synchronization,
and selected profile-media projections for HTTP and MCP. A public sync summary
contains only id, source, start/finish times, status, and a fixed status
message. Public profile slots contain only id, slot, source, sourceUrl,
status, isActive, and publicUrl. Cache paths, source references, download
errors, and candidate metadata remain available through authenticated admin
reads. Per-source counters, raw fetch errors, job IDs, and internal source
item IDs stay in the guarded admin API. `GET /api/dashboard/revision` reports
a small revision hint without external work, and
[`/api/dashboard/events`](#revision-stream) pushes the same number as it
changes. Timestamps are parseable ISO 8601 strings; display and date-window
boundaries use Asia/Tokyo.

`/api/dashboard`, `/api/calendar`, `/api/posts`, `/api/posts/months`,
`/api/videos`, `/api/videos/months`, `/api/works/uploads` and the
[collection reads](#journey-and-works-collections) and the
[LINE BLOG archive](#line-blog-archive) reads send
`Cache-Control: no-cache` and a weak ETag, a SHA-256 prefix of the
uncompressed JSON body. The dashboard's tag leaves out `meta.generatedAt`,
which changes on every read; the other time-derived fields, such as
`isUpcoming` and `liveStatus` in every one of these reads, are included, so
the tag changes with them as well as with stored data. A matching
`If-None-Match`, weak or strong, answers 304 without a body, and the client
keeps its stored copy, including that copy's `generatedAt`. Because the tag is
weak it validates the gzip and identity encodings alike.
`/api/dashboard/revision` is `no-store`.

### Board Projection

The dashboard carries a locale-neutral `projection` for shared board
decisions. Its values are codes, booleans and record IDs, never localized
copy. Every ID names a record in the same response; the browser joins the
returned records by these IDs and does not infer their order from localized
text.

| Projection field   | Value                                                                                        |
| ------------------ | -------------------------------------------------------------------------------------------- |
| `version`          | Contract version; `1` is the current one                                                     |
| `nextTransitionAt` | Next ISO instant that can change a time-dependent decision, or `null` when none is known     |
| `focus.eventIds`   | Ordered event candidates of the focus                                                        |
| `focus.videoIds`   | Ordered video candidates of the focus                                                        |
| `sales`            | Ordered IDs of every sale event the board's on-sale list shows, in place or in its sheet     |
| `spotlight`        | The [spotlight](#spotlight), or `null` when there is neither a next entry nor a latest video |
| `shelf`            | The [video shelf](#video-shelf)                                                              |

#### Spotlight

The spotlight is the stage's one ticket: the next or current entry, or the
latest video when nothing is next.

| Spotlight field    | Value                                                                                                                                                                                               |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `mode`             | `next` for a current or upcoming entry; `latest` when the latest video is the fallback                                                                                                              |
| `eventId`          | The shown event, `summary.nextEvent`, or `null`                                                                                                                                                     |
| `videoId`          | The shown video: `summary.nextStream` in `next` mode, `summary.latestVideo` in `latest` mode, or `null`                                                                                             |
| `liveNow`          | `true` when the shown entry is the dashboard's `liveNow` broadcast, which has neither an event nor a video                                                                                          |
| `isLive`           | `true` when the shown entry is confirmed on air or its start time has passed                                                                                                                        |
| `membersChip`      | `true` when the shown event is a `member` entry paired with its video, the video is not members-only, private or removed, and a `game`, `talk` or `song` label takes the type's place on the ticket |
| `companionVideoId` | `summary.latestVideo`, shown beside the ticket when the shown entry has no video thumbnail and is not that video; otherwise `null`                                                                  |

`liveNow` says which entry is shown; `isLive` says whether it is on air, for
an event or a video as well. An event and a video are both set when the next
event and the next stream share a stream link. In `latest` mode only
`videoId` is set, and the booleans are `false`.

#### Video Shelf

The shelf lists IDs from the returned `videos`. The lead is in none of the
four lists.

| Shelf field      | Value                                                                                                                                                    |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `leadId`         | The Featured video; otherwise the newest work that is a premiere or a song, then the newest work, neither private nor removed; `null` when there is none |
| `leadIsFeatured` | `true` when `leadId` is the operator's Featured video                                                                                                    |
| `reservations`   | Videos that are not past, and cancelled ones whose start is still ahead, earliest first                                                                  |
| `works`          | Past uploads and premieres that are not Shorts, newest first                                                                                             |
| `archives`       | Past streams that are not Shorts, newest first                                                                                                           |
| `shorts`         | Past Shorts, newest first                                                                                                                                |

### Event and Video Views

Every event and video carries a `view` of codes and booleans, never
localized copy. Calendar, archive and MCP `schedule_list` reads carry the
same event and video `view` objects as the dashboard. Clients keep an
additive fallback for responses without `projection` or `view` so a retained
snapshot remains readable while a deployment serves mixed versions.

| Event view field  | Value                                                                                                     |
| ----------------- | --------------------------------------------------------------------------------------------------------- |
| `state`           | One of the seven states below                                                                             |
| `typeCode`        | `stream`, `member`, `event`, `release`, `video` or `sale`, or `null` when the stored type is outside them |
| `activityCode`    | `live`, `game`, `talk`, `song` or `video`, or `null`                                                      |
| `activityContext` | `stream` or `work` when `activityCode` is set, otherwise the empty string                                 |
| `isWork`          | `true` only for the `release` and `video` types                                                           |
| `calendarEntries` | The entry's calendar occurrences, sale-expanded                                                           |

`state` is `upcoming` when the entry has not started, `live` when a linked
broadcast is on air, `open` when a sale period is open, `done` when a
non-pending entry or sale has ended, `pending` when the time or status still
needs confirmation, `cancelled` when the entry is cancelled, and
`cancellation_review` when cancellation evidence is awaiting confirmation.

An ordinary event has one calendar entry. A sale period has its opening and
its closing entry; a sale deadline has only its closing entry.

| Calendar entry field | Value                                                                |
| -------------------- | -------------------------------------------------------------------- |
| `saleMark`           | `null` for an ordinary event; `opens` or `closes` for a sale's end   |
| `startsOn`           | Japan date (`YYYY-MM-DD`) the occurrence is shown on                 |
| `startsAt`           | ISO instant of the occurrence, or `null` when only the date is known |
| `timePrecision`      | The stored `exact`, `approximate` or `unknown`                       |

| Video view field | Value                                                                                                      |
| ---------------- | ---------------------------------------------------------------------------------------------------------- |
| `phase`          | `upcoming`, `live` (confirmed on air), `started` (start passed, not confirmed live), `past` or `cancelled` |
| `formCode`       | `short`, `upcoming`, `stream`, `archive`, `premiere` or `video`, or `null` when no public form applies     |
| `shelfGroup`     | `reservation`, `work`, `archive` or `short`, or the empty string for a video omitted from the shelf        |
| `latestEligible` | Whether the video may be the stage's `latestVideo` fallback                                                |
| `focusEligible`  | Whether the video may be a focus candidate                                                                 |
| `tagContext`     | `stream` or `work`: whether a content tag is worded for a stream or a work                                 |

## Revision Stream

`GET /api/dashboard/events` is a `text/event-stream` of the dashboard
revision. On connect it sends `retry: 5000` and the current revision as one
`data:` line, then one `data:` event for each later change, and a comment
every 25 seconds so proxies keep a quiet stream open. An event carries only
the revision number, the value `/api/dashboard/revision` already reports;
snapshot data, job IDs, counters and errors never travel on it. A client that
reconnects receives the current revision again, so it cannot miss a change.

One watcher goroutine in the serving process reads the stored revision every
second while at least one stream is open and offers each change to every
stream; it stops with the last stream. Reading the stored value covers every
writer, including `kano-go sync` and other processes that write SQLite
directly. A stream answers `Cache-Control: no-store, no-transform` and
`X-Accel-Buffering: no` and is never compressed. The listener's read and
write timeouts do not end it; each write instead has its own 10-second
deadline, so a client that stops reading releases its stream. At most 1,000
streams are open at once: another one answers 503 `stream_unavailable` with
`Retry-After`, and a database failure before the stream starts answers 503
`database_unavailable`. Shutdown ends every stream before the listener
closes. The [reverse-proxy settings](../operations/docker.md#reverse-proxy)
the stream needs are with the deployment guide.

Media responses follow the [media HTTP contract](media-cache.md#http-contract).
Selected profile media uses `/media/profile/avatar` and
`/media/profile/banner`, the LINE BLOG archive's files use
[`/media/lineblog/<name>`](media-cache.md#line-blog-archive-media), and the
discography covers use [`/media/works/<name>`](media-cache.md#discography-covers).
With `CDN_PUBLIC_BASE` set, the reads name these routes on the CDN, which pulls
them from the same paths here.

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
summary candidates (see [Duplicate merging](data-model.md#duplicate-merging)
and [Events of hidden posts](data-model.md#events-of-hidden-posts)).

Every public event carries `endsOn`, the Japan close date of a
[sale period](data-model.md#sale-periods) or null. A range lists a sale when
it contains the sale's opening or close date, and `dayCounts` and the adjacent
dates count the close date too.

Every public event carries `tag`: `live`, `game`, `talk`, `song` or `video`,
or null when the entry is unlabelled. Every read derives it the same way from
the event and its YouTube videos, so the board, `/api/calendar`, MCP
`dashboard_read` and `schedule_list` report the same value, and an entry
without a video link can carry one. Readers translate the code for display
and fall back to the title when it is null; see
[Shown label](data-model.md#shown-label) for the order.

Dashboard SQL selects up to 100 recent post previews plus the latest-post
fallback, up to 100 current-week event previews plus 30 upcoming and 30 live
focus candidates, up to 10 [sale periods](data-model.md#sale-periods) on sale
or opening within seven days (closing soonest first), and at most one linked
event per returned video, and bounded
video candidates plus Featured selection and the latest completed video.
Live event eligibility is applied before the 30-row limit: cancelled and
suspected cancellations, tombstones, non-stream activities and expired streams
do not consume the shortlist. Eligible live events use ascending start instants
and stable ID ties, matching the frontend focus ordering, including streams
spanning earlier weeks. Known end times remove finished streams before limiting
focus candidates and retain streams with explicit longer durations; an absent
end gives streams and memberships the strictly less-than-three-hour grace
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
category is its shown [tag](data-model.md#video-form-and-tags) (`game`,
`talk` or `song`: the operator's, else its schedule entry's label, else the
title inference's), or `other` without one; each read derives it, and every
video projection carries the tag as `tag`, `null` without one, and who can
watch it as `availability` (`public`, `unlisted`, `members`, `private`,
`removed`, or `null` until a check could tell; see
[Video availability](sources.md#video-availability)). An invalid range returns 400
`invalid_range`, an invalid handle 400 `invalid_account` and an unknown
category 400 `invalid_category`. Reads revalidate through their
[ETags](#public-snapshot-contract), negotiate gzip like the other public JSON
reads and never fetch platforms. The board's schedule and post month dialogs
use the ranges with the calendar; its video dialog uses the timeline.

Without `from` and `to`,
`GET /api/videos?until=2026-09-30&page=1&pageSize=30&category=game` is the
video timeline: the stored videos dated on or before the inclusive Japan date
`until`, or every dated video without it, newest first, paged, projected and
filtered like a range. Its `categoryCounts` count every video it pages
through, `adjacent.previous` is always `null`, and `adjacent.next` is the
nearest later Japan date holding a video, so a timeline that starts at an
earlier month knows that newer videos exist. An invalid `until`, a lone `from`
or `to`, or `until` together with a range returns 400 `invalid_range`.

`GET /api/posts/months` and `GET /api/videos/months` are the month indexes of
the post archive and the video timeline: `months` lists every Japan month
(`YYYY-MM`) that holds a shown post or a listed video, oldest first, each with
its `total` and its `accountCounts` per lowercase handle or `categoryCounts`
per category, plus the `revision`. Hidden posts and records without a
parseable time are not counted. They take no parameters, revalidate through
their ETags and negotiate gzip like the archive pages.

## Journey and Works Collections

`GET /api/journey`, `GET /api/works` and `GET /api/works/upload-list` serve
the data of the Journey and Works pages, so a client other than the web page
can draw them. Their bodies ship with the build: the export scripts
(`npm run journey:export`, `npm run works:export`) write them from the
pages' tracked data with the pages' own modules into
`backend/internal/journey/` and `backend/internal/works/`, which embed them,
and the frontend tests fail when a committed body differs from the data
([Journey](../development/history-archive.md#content-and-provenance),
[Works](../development/works-archive.md#content-and-media)). They read no
snapshot rows, so they carry no `revision` and their ETags change only with
a build that changes the data. Each answers `GET` alone (405 with
`Allow: GET` otherwise), revalidates through its
[ETag](#public-snapshot-contract) and negotiates gzip; a deeper path, such
as `/api/journey/<id>`, is 404 `api_not_found`.

The bodies are locale-neutral. A localized text is `{ja, zh-CN, en}`, with
`null` for a language the data lacks; the server never picks a language.
Kinds, roles, formats and labels are codes the client words itself, and so is
the fixed wording around them (section titles, rights notices). Images are
same-host relative URLs (`/media/journey/<name>`, `/media/works/<name>`), or
those routes on `CDN_PUBLIC_BASE` when it is set, served under the
[media HTTP contract](media-cache.md#http-contract);
third-party addresses appear only as provenance links, never as files a
client must load. `updatedAt` is a Japan date (`YYYY-MM-DD`).

### Journey

`GET /api/journey` returns `updatedAt`, the later of the newest check date of
the Journey images and the Works collection's date, `branches`, `milestones`
in page order (by stored date, oldest first) and `galleryOnly`, the images
that belong to no milestone and appear only in the gallery.

| Branch field | Value                                                                |
| ------------ | -------------------------------------------------------------------- |
| `id`         | Identity code, such as `singer` or `mahoro`                          |
| `tone`       | Colour code of the identity, such as `honey`                         |
| `since`      | Year the identity starts                                             |
| `label`      | Localized name                                                       |
| `portrait`   | The identity's portrait as a [media item](#journey-media), or `null` |

| Milestone field | Value                                                                                              |
| --------------- | -------------------------------------------------------------------------------------------------- |
| `id`            | Stable milestone code                                                                              |
| `date`          | As stored: `YYYY`, `YYYY-MM` or `YYYY-MM-DD`; never narrowed to a guessed day                      |
| `datePrecision` | `year`, `month` or `day`, the part of `date` that is known                                         |
| `dateEnd`       | Last day (`YYYY-MM-DD`) of a period, or `null`                                                     |
| `dateNote`      | A name written after the date, such as the event `C93`, or `null`                                  |
| `branch`        | Branch `id`                                                                                        |
| `kind`          | Code, such as `release`, `debut`, `stream` or `artwork`                                            |
| `highlight`     | Whether the timeline emphasizes it as a key milestone                                              |
| `title`         | Localized text                                                                                     |
| `detail`        | Localized text, or `null`                                                                          |
| `sourceUrl`     | The strongest public page for the entry, or `null` when the only evidence is a third-party archive |
| `links`         | Further pages as `{url, label}`; `label` is a code (`agency`, `x`, `cancelledRelease`)             |
| `releaseId`     | ID of the Works release whose covers it shows, or `null`                                           |
| `media`         | Its [media items](#journey-media), catalogued images first, then the release's covers              |

#### Journey Media

A media item is one image of the timeline or gallery. `origin` is `fixed` for
an image of the Journey catalog and `release` for a cover of the milestone's
Works release, so a client draws the Journey without `/api/works`. Every
item has every field below; a field that does not apply to its origin is
`null`.

| Field                         | Value                                                                                                                                               |
| ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `origin`                      | `fixed` or `release`                                                                                                                                |
| `id`                          | Stable image code; a release cover's is `<releaseId>/<name>`                                                                                        |
| `url`                         | `/media/journey/<name>` or `/media/works/<name>`                                                                                                    |
| `width`, `height`, `mimeType` | Size in pixels and `image/jpeg` or `image/png`                                                                                                      |
| `branches`                    | The branch `id`s whose gallery lists it; a release cover adds the identities the release also belongs to                                            |
| `category`                    | `artwork`, `goods`, `frame`, `model` or, for a release, `cover`                                                                                     |
| `title`                       | Localized text; a release cover's is the release title in every language                                                                            |
| `date`                        | Japan date the image is dated by, or `null` when unknown                                                                                            |
| `dateKind`                    | What that date is: `post`, `announcement`, `publication`, `stream`, `outfit`, `capture` or `release`                                                |
| `sourceKind`                  | Who published the file: `artist`, `organizer`, `creator`, `video`, `streamArchive`, `shop`, `brand`, `agency`, `officialCover` or `thirdPartyCover` |
| `note`                        | Code of the provenance note the page shows, or `null`                                                                                               |
| `focus`                       | CSS `object-position` that anchors a cropped thumbnail, or `null`                                                                                   |
| `credit`                      | Credit line the publisher requests, shown as written, or `null`                                                                                     |
| `creator`                     | Who made the picture, as written, or `null`                                                                                                         |
| `frameTime`                   | `HH:MM:SS` of a video frame, or `null`                                                                                                              |
| `sourcePage`                  | The page the image comes from (a release cover's file page), or `null`                                                                              |
| `sourceUrl`                   | Original third-party file address (provenance only), or `null`                                                                                      |
| `originalStream`              | Original stream address at the frame of a third-party archive frame, or `null`                                                                      |
| `releaseId`                   | The Works release of a cover                                                                                                                        |
| `cancelled`                   | Whether every edition of that release was withdrawn                                                                                                 |
| `cover`                       | The cover as a [Works image](#works-images), with its caption, credit and third-party mark                                                          |

### Works

`GET /api/works` returns `updatedAt`, the Japan date the collection was
checked through, `summary` (`personal`, `participation` and `uploads` counts
and `firstYear` and `lastYear`), `personal` and `participation`, oldest
first as the collection lists them. Titles and names keep the collection's
wording. The collection's phrase-coded wording (see
[Wording](../development/works-archive.md#wording)) is rendered into
localized text with the page's own phrases, so a client needs no phrase
dictionary; such a field is `null` when the collection gives no text.

| Personal release field | Value                                                                                                                                                                                    |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `id`, `date`, `title`  | Release ID (Japan date and a title hash), first Japan release date, title as written                                                                                                     |
| `format`               | `album`, `single`, `ep` or `doujin`                                                                                                                                                      |
| `digital`, `cancelled` | Released for download or streaming; every edition withdrawn                                                                                                                              |
| `unit`                 | Unit name of a unit work, or `null`                                                                                                                                                      |
| `label`, `catalog`     | Localized text                                                                                                                                                                           |
| `covers`               | [Works images](#works-images)                                                                                                                                                            |
| `disc`                 | Photo of the disc for the shelf, or `null`                                                                                                                                               |
| `printedDisc`          | The record shows the cover printed on a disc instead of a drawn one                                                                                                                      |
| `cutout`               | Figure cut out of the cover on a transparent ground, or `null`                                                                                                                           |
| `tracks`               | The [tracks](#works-tracks) every edition shares                                                                                                                                         |
| `discs`                | Photos of the shared discs as `{disc, image}`                                                                                                                                            |
| `editions`             | Editions on sale; empty for a release sold in one plain version                                                                                                                          |
| `storeBonuses`         | Bonuses for any edition as `{store, item, images}`; `store` and `item` are localized text                                                                                                |
| `stores`               | Streaming and download listings for any edition as `{item, links}`; `item` is localized text or `null` for the main item, and each link is `{platform, url, hiRes}` with a platform code |

An edition has `name`, `catalog`, `price`, `format` and `packaging` as
localized text, `date` (its own Japan date when it came out after the
release, or `null`), `cancelled`, `jan` (a string, empty when unknown),
`covers`, `bonuses` (localized texts), `bonusImages`, `storeBonuses`,
`stores`, its full `tracks`, `discs` and `printedDisc`, shaped as on the
release.

#### Works Tracks

A personal track has `disc`, `discLabel` (localized text or `null`), `no`,
`title`, `credit` and `note` (localized text or `null`) and `listen`, the
uploads of the same recording, or `null`. `listen` keeps platform video IDs
in the shape of the [upload list](#works-upload-list): `niconico` and
`bilibili` (an ID or `null`), `youtube` and `youtubeMembers` (lists of IDs).

A participation work has `id`, `date`, `title`, `roles` (codes `vocal`,
`lyrics`, `composition`, `voice`, `guest`), `forms` (codes such as `digital`,
`bonus`, `drama`, `cdr`, `boxSet`, `notForSale`, `promo`), `unit` (the unit
name, or `null`), `label`, `event` and `catalog` as written (empty when
unknown), `sourceUrl` (the page
the record follows), `cover` (a Works image or `null`) and `tracks`, each
`{disc, no, title, credit, kano, kanoCredits}`: `credit` is the database's
artist line as written, `kano` whether Kano took part in the track, and
`kanoCredits` her further credit codes (`lyrics`, `composition`,
`arrangement`, `animation`, `other`).

#### Works Images

Every cover, disc photo, bonus image and cutout is `{url, width, height,
mimeType, caption, sourceUrl, sourceLabel, credit, thirdParty}`: `url` is
`/media/works/<name>`, `caption` localized text or `null`, `sourceUrl` the
page the file was taken from or `null`, `sourceLabel` the site named instead
of a page or `null`, `credit` `{role, name, url}` (role `scan`, `photo` or
`fan`; `name` and `url` may be `null`) or `null` for a file from a person's
own copy, and `thirdParty` whether the file is a store's, database's or
collector's copy of the release's artwork rather than the publisher's file
(`null` for a cutout).

### Works Upload List

`GET /api/works/upload-list` returns `updatedAt` and `uploads`, the
collection's uploads oldest first, each `{id, date, song, originalArtist,
kind, with, niconico, youtube, youtubeMembers, bilibili, bilibiliTitle,
albums, tracks}`. `kind` is `cover`, `original`, `mv`, `project`,
`compilation`, `game` or `composition`. Videos are platform IDs, never URLs:
`niconico` and `bilibili` are an ID or `null`, `youtube` and
`youtubeMembers` lists of IDs. `originalArtist`, `with` and `bilibiliTitle`
are free text, empty when there is none; `albums` (the albums that include
the recording) and `tracks` (a compilation's songs) are lists of free-text
names as the collection writes them, not release IDs. The list never joins
stored data; [`/api/works/uploads`](#works-uploads) adds each upload's cover
and stored videos by the same upload ID.

## Works Uploads

`GET /api/works/uploads` joins the Works page's packaged upload list
([Upload covers](../development/works-archive.md#upload-covers)) with stored
data by video ID, on each read and without writing. It returns `updatedAt`,
the list's date, `revision`, and `uploads`, keyed by upload ID, for each
upload with a ready cover or a stored video: `cover` (`{url, width, height}`:
a `/media/<id>?v=<sha256>` URL of this site (on `CDN_PUBLIC_BASE` when set)
with smaller copies and the size, with
`width` and `height` `null` while unknown; or `null` without a cover) and
`videos`, the stored YouTube and Bilibili videos of the upload in list order
as `{source, id, title, url, publishedAt, scheduledAt, durationSeconds,
availability, translations, sourceLanguage}`, where `publishedAt`,
`scheduledAt`, `durationSeconds` and `availability` are `null` while
unknown. Uploads with neither are left
out. A stored video's title is its shown one (an operator's correction
first); raw JSON and thumbnails' source URLs never appear.

`recent` lists, newest first, at most 48 of the stored videos dated after
`updatedAt` that the page shows above the list, derived on each read under
the rules in
[Uploads after the collection date](../development/works-archive.md#uploads-after-the-collection-date);
`recentTotal` counts them all.
Each is `{id, source, title, url, publishedAt, cover, origin, translations,
sourceLanguage, alternates}`: `source` is `youtube` or `bilibili`,
`publishedAt` is a premiere's scheduled time or the publish time, `cover`
is its ready cached thumbnail in the shape above or `null`, `origin` is
`auto` (listed by its tag, form and format) or `operator` (an operator's
decision includes it), and `alternates` names, for a YouTube video, its
merged Bilibili uploads as `{source, url}`. No kind, artist, credits or
albums are inferred, and the
operator's decision itself, availability and counters stay out. Other videos
outside the list never appear. The read revalidates through its
[ETag](#public-snapshot-contract) and never fetches platforms.

## LINE BLOG Archive

`GET /api/lineblog`, `GET /api/lineblog/posts/{id}` and
`GET /api/lineblog/posts/{id}/comments` serve the
[LINE BLOG archive](../development/lineblog-archive.md) page's articles and
comments to other clients. The server embeds them from the tracked page data
([Public API Export](../development/lineblog-archive.md#public-api-export)),
so they change only with a release, never read SQLite, and carry no
`revision`. Each body carries `updatedAt`, the Japan date of the newest
archived article or comment. Text is locale-neutral: a localized value is
`{"ja", "zh-CN", "en"}` with `null` for a language the archive lacks, and no
read picks a language from the request. Fixed wording, such as the rights
notice and labels, stays in each client. Media are same-host
`/media/lineblog/<name>` URLs (on `CDN_PUBLIC_BASE` when it is set);
third-party addresses appear only as
provenance links that the page opens on request, never as something a client
must load.

The reads take only `GET` (other methods answer 405 with `Allow: GET`),
revalidate through their [ETag](#public-snapshot-contract) and negotiate
gzip. `{id}` is an article number; an unknown or malformed one answers 404
`not_found`, and any other path below `/api/lineblog/` (an empty, further or
trailing segment) answers 404 `api_not_found`.

A **media object** is `{url, width, height, mimeType}`: `url` is the
`/media/lineblog/<name>` address, `width` and `height` the displayed size of
an image (`null` for a video), and `mimeType` `image/jpeg`, `image/png` or
`video/mp4` (always H.264 with AAC audio). A missing file answers 404 on its
route; clients show an unavailable state with the provenance link.

`GET /api/lineblog` lists the archive:

| Field             | Value                                                                                                                                                                                                                                                            |
| ----------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `updatedAt`       | `YYYY-MM-DD`                                                                                                                                                                                                                                                     |
| `originalBlogUrl` | The blog's original address, as provenance                                                                                                                                                                                                                       |
| `posts`           | Every article, newest first: `id`, `published` (ISO instant), `title` (localized), `thumbnail` (a media object of its first archived picture or video cover, always an image with its size, or `null`) and `commentCount` (its archived comments, 0 without any) |
| `months`          | Each Japan month (`YYYY-MM`) with articles, newest first, with its `total`                                                                                                                                                                                       |

`GET /api/lineblog/posts/{id}` returns one article with its neighbours in
the page's pager order:

| Field        | Value                                                                                                                                                                                                                        |
| ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `updatedAt`  | `YYYY-MM-DD`                                                                                                                                                                                                                 |
| `post`       | `id`, `published`, `title` (localized), `url` (the original address), `archiveUrl` (the Wayback Machine capture it was recovered from), `commentCount` (its archived comments, 0 without any) and `blocks`, in reading order |
| `previousId` | The next older article (the page's 前の記事), or `null` for the oldest                                                                                                                                                       |
| `nextId`     | The next newer article (the page's 次の記事), or `null` for the newest                                                                                                                                                       |

Each block has a `type`:

| `type`      | Fields                                                                                                                                                                                                                                                                         |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `paragraph` | `content` (inlines) and `translations`: `{"zh-CN", "en"}`, each a list of translation paragraphs (each a list of inlines) shown after the paragraph, or `null`. Both are machine translations of the Japanese. A paragraph of only non-pictograph pictures is a row of figures |
| `quote`     | `content` (inlines)                                                                                                                                                                                                                                                            |
| `rule`      | none                                                                                                                                                                                                                                                                           |
| `video`     | `video` (media object)                                                                                                                                                                                                                                                         |
| `youtube`   | `videoId`, `watchUrl`, `video` (the downloaded copy, a media object or `null`), `cover` (media object or `null`), `coverSourceUrl` and `archiveUrl` (provenance or `null`)                                                                                                     |
| `card`      | A link card: `title`, `url`, `articleId`, `archiveUrl` and `description` (or `null`), and `image` (an image inline or `null`)                                                                                                                                                  |

Inlines also have a `type`:

| `type`  | Fields                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `text`  | `text`; line breaks are `\n`                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `link`  | `href`, `content` (inlines) and `articleId`, the archived article the link opens, or `null`                                                                                                                                                                                                                                                                                                                                                                                          |
| `image` | `image` (media object, or `null` for a picture the archive never captured), `alt` (readable alternative text or `null`), `emoji` (a pictograph drawn at text size, not opened in a viewer), `href` (the link the picture opened, or `null`), `sourceUrl` (where the archived bytes came from, or `null`), `remoteUrl` (the original address of a picture never captured, or `null`) and `recovery` (`stand-in` or `tweet` for a picture added after the recovery archive, or `null`) |

`GET /api/lineblog/posts/{id}/comments` returns `updatedAt` and the
article's archived `comments`, newest first, in Japanese as archived (`[]`
for an article without any): `number` (deleted comments leave gaps),
`author` (the display name), `posted` (ISO instant, to the minute in Japan
time), `body` (plain text with `\n` line breaks) and `rating`
(`{icon: "star" | "face", value: 1–5}` or `null`). Commenters' links are
never kept.

## Public Feeds

`GET /api/calendar.ics` is an iCalendar (RFC 5545) subscription of the events
dated 30 days before to 90 days after today's Japan date, at most 500 events,
each once: events from today on fill the cap first and the most recent past
events take what is left. They are read through the same bounded calendar
pages and event projection as `/api/calendar`. The calendar names no time
zone, so it reads the same in every viewer's zone: times are UTC, and an event
without a time, or of `unknown` precision, is a floating all-day entry
(`DTSTART;VALUE=DATE` with an exclusive `DTEND` the next day) on its Japan
date. All-day entries also carry `X-MICROSOFT-CDO-ALLDAYEVENT:TRUE`, which
keeps Outlook from drawing them across two days; timed entries do not. A
timed event without a stored end after its start lasts an hour when it is a
stream (the `stream` and `member` types the board treats as streams) and half
an hour otherwise, since RFC 5545 would make it zero-length. A sale period
is written at its ends, as the board shows it: its opening (titled "Sales
open:" in the feed's language) and, when it closes on a later date, its close
("Sales close:"), or the close alone for a deadline (a close on the opening
date without a stored opening time; a stored time of `unknown` precision
makes an all-day opening, as on the board). A closing entry with a
time ends at that time and starts half an hour earlier. A UID is the
public event ID with `@kano-navi`; the closing entry of a period adds
`-close` to the ID. `LAST-MODIFIED` is the event's stored
`updated_at`, and `SEQUENCE` is that time in whole seconds since 2020-01-01
UTC, so it grows whenever the event is written and clients replace a changed
entry; an event without a parseable update time has neither. `STATUS` is
`CANCELLED` only for a cancelled status or an operator's confirmation
(`manual_confirmed`), `TENTATIVE` for a suspected cancellation or a
`tentative` status, and `CONFIRMED` otherwise. The description carries the
event's detail, its provenance and its public link; only absolute HTTP(S)
links are written. Text is escaped and lines are folded at 75 octets without
splitting a character.

`GET /api/feed.xml` is an Atom 1.0 feed of at most 50 public videos (YouTube
videos and reservations and unmerged Bilibili uploads, as the video archive
lists them) dated 30 days before to 11 days after today, newest first. An
entry is dated by its publication (a future time is dated at the start of
today's Japan day until it passes, so the entry does not move between polls),
says when a reservation is scheduled, and links the video and any
merged platform copy. Its categories name the form (`stream`, `premiere`,
`upload`, `short` for a Short, or `video` while the form is unknown) and
then the video's tag, when it has one. Entry and feed IDs are `urn:kano-navi:` URNs. The
server never builds links from the request's `Host`: with `PUBLIC_ORIGIN` the
feed links the site absolutely and adds a `rel="self"` link to
`/api/feed.xml`; without it the site link stays `/` and there is no self link.

Both take an optional `lang` query, `en`, `ja` or `zh-CN`; a missing or
unknown value is the English base, so the bare paths stay English. Titles
resolve [translations](data-model.md#translations) the way the board does:
each reader gets the locale's translation when one exists, except that
Japanese readers keep wording detected as Japanese (an event only through its linked
video's title). The fixed wording the server writes — the calendar and feed
names (the board's `site.name`), the event descriptions' provenance and
unofficial notes, the generated YouTube details (`YouTube Live`,
`YouTube Premiere` and the older stored forms it still recognizes) and the
Atom subtitle and summaries — comes from a small table in
`backend/internal/feeds/locale.go`. Stored data is unchanged: only
rendering picks the language. A non-English Atom feed declares `xml:lang`, has
the feed ID `urn:kano-navi:feed:<lang>` and its self link keeps the `lang`
query.

Both answer `GET` and `HEAD`, never fetch platforms or models, negotiate gzip
like the [public JSON reads](backend.md#text-transfer-and-static-caching), and
send `Cache-Control: public, max-age=600` with a weak ETag that validates both
encodings. The ETag names the language, the build version (so an upgrade's
rewording is not hidden by a cached copy) and a hash of the rendered body.
Every calendar `DTSTAMP` is the start of today's Japan date, so the
calendar changes only with the snapshot, the date and the build; its ETag
also names the dashboard revision. The Atom feed also follows the clock,
since a reservation turns into an aired video when its time passes. A
matching `If-None-Match` answers 304. A database failure answers 503 without
an ETag.
The routes sit under `/api/`, so the development proxy and the
[read-only mode](backend.md#read-only-mode) serve them unchanged.

## MCP Boundary: `/mcp`

`/mcp` uses stateless Streamable HTTP. `dashboard_read`, `schedule_list`,
`posts_list`, `health_read` and `sync_status` are public read-only tools and
do not require a credential. Public `sync_status` lists the recent jobs' kind,
status and times only: it carries no job ID and takes no `jobId`. Control
tools are only registered for requests carrying
`Authorization: Bearer <MCP_CONTROL_TOKEN>`: they can advance a dashboard
revision, start an asynchronous sync, or run the automatic schedule scan. For
those requests `sync_status` also reports each job's `id` and looks a job up
by `jobId`, so a started job can be followed. The token is independent of
`ADMIN_PASSWORD` and browser sessions.

A JSON-RPC batch holds at most 100 messages and runs at most 10 tool calls,
since each can read a whole snapshot. Later `tools/call` requests in the same
batch are answered with the tool error `Too many tool calls in one batch (at
most 10)` without running; other requests in the batch are answered as usual.

The endpoint deliberately has no tool for manual event confirmation, editing,
deletion, bulk approval, profile-media selection/upload, SQL, file paths, DOM
commands, or arbitrary URL proxying. Human confirmation remains a human-only
workflow at `/admin`.

## YouTube WebSub Callback: `/api/websub/youtube`

The route is active only while YouTube push is enabled in `/admin`
(`/api/admin/sources/youtube/push`); otherwise it answers like an unknown API
path. It answers Google's hub verification for this channel's topic and
accepts signed Atom notifications, which only wake a queued YouTube sync. See
[Sources](sources.md#websub-push) for the settings, verification, signature,
debounce and queue rules.

## Web Bot Auth Key Directory

`GET /.well-known/http-message-signatures-directory` (and `HEAD`) serves the
public key that signs the shop source's outbound requests, as
`application/http-message-signatures-directory+json`: a JWKS with one Ed25519
key (`kty`, `crv`, `x`) and never a private member. The response carries its
own `Signature-Input` and `Signature` over `("@authority";req)`, tagged
`http-message-signatures-directory`. It answers `404 {error: "not_found"}`
while Web Bot Auth signing is not active and 405 to other methods. See
[Web Bot Auth](web-bot-auth.md#key-directory).

## Related Docs

- [Backend](backend.md)
- [Admin API](admin-api.md)
- [Data model](data-model.md)
- [Web Bot Auth](web-bot-auth.md)
- [Media cache](media-cache.md)
- [Deployment security](../operations/security.md)
