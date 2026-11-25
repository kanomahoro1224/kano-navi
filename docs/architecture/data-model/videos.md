# Video Corrections and Merging

How video corrections, their history, and merged videos are stored.

## Video Corrections and History

Synchronization owns a video's platform fields (`title`, `kind`,
`thumbnail_url` and its media link) and keeps rewriting them on every
successful page read. An operator correction lives beside them in
`manual_title`, `manual_kind`, `manual_tag`, `manual_is_short`,
`manual_availability` and
`manual_thumbnail_media_id` / `manual_thumbnail_sha256` with
`manual_updated_at`; synchronization never
writes these columns, and clearing one shows the platform value again. Every
public reader (dashboard, video archive, MCP) and the console show
`COALESCE(manual, platform)`, and a pinned thumbnail is served while its file
is still cached, otherwise the platform thumbnail. Machine translations
follow the corrected title. A correction is not an event lock:
reservation events keep their own provenance and manual edits.

Nothing the platform replaces is discarded. A page read that reports a new
title or thumbnail image inserts the previous `title`, `thumbnail_url`, `kind`
and the then-current thumbnail file (`thumbnail_media_id`,
`thumbnail_sha256`) into `video_revisions` with `origin = sync` and
`replaced_at`. For YouTube, the sizes, formats and numbered hosts of one
video's `*.ytimg.com` thumbnail are the same image (`ytsource.SameImage`),
so the Data API's `hqdefault` and the watch page's `og:image` record no
revision; a live placeholder (`*_live`) or a different `?v=` version on both
URLs is a new image. History pages fill a missing thumbnail but never replace
a stored one. A thumbnail revalidation that downloads new bytes for the same
URL keeps the old file in `media_versions` and records a `media` revision for
every linked video. `duration_seconds` and `refresh_requested_at` are
described in [Sources](../sources/youtube-videos.md#follow-up-checks-and-refresh-requests).
Public readers (dashboard, video archive, MCP) add the length as
`durationSeconds`, null while it is unknown.

`is_short` is the YouTube format verdict: 1 for a Short, 0 for any other
upload, `NULL` while unknown. The feed writes both values; the Shorts
playlist that stands in for a failed feed writes only 1 (see
[Sources](../sources/youtube-videos.md#shorts)).
`manual_is_short` is the operator's correction with the same values and
`NULL` for none; synchronization never writes it. Public readers add
`isShort = COALESCE(manual_is_short, is_short)` as `true`, `false` or `null`,
and the board treats only `true` as a Short. A row without a verdict stays
unknown rather than being guessed from its length.
`is_premiere` is the stored premiere verdict of a YouTube reservation: 1 for
a premiere (a `release` event), 0 for live content, `NULL` while no check
could tell, which is also what the upgrade gives existing rows (see
[Sources](../sources/youtube.md)). It is never derived from the kind; the kind
follows it.
`availability` is who can watch a YouTube video: `public`, `unlisted`,
`members`, `private` or `removed`, `NULL` until a check could tell, which is
also what existing rows and Bilibili uploads keep (see
[Video availability](../sources/youtube-videos.md#video-availability)). A check that cannot
tell keeps the stored value. `manual_availability` is the operator's
correction with the same values and `NULL` for none; synchronization never
writes it. Public readers add
`availability = COALESCE(manual_availability, availability)`.
`manual_work` is the operator's Works decision: 1 lists the video among the
Works page's uploads after the collection date, 0 keeps it off, `NULL`
leaves it to the rules (see
[Uploads after the collection date](../../development/works-archive.md#uploads-after-the-collection-date)).
Synchronization never writes it, a decision on a video outside the list's
date or scope stays stored without effect, and no public reader reports it.
Startup adds the columns, both tables and `video_revisions_video_idx` to
existing snapshots without rewriting videos or media rows.

## Video Merging

A Bilibili upload of the same work as a YouTube video is attached to it, so
readers list one video with both links. The additive `videos` columns
`merged_into` (the YouTube video's ID), `merge_source`, `merge_reason` and
`merged_at` hold the decision on the Bilibili row. No source writer names
them, so rereading either platform keeps the merge. `merge_source` names the
owner as for [event merges](duplicate-merging.md):

- `rule`: the `reconcile` workflow step
  (`backend/internal/store/sqlite/video_merge.go`), which recomputes its
  decisions from the current rows on every run and releases a merge whose
  basis no longer holds. `merge_reason` is `rule:broadcast` for a live
  replay and `rule:time_duration_title` otherwise.
- `manual`: an operator merge (`merge_reason` `manual`), which the rule never
  changes. `manual` with a null `merged_into` is an undone merge
  (`unmerged_from:<id>`) or an upload an operator kept apart
  (`kept_separate`); the rule never merges that upload again.

A live replay (an upload whose settled form is `stream`) merges by rule into
the YouTube stream archive linked to the schedule entry of the broadcast it
replays. The replayed broadcast is the one among those
[Bilibili form](video-form.md#bilibili-form) reads that ended no more than 48 hours
before the upload was published, the latest end first, with the upload at
least half its length when that length is known; a live room's record
stands for the schedule entry it overlaps. The replay merges only when that
entry links a YouTube video of form `stream` (its `youtube-<id>` event,
source item or event source). A replay whose broadcast links no such
archive, such as one of a Bilibili-only broadcast, never reaches an earlier
broadcast's archive: it is public on its own unless the rule below merges
it.

Any other upload merges by rule into a YouTube video only when all of these
hold:

- the YouTube anchor (`scheduled_at`, else `published_at`) is within six
  hours of the upload's `published_at`;
- both `duration_seconds` are known and differ by at most three seconds;
- the song titles match. A Bilibili title is reduced to its `《…》` name (or,
  without one, the title less `【…】` labels) and a YouTube title to the part
  before `（`/`(`, `-`, `歌ってみた` or `【`, after leading `【…】` labels.
  Full-width ASCII is folded, punctuation and brackets split the rest, and
  parts that only name the performer or format (`鹿乃`, `翻唱`, `cover`,
  …) are dropped. The names match when they are equal, however short (`奏`),
  or share a part of at least two characters.

Effective titles (`COALESCE(manual_title, title)`) are compared. Several
complete matches, or one video claimed by several uploads, leave the upload
unmerged and are counted as `ambiguous`. A YouTube video receives at most
one upload: a video an operator merged an upload into is no rule target. A
video without `duration_seconds` leaves its upload to the operator. A Short of the same song, published another
day or with another length, never matches. An upload with no candidate
within six hours whose song title and known length (within three seconds)
match a video published further apart, such as a copy posted days later,
is never merged by rule; it is put to the operator as `unverified`.

Whenever a merge is written or a merged upload's effective title changes (a
Bilibili read, an operator correction or the reconcile step), the upload's
title is stored in the same transaction as the YouTube video's `zh-CN`
platform translation with platform `bilibili`; a release or undo removes it.
A change of either moves the dashboard revision. The machine translation
scope excludes merged rows.

Public readers (the board's shelf and summary, the video archive with its
month index and category counts, the single-video read, the Works page's
recent uploads, MCP and the Atom feed) list every YouTube video and each
Bilibili upload that is not merged into another video and whose form is
settled. They add `alternates` (`[{source, url}]`) to each YouTube video for
its merged uploads; a merged upload is listed only that way, so a work shows
once. Startup adds the columns and the partial `videos_merged_into_idx`
index to existing snapshots without rewriting videos.
