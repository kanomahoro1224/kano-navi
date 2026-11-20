# YouTube Video Details

Localized titles, Shorts, availability and follow-up checks for stored YouTube videos.

## Localized Titles

Titles the creator localized on YouTube are stored as
[platform translations](../data-model/translations.md) with platform `youtube`.
Only a `videos.list` answer carries them, from its `localizations` part, which
costs no extra quota unit; both the recent step's call and the live-status
probe's call write them. Language keys map to the target locales: `en` and
any `en-*` variant to `en`; `zh`, `zh-CN`, `zh-Hans` and `zh-Hans-*` to
`zh-CN`. Traditional Chinese (`zh-TW`, `zh-Hant`, …) and other languages are
ignored. When several keys fill one locale, the key naming the locale wins,
then the first in sort order. A blank title, one equal to the video's
original title, or one over `MaxTranslationRunes` is no translation.

For every stored YouTube video an answer described, the writer upserts the
locale's `youtube` row in the source's transaction and deletes it when the
answer no longer fills that locale. A video the API did not answer, or a run
whose call failed, keeps its rows. Manual and machine rows are never touched,
and the dashboard revision moves only when readers see the change: a manual
row of the locale hides the platform one. A video whose locales are all
covered is not sent to the [translate step](translation.md)'s model.

Without `YOUTUBE_API_KEY` no localized title is read. A watch page requested
with `hl=en` or `hl=zh-CN` shows the localized title in
`videoPrimaryInfoRenderer`, but reading it would add two page requests per
video to the bounded watch-page budget and the public pages' rate limits, and
the page localizes by more than the query, so localized titles need the key.

Video thumbnails are registered and linked to their rows, then the bounded media
stage downloads pending files from YouTube's `*.ytimg.com` thumbnail CDN
subdomains (including `i.ytimg.com` and its numbered hosts). A size YouTube
never generated falls back to the next smaller one, and the thumbnail of an
upcoming or recent video is revalidated because its URL stays the same when the
creator replaces the image; see [Media cache](../media-cache.md).

## Shorts

The feed is the signal that tells a Short apart. Each entry's
`<link rel="alternate">` points a Short to `https://www.youtube.com/shorts/<id>`
and every other upload, stream or premiere to its `watch?v=<id>` page
(`ytsource.Video.Short`). Only an HTTPS link on `www.youtube.com` or
`youtube.com` naming the entry's own ID counts; no link, another host or a
link for a different ID leaves the format unknown. Every run records the
verdict of each listed entry as `videos.is_short` (1 or 0) on the stored row,
new or old, after the video upsert; a verdict for a video the snapshot does
not hold adds no row, and a changed value advances the dashboard revision.
The stored `url` stays the canonical watch page. Watch pages, the live probe,
manual fetches and Bilibili never write `is_short`, so a later detail check
cannot clear it. Rows neither the feed nor the Shorts playlist below named
stay `NULL` (unknown); the length is never used to guess. The step result's
`shorts` counts the uploads marked as Shorts in that run, so a feed that
stops naming the format shows as zero.

