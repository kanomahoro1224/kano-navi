# Bilibili Stickers

How Bilibili stickers are stored and resolved.

A Bilibili dynamic's stickers (custom emoji) stay out of `posts.text`,
`quote_json.text` and the pool's `text`, so they never change a translation
hash or extraction input. `raw_json.nodes` keeps every rich-text node in
order, an emoji node with its caption as `text` and its picture as
`emoji.url`; a forward's `raw_json.orig` keeps the forwarded dynamic's
`title` and `nodes` the same way. The pictures are `media_links` of the post
with the role `post-emoji`, or `quote-emoji` for the forwarded dynamic's,
one link per distinct picture (see [Bilibili dynamics](../sources/bilibili-dynamics.md#dynamics)).

For a Bilibili post whose `raw_json` holds an emoji node, the public
dashboard, the post archive and MCP add `rich` to the post, and to its
available quote: the original wording as ordered segments, `{text}` or
`{emoji, url}`, where `emoji` is the caption and `url` the local
`/media/<id>?v=<sha256>` address of the ready cached picture, or `null`
while it is not ready. The read places each sticker by matching the nodes'
text in order against the stored text, each node where the last one ended
(past a title on its own line, allowing only the whitespace the trimming
removed), so the `text` segments joined are exactly the post's `text` and no
wording is skipped; a node not found there (cut off by the length bound)
leaves out the stickers after it. An opus page's paragraphs are joined by a
line break, which `raw_json.nodes` keeps as a text node of its own, so a
sticker that opens a paragraph stays on that paragraph's line. A post
without stickers has no `rich`.

Every available X quote is also a row of the X status pool `x_statuses` (see
[Quoted posts](../sources/quoted-posts.md)), which also holds every status of a
[related account](../sources/x-accounts.md#related-accounts): `id`, `author_handle`,
`author_name`, `text`, `published_at`, `url`, a sanitized `raw_json` (a
quote's `{id, text, created_at, url, author, mediaURLs}`, or the timeline
status as an own post stores it), `quoted_at`, the publication time of the
newest own post that quotes it, `reposted_at`, when an own account's repost
of it was first seen, `listed_at`, when its related
author's timeline first listed it, `keyword` (1 while a keyword rule
matches), the operator's `manual_decision` (`promote` or `demote`) with
`manual_reason` and `manual_at`, `demoted_at`, set while an automatic
demotion hides its post, `first_seen_at` and `updated_at`. A quote's `text`
equals the `text` of the `quote_json` written with it, since both come from
one projection; a status its related author's timeline listed keeps that
listing's wording and payload. No public reader reads or joins the table.

Every available forwarded dynamic is likewise a row of the Bilibili status
pool `bilibili_statuses` (see [Forwards](../sources/bilibili-dynamics.md#forwards)): `id` (the
post ID `bilibili-<dynamic ID>`), `author_mid`, `author_name`, `text`,
`published_at`, `url`, a sanitized `raw_json` (`{id, text, publishedAt, url,
author: {mid, name}, mediaURLs}`), `forwarded_at`, the publication time of
the newest own forward of it, `first_seen_at` and `updated_at`. Its `text`
equals the forward's `quote_json` `text`, and no public reader reads or joins
the table. A fresh database and an upgraded snapshot start with it empty.

`posts.promoted_at` marks a pooled status, a related account's or a reposted
one, promoted into `posts` (`NULL` for an own post), whose `account_handle`
is its author. Readers that
are an own account's (the dashboard's latest post and per-account newest
posts) leave it out, so a promoted post appears only in the combined post
wall and the archive, which also leave out a promoted post that a shown own
post quotes with its quote shown: the quote card already shows it. That is a
display rule computed at read time; it sets nothing, and the post returns
when the quoting post or its quote is hidden. `x_reposts` holds one row per reposted
status and own account (`status_id`, `account_handle` in lower case,
`author_handle` or `NULL`, `first_seen_at`, and `retweet_id`, the repost's
first known own status ID once a detail answer or an X API answer named it, else
`NULL`); see [Reposts](../sources/quoted-posts.md#reposts). Public post reads list a
promoted post's rows as its `repostedBy`, each account in the spelling of
its stored own posts. `x_repost_ids` holds every distinct repost's own
`retweet_id` as its primary key, with `status_id`, `account_handle` and
`first_seen_at`. Reposting the same original again adds its new ID while
keeping every earlier ID. No post is ever stored under one of these IDs
(see [Reposts stored as posts](posts.md#reposts-stored-as-posts)).

`posts.quote_hidden_at` records an operator's decision to keep a post's
quote off the public pages: the dashboard, the post archive and MCP then
omit `quote` entirely, while the console still shows it. No synchronization
path, refresh or backfill writes this column, so the decision survives every
rewrite of `quote_json` and the `quote-image` links, like a manual lock.
`posts.hidden_at` is the same decision for the whole post: the dashboard
(its post wall, latest post and post count), the post archive and MCP leave
the post out, extraction never selects it, and the automatic events it alone
supports leave the public pages (see
[Events of hidden posts](posts.md#events-of-hidden-posts)). The console still lists
the post, and no synchronization path writes the column. `posts.deleted_at`
marks a soft-deleted post, which no reader shows, the console included (see
[Reposts stored as posts](posts.md#reposts-stored-as-posts)). `posts.refresh_requested_at` is a
queued refresh from the source, cleared once the X synchronization made its
detail request (see
[Refresh from the source](../sources/quoted-posts.md#refresh-from-the-source)). All three are
additive columns, `NULL` by default.

Startup creates these tables in existing snapshots and adds
`x_reposts.retweet_id` empty. It copies every known `x_reposts.retweet_id`
into `x_repost_ids` without replacing existing identities. On every start it adds
to the pool the quote of every stored X post whose available quote the pool
does not hold as quoted by that post yet, mapped from the post's `raw_json`
like a source write, without a request, and marks every pooled status that
an `x_reposts` row names as reposted, with the earliest sighting as
`reposted_at`. A repost link whose status the pool does not hold stays a
link: no stored data carries its wording.

Startup adds `quote_json` additively. It then fills it for every X post
whose `raw_json` names a quoted status (an object under `qrt`, `quote`,
`quotedTweet` or `quoted_tweet`, or a `qrtURL`) and still has no
projection, and moves quoted images stored as the post's own. Such a post
lists its own images first and the quoted status's after them, all as
`post-image`; a `post-image` link moves to
`quote-image` when its image is one of the quote's and not one the post's
`raw_json` declares as its own, so the remaining own links keep their
positions, and an image both declare stays the post's. `media_url` then
names the first own image or `NULL`. A post without own images took the
quote's first image as its schedule candidate, so a `schedule-x-<post id>`
asset whose image is one of the moved quoted images is removed with its
review and `schedule-image` link, unless an operator labelled it or set its
period. The step reads only rows without a
projection, so it is idempotent; it never clears a post or another link,
and it increments the dashboard revision when it changes a row. A timeline
page that lists a stored post again leaves it unchanged, so a stored post
without a projection gets its quote from this step; any later detail refresh
rewrites it from the source.

The dashboard reads a bounded current-week/focus selection, not all history.
Event totals and upcoming counts use SQL aggregates over all visible records;
post counts describe the requested date window, even if its preview exceeds
100 posts. Media cache totals aggregate recorded statuses; each displayed asset
is independently checked for file availability. Calendar pages contain at most
100 events in an inclusive range of at most 42 days (a Monday-first month
grid), total/day counts, the snapshot
revision and adjacent dates. Tombstones are excluded on every public path.

Additive indexes on `(source, source_item_id, id)` and locally normalized
`stream_identity(url)` support dashboard video/event lookups. The latter excludes
null URLs and uses the same deterministic YouTube/exact-URL identity rule as the
frontend. Source evidence uses the existing `event_sources` identity index.
Startup adds these indexes to existing snapshots without rewriting events,
manual locks, tombstones, cancellation evidence or operator settings.
