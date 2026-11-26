# Manual Fetches and Historical Backfill

Manual fetches, historical windows, budgets and checkpoints.

The console's Sources page starts manual fetches of X, the related X
accounts, YouTube, the Bilibili uploads and dynamics, and the official shop;
it is the one place to start them. The Posts and Videos pages open on their lists and
link to it from their header. A fetch names one listing method first, then
what the method reads. `POST /api/admin/runs` with
`{ "kind": "fetch", "source": "x", "method": "timeline", "window": { "mode": "count", "count": 50 } }`
starts one; the [Admin API](../admin-api/runs.md#runs) describes the body and its
options.

| Source            | Method     | Needs                | Modes                              | Listing                                                  |
| ----------------- | ---------- | -------------------- | ---------------------------------- | -------------------------------------------------------- |
| X                 | `timeline` |                      | `count` (1–200), `all`             | public FxTwitter timeline, 20 statuses a page            |
| X                 | `api`      | `X_API_BEARER_TOKEN` | `count` (1–200), `range`, `before` | full-archive search, 100 posts a page                    |
| X related         | `timeline` |                      | `incremental`                      | one related account's FxTwitter timeline                 |
| YouTube           | `api`      | `YOUTUBE_API_KEY`    | `count` (1–200), `all`             | uploads playlist, 50 entries and one quota unit a page   |
| YouTube           | `rss`      |                      | `count` (1–15)                     | public feed, its newest 15 uploads in one request        |
| YouTube           | `ytdlp`    | `YTDLP_BINARY`       | `count` (1–200), `all`             | configured yt-dlp upload listing, without media download |
| Bilibili uploads  | `space`    |                      | `count` (1–200), `all`             | the account's upload listing, 20 entries a page          |
| Bilibili dynamics | `space`    | a Bilibili login     | `count` (1–200), `all`             | the account's dynamics listing, about 12 items a page    |
| Shop              | `catalog`  |                      | `all`                              | the whole product catalog, at most 6 pages of 250        |

An X fetch reads every account of `X_HANDLES`, or the one its selection's
`account` names; a one-account fetch takes the whole X request budget and
leaves the rotation that spreads the budget across all-account fetches as
it is. An `x_related` fetch names one related account and is the
[`x_related` step](../sources/x-accounts.md#related-accounts) for that account alone:
its timer's incremental read from the account's cursor, bounded by the
`x_related` schedule's `max`, through the FxTwitter timeline only. It fails
once, without a retry, with the account's failure code, or with
`x_related_account_unknown` when the account was removed while the fetch
waited in the queue.

- `count` reads the newest `count` records per X account, for the channel or
  for the Bilibili account, stored or not, whatever their age.
- The shop's `catalog` fetch is the `shop` step alone, with no option. It
  reads the sale periods that the timer's newest-50 window never listed and
  writes them as every [shop read](../sources/shop.md#official-shop) does; it waits
  out a 429's `Retry-After` like the timer, and keeps no checkpoint.
- `all` pages the listing to its end.
- `range` reads `startDate` to `endDate`, at most 365 days; both dates are
  inclusive in `Asia/Tokyo`, and the internal bounds are start-inclusive and
  end-exclusive.
- `before` reads `days` (1–365) before the oldest persisted post of each
  account, rather than before a UI page's limited list. Accounts without
  posts are skipped and reported while the others backfill; with no posts at
  all, fetch a date range first.

The method IDs are stable, since runs freeze them. `GET /api/admin/sources`
describes each source's methods: their modes, count bound, page size, the variable a method needs
(never its value) and whether it can run now.

A manual fetch never falls back. A method that cannot run now, because its
credential or adapter is missing, is refused with a validation message, and so
is a mode or count the method does not serve. A method that fails fails the
run with its controlled code: unlike a timed source step, the fetch step's
failure is not tolerated, so the steps after it do not run.

The options add steps or change what is written:

- `extract` (X, related-account and Bilibili dynamics methods) appends the `schedule` step. Schedule extraction picks
  its candidates among the stored posts within the extraction lookback by
  its keyword and message rules, unattempted posts first (see
  [Schedule extraction](../sources/extraction.md)), so fetched
  posts that match join that queue whichever run extracts them: this run with
  `extract`, otherwise the X chain's next `schedule` step. Posts the rules do
  not select, and fetched posts published before the lookback, are not
  extracted either way; the lookback is the same for every scan, so a
  backfill cannot hand the appended step more history than its per-run limit
  would ever finish. A `range` with `extract` that ends before the lookback
  is refused. Older posts are extracted by a reprocess request from the
  Posts page, which bypasses the lookback, or after raising the lookback in
  the [sync settings](../../operations/configuration/sync.md#sync-settings).
- `translate` appends the `translate` step.
- `refresh` (YouTube methods) rewrites the videos the listing names that are
  already stored: their title (keeping a revision), publication time and URL.
  The listing leaves their reservation state (kind, upcoming flag, scheduled
  time) and stored thumbnail alone; the `api` method then checks their details
  as it does for new videos (see [YouTube methods](fetch-methods.md#youtube-methods)). Without
  it, stored videos are left as they are.

X fetches only fill gaps: a post that is already stored keeps the text, media
and schedule candidate it has, so extraction fingerprints do not change.
Bilibili fetches write every listed record as the timed reads do, since a
listing describes the whole upload or dynamic, so they take no `refresh`
option either.

Each fetch step reports `listed`, the records the selection took from the
listing, split into `new`, `stored` (already stored, refreshed or not) and
`failed` (listed without usable content); `pages`, the requests it made;
`reachedEnd`, false when the page budget or a failed later page stopped it
before the end of what it asked for, in which case repeating the fetch
continues it (an X timeline `count` fetch instead reads the newest pages
again); and `items`, the first 20 new records as `{id, title, account}`
(an X post's first line of text, cut to 80 characters, or a video's title).
An X result also lists the same counts per account under `accounts`, with
`scheduleAssets` and `discovery` (`timeline` or `api`); an `api` result adds
`apiRead`, the posts its answers returned. A YouTube `api` result
adds `inspected`, the videos whose details it checked, and `apiRequests`, its
Data API calls (quota units). Every YouTube result also adds `members` and
`membersFrom` from the channel's members-only playlist, which a YouTube fetch
reads after its pages as the recent step does (see
[Video availability](../sources/youtube-videos.md#video-availability)). The Sources page shows
the newest fetch of the source under its form.

The run freezes the validated selection (method, mode, its count, days or
dates, and `refresh`) in `workflow_runs.fetch_windows_json`, so a recovered or
retried run never changes it. A run frozen without a method, or with a mode no
method serves, such as a `recent` window or a YouTube date window, reads in the
run history as it was stored; running it again fails its fetch step with
`this fetch uses a retired selection; start a new fetch`.

These requests run the chosen source and media stages, with the steps their
options add, through the single-flight queue; opening a page does not fetch
platforms. They never read or move the incremental cursors, the pending queue
or timeline gaps. Schedule-image candidates on fetched X pages are registered
with each page (see [Schedule images](../sources/schedule-images.md)). Source
schedules, Run now, YouTube pushes and one-off step runs carry no window: they
read with the schedule's frozen sync setting (see
[Source schedules](source-schedules.md#source-schedules)), whatever the credentials. A full sync
or scan carries neither and stays incremental.

## Budgets and checkpoints

The manual fetch pages setting bounds the pages of one fetch per X account,
for the channel or for a Bilibili listing (4 by default, 1–20), for every
paging method. X fetches
share the X status requests per run across accounts as a page budget, including failed
requests: each account gets an even share of what is left, and the first
account rotates across runs per mode (`sync_state` source `x-budget`, key
`window-<mode>`), so a small budget still reaches every account. An account
left without budget reports `X request budget exhausted`. A `before` fetch
skips an account with no stored posts. The step fails (`X fetch failed for
every account`) when no account's fetch answered, so skipped accounts never
turn a failure of every attempted account into a partial result.

Each successful page and its continuation checkpoint commit in one
transaction with the page's posts or videos and schedule candidates. The
checkpoint is a `sync_state` row keyed by account and a hash of the selection
(mode with its count or dates), under the method's own source:
`x-timeline-window` for timeline cursors (an `all` fetch's; a `count` fetch
keeps its read time there without a cursor), `x-window` for X API next tokens,
`youtube-window` for playlist page tokens, `bilibili-window` for upload page
numbers and `bilibili_dynamics-window` for dynamics offsets, each Bilibili
page in one transaction with the account state as well. The RSS method reads
one request and keeps no checkpoint. Repeating an interrupted fetch resumes its next page; a
completed one starts again at the newest page. When a resumed cursor or token
fails before any page of the fetch is committed, the fetch starts again from
the newest page, in the same run if the budget allows. A continuation the
source rejects is cleared from its checkpoint at once, so the next fetch
begins at the newest page even when this one had no page left: any failed
timeline cursor, an X API `next_token` answered with HTTP 400, a
playlist token answered with an HTTP error or rejected as malformed, and a
Bilibili page number or offset whose page failed with anything but a
captcha, the backoff or a login problem. Other
API answers, such as rate limits, quota and outages, keep the stored
continuation. `before` runs finish a pending interval (`pendingWindow`) before
moving farther back, using `x-backfill` search boundaries. A completed empty
interval also moves this boundary backward, preventing an endless retry over
the same empty month. Failures retain all records and the last successful
checkpoint.
