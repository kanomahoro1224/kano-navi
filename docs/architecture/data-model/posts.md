# Posts

How quoted posts, reposts, hidden posts, posts deleted upstream, imported posts and post LLM states are stored.

## Quoted Posts

A post that quotes another status keeps it in `posts.quote_json`, a
sanitized projection (`id`, `authorHandle`, `authorName`, `text`,
`publishedAt`, `url`, and `unavailable` for a quote the source names but
does not carry). An X post quotes an X status; a Bilibili
[forward](../sources/bilibili-dynamics.md#forwards) quotes the dynamic it forwards, with the
author's mid as `authorHandle` and the post ID `bilibili-<dynamic ID>` as
`id`. The quoted status's images are `media_links` with the role
`quote-image` and the quoting post as owner; `quote_json` holds no image
address, and a post's `post-image` links, `media_url` and
`raw_json.mediaURLs` are its own images only. A post without a quote stores
`NULL`. See [Quoted posts](../sources/quoted-posts.md) for how the X adapter
reads it and why quoted images are never schedule candidates.

The public dashboard, the post archive and MCP add `quote` to each post:
`{id, authorHandle, authorName, text, publishedAt, url, unavailable, media,
translations, sourceLanguage}`, where empty fields are `null` and `media` has
the shape of the post's own `media` but lists only ready, contained opaque-ID
images ([Translations](translations.md) describes the last two). The read path
accepts a projection only for its post's source. On an X post, it drops one
whose ID is not a status ID or whose URL is not that status's x.com address.
On a Bilibili post, it drops one whose ID is not `bilibili-<digits>` or
whose URL is not exactly that dynamic's `https://www.bilibili.com/opus/` page
or, for an available forwarded upload, a `https://www.bilibili.com/video/`
page. An unavailable quote has only `id` and `url`; an unavailable forward
of a dynamic whose ID the listing did not give has a `null` `id` and the
forward's own opus page as `url`. The console's post list keeps a post's own
images apart from its quote.

## Events of Hidden Posts

An automatic, unlocked event whose every active source is a hidden post is
hidden by its posts: the active `event_sources` rows of the event and of the
events merged into it all name a stored X or Bilibili post with
`posts.hidden_at` or `posts.upstream_deleted_at` set (an operator hid it, or
[X no longer has it](#posts-deleted-upstream)), and there is at least one. It
is a visibility rule computed at read time, not a deletion or tombstone: the
row, its evidence and its merges stay, and showing a source post, or X
listing it again, shows the event again. An event
with `manual_locked = 1` or `provenance = manual` is never hidden this way,
since the operator's decision outranks hiding; confirming an event keeps it
shown while its post stays hidden. An event with any other active source,
such as a YouTube reservation or a visible post, or with no evidence at all,
stays shown. Every public event reader leaves such an event out, as it leaves
out merged rows (see [Duplicate merging](duplicate-merging.md)), and so does a
video's linked event. The console lists it with `hiddenByPosts`.

## Reposts Stored as Posts

An own account's repost has a status ID of its own, which a detail answer
or an X API answer names (see [Reposts](../sources/quoted-posts.md#reposts)). An own X
post stored under that ID, such as one an archive import wrote, is
soft-deleted in the transaction that records the repost: `posts.deleted_at`
is stamped and its refresh request cleared. Nothing is removed: the row, its
`media_links`, its `schedule-x-<id>` asset with that asset's links and
review, `post_llm_states`, `schedule_extractions`, and its `post`
translations and translation state stay as they were. Every reader skips a
deleted post and its schedule-image candidate: the dashboard, the post
archive and its month index, MCP, the console's lists, counts and failures,
extraction, translation, refresh and the schedule-image check. Its active
`event_sources` rows are retired as an extraction that no longer names them
retires them: an automatic, unlocked event left without active evidence is
retired, which releases the rule merges into it, while an event with other
active evidence stays and a manual or locked event keeps its row and
decisions (see [Event precedence](event-precedence.md)). A model's suspected
cancellation that the post raised is withdrawn; an operator's confirmed
cancellation stays. A promoted post, another account's post or a post
already deleted is never touched this way. The ID stays in `x_repost_ids`,
so no synchronization, fetch or promotion writes a post under it again; a
write that names it skips it, with its schedule candidate, and the deleted
row keeps its stored content.

## Posts from Account Exports

A post written from an [account export](../sources/x-accounts.md#account-exports) carries
`raw_json.archive = {origin: "x-export", capturedAt}` beside the detail-shaped
fields Normalize reads (`tweetID`, `text`, `date_epoch`, `mediaURLs`,
`mediaKinds`, `qrt`), and is otherwise an own X post: synchronization
rewrites it like any stored post. A post an earlier third-party import wrote
carries `raw_json.import` instead. When an export does not list such a post,
the export import hides it (`hidden_at`) and records why in
`raw_json.import.reconcile`: `{decision: "hidden", reason, at}`, where
`reason` is `at_reply` for wording that opens with another account's handle
and `unlisted` otherwise, and `at` equals the `hidden_at` it set. A review adds
`outcome` (`top_level`, `self_reply`, `reply`, `foreign`, `repost` or
`gone`) and `checkedAt`; an own top-level post or thread reply whose
`hidden_at` still equals `at` is shown again with `decision: "shown"`. A hide
whose time differs from `at` is an operator's, which no import or review
changes.

The app setting `x_archive_accounts` (a JSON array, absent until an export of
such an account is imported) lists the X accounts whose posts come only from
an export: the post archive's account filter offers them after `x_accounts`,
and the console's account avatars list them as own accounts. They are not
`X_HANDLES`, so no synchronization reads them.

### Post LLM states

`post_llm_states.route` records which detection path (`schedule_message` or
`schedule_board`) handled a post.

`post_llm_states.failure_count` counts consecutive failed extractions of the
input named by `failure_fingerprint` (a hash of the post's input fingerprint
and the ordered targets that were tried), and `next_attempt_at` is the ISO
time the automatic scan may try it again; a positive count with a null
`next_attempt_at` means automatic retries stopped. A success or uncertain
outcome, cached or not, and an operator reprocess request reset all three
(`0`, `NULL`, `NULL`); startup adds them to existing snapshots with those
values, so a stored failure is retried once more before backing off. An
operator's dismissal sets `status` to `dismissed` with a positive count and a
null `next_attempt_at`, keeping `failure_fingerprint`. See [Extraction failure backoff](../sources/extraction.md#extraction-failure-backoff).

`post_llm_states.settled_stamp` names the input a post's cache hit recorded:
the SQLite function `schedule_input_stamp`
(`internal/store/sqlite/schedule_stamp.go`) hashes the detection stage, the
extractor version, the post's `source`, `text`, `published_at`, `url` and
`media_url`, the image lists `raw_json` declares (`mediaUrls`, `media_urls`,
`mediaURLs`, or the whole record when it is not a JSON object), the shown
quote's `authorHandle`, `authorName`, `text` and `unavailable` as the
extractor decodes them, and every `post-image` link in `position, id` order
with its media ID and the asset's `status`, `sha256`, `mime_type` and
`cache_path`. It covers every input of the extraction identity and the image
check, so a change to any of them changes the stamp; a stamp that changes
when the identity does not only has the post judged once more. The extractor
reads the same image lists through `schedule_media_json`, an object of the
record's `mediaUrls`, `media_urls` and `mediaURLs` members as stored (NULL
when the record is not a JSON object), so the stamp and the extractor parse
the record alike.

Only a cache hit writes the stamp, in the same statement as the state that
hit records. Every other state the extractor writes, including a fresh
model result, stores NULL, as do an operator reprocess request and a
dismissal. A post is settled for a detection stage when its `status` is
`success` or `uncertain`, its `route` is that stage's, `reprocess_requested`
is 0, `last_error`, `failure_fingerprint` and `next_attempt_at` are NULL,
`failure_count` is 0, and `settled_stamp` equals the stage's current stamp;
the scan then counts it as a cache hit without loading it (see
[Schedule extraction pipeline](../sources/extraction.md)).
Startup adds the column empty, so every post is judged until a cache hit
records its stamp.

A settled post's images are not read, so a cached file deleted from disk
after the post settled goes unnoticed: the post keeps its stored result
instead of waiting as `media_pending`, until its input changes or an
operator requests a reprocess.

## Posts Deleted Upstream

An own X post whose status X no longer has is soft-deleted upstream: when an
account's run drops a stored post's ID with `x_status_gone` (see the
[pending retries](../sources/x.md)), the write stamps
`posts.upstream_deleted_at`. Nothing is removed or rewritten: the row, its
`raw_json`, its media links and files, its schedule-image candidate and
review, its LLM states, extractions and translations, and its events and
their evidence stay as they are. Public reads leave the post out, and with
it each automatic, unlocked event it is the only source of (see
[Events of hidden posts](#events-of-hidden-posts)): the board's
wall and latest post, the post archive and its month index, and with them the
public API and MCP; the board's and the calendar's schedule images leave out
its schedule-image candidate. The console lists the post with
`upstreamDeletedAt`. The mark is a state of the source, not an operator
decision: any later answer that carries the status clears it, whether a
detail answer, a timeline or X API page listing the stored post, or a manual
fetch. Another account's run, a promoted post and a post already deleted
(`deleted_at`) are never marked.
