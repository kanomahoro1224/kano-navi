# Posts and External Posts

Reading and editing posts and external posts.

## Posts

| Method and path                  | Body / query                                                                                            | Result                           |
| -------------------------------- | ------------------------------------------------------------------------------------------------------- | -------------------------------- |
| `GET /posts`                     | `page`, `pageSize` (≤ 200), `source`, `account`, `from`, `to`, `search`, `llm`, `queued`, `translation` | list of posts                    |
| `PUT /posts/{id}/hidden`         | `{hidden}`                                                                                              | `{state}`                        |
| `POST /posts/{id}/reprocess`     | `{route?}`                                                                                              | `{state}`                        |
| `POST /posts/{id}/dismiss`       | none                                                                                                    | `{state}`; `409 post_not_failed` |
| `POST /posts/reprocess`          | `{ids}`                                                                                                 | `{queued}`                       |
| `POST /posts/dismiss`            | `{ids}`                                                                                                 | `{dismissed}`                    |
| `POST /posts/refresh`            | `{ids}`                                                                                                 | `{queued}`                       |
| `PUT /posts/{id}/schedule-image` | `{index, reason, start?, end?}`                                                                         | `{image}`                        |
| `PUT /posts/{id}/quote/hidden`   | `{hidden}`                                                                                              | `{state}`                        |
| `POST /posts/{id}/refresh`       | none                                                                                                    | `{state}`                        |

