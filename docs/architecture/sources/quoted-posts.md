# Quoted Posts

How quoted posts and reposts are resolved and refreshed from the source.

A status a post quotes (`qrt` in the detail answer, `quote` on a timeline
page) is never a post of its own: it does not enter `posts`, the timeline
cursor or the pending queue, and extraction reads it only as context of the
quoting post. The adapter
(`backend/internal/source/x/quote.go`) reads it into a sanitized
projection stored beside the quoting post in `posts.quote_json`: the quoted
status ID, its author's handle and display name, its wording (as written,
untranslated), its publication time, and an x.com URL built from the
validated ID and handle rather than copied from the payload. Its images (a
photo, or a video's or GIF's thumbnail, at most four) are linked to the
quoting post with the `quote-image` role and cached like the post's own
(see [Media cache](../media-cache.md)); `quote_json` holds no image address.

A quote the payload names but does not carry is stored as unavailable with
only its ID and URL: vxtwitter's `qrtURL` without `qrt`, an FxTwitter quote
typed as anything other than a status (a tombstone), or a quoted status
without an author or content. A payload that names no usable status ID
stores no quote. X API answers carry the quoted status as an expansion; one
X does not return is stored as unavailable in the same way.

Every available quote also enters the X status pool, `x_statuses`, in the
quoting post's transaction, whatever its author (`backend/internal/store/sqlite/x_pool.go`).
The pool row and `quote_json` come from the same projection, so the pool
text is the text the quote card shows. The row keeps the status ID, author,
wording, publication time, x.com URL and a sanitized `raw_json` with the
quoted images' addresses, and `quoted_at`, the publication time of the
newest own post that quotes it. A quote of a stored post (a self-quote)
enters the pool too, but its translations stay the stored post's. No public
reader reads a pooled status (public reads look up only a promoted post's
demotion, see [Post visibility](../data-model/posts.md#post-visibility)), and
nothing of a pooled status gets a card,
a media link or a schedule candidate of its own. Startup fills the pool from
the quotes stored posts' `raw_json` already hold, with no request.

The [translate step](translation.md) translates quoted statuses by status ID,
so a status quoted several times is translated once, and the quote card
resolves those translations against its own wording (see
[Translations](../data-model/translations.md)). Schedule extraction reads the
quoted wording as context of the quoting post (see
[Schedule extraction pipeline](extraction.md)).

An operator can hide a post's quote from the public pages
(`posts.quote_hidden_at`); synchronization, the X API fetch method and the
startup quote backfill (which fills a missing `quote_json` from the post's
stored `raw_json`) keep updating the stored quote underneath and never write
that decision. Hiding or showing the whole post (`posts.hidden_at`,
`posts.shown_at`) works the same way: synchronization keeps rewriting the
stored post and never writes either (see
[Post visibility](../data-model/posts.md#post-visibility)).

## Reposts

Timeline pages list an own account's repost under the original status's ID,
time and author, with `reposted_by` naming the account; X API answers list
it as a post of its own ID that references the status as `retweeted`. A
vxtwitter detail answer for a repost's own ID names the reposting account as
its author, words itself `RT @author: …` and carries the reposted status
under `retweet`. Before using that evidence, the detail answer must explicitly
name the requested outer status ID (`tweetID`, `tweetId` or `id`); a missing
or different ID fails with `x_invalid_response` and keeps the stored post.
None of them is ever a post or a cursor, pending or gap
entry: a verified detail answer with `retweet` drops a pending ID with
`x_status_repost` and finishes a refresh. Recent synchronization, detail
requests and manual fetches record each repost in `x_reposts`: the reposted
status ID, the reposting account and, when known, the status's author and
the repost's first known own ID, with the time it was first seen, which a
later listing keeps. Every distinct own ID is retained separately as the
[data model](../data-model/posts.md#reposts-stored-as-posts) describes, including
multiple reposts of one original in an X API page. The board shows a reposted
status only once an operator promotes it (below). The timed X API method asks for the
account's posts without retweets, so it records none.

A detail answer that carries `retweet` with the reposting account as its
author is never a post: when the reposted status has no ID, or the requested
one, the answer fails with `x_invalid_response` and keeps the stored post.

A stored own post under a repost's own ID (an archive import can write one)
is soft-deleted with everything keyed to it kept, and the events it alone
supported are retired, in the same transaction (see
[Reposts stored as posts](../data-model/posts.md#reposts-stored-as-posts)). No later
write stores a post under that ID.

Every reposted status the listing or answer carries enters the pool with
`reposted_at`, whatever its author, and its reasons are evaluated again; a
repost item without the status only marks a pooled row reposted. A
[related account](x-accounts.md#related-accounts)'s status, unless its own timeline
stored it first, is translated for the `reposted` reason and promoted by a
keyword or by hand; another author's is stored untranslated and promoted
only by hand. The console's External posts list both under `reposted` with
the reposting accounts.

The wall is Kano's own words and a repost is someone else's, so no rule puts
a repost on the board; an operator picks the few worth showing, such as a
collaborator's song release, through the
[related-account promotion](x-accounts.md#related-accounts) and its lock. A promoted
repost is its author's post like any promoted post: it shows their handle,
name, wording and images, appears only on the combined wall and in the
archive, and is translated, extracted and cached as a post. The public post
names the own accounts that reposted it, from `x_reposts` and first
sighting first, as `repostedBy` (see
[Public snapshot contract](../public-api/snapshot.md)).

## Refresh from the Source

An operator's refresh of an own X post (`POST /api/admin/posts/{id}/refresh`,
or `POST /api/admin/posts/refresh` for up to 200 posts at once) only stores
`posts.refresh_requested_at` and removes the status's pending failure
backoff. The next X recent synchronization reads up to 20 such posts per
account, oldest request first, and puts them ahead of the queue. Under a
`count` sync setting, whose timeline read leaves the incremental state as it
is, the step makes these detail requests after that read and alone: no
discovery, pending queue or change to the incremental cursor, pending IDs or
timeline gap. Each takes
a detail request even when a timeline page lists the status (which would
otherwise store it from the page and leave the stored post unchanged) and
even while a failure backoff holds it. A usable answer rewrites the post,
its quote and its `quote-image` links like any detail answer; the hide
decision stays. An answer that the status is the account's repost
soft-deletes the post and records the repost instead (see [Reposts](#reposts)). The request is cleared once its detail request was made,
whatever it answered; a failed request is then pending like any other. A
refresh shares the X status-request budget, so with a budget of 0 it
waits. The step result counts made refreshes as `refreshed`, only when
there were any.

**Quoted images are never schedule candidates.** A post's `MediaURLs`, its
`raw_json.mediaURLs` and its `post-image` links are its own images only, so
a post that has only a quoted image becomes a text post for keyword
candidates and extraction, whatever its wording says.
