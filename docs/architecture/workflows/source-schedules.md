# Source Schedules and Method Chains

Source schedules, read modes and method chains.

## Why source schedules and read modes are separate

Each source has its own cadence because X, YouTube, Bilibili, related X accounts, and the shop have different freshness, quota, and access constraints. The single-flight queue remains shared so source writes are serialized and one process owns the timers. A schedule's incremental cap limits newly discovered records, while count mode intentionally re-reads a bounded newest window; timeline gaps preserve continuity when a capped or interrupted read cannot reach its cursor.

Listing method chains are configurable for X, YouTube, both Bilibili timers and the shop. The Bilibili dynamics chain uses the desktop feed before its WBI fallback; risk-control and login failures stop that chain immediately. The chain only covers listing methods. Detail and helper requests keep their own bounded behavior so a failed later page does not discard records already read.

## Source schedules

Each source has its own timer
([source schedules](#source-schedules)), stored in
the additive `source_schedules` table and edited on the console's Sources
page. A fresh installation keeps all six disabled.

| Source              | Interval (default, bounds)                        | Fixed chain                                           |
| ------------------- | ------------------------------------------------- | ----------------------------------------------------- |
| `x`                 | 30 min, 15 min – 7 days                           | `x`, `media`, `schedule`, `translate`                 |
| `x_related`         | 180 min, 15 min – 7 days                          | `x_related`, `media`, `schedule`, `translate`         |
| `youtube`           | 60 min, 15 min – 7 days                           | `youtube`, `media`, `translate`                       |
| `bilibili`          | 180 min, `BILIBILI_MIN_INTERVAL_MINUTES` – 7 days | `bilibili`, `media`, `translate`                      |
| `bilibili_dynamics` | 180 min, `BILIBILI_MIN_INTERVAL_MINUTES` – 7 days | `bilibili_dynamics`, `media`, `schedule`, `translate` |
| `shop`              | 360 min, 180 min – 7 days                         | `shop`                                                |

X has two timers because its two reads serve different purposes: `x`
catches Kano's own posts quickly, while `x_related` reads the
[related accounts](../sources/x-accounts.md#related-accounts), whose announcements land
days ahead of the event, at a slower cadence. Both reach FxTwitter, so the
single-flight queue keeps their runs apart, and a failure of one leaves the
other reading. The related accounts' chain carries `media`, `schedule` and
`translate` so a status the read promotes is processed in the same run.

Bilibili has two timers because its two reads serve different purposes:
the uploads are an archive, like YouTube's, while the dynamics announce
schedules, like X posts, and need the login cookie. Each timer has its own
interval, runs and Run now; both share the client, device identity and
captcha backoff, so a captcha met by either read holds both, and their
requests to the same address add up. The
[official shop](../sources/shop.md#official-shop) writes its sale periods itself, so
its chain has no media, extraction or translation step; its listing changes
a few times a month, hence the long default.

When a schedule is due, the server process submits its chain as the ad hoc
run `@source-<source>` (`reconcile` is appended as for every run with a
source step), labelled `source-schedule` in the run history. **Start now**
on the console overview's queue, and **Run now** in a source's Sources page
summary while its timer runs, start the timer
(`POST /api/admin/sources/{source}/schedule/start`): the schedule becomes due
at once, so the timer itself submits its chain, immediately when the queue
is free and otherwise after the current run and the schedules due before
it. While the timer is off or timers do not run in the process, Run now
(`POST /api/admin/sources/{source}/run`) submits the same chain once,
triggered by the operator, and leaves the schedule's next run where it was;
the history shows it as "X · run now". The follow-up
steps always run, even when the source wrote nothing: each works from its
own queue and makes no provider call while that queue is empty, so failed
extractions, pending image reviews, download retries and operator
translation requests keep moving while a source is quiet. YouTube pushes
submit the `youtube` chain as well (labelled `websub`). The operator's
single-video refresh keeps its own steps, and the
[live-status probe](steps-and-timers.md#live-status-probe) has its own timer. X has no push, so its
timer is how new posts arrive; YouTube push and the probe catch most
YouTube changes and its timer is a fallback; neither Bilibili interval can
go below the environment floor, and the adapter's own interval and backoff
checks stay as the safety net.

The X, related-accounts and YouTube schedules carry a sync setting that
says how much a run reads
([source schedules](#source-schedules)). Each run
freezes it in `workflow_runs.source_sync_json`:

- `{ "mode": "incremental", "max": 50 }`, the default: read newest first until
  a stored record, at most `max` (1–200) records that are not stored yet, per
  X account, per related account or for the YouTube channel. An X run that
  stops short leaves a
  [timeline gap](../sources/x.md) that later runs read; a YouTube run that
  leaves new uploads above its cursor keeps the cursor, and later runs take
  them ([YouTube](../sources/youtube.md)).
- `{ "mode": "count", "count": N }`: read the newest `N` (1–200) every run,
  stored or not. X reads them as a `count` window from the timeline's newest
  page, which leaves the incremental cursor and pending queue alone; when no
  account answers, the step fails with `x_source_failed`, retryable like an
  incremental read. YouTube reads the
  uploads listing. The related accounts read incrementally only, so their
  schedule refuses a `count` setting.

Every other run with an X, related-accounts or YouTube step reads that
source with these
settings too, frozen on the run: a YouTube push, Run now, a
[one-off step run](steps-and-timers.md) and the operator's single-video refresh.
A [manual fetch](manual-fetches.md) reads through its
method instead. Full syncs (console, MCP, `kano-go sync`) carry no setting
and run incrementally with the full-sync post cap for X and its related
accounts and 50 uploads for YouTube.

### Method chains

Each X, YouTube, Bilibili and shop source schedule orders its listing methods into a fallback
chain ([method chains](#method-chains)), stored
in `source_schedules.chain_json` and edited in the timed reads of the
Sources page. A read tries the enabled methods from the top and falls to the
next when the first page fails, when the answer lists none of the account's
or channel's own records, or when a method is skipped. Bilibili dynamics
stops on login and risk-control failures, including 429, without making a
fallback request, and the shop stops on a 429, a timeout or an unreachable
host and follows the [shop request budget](../sources/shop.md#official-shop).
A later page that fails keeps the earlier pages and leaves
the run partial without falling back.

| Source              | Methods, in default order                                                                                              | Default                                |
| ------------------- | ---------------------------------------------------------------------------------------------------------------------- | -------------------------------------- |
| `x`                 | `timeline` (FxTwitter timeline), `profile` (logged-out `x.com/<handle>`, IDs only), `api` (X API)                      | `api` off                              |
| `youtube`           | `api` (Data API uploads playlist), `rss` (public feed), `ytdlp` (metadata), `videos_page` (uploads tab, IDs only)      | api, rss and videos_page on; ytdlp off |
| `x_related`         | `timeline` (FxTwitter timeline)                                                                                        | fixed                                  |
| `bilibili`          | `list` (the upload list), `dynamics` (signed-in dynamics listing), `app` (official client's listing)                   | all on                                 |
| `bilibili_dynamics` | `desktop` (signed-in desktop feed), `dynamics` (signed-in WBI listing)                                                 | both on; desktop first                 |
| `shop`              | `signed` (newest 50, Web Bot Auth signed), `new_arrivals` (newest 50, unsigned), `atom` (catalog Atom feed, newest 25) | all on; signed first                   |

The default keeps the existing order and leaves yt-dlp off until it is
configured, so the default chain applies when no stored chain is available. A method whose
credential the process lacks is skipped with `x_api_unavailable` or
`youtube_api_key_missing`; a Bilibili method without its login or key pair
is passed over without a request, and so is the shop's `signed` without
an active Web Bot Auth key (`shop_no_signing_key`). Risk-control and login failures on the
Bilibili dynamics chain stop it before a fallback request. The chain covers listing methods only: detail
requests (vxtwitter, watch pages) and fixed helper reads (the streams page,
and the feed read for Shorts verdicts while the Data API lists) stay outside
it. [X](../sources/x.md), [YouTube](../sources/youtube.md),
[Bilibili](../sources/bilibili.md) and the [official shop](../sources/shop.md#official-shop) describe the methods,
and [X API in the chain](../sources/x.md#x-api-in-the-chain) its gate and daily
cap. An X `count` sync setting reads the timeline's count window whatever
the chain says. Manual Bilibili dynamics fetches use their explicit `space`
method; this chain controls scheduled dynamics reads and runs of that
source.

Each run freezes its chain in `workflow_runs.source_chain_json`, so a retry
or recovery reads the same way: timer runs, Run now, YouTube pushes, one-off
step runs and the [live-status probe](steps-and-timers.md#live-status-probe), which reads the
YouTube chain. Full syncs and runs stored without a chain read through the
default. The step result names the method that served each X account in
`discovery` (`<account>: <method>`, or `none`) and the channel in `listing`,
and lists every method tried, in order, in `tried`: `<method>` for the one
that served and `<method>: <code>` for the others, each prefixed with the
account for X.

Enabling a schedule or changing its interval plans the next run one interval
later, never immediately; a sync change alone keeps the plan. Claiming a run
the timer submitted plans the next one an interval after the claim. Only the
first claim plans: claiming it again after a retry or after recovery from an
expired lease (both counted in `recovery_count`) keeps the plan, and the
operator's one-off run of the chain does not move it either. Starting the
timer moves the plan to the moment it is pressed unless the schedule is
already due, and the claim plans the next run as for any timed run.

Each plan adds a random offset of whole seconds, uniform in ±10% of the
interval (a 30-minute X timer runs 27–33 minutes apart), so timed reads do
not reach FxTwitter, YouTube, `api.bilibili.com` or the shop on a fixed
cadence. An offset that would leave the interval bounds is reflected back, so
a plan is never shorter than 15 minutes (the shop's 180) nor longer than 7
days. A Bilibili timer's
offset is never negative (0 to +10%): its interval is never below
`BILIBILI_MIN_INTERVAL_MINUTES`, so a plan is never sooner than the floor and
the adapter's interval check does not skip a timed run. Run now, YouTube
pushes and the live-status probe have no offset. The draw is the store's
injectable `SetScheduleOffset`, which tests replace for deterministic plans.
`WORKFLOW_SCHEDULER_ENABLED=0` stops source schedules as well, and `SKIP_X`,
`SKIP_YOUTUBE`, `SKIP_BILIBILI` and `SKIP_SHOP` disable that source's
schedules (both X timers for `SKIP_X`, both Bilibili timers for
`SKIP_BILIBILI`), which the console reports as
`{ skipped: true, reason: "environment" }`.

### The `workflows` table

Per-source timers are the only timers. The `workflows` table, with its
`source_sync_json` and `source_lookback_json` columns, remains in the schema
and is empty on a fresh installation; no timer is derived from it and current
runs do not read or write workflow definitions. A run whose `workflow_id` is
not a reserved `@` ID keeps that identifier in history without joining
`workflows`.

Every start of `kano-go serve` raises a stored interval of either Bilibili
timer below `BILIBILI_MIN_INTERVAL_MINUTES` to that floor. The
`bilibili_dynamics` and `x_related` timers are timers of their own: each
stays off until the operator turns it on, whatever the switch of the Bilibili
or X timer.