When the Data API supplies the listing, the feed is still read for these
verdicts alone, a fixed helper read whether or not the chain enables `rss`. If it fails, its code is recorded and one `playlistItems.list`
page (one quota unit, counted in `apiRequests`) of the channel's Shorts
playlist (`UUSH` and the channel ID after `UC`) stands in: every upload it
names that the run listed or the snapshot holds is marked a Short (1). The
playlist is undocumented, and an upload missing from it is not proof of a
regular video, so it never writes 0; only the feed does. It goes to the same
Data API destination under the same rules as the uploads playlist. A failure,
such as the 404 of a channel without Shorts, records its `youtube_api_` code
after the feed's, and that run marks no Shorts. Without a key, a failed feed
marks none. An operator correction (`manual_is_short`) lies over the stored
verdict; see [Video corrections](../data-model/videos.md#video-corrections-and-history).

## Video availability

Every check that can tell records who can watch a YouTube video in
`videos.availability`:

- A `videos.list` answer (the request names the `status` part, at no extra
  quota) gives `public` or `unlisted` from `status.privacyStatus`. The API
  cannot tell a members-only video from a public one, so it keeps a stored
  `members`.
- Every recent step run ends with the channel's members-only playlist
  (`UUMO` and the channel ID after `UC`): every stored video it names is
  `members`, whatever its age, and a video checked in the same run carries
  the verdict, so its entry is a `member` entry from that run on. The
  playlist is undocumented, and absence from it proves nothing, so it records
  no other value. With a key the run reads its newest page (50 entries), one
  `playlistItems.list` call and one quota unit counted in `apiRequests`, at
  the same Data API destination under the same rules as the uploads
  playlist, unless an earlier Data API call of the run failed, since a
  refused key, quota or rate limit holds for it too. Without a key, or when
  that call is skipped or fails, the playlist's public feed
  (`https://www.youtube.com/feeds/videos.xml?playlist_id=UUMO…`, its newest
  15 entries) stands in, a fixed helper read whether or not the chain enables
  `rss`, under the channel feed's codes, retries and limits. A run whose
  channel feed read failed skips it, since both are the same feed endpoint.
  Each failure, such as the 404 of a channel without members-only uploads,
  records its code and costs only the verdicts. The step result reports
  `members`, how many videos the playlist named, and `membersFrom`, the read
  that answered (`api` or `rss`, empty when none did). An operator's YouTube
  [manual fetch](../workflows/manual-fetches.md) ends
  with the same read and reports it the same way: through the Data API with
  the `api` method unless a Data API call of the fetch failed, otherwise the
  feed.
- A watch page that passes the completeness checks, a playable page or a
  `LIVE_STREAM_OFFLINE` waiting room, is `public`, or `unlisted` when its
  microformat says so; an `UNPLAYABLE` or waiting-room page that names a
  members-only reason and still names its start is `members`.
- A failed page tells only when YouTube answered it: `youtube_watch_private`
  is `private` and `youtube_watch_members` is `members`. For a video that a
  `videos.list` call which succeeded left out, as the API does private and
  removed videos, a sign-in page that does not name the bot check is
  `private` and any other page without the details is `removed`. A page that
  was not answered (a timeout, a 5xx) and the bot check tell nothing.

A failed page that told, and the members-only playlist, write only the
availability of the stored video, and a failed page that told counts as no
run error. A check that cannot tell keeps the stored value, and
nothing writes `NULL` back. A `members` video is a `member` entry whatever its
title says. Bilibili uploads keep `NULL`. An operator's correction
(`manual_availability`, see
[Video corrections](../data-model/videos.md#video-corrections-and-history)) lies over
the recorded value on the board, which checks keep writing beneath it. The
board treats `NULL` as watchable; the stage's latest video skips `private` and
`removed`, and their cards open no YouTube page (see
[Public API](../public-api.md)).

## Follow-up checks and refresh requests

Creators often replace a premiere's placeholder thumbnail, or retitle an
upload, after it goes out. A finished video therefore has its watch page read
again 1 hour, 1 day, 3 days and 7 days after its anchor
(`ytsource.FollowUpOffsets`): the broadcast end the linked event records, or
otherwise its start or publication. A video is due when the newest passed
offset has no successful check (`live_checked_at`) at or after it; anchors
older than eight days are left alone, so older videos are not reread. The YouTube step adds at most ten due videos per run behind the
active reservations, so they never starve new discoveries, and uses the same
detail budget, completeness checks and rotating queue.

An operator can also ask for one video from `/admin` (see
[Admin API](../admin-api/video-merging.md#video-corrections-and-history)). The request is
stored as `videos.refresh_requested_at`; the next YouTube step reads that page
before any other detail work, and a successful check clears it. The page is
read even when the [Data API](youtube.md#data-api) describes the video, so a refresh
can settle a premiere verdict that the API answer of an ended broadcast only
repeats; when the page fails, the API answer is used. A request
that no successful check answers within 24 hours stops going first.

An operator also adds or re-reads one video by ID or link from `/admin` (see
[Admin API](../admin-api/videos.md#youtube-videos)). That read runs at once, outside
the queue: the Data API describes the video under the recent step's rules
when a key is configured, and the watch page does without one or when the
answer cannot classify an ended broadcast. Only a video whose channel
(`channelId` in the API snippet or the page's video details) is
`YOUTUBE_CHANNEL_ID` is accepted (`youtube_other_channel` otherwise). It is
written like a manual fetch's listed and checked video, so manual locks and
deletion tombstones hold. Neither the incremental cursor nor the detail
queue moves.

Every detail check stores the player's `lengthSeconds` as
`duration_seconds` once a video is neither upcoming nor live. When a check
reports a new title or thumbnail URL, the replaced platform state is kept as a
`video_revisions` row (`origin = sync`); a YouTube API history page does the
same for a new title only, because the API names thumbnails differently from
the watch page. After any successful page read, the media step revalidates the
video's thumbnail file whatever its age, and new bytes at the same URL keep
the replaced file as a version (`origin = media`). Operator corrections of the
title, kind, tag or thumbnail are separate columns that synchronization never
writes; see [Video corrections](../data-model/videos.md#video-corrections-and-history).
The kind is the video's form, decided from these platform facts alone, and
what a video is about is its tag; see
[Video form and tags](../data-model/video-form.md#video-form-and-tags).
