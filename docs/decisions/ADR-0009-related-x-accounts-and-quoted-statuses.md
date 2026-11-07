# ADR-0009: Related X Accounts and Stored Quoted Statuses

- Status: Accepted
- Date: 2026-11-07
- Builds on: [ADR-0002](ADR-0002-count-based-source-sync.md) (its cursor,
  cap and gap model serves a second account role) and
  [ADR-0004](ADR-0004-method-fallback-chains.md) (related accounts stay
  outside the X method chain)

## Context

The board reads the X accounts named by `X_HANDLES` (the `x_accounts`
setting) from the public FxTwitter timeline. Those are Kano's own accounts,
and the account list does more than choose what to read: it drives the post
wall's tabs, the dashboard's newest posts per account, schedule-image
candidates at ingest, keyword extraction and media download (see
[Sources: X](../architecture/sources.md#x)).

Two kinds of status reach Kano's timeline without being her posts:

- **Reposts.** The timeline marks a repost with `reposted_by`, but its `id`
  and `created_at` are the original's; there is no repost ID or repost time.
  Reposts carry another author and are skipped.
- **Quotes.** A quoted status is stored only as the sanitized
  [projection](../architecture/data-model.md#quoted-posts) in
  `posts.quote_json` and shown as a card inside the quoting post, as
  written. It has no [translation](../architecture/data-model.md#translations)
  of its own unless its ID is also a stored post, which only a self-quote is,
  and extraction never reads it.

Kano's agency, ミリプロ (`@Mil_Pro_`), announces her appearances, goods and
events. Read-only probes on 2026-11-07 showed:

- `@Mil_Pro_` posted 20 statuses in about six days, half with media; none of
  the 20 mentions Kano.
- Kano's newest 20 statuses held 2 reposts and 5 quotes. One repost was a
  `@Mil_Pro_` announcement with no keyword in it.
- A timeline `quote` object carries the quoted status's full text and media.

Reading the agency's timeline into `posts` would flood the board, the
extraction queue and the media cache with statuses about other talents.
Kano's own posts often say little more than "これ！" over a quoted
announcement, so their schedules are in wording that extraction does not
see, and visitors who do not read Japanese see that wording untranslated.

Hiding a post (`posts.hidden_at`) keeps it off the public pages, but
extraction still reads it.

## Decision

### Account roles

`X_HANDLES` and `x_accounts` keep their meaning: Kano's own accounts, with
every behavior listed above. A **related account** is a separate role,
configured on the console's Sources page beside the X API limits (app
setting `x_related_accounts`, at most 10 accounts with up to 30 keyword
rules each), with its keyword rules. Its own timeline is read in full on
each run of the X step, after the own accounts and whatever the X schedule's
sync setting, `count` included. Its reposts, other authors' statuses and
statuses without usable content are skipped. A handle is in one role at a
time: a related account cannot be one of `X_HANDLES`.

### A raw pool: `x_statuses`

A new table, `x_statuses`, holds every status of a related account, every
status an own account quotes, whatever its author, and a related account's
status an own account reposts: the status ID, author handle and display
name, text, publication time, the validated x.com URL and a sanitized
`raw_json` (with `mediaURLs`, so a later promotion needs no new request). It
also holds the relevance state below: `quoted_at` (the newest quoting own
post's publication time), `reposted_at`, `listed_at` (when the author's own
timeline listed it), `keyword`, the operator's `manual_decision` with
`manual_reason` and `manual_at`, and `demoted_at`.

No public path reads `x_statuses`, and none joins it. The dashboard, archive,
public API and MCP read `posts`, `quote_json` and `translations`; a status
reaches them only by promotion or as a translated quote (below), so the pool
fails closed.

A related account's read keeps the [count-based model](ADR-0002-count-based-source-sync.md)
unchanged. Its cursor, known IDs and gap ceilings come from the
`x_statuses` rows its own timeline listed (`listed_at`), never from `posts`
or from statuses held only as quotes, in its own `sync_state` rows (source
`x-related`, keyed by the lower-case handle), so a role change never mixes
cursors. The incremental `max`,
the page budget and timeline gaps work as for an own account. Timeline pages
carry each status, so the read makes no detail request. A quoted status is
written to the pool in the same transaction as the quoting post.

No media of a pool status is registered or downloaded until it is promoted,
because runtime media is never garbage-collected
([Media cache](../architecture/media-cache.md)). Quoted images keep their
`quote-image` links on the quoting post, as today.

### Reasons, processing and promotion

Each pool status stores every reason that applies to it:

| Reason     | When                                                                    | Effect              |
| ---------- | ----------------------------------------------------------------------- | ------------------- |
| `keyword`  | a related account's status whose text contains one of its keyword rules | promoted            |
| `manual`   | an operator promoted a related account's status                         | promoted            |
| `quoted`   | an own account quoted it, whatever its author                           | translated, no card |
| `reposted` | an own account reposted it, and its author is a related account         | translated, no card |
| `none`     | none of the above, or an operator demoted it                            | stored only         |

The keyword rules default to `鹿乃`, `@kano_2525` and `#鹿乃まほろ` for a
newly added account and match anywhere in the text, ignoring letter case.
Reasons are evaluated on ingest, when an own account reposts a stored
status, when an operator decides on one, and for every pooled status when
the accounts or rules are saved. A later quote records `quoted_at`, which
needs no evaluation, since `quoted` never promotes.

**Promotion** writes the status to `posts` as an X post whose
`account_handle` is its author (for example `Mil_Pro_`), marked as promoted
(additive column `posts.promoted_at`) so readers tell it from an own post
without comparing handles to the current account list. It needs the
status's publication time, which timeline pages and quotes carry. It is the only
way a pool status gets a card of its own; only `keyword` and `manual`
promote. A promoted post reaches the board, translation, extraction and
media through the paths every post takes. Its images are registered from
the pool's `raw_json` at promotion. A promoted post never registers a
schedule-image candidate, and its own quote gives the quoted status no
`quoted` reason. The board shows a promoted post with its account's initial
in place of Kano's portrait, and public posts report it as `promoted`.

**Demotion** sets `posts.hidden_at` on the promoted post and keeps its row,
its media and its events. An automatic demotion also records
`x_statuses.demoted_at`, so an automatic promotion shows again only a post
the rules hid, and an operator's hide in Posts stays. A manual promote or
demote is a durable lock, with a reason, that automatic re-evaluation never
overrides, like the
[manual event locks](../architecture/data-model.md#event-precedence); only a
related account's status can be promoted by hand. A manual promote of a
demoted post clears its `hidden_at`, and returning a status to the rules
(`automatic`) removes the lock.

**Quoted promoted posts.** The post wall and the archive leave out a promoted
post that a visible own post quotes with its quote shown, because the quote
card already shows it. This is a display rule computed at read time: it sets
no lock and removes nothing, and the card returns when the quoting post or
its quote is hidden.

### Quoted and reposted statuses

- A quoted status stays in `x_statuses` and is never promoted for being
  quoted. A quote whose ID is a stored post (a self-quote, or a related
  status promoted by keyword or by hand) reuses that post's row and
  translations, as the read-time projection does today.
- `posts.quote_json` stays the public projection, and `quote_hidden_at`
  keeps its meaning.
- Startup fills the pool from the quotes already stored in posts'
  `raw_json`, as the quote backfill does, with no request, so existing
  quotes within the translation lookback are translated too.
- Every other quoted status, and every reposted status of a related account,
  is translated through the generic `translations` and `translation_states`
  tables with `owner_type = 'x_status'` and field `text`, keyed by the status
  ID, so a status quoted or reposted several times is translated once. The
  pool text and the quote projection come from the same mapping of the same
  payload, so the board's quote card resolves its `translations` and
  `sourceLanguage` from `quote_json` and these rows, comparing the source hash
  with the projection's text, without reading the pool. A status its related
  author's timeline listed keeps that listing's wording, which the trimmed
  source hash matches. The usual manual, platform, machine precedence
  applies.
- The translate step reads its `x_status` candidates from the pool: statuses
  with a `quoted` or `reposted` reason and no `posts` row. They share its
  per-run budget and lookback (by the quoting post's publication time, or
  when an own repost was first seen, since a repost has no time of its own),
  after posts and videos; operator requests still go first.
- Quoted images stay non-candidates for schedule images.

### Extraction

- **Quoted text as context.** Extraction gives the model the quoted status's
  text from the quoting post's `quote_json` as context, marked as quoted
  wording, because a quoted status reaches extraction no other way. The
  quoted text also counts toward the single-message heuristic and the
  reading of an entry's platform, so a post that only points at a quoted
  announcement is selected; the board path's keywords read the post's own
  wording. An unavailable quote adds nothing. The quoted text joins the post's extraction
  input and fingerprint, so posts with quotes extract once more within the
  lookback; event locks and tombstones protect manual edits. Quoted images
  stay outside the input, as they stay non-candidates.
- **Hidden posts are skipped.** Extraction selects no post with
  `posts.hidden_at` set: a demoted promoted post and every hidden own post
  alike. Unhiding a post makes it a candidate again.

- **A hidden quote gives no context.** Extraction gives the quoted text only
  while the public pages show it: a post with `posts.quote_hidden_at` set is
  extracted from its own wording alone. Hiding or showing a quote changes the
  extraction input, so that post extracts once more.
- **Events of hidden posts are hidden.** An event whose every source post is
  hidden is left out of the public pages (board, calendar, archive, public
  API and MCP) and is listed in the console as hidden by its posts. This is a
  visibility rule, not a deletion or tombstone: the row stays, and unhiding a
  source post shows the event again. It applies to automatic events only. An
  event an operator edited, confirmed or added (`manual_locked`, or manual
  provenance) is never hidden by its posts, since the operator's decision
  outranks hiding. An event with any visible source, such as an X notice
  merged into a YouTube event, stays shown. The active evidence of the
  events merged into an event counts as its own, and an event with no
  active evidence at all stays shown. A video's linked event follows the
  same rule. A reprocess request for a hidden post waits until it is shown
  again.

### Pure reposts

An own account's repost records a link (table `x_reposts`: reposted status
ID, reposting account, the status's author when known, first-seen time). A reposted status of a
related account enters the pool from the repost item if its own timeline has
not stored it yet, and is translated by the `reposted` reason. A repost of
any other author records only the link. Timeline pages mark a repost with
`reposted_by`, and X API answers with a `retweeted` reference; the timed X
API read asks for posts without retweets, since each returned post is
billed, so it records none. The board shows no reposts; the link
and the translation are kept for a later repost display, whose time would be
the original's or an estimate bounded by the neighbouring own statuses on
the page where the repost first appeared.

### Console

A new view in the `data` group of `src/admin/index.tsx`, External posts,
lists the pool: author, time, text, reasons and state, filtered by author,
reason, state and text. Its actions are a manual promote or demote (the
lock above), editing or requesting a quoted or reposted status's
translation through the translation routes with owner `x_status`, and
links to the quoting posts in Posts and the accounts that reposted it.
Related account configuration stays on the Sources page and promoted posts
are managed in Posts, which marks them. The view has no review badge. The
admin API adds `GET`/`PUT /sources/x/related`, `GET /x-statuses` and
`PUT /x-statuses/{id}/decision`.

### Cost and outbound requests

- A related account is read through the FxTwitter timeline only. It has no
  profile-page fallback, because the profile page lists IDs only and
  relevance would need a detail request per status, and it stays outside the
  X API in the [method chain](../architecture/workflows.md#method-chains):
  `GET /2/users/{id}/tweets` bills every returned post, about 100 a month for
  `@Mil_Pro_`, against the shared daily cap. An FxTwitter outage stops
  related reads until it ends; a timeline gap covers the stretch.
- **Unverified alternative:** recent search with `from:<handle>` and the
  keyword rules bills only matching posts, but covers seven days and misses
  reposted statuses without a keyword.
- The reads use `api.fxtwitter.com`, which the X client already reaches with
  its timeout, response limit, redirect policy and typed errors. No new host
  is added.
- Model cost covers promoted statuses, quoted and reposted statuses, and the
  one extra extraction of posts with quotes: a few calls a week.

## Schema Changes

All additive:

- the `x_statuses` table (with an index on the author) and the `x_reposts`
  table;
- `posts.promoted_at`, `NULL` for own posts;
- `owner_type = 'x_status'` in `translations` and `translation_states`;
- `sync_state` rows of source `x-related`;
- the `x_related_accounts` setting, empty on fresh initialization and left as
  it is by seeding.

No column is dropped and no snapshot is rewritten.

## Consequences

- These consumers stay unaware of `x_statuses` and see only `posts`,
  `quote_json` and `translations`:
  - the dashboard (post wall, latest post, per-account newest posts, counts);
  - the post archive;
  - the public HTTP API;
  - MCP reads and control;
  - extraction, which takes quoted text from `quote_json`;
  - schedule-image registration and the image check;
  - media registration and download.

  The translate step reads the pool only for its `x_status` candidates and
  never treats a pool status as a post.

- Promoted posts change what account-keyed readers see. The dashboard's
  latest post, the per-account newest posts and the post-wall tabs read own
  accounts only, so a promoted post appears in the combined wall and archive
  and never becomes the profile's latest post or a tab of its own.
- Hiding a post changes meaning for every post, Kano's own included: it also
  stops extraction. The current docs that state or imply otherwise are
  rewritten with the implementation:
  [Data model: Quoted posts](../architecture/data-model.md#quoted-posts)
  ("extraction still reads it"), the post-hiding route in
  [Admin API](../architecture/admin-api.md) (which lists what hiding
  affects), and the candidate rules of the
  [Schedule extraction pipeline](../architecture/sources.md#schedule-extraction-pipeline).
- Hiding a post also hides the automatic events it alone supports, so every
  public event read (board, calendar, archive, public API, MCP schedule
  reads) applies the visibility rule, and the console shows why an event is
  hidden. To keep a post's wording off the board while its event stays
  shown, an operator confirms the event, which locks it.
- The pool keeps every related-account status. It is text only and grows by
  about a hundred rows a month per agency-sized account, which is not
  pruned, matching the retention of other snapshots.
- A pool status that a later own status quotes or reposts is translated then;
  nothing is fetched again.

## Alternatives Considered

- **Adding the agency to `X_HANDLES`.** Every status would become a post,
  extraction candidate and cached image, and the account would gain a tab
  and a latest-post slot.
- **Promoting quoted and reposted statuses as cards.** A quoted status would
  show twice, as its own card and as the quote card inside Kano's post; a
  reposted one would show as a card the board otherwise never shows for a
  repost.
- **Filtering at ingest and storing only matches.** Rule changes, and
  statuses Kano quotes or reposts later, could not be re-evaluated without
  reading the timeline again, which may no longer reach them.
- **Translating quotes inside `quote_json`.** Each quoting post would hold its
  own copy, and a status quoted twice would be translated twice.
- **Removing a demoted post's row.** Its automatic events and media links
  would go with it, and a later manual promote would have to rebuild them;
  hiding keeps them and is undone by one write.
- **The X API for related accounts.** Discussed above; it bills unrelated
  statuses.

## Related Docs

- [Sources: X](../architecture/sources.md#x)
- [Sources: Quoted posts](../architecture/sources.md#quoted-posts)
- [Sources: Translation](../architecture/sources.md#translation)
- [Data model: Translations](../architecture/data-model.md#translations)