Each post reports `promotedAt`, when a related account's status was promoted
into posts, or null for an own post; External posts holds its decisions.
`upstreamDeletedAt` is when synchronization found the X status
[deleted on X](../data-model/posts.md#posts-deleted-upstream), or null.
`account` is an X handle, matched without case or a leading `@`; `llm` is `never`, `pending`, `processed`, `failed`,
`dismissed`, `skipped` or `uncertain`; `queued` keeps the posts with an open
operator request: `extraction` (queued for re-extraction), `refresh` (a
refresh from the source), `translation` (a machine translation) or `any` of
them;
`translation` is `incomplete`, `complete`, `requested` or `failed` (the
machine pass of the current wording failed and a locale still lacks a usable
translation).

`from` and `to` are inclusive Asia/Tokyo publication dates. Search matches
text, URL, image alt text and the stored post ID before pagination, so older
matching posts remain reachable. The console preserves these filters while
paging and resets to page one when a filter changes.

A post whose extraction keeps failing carries `llmRetryAt`, the earliest time
the automatic scan tries it again, or `llmRetryStopped: true` once the scan
no longer retries its current input (see
[Failure backoff](../sources/extraction.md#extraction-failure-backoff)). Both stay
`null`/`false` for other posts and for a post with a queued reprocess request,
which runs regardless. `POST /posts/{id}/reprocess` clears the backoff.
`POST /posts/{id}/dismiss` turns a `failed` extraction into `dismissed` and
stops its automatic retries (see
[Failure backoff](../sources/extraction.md#extraction-failure-backoff)); any other state
answers `409 post_not_failed`.

`POST /posts/reprocess`, `POST /posts/dismiss` and `POST /posts/refresh`
apply the same changes as their single-post routes to 1 to 200 listed posts
in one transaction. They skip unknown IDs and, for a dismissal, posts whose
extraction has not failed, for a refresh, posts that cannot be refreshed,
and answer how many posts they queued or dismissed; a reprocess keeps each
stored route, and a post whose refresh is already queued counts as queued
and keeps its place.

`scheduleImage` is the post's schedule image record, or `null`: its asset
`id`, the `index` of the post's own image it shows (`null` for none of them),
`llmStatus`, `manualStatus` and `picked`. `PUT /posts/{id}/schedule-image`
makes the X post's image at `index` (in `media` order) its schedule image,
labelled `schedule` with the required `reason` (see
[Schedule images](../sources/schedule-images.md)). `start` and `end` set its
manual period and come together; without them the stored manual period stays.
An unknown or non-X post answers `404 post_not_found`, and an index without
an image `400`.

An X post carries `translation` (see [Translations](models.md#translations)); other
posts carry `null`. `GET /videos` does the same for YouTube videos.

Each post's `media` lists its images in order (linked post images, or else
the stored media URL) as `{url, status, alt}`. `url` is the local
`/media/<id>?v=<sha256>` URL of the cached file, or `null` until that file is
ready; the remote address stays in `mediaUrl` and is never shown. `status` is
`ready`, `pending`, `failed` or `missing`. `mediaStatus` repeats the first
image's status, or is `none` for a text-only post.

Each post also carries `quote`, the stored quoted status read-only (see
[Quoted posts](../data-model/posts.md#quoted-posts)), or `null`:
`{id, authorHandle, authorName, text, publishedAt, url, unavailable, media}`.
A Bilibili forward's quote is the dynamic it forwards, with its author's mid
as `authorHandle` and an empty `id` when the listing did not name it.
Its `media` has the `{url, status, alt}` shape but lists only ready cached
quote images, never the post's own. `quoteHiddenAt` is when an operator hid
the quote from the public pages, `hiddenAt` when an operator hid the whole
post, and `refreshRequestedAt` is a queued refresh from the source; each is
`null` otherwise. The console never edits
the quoted wording, author or images.

`PUT /posts/{id}/hidden` with `{"hidden": true}` hides the whole post from the
dashboard, the post archive and MCP; `false` shows it again. A hidden post is
never selected for extraction, and an operator's reprocess request for it
waits until it is shown again (the extraction backlog does not count it). The
automatic events it alone supports leave the public pages while it is hidden
(see [Events of hidden posts](../data-model/posts.md#events-of-hidden-posts)). It
follows the quote rules below, except that any stored post can be hidden.

`PUT /posts/{id}/quote/hidden` with `{"hidden": true}` hides the post's
quote from the dashboard, the post archive and MCP; `false` shows it again.
`hidden` must be a JSON boolean. Hiding again keeps the first decision's
time. A post without a stored quote answers `404 quote_not_found`.
Synchronization never writes the decision, so it outlives every rewrite of
the stored quote. A change increments the dashboard revision.

`POST /posts/{id}/refresh` queues an own X post for a detail request in
the next X synchronization and removes its pending failure backoff; it
contacts no source itself. A repeated request keeps the first request's
place. Posts that are not own X statuses with a known account, promoted
statuses included, answer `400 invalid_input`. See
[Refresh from the source](../sources/quoted-posts.md#refresh-from-the-source). The three
routes answer `{state: {postId, hiddenAt, quoteHiddenAt, refreshRequestedAt}}`;
MCP control has none of them.

## External posts

| Method and path                 | Body / query                                                      | Result           |
| ------------------------------- | ----------------------------------------------------------------- | ---------------- |
| `GET /x-statuses`               | `page`, `pageSize` (≤ 100), `author`, `reason`, `state`, `search` | list of statuses |
| `PUT /x-statuses/{id}/decision` | `{decision, reason}`                                              | `{status}`       |
| `GET /bilibili-statuses`        | `page`, `pageSize` (≤ 100), `author`, `search`, `kind`            | list of dynamics |

The list pages the X status pool, newest status first (see
[Related accounts](../sources/x-accounts.md#related-accounts)), without the statuses of
the own accounts (`X_HANDLES`) or any own post the pool holds as a quote;
those belong to Posts. `author` is a handle,
matched without case or a leading `@`; `reason` is `keyword`, `manual`,
`quoted`, `reposted` or `none`; `state` is `promoted` (a shown promoted
post), `demoted` (a hidden one) or `stored` (pool only); `search` matches
the text or the status ID. Each
status is `{id, authorHandle, authorName, text, publishedAt, url, images,
reasons, state, related, decision, decisionReason, decidedAt, quotedAt,
repostedAt, listedAt, firstSeenAt, quotedBy, repostedBy, translation}`:
`images` counts its pooled image addresses, `related` says its author is a
related account, `quotedBy` lists up to five own posts quoting it, newest
first, `repostedBy` the own accounts that reposted it, and `translation` is
its `x_status` translation summary (see [Translations](models.md#translations)), or
null for a status whose post owns the translations.

`decision` is `promote` or `demote`, which need a `reason` of at most 500
characters and become a durable lock, or `automatic`, which removes the lock
so the keyword rules decide again. A promote needs a related account's
status or a reposted one (otherwise `400 invalid_input`), writes or shows
its post and answers the status; a demote hides the promoted post. An own post, or a promote of an own account's status, answers `400 invalid_input`, a missing status
`404 not_found`. A change moves the dashboard revision. Translations of a
pooled status use the [translation routes](models.md#translations) with owner
`x_status`.

`GET /bilibili-statuses` pages the Bilibili status pool (see
[Forwards](../sources/bilibili-dynamics.md#forwards)), newest dynamic first, without a dynamic
that is a stored post (a forward of an own dynamic), whose translations
belong to Posts. `author` is an exact mid or part of the author's name,
matched without case; `search` matches the text or the dynamic or post ID;
`kind` is `dynamic` or `video` (an upload, whose URL is its video page), and
any other value answers `400 invalid_input`. Each dynamic is `{id,
authorMid, authorName, text, publishedAt, url, kind, images, reasons,
forwardedAt, firstSeenAt, forwardedBy, shown, translation}`: `id` is its
post ID, `images` counts its pooled image addresses, `reasons` is
`["forwarded"]`, `forwardedBy` lists up to five own posts forwarding it,
newest first, as `{postId, hidden, quoteHidden, quoteAvailable}`, and
`translation` is its `bilibili_status` translation summary. `shown` is
`public` when a shown forward shows it with its quote shown, `hidden` when
every forward that shows it available is hidden or hides its quote, and
`gone` when no forward shows it available. The pool has no decision route:
a quoted status appears inside its quoting post and never as a post of its
own ([related accounts](../sources/x-accounts.md#related-accounts)),
and the forward's quote already shows the dynamic. Its translations use the
translation routes with owner `bilibili_status`.
