# X

How X timelines are read and how the X API joins the read chain.

The synchronizer discovers status IDs for each account named by `X_HANDLES`
(default `kano_2525,_Kanotic`) from the public FxTwitter timeline
`api.fxtwitter.com/2/profile/<handle>/statuses`, which lists the account's
statuses newest first in pages of 20 with a continuation cursor; the page
size is fixed whatever `count` a request names. Discovery
pages back until a page's oldest own status is already stored or is not newer
than the account's stored cursor, so posts published between runs are found
even when several arrive at once. Before an account has a cursor, statuses
older than the first-run age limit (seven days by default; see
[sync settings](../../operations/configuration/sync.md#sync-settings)) also end the first run. A repost
lists the original status's ID, time and author, so it is never a post: the
run records it as a repost link (see [Reposts](quoted-posts.md#reposts)). The cap, the full-sync post cap or
the X schedule's incremental `max` (see
[source schedules](../workflows/source-schedules.md#source-schedules)), counts the
statuses that are not stored yet and sets the pages of 20 (at most 10) per
account and run. A failed later page keeps the newer pages already read.

A run that stops short of the stored cursor, because of the cap, the page
budget, a failed later page or a profile fallback, records the unread stretch
in `sync_state.metadata_json.timelineGap` as `{cursor, floor, older}`: the FxTwitter
continuation (empty for the newest page) and the cursor the run started from.
Later runs read the newest pages first and, when those reach a stored post,
continue the gap with a page budget of their own until a status at or below
`floor` appears with no status above `floor` after it on the page, so an old
pinned post leading the newest page does not close the gap. A second gap is
read first and keeps the earlier one, with its own continuation, as `older`;
once the newer gap reaches its floor, reading goes on from where the older one
stopped. An account keeps at most four gaps; past that the oldest merge into
the last kept one, which then reads on to the oldest floor. A failed
continuation restarts at the newest page. A timeline that ends
before the floor closes the gap and every older one and reports
`x_timeline_gap`, so an operator
can fill it with a timeline `all` fetch or an X API date range. The step result counts
accounts with an open gap as `timelineGaps`
([source schedules](../workflows/source-schedules.md#source-schedules)).

The timeline is the first method of the X schedule's default
[method chain](../workflows/source-schedules.md#method-chains). When its first page fails, is
not a timeline, or yields none of the account's own statuses, the chain goes
on to the logged-out profile page `x.com/<handle>`, which lists only the
newest few statuses. Only links whose path is `/<handle>/status/<id>` (handle
case-insensitive, plain or URL-encoded) count; quoted statuses, reposts and
other handles' links are left out, as the timeline leaves them out. The
chain may also enable the [X API](#x-api-in-the-chain). The run records the
method that served in `sync_state.metadata_json.discovery` (`timeline`,
`profile` or `api`) and counts accounts the profile page served as
`profileFallbacks` in the step result. A fallback alone does not fail the
account or mark the run partial. When a method other than the timeline
serves, every gap keeps its floor and drops its FxTwitter continuation, so
the timeline later reads it again from the newest page.

Before an account has a cursor, the Snowflake timestamps of discovered IDs are
decoded and IDs older than the first-run age limit are skipped. Each account has
its own cursor in `sync_state`. Later runs prioritize unknown IDs and may
refresh only a small budget of known posts: stored statuses the listing names,
read through a detail request. Such a refresh is optional. When it fails, the
run notes `<id>: <code>` and leaves it out of the pending queue, and the
failure never fails the account, so a quiet run whose refresh times out still
succeeds.

The timeline pages a run reads already carry each status, so a new or pending
status that a page lists with usable content is stored from the page, through
the same mapping as an operator's manual fetch (below), without a detail
request; posts the [X API](#x-api-in-the-chain) returns are stored the same
way. An `api.vxtwitter.com` detail request remains the fallback for a
status the timeline lists without content, for a pending status the pages no
longer list, for the known-post refreshes, and for every status found
on the logged-out profile page, which lists IDs only.
The status requests per run setting is the total detail-request budget shared by all
configured accounts; posts read from the timeline do not use it, and the step
result counts them as `timelinePosts`. A stored post that a page lists again
keeps the text, media and schedule candidate it has, as in a manual fetch.
Successful posts are stored with `account_handle` and returned as one
time-ordered feed. Image-only posts are retained as candidates as long as they
have a publication time and a media URL. A status whose ID is stored as
another account's post (one stored before a handle rename, or an imported
one) keeps that row, its media and its schedule-image candidate as they are:
the write skips it with the note `<id>: x_post_owner_conflict`, removes it
from the pending queue like a stored post, and writes the rest of the
account's run.

Both paths store the same values for one status, so a post first stored from
the timeline and refreshed through a detail request later keeps its
extraction fingerprint. Text, publication time, URL and counters match as
they are. FxTwitter appends `?name=orig` to a photo's URL and the detail
answer does not, so the timeline mapping drops that size. The detail answer
lists a video's or GIF's file in `mediaURLs`; the adapter reads its
`media_extended` instead and keeps the thumbnail, as the timeline does, and
rewrites `raw_json.mediaURLs` to match. A timeline post's `raw_json` lists its
own images (not a quoted status's) as `mediaURLs` too, because schedule
extraction declares a post's images from that field. A stored video or GIF
post that still names its file takes the thumbnail on its next refresh. A
quoted status is read into the same
[quote projection](quoted-posts.md) on both paths.

Every path also records what each image shows, a photo or the thumbnail of a
video or GIF: the detail answer's `media_extended[].type` (`image`, `video`,
`gif`), the timeline's `media` types (`photo`, `video`, `gif`) and the X
API's media `type` (`photo`, `video`, `animated_gif`), for the post's own
images and a quoted status's alike. The X API mapping and a pooled status
keep the kinds beside their images as `raw_json.mediaKinds`; an image no
source types is a photo. The kind is stored on the image's media link (see
[Media cache](../media-cache/layout-and-model.md#database-model)); a video or GIF file itself is
never requested.

The operator's timeline fetches read the same timeline differently (see
[Manual fetches](../workflows/manual-fetches.md)): they
read the same pages of 20 and store each own status from the page itself.
The adapter maps FxTwitter's `created_timestamp`, `text`, counters and
`media` (a photo's URL without the `?name=orig` size, or a video's or GIF's
thumbnail, in post order; a quoted status's images belong to its quote) onto
the same post fields the detail path fills, and keeps only the author's name
and handle of the repeated profile, and of a quoted status's, in `raw_json`. Such fetches leave stored posts, the
cursor and the pending queue unchanged.

Both discovery hosts and the detail host share one client: a fixed timeout and
User-Agent, a 4 MiB response limit, rejected redirects, typed error codes, and
no credentials or personal data in URLs. A failed source keeps stored posts and
cursors.

Normal incremental runs persist admitted but unfinished status IDs in the
account's `sync_state.metadata_json.pendingStatusIds` before detail requests.
The array's order is the per-account retry order. Newly admitted IDs join after
existing pending work; IDs selected within the account's request allocation move
behind unattempted work, with that rotation persisted before detail requests.
Thus a repeatedly failing ID cannot keep blocking other admitted work, even
after a process restart. Pending work remains retryable when it falls out
of the profile or ages beyond the bootstrap window, and a pending status that
a timeline page lists with content is stored from the page whatever its
failures. Successful posts, pending-work removal, the failure counts and the
latest successful cursor commit atomically. Budget exhaustion retains
unfinished IDs; an all-failed run does not record a successful cursor or
timestamp. An account whose listing served records `last_success_at` even
when it found nothing new, whether the timeline or the X API served it; the
account fails only when its listing failed or every status it requested
failed. Any failed account still fails the step with `x_source_failed`
when the run stored no post. The step's notes (gap codes, failed optional
refreshes and owner conflicts, as `<account>: <note>` in `errors`) leave
the step `partial` without failing it. Manual fetches neither consume this queue nor update normal
incremental state.

Failed detail requests are counted per ID in
`sync_state.metadata_json.pendingRetries` (`{failures, gone, retryAt}` by
ID, kept beside the `pendingStatusIds` array; rows without it start with no
failures). A 404 or 410 answer counts as gone, and the second consecutive
gone answer drops the ID with `x_status_gone`. When the account has the ID
stored as its post (such as one an operator's refresh queued), the post is
marked [deleted upstream](../data-model/posts.md#posts-deleted-upstream). An answer that the status is
another account's (an ID an older profile fallback queued from a quote, say)
is no failure: it drops the ID at once with `x_status_foreign`, without an
error, retry state or partial run, though it still counts as requested, and
marks a stored post of the account `foreign` (see [Post kinds](#post-kinds)). An
answer that the status is the account's own repost of another status (see
[Reposts](quoted-posts.md#reposts)) drops the ID the same way with `x_status_repost`. Any
other failure (timeout, 429, 5xx, invalid JSON or response) waits 1 hour after the
first consecutive failure, 6 hours after the second and 24 hours after the
third, as the extraction failure backoff does, and the fourth drops the ID
with `x_retry_limit`; a gone answer waits the same way before its second
request. A waiting ID makes no request, uses none of the budget and keeps its
place in the queue, and a cancelled run counts no failure. A dropped ID leaves the queue for good; a manual fetch can
still store it. The step result counts waiting IDs as `pendingHeld` and lists
dropped ones as `pendingDropped` (`<account>: <id>: <code>`); both keys
appear only when used.

Each discovered image URL is registered and linked to its post. A bounded media
stage later in the same command downloads pending image files from X's
`*.twimg.com` media CDN subdomains.

## Post kinds

Every answer that can tell records what a stored post is in
`posts.upstream_kind` (see [Post visibility](../data-model/posts.md#post-visibility)),
through one mapping (`StatusKind` in `backend/internal/source/x/classify.go`)
that the account-export review shares: another author's status is `foreign`,
the account's repost `repost`, and otherwise the reply fields give
`top_level`, `self_reply` (a reply to the account itself) or `reply`. A
timeline page tells it by `replying_to`, a detail answer by `replyingTo`,
`retweet` and its author, and an X API answer by its `replied_to` reference,
which the post's `raw_json` keeps in FxTwitter's `replying_to` shape; a parent
X does not return counts as a reply. Writing a post records its kind, as does
a listing of a post already stored, and a pending stored post's
`x_status_foreign` answer records `foreign`; each such answer also clears the
post's deleted-upstream mark. An answer without an author or without any
reply field, even a null one, cannot tell and keeps the stored kind; nothing
writes `NULL` back. A promoted post's kind is read for its own author.

## X API in the chain

The X schedule's [method chain](../workflows/source-schedules.md#method-chains) can enable the
official API (`api`, off by default), which needs `X_API_BEARER_TOKEN`
([method chains](../workflows/source-schedules.md#method-chains)). It reads
`https://api.x.com/2/users/<id>/tweets` without reposts and replies, newest
first, with the account's cursor as `since_id`, up to the run's cap of new
posts in pages of 5 to 100 and at most 10 pages; before an account has a
cursor it reads from the first-run age limit ago instead. The numeric user ID
comes from `/2/users/by/username/<handle>` once per process. Requests share
the manual X API method's client (see
[X API method](../workflows/fetch-methods.md#x-api-method)): the token only in the
`Authorization` header, the request timeout setting, the 4 MiB limit,
rejected redirects and `source_api_*` codes.

A read that stops short of the end (its cap, the daily cap, the page bound
or a failed later page) opens a [timeline gap](<>) at the cursor. Once the
newest posts are read, each gap is read between its floor (`since_id`) and
its ceiling (`until_id`), so it closes exactly and needs no continuation; a
read cut short resumes below the oldest status it stored. A gap records as
its ceiling the oldest status above the floor that the read which opened it
reached (on a timeline page, not counting a pinned post listed out of
order), and keeps it while the timeline reads on. A status stored inside the
gap later, such as a post pinned during the burst or one fetched by hand,
therefore never lowers the bound and leaves the statuses between them
unread. A gap stored before it carried a ceiling uses the oldest stored
status above its floor.

The answer carries what the timeline and detail paths store, and each post
is stored exactly as they store it, so a later timeline read or detail
refresh keeps its extraction fingerprint: a long post's whole text
(`note_tweet`), its `t.co` links replaced by their targets and its own
media links removed, HTML entities decoded; its own images in attachment
order (a photo's `url`, or a video's or GIF's `preview_image_url`); the
like, repost and reply counts; `raw_json.mediaURLs`; and the
[quote projection](quoted-posts.md) of the status it quotes, read from the
answer's expansions. Another author's post fails the answer with
`source_api_invalid_records`. Posts read this way need no detail request
and count as `apiPosts` in the step result.

**Gate.** The chain reads the API only after the account's timeline failed
`apiGate` consecutive runs (3 by default, 0–100; 0 opens it at once). The
current run's failure counts, so the third failing run already reads through
the API. A run the timeline serves starts the count over, and a cancelled run
changes nothing. A chain without the timeline has nothing to count and never
waits. A gated method records `x_api_gated` in `tried`, and the chain goes
on without making the run partial.

**Daily cap.** `apiDailyCap` (100 by default, 1–10,000) bounds the posts
X API answers return per Japan day, the unit X bills. The accounts post a few
times a day and a `since_id` read that finds nothing new returns nothing, so
an ordinary day of fallback reads returns a few dozen posts; 100 leaves room
to fill a gap after an outage while a month stays near 3,000 posts at worst.
A request never asks for more posts than are left. With fewer than five left
(the endpoint's smallest page), the chain skips the API with
`x_api_daily_cap` until midnight in Japan, and the run is partial. Manual
X API fetches ignore the gate and the cap, but the posts they read count
toward the day.

Each account's failure streak and the day's usage are kept in `sync_state`
(source `x-api`; see [Sync state](../data-model/sqlite-snapshot.md#sync-state)). The Sources
page shows today's usage beside the cap and each account's streak, and the
step result adds `apiPosts` and `apiRead`, the posts the answers returned,
when the API was used.
