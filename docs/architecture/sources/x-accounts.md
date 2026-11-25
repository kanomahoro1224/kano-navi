# X Related Accounts and Account Exports

Which related X accounts are read and how imported account exports enter the post pool.

## Related-account rationale

Related accounts stay in a private status pool because their timelines contain announcements for many people. Keyword and operator promotion put only relevant statuses on the board, while quoted and reposted statuses can supply context and translation without becoming public cards. Separate cursors and a separate timer keep this filtering independent from Kano's own accounts.

## Related accounts

A related account (such as the agency, `@Mil_Pro_`) is a second account
role beside the own accounts of `X_HANDLES`, configured on the console's
Sources page beside its timer, with its keyword rules in the app setting
`x_related_accounts` (see [Configuration](../../operations/configuration/sync.md#related-x-accounts)
and [Related accounts](#related-accounts)).
A handle is in one role at a time, and the setting starts empty.

The related accounts have their own timer and step, `x_related`
([Source schedules](../workflows/source-schedules.md#source-schedules), every 180 minutes by
default), apart from the own accounts' `x`. Each run reads each related
account's own timeline into the X status pool (`x_statuses`, see
[Quoted posts](quoted-posts.md)):

- It reads the FxTwitter timeline alone, at most the schedule's incremental
  `max` (1–200, default 50) new statuses per account, else the X discovery
  limit, and on a first read back the run's lookback days, else the X
  first-run age. It has no `count` setting, no profile-page fallback and no
  X API method, and it makes no detail request, since a page carries each
  status; a status without usable content is left out. Its reposts and other
  authors' statuses are left out too.
- The incremental cursor and gap model follows the own-account rules, with its
  own `sync_state` row of source `x-related` (the lower-case handle as account): the cursor, the timeline
  gap and the known statuses come from the pool's statuses that the
  account's timeline listed (`listed_at`), never from `posts`, so a role
  change never mixes cursors.
- A failed read keeps the cursor and the pool and records its code in the
  row's `lastError`. While FxTwitter is down, related accounts are not read;
  a timeline gap covers the stretch.

The step result is `{source: "x_related", accounts, listed, promoted,
demoted}`. Without a related account the step records `{skipped: true,
reason: "no_accounts"}` and makes no request. An account whose read failed,
and a read's notes such as a timeline gap, leave the step `partial` with
`errors` as `@<handle>: <code>`; when every account failed the step fails
with `x_related_failed`. An operator's
[manual fetch](../workflows/manual-fetches.md) of one
related account runs the same read for that account alone and fails once
with its code. The own accounts' read and the related accounts'
read never decide each other's outcome, and quotes and reposts of the own
accounts write the pool in the `x` step whether or not `x_related` runs. Both
reads extract normalized `author.avatar_url` values (`_400x400`) into
`account_avatars` and queue new URLs into `media_assets` without delaying post
ingestion.

Every pooled status stores the reasons that apply to it:

| Reason     | When                                                                    | Effect              |
| ---------- | ----------------------------------------------------------------------- | ------------------- |
| `keyword`  | a related account's status whose text contains one of its keyword rules | promoted            |
| `manual`   | an operator promoted a related account's status or a reposted status    | promoted            |
| `quoted`   | an own account quoted it, whatever its author                           | translated, no card |
| `reposted` | an own account reposted it, whatever its author                         | see below, no card  |
| `none`     | none of the above                                                       | stored only         |

A reposted status of a related account is translated; any other author's is
stored untranslated until an operator promotes it (see [Reposts](quoted-posts.md#reposts)).
Keyword rules apply to related accounts' statuses alone and match anywhere
in the text, ignoring letter case. Reasons are
evaluated when a related timeline lists a status, when an own account
reposts it, when an operator decides on it, and for every pooled status when
the related accounts or their rules are saved.

**Promotion** writes the status from its pooled `raw_json` to `posts` under
its author's handle, with `posts.promoted_at` set. It registers the status's
own images and quoted images as any post does, but never a schedule-image
candidate, and gives the status it quotes no `quoted` reason. From then on
the post takes the paths every post takes: the wall, the archive,
translation, extraction and media. **Demotion** keeps the row, its media
and its events and writes nothing on the post: public reads leave a demoted
post out, and with it the automatic events it alone supports (see
[Post visibility](../data-model/posts.md#post-visibility)). Without an
operator's decision the rules record their demotion in
`x_statuses.demoted_at`, which a later match clears. An operator's promote
or demote (`manual_decision`, with its reason and time) is a durable lock
that the rules never override; returning a status to the rules lets them
decide again. The operator's show or hide of the post in Posts lies above
both and changes neither, and a promote or demote withdraws an operator's
hide or show of the promoted post, so the decision takes effect. A related
account's status and any reposted status can be promoted by hand. Returned
to the rules, a status they do not promote (any but a related account's
status that matches its keywords) stays unpromoted, or demoted when it was
promoted. A pooled status that is an own post, or whose author is an own
account (a configured handle, or one with own posts or reposts stored), is
never promoted. When an account becomes an own account, its promoted posts
become own posts, and an operator's demote of one becomes their hide of the
post.

## Account exports

An account export is the JSON array a browser exporter writes for an X
profile's posts (`<handle>.json`), with its photos in `<handle>-media.zip`
(`<handle>_<status>_photo_<n>_<date>.<ext>`). It lists the account's current
top-level posts only: no replies, thread replies, reposts or deleted posts.
`kano-go x-archive import` reads it locally and makes no request
(`ParseArchive` in `backend/internal/source/x/archive.go`): of each own
record whose `in_reply_to` and `retweeted_status` are empty, it builds a
detail-shaped payload from X's status record in `metadata` (the wording with
t.co links expanded and media links and the quoted status's link removed, a
note post's full wording, the photos and video or GIF thumbnails with their
kinds, the counts, and the quoted status from `quoted_status_result`) and
normalizes it like a detail answer. All exports of a run are written in one
transaction (`ImportXArchives`):

- A post synchronization stored keeps its record. A post an earlier
  third-party import or an earlier export import wrote takes the export's
  record, and moves to the export's account when the earlier import filed it
  under another one (a repost written as the reposter's post). Missing posts
  are added. Writes go through the own-post path, so a known repost ID is
  never stored and the operator's decisions stay. Every post the export
  lists is `top_level`, a synchronized one included unless an answer told
  its kind.
- After every account's posts, the earlier third-party import's posts of each
  account that its export does not list are marked `unlisted` with their
  reason (see
  [Posts from account exports](../data-model/posts.md#posts-from-account-exports)),
  which keeps them off the public pages; `-keep-unlisted` leaves them
  unmarked. A later export that lists a post an earlier run marked shows it
  again.
- An account that is not in `X_HANDLES` joins `x_archive_accounts`: its posts
  appear in the post archive under its own filter, never on the board, and
  nothing synchronizes it.
- After the commit, the media archive's photos of the written posts are
  stored as their pending assets (`SaveXArchiveImage`, with the media step's
  type and size checks); video and GIF thumbnails, and photos the archive
  lacks, wait for the [media step](../media-cache.md).

`-dry-run` counts the same work in a transaction that is rolled back and
stores no photo.

The unlisted posts mix replies, thread replies, other authors' statuses and
deleted posts. `kano-go x-archive check`, run against a copy of the database,
requests each own post whose kind is still `unlisted` and that X has not been
found to have lost, whatever the import's reason, from the vxtwitter detail
host with the X client's bounds, and appends one JSON line per post to a
decisions file (`ClassifyDetail`, which reads the answer as
[Post kinds](x.md#post-kinds) does): another author is `foreign`, the
account's retweet `repost`, a `replyingTo` of the account `self_reply`, of
anyone else `reply`, none `top_level`, and a 404, 410 or vxtwitter's "Failed
to scan your link" page `gone`. With `-capture`, the
third-party import's own capture (`responses/posts/**/*.json`) decides
without a request every post it marks with a `repost` object: those are
other authors' statuses, `foreign`. That page also stands in
for vxtwitter's own failures, so `gone` lines wait for the next readable
answer; after 30 in a row the run reads the newest shown post an export
listed, keeps them and goes on when that post reads, and otherwise stops
without them. Requests are paced
around an interval (6 seconds by default, spread 70-130%) that shortens by a
tenth after 25 answers in a row, never below its floor (4 seconds), and
doubles after a failure up to its ceiling (10 minutes), waiting at least a
minute after a rate limit. A post that fails three times is left for the next
run; eight failures in a row stop it. A rerun skips the posts the file
already decides. `kano-go x-archive apply` records each line's outcome on an
own post that is `unlisted` or carries the import's record
(`ApplyXReconcile`): the outcome becomes its kind, so an own top-level post or
thread reply shows again and every other outcome stays off the public pages
for an operator to judge, and `gone` marks the post deleted upstream at the
line's `checkedAt`. The operator's own hide or show stays. [Database operations](../../operations/database.md#x-account-exports)
gives the steps.
