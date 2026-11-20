# YouTube

How YouTube channels and videos are read, including the Data API.

The `youtube` step lists the uploads of `YOUTUBE_CHANNEL_ID` (the current
main channel by default) through the YouTube schedule's
[method chain](../workflows/source-schedules.md#method-chains): by default the Data API's uploads
playlist when `YOUTUBE_API_KEY` is set (see [Data API](#data-api)), then the
RSS feed, configured yt-dlp when its method is enabled, and the uploads tab. It takes the newest listed uploads and stores the ones
not stored yet; see [source schedules](../workflows/source-schedules.md#source-schedules). An
incremental run takes at most the YouTube schedule's `max` (50 by default and
for full syncs; the feed lists 15) uploads that are not stored yet, plus the
stored ones among the newest `max` as detail candidates. A `count` run takes
the newest `count` entries, stored or not. Without a key, a `count` above the
feed's 15 is partial and reports `youtube_count_limited`. The operator's
manual fetches list uploads through their own method instead (see
[Manual fetches](../workflows/fetch-methods.md#youtube-methods)).
The cursor is the newest listed upload. An incremental run that leaves a new
upload above the stored cursor unread, because of its cap or a failed
playlist page, keeps the cursor, so later runs take the rest while the
listing still reaches it; without a cursor the first run is bounded on
purpose. It separately inspects the streams page and keeps rechecking active or
recent reservations for `scheduledStartTime`, writing them to both `videos` and
`events`. When no schedule time is available, the step does not invent one:
an upcoming yt-dlp entry that names its `release_date` but no
`release_timestamp` is a date-only reservation, with that date as the
event's `starts_on`, no `starts_at` and `time_precision` `unknown`, and no
`scheduled_at` on the video, until a check reports its start; a date-only
answer never replaces a known start. RSS insertion
does not count as a reservation detail check, but RSS lists waiting rooms and
premieres like ordinary uploads, so the uploads a run takes are detail
candidates as well, after stream-page discoveries. A watch
page whose player reports `isLiveContent: false` with a start time is a
premiere: its event is a `release` and its video's kind is `premiere`.
`isLiveContent: true` is a stream verdict only on a page that reports the
broadcast over, since a premiere can read as live content before it ends.
The verdict is stored as `videos.is_premiere` (1 for a premiere, 0 for live
content), and a check that cannot tell, such as a page without
`isLiveContent` or a waiting room or broadcast on air that reads as live
content, keeps the stored one for the video and its event. Until a verdict
exists the video and its event are a stream.
A finished stream or premiere writes the `endTimestamp` from its
`liveBroadcastDetails` as the event's `ends_at`; a later check without one keeps
the stored end. Every successful watch-page check also records the broadcast
state the page reported (see [Live-status probe](youtube-live.md#live-status-probe)). The
reservation event stores the status code `scheduled`, `live` or `ended`, and a
broadcast reported live is `live`, never `ended`; its detail is `YouTube Live`
or `YouTube Premiere`, and a members-only title sets the `member` type (see
[Stored event text](../data-model/events.md#stored-event-text)). Successful detail checks are recorded in incremental
`sync_state.metadata.streamDetailsCheckedIds` for currently discovered, recent
RSS and active videos. Eligible discoveries and reservation
refreshes share the durable rotating detail queue; failed checks remain eligible,
and failed refreshes retain existing video/event snapshots. HTTP 200 alone is
not success: a reservation needs a parsed timestamp and page title; an ordinary
video without a reservation needs a complete, playable player response with a
matching video ID and title. Missing/truncated details, invalid timestamps, and
upcoming pages without a start time remain retryable within the existing request
budget. They record the protected `youtube_watch_details_incomplete` diagnostic
without updating video/event details or claiming a successful check. A page
that shows YouTube's bot check instead records `youtube_watch_login_required`:
a `LOGIN_REQUIRED` player whose reason names the check ("confirm you're not a
bot") or gives none that is recognized, or the check's page without a player.
YouTube puts a host behind this check for every watch page at once, so the
run requests no further watch page, and the code appears once. A
`LOGIN_REQUIRED` reason that concerns one video is not the check, and the run
goes on: a private video reads as `youtube_watch_private` and an age gate
records `youtube_watch_details_incomplete`. Reasons are recognized in English and
Japanese, in the player's reason, messages and error screen. The queued IDs it did not reach
keep their places ahead of the rotated one, and an unclassified Data API
answer stands in for each of them as for a failed page. A members-only page
answers `UNPLAYABLE`: one that still names its start, as a stream archive
does, passes as members-only, and one that does not fails as
`youtube_watch_members`. A page that tells who can watch the video (see
[Video availability](youtube-videos.md#video-availability)) is no error and does not stop the
run's page requests. Incremental
metadata distinguishes successful `inspected` checks from `detailRequested`
attempts. Manual fetches
do not write this incremental metadata.

Feed failures record codes with their own `youtube_feed_` prefix
(`youtube_feed_upstream_5xx`, `youtube_feed_unavailable`,
`youtube_feed_http_error`, `youtube_feed_timeout`,
`youtube_feed_rate_limited`, and so on), so the run view tells them apart
from the streams page, the uploads tab and watch pages, which keep the
`youtube_` prefix. The public RSS feed returns intermittent HTTP 5xx
responses that clear within seconds, so the feed request alone is repeated up
to two more times after an upstream 5xx (`youtube_feed_upstream_5xx`) or a
connection failure (`youtube_feed_unavailable`), waiting 0.5 and then 1.5
seconds. Each try keeps the 10-second request timeout, and cancellation stops
the wait. Rate limiting, other 4xx responses (`youtube_feed_http_error`),
timeouts, redirects, and oversized bodies are never repeated. Streams and
watch pages are requested once; their 5xx responses record
`youtube_upstream_5xx`.

The feed can also answer 404 for hours at a time, so a failed listing does not
stop the step. It records the code and still works through stored detail
work: refresh requests, the rotating queue, active reservations and due
follow-ups. The chain's next method, by default the channel's uploads tab
(`/channel/<id>/videos`), is read once in its place, and its newest 15 IDs
are detail candidates, after the streams page's. The tab has no publication times, so
those IDs add no unchecked rows and leave the cursor alone; a watch page
supplies a new video's time from `publishDate` (a stored time always wins).
The step result reports where the uploads came from as `listing`: `api`,
`rss`, `ytdlp`, `videos_page`, or empty when nothing answered, `tried` lists every
listing method tried with its code, and a run that recorded
failures lists their controlled codes as `errors`, each once in order of
first appearance with `×n` when it occurred n times (for example
`youtube_watch_details_incomplete ×12`). The run view in `/admin` shows such
lists of plain values joined by commas. Only a run with no
listing (the uploads tab counts as one) and no successful detail check
fails, as retryable `dependency_unavailable` (or `timeout`/`rate_limited`)
after the feed's code. It writes no video, event, cursor or incremental
metadata, so existing snapshots remain; it stores only the rotated detail
queue, so IDs whose checks keep failing do not starve the rest.

## Data API

With `YOUTUBE_API_KEY` set and the chain's `api` method on, the recent step
asks the YouTube Data API:

- `playlistItems.list` on the channel's uploads playlist (`UU` and the channel
  ID after `UC`) is the listing, read like feed entries: the video's
  publication time (or the entry's), the title and the largest `*.ytimg.com`
  thumbnail. Private entries are skipped. Pages of 50 (one quota unit each,
  at most four) follow `nextPageToken` until the run has its uploads (new
  ones, for an incremental run). An incremental run also stops at a page that
  reaches the stored cursor (an entry at or below its time), or without a
  cursor at a page that holds a stored video, so uploads an earlier capped
  run left below newer stored ones are still read. A failed
  later page keeps the earlier ones. When the first page fails, the feed and
  then the uploads tab are tried as above.
- `videos.list` calls (`snippet`, `contentDetails`, `liveStreamingDetails`,
  `localizations`) cover the detail queue in order, 50 IDs per call and at
  most five calls (`ytsource.MaxAPIDetailCalls`), so a 200-video `count`
  window fits in one run. Any room left in the last call goes to the newest
  listed uploads of the run, stored or new. A failed call records its
  `youtube_api_` code and ends the lookup, since a refused key, quota or rate
  limit holds for the later calls too; the IDs it and later calls would have
  covered fall back to watch pages. A described queued video counts as a
  successful detail check without a watch-page request: its title, length
  (ISO 8601 `duration`), and for a
  broadcast the actual start (or the reservation), the actual end and
  `liveBroadcastContent` as `upcoming`/`live`/`ended`. The watch-page budget
  (the video pages per run setting) then goes to the queued IDs the API left out.
  An answer for a listed upload outside the queue only refreshes its
  [localized titles](youtube-videos.md#localized-titles).
- The API has no `isLiveContent`. An upcoming or live broadcast with a length
  is a premiere, since a stream reports `P0D`; one without a length gets no
  verdict, since a premiere can report `P0D` before it ends too.
- An ended broadcast is classified by the channel's per-format playlists.
  When a queued video is an ended broadcast that has no stored premiere
  verdict, the run reads the newest page of the live playlist (`UULV` and the
  channel ID after `UC`) and of the long-form playlist (`UULF` and the
  channel ID after `UC`), one `playlistItems.list` call and one quota unit
  each, counted in `apiRequests`, at most once per run. YouTube lists
  streams on the live playlist and premieres with ordinary uploads on the
  long-form one, so an ended broadcast on the live playlist is a stream and one
  on the long-form playlist is a premiere. The playlists are undocumented,
  like the [Shorts playlist](youtube-videos.md#shorts), and are read only after the
  end, so a listing before or during a broadcast decides nothing. A video on
  neither page (members-only streams, Shorts, older broadcasts) or on both
  gets no verdict from them, and a failed read records its `youtube_api_` code
  and decides nothing. A stored premiere verdict is final and is never looked
  up, while a stored stream verdict is looked up again and replaced when the
  long-form playlist names the video.
- Without such a verdict, an ended broadcast is classified from its stored
  row only when an earlier detail check gave it a start time and a stored
  `is_premiere` verdict. Otherwise, as for a row without a
  stored verdict, an untitled video or an upcoming one without a start, its
  watch page decides.
- When that watch page fails for an ended broadcast the API gave a start
  (`ytsource.Video.Unclassified`), the API answer is used anyway rather than
  leaving the video unchecked, since some hosts never get a readable page. It
  writes the title, thumbnail, actual start and end, length and `ended`
  status, sets `live_checked_at` and answers a refresh request, while a
  stored `stream` or `premiere` kind (otherwise the kind is `unknown`), the
  `is_premiere` verdict and the reservation event's type and detail stay. Such
  an answer updates an existing reservation event but creates none, and the
  failed page's code is still recorded. The `reconcile` step adds the event of
  an ended broadcast that has none (see
  [Duplicate merging](../data-model/duplicate-merging.md)). A later readable page
  settles the verdict as usual.
- A thumbnail that is the same image as the stored one keeps the stored URL,
  so the watch page's `?v=` version does not read as a replacement; the media
  step still revalidates the file. See [Video corrections](../data-model/videos.md#video-corrections-and-history)
  for what counts as the same image.

A run that finds a stored video on its first playlist page spends three
quota units (of the default 10,000 a day), the members-only playlist page
included (see [Video availability](youtube-videos.md#video-availability)), while its detail
queue holds at most 50 IDs; each further 50 queued IDs add one, up to five `videos.list` calls,
each further playlist page adds one, and the per-format playlists two more
when a queued ended broadcast needs them. The key is sent only in the `X-Goog-Api-Key` header to
`https://www.googleapis.com/youtube/v3/`, with the client's 10-second timeout,
4 MiB limit and rejected redirects. Failures record controlled codes
(`youtube_api_forbidden` for a refused key or quota, `youtube_api_rate_limited`,
`youtube_api_http_error`, `youtube_api_invalid_response`, and so on) and fall
back to the public pages; `apiRequests` in the step result counts the calls.
The [live-status probe](youtube-live.md#live-status-probe) asks the same `videos.list`
first, one call and one quota unit per probe run, under the same
classification rules. An operator's manual API fetch checks the videos it
lists under these rules too, without watch pages; a whole-playlist fetch
reads the per-format playlists to their end rather than their newest pages
(see [YouTube methods](../workflows/fetch-methods.md#youtube-methods)).

A YouTube chain with `api` off makes no Data API call for its runs, key or
not: no listing, `videos.list`, per-format, Shorts or members-only playlist, and the
[live-status probe](youtube-live.md#live-status-probe) reads watch pages only. Watch pages
and the members-only feed take over as they do without a key.
