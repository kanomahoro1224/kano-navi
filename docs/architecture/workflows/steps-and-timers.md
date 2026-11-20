# Modular Steps and Timers

The step catalog, timers and the live-status probe step.

Synchronization is split into modular steps declared in
`backend/internal/jobs` (`StepOrder`), executed in this order: `x`,
`x_related`, `youtube`, `bilibili`, `bilibili_dynamics`, `shop`, `media`,
`schedule`, `translate`, `reconcile`. `kano-go sync` (`make sync`) runs every step
and records the ones disabled by `SKIP_*` as skipped.
A one-off step run (the Runs page, `POST /api/admin/runs` with
`{ "kind": "workflow", "steps": [...] }`) picks any subset from the catalog
(`GET /api/admin/runs/steps`) and runs it once in catalog order; nothing is
saved. Its run records `sync_runs.source = workflow` and exposes per-step progress (`pending`, `running`, `completed`,
`partial`, `failed`, `skipped`) through `/api/admin/runs`. When a selected
step is disabled by `SKIP_*`, its result records `{ skipped: true, reason:
"environment" }` instead of silently disappearing. Every run keeps its own
steps, sync settings and windows, so a failed run with a retryable failed
step can be retried without any saved definition.

`bilibili` and `bilibili_dynamics` pace themselves: `bilibili` reads the
account's upload list and `bilibili_dynamics`, with a login, its
dynamics, each at most once per `BILIBILI_MIN_INTERVAL_MINUTES` on its own
clock and never during the shared captcha backoff, so either is safe in a
manual run soon after its timer (see [Bilibili](../sources/bilibili.md)).
`shop` makes no request while a 429's `Retry-After` lasts (see
[Official shop](../sources/shop.md#official-shop)). A read skipped without a request
records `{ skipped: true, reason }` and succeeds; a failed read makes its step
`failed`.

`reconcile` derives the YouTube events of stored videos and applies the
deterministic duplicate-merge rules in
[Duplicate merging](../data-model/duplicate-merging.md) to the Japan dates from
30 days ago to 60 days ahead, then the
[video merging](../data-model/videos.md#video-merging) rules to every Bilibili upload.
Every run that includes `x`, `x_related`, `youtube`, `bilibili`, `shop` or `schedule` (sync, scan,
manual fetches, source schedules, Run now, step runs, WebSub pushes and MCP) appends it as its last
step; it can also be selected alone. Its
result counts `releasesPaired` (see
[Duplicate merging](../data-model/duplicate-merging.md#bilibili-releases)), `considered`,
`merged`, `retargeted`, `released`, `kept` and
`ambiguous` events, `eventsTimed` and `eventsCreated` YouTube events, and
`videosConsidered`, `videosMerged`, `videosReleased`, `videosKept` and
`videosAmbiguous` uploads, and `tagged` surviving events whose label changed
to the label of a notice merged into them (see
[Event labels](../data-model/events.md#event-labels)). It advances the dashboard
revision only when a merge decision, a derived YouTube event, an inherited
label or a merged upload's translation changed.

`x` reads the own accounts of `X_HANDLES` through the X method chain; their
quotes and reposts write the X status pool with each post. `x_related` reads
every related account's timeline into the pool (see
[Related accounts](../sources/x-accounts.md#related-accounts)): without a related account
it records `{ skipped: true, reason: "no_accounts" }` and succeeds without a
request; a failed account leaves it `partial` with `@<handle>: <code>` in
`errors`, and when every account failed it fails with `x_related_failed`,
retryable like `x_source_failed`. A manual fetch of one related account
reads that account alone (see
[Manual fetches](manual-fetches.md)). Neither step's
outcome affects the other's.

`translate` first settles the forms of Bilibili uploads by rule (see
[Bilibili form](../data-model/video-form.md#bilibili-form)), then machine-translates X
post text, the titles of public videos and the X
statuses own posts quote that lack a usable translation (see
[Translation](../sources/translation.md)), then labels
unlabelled YouTube reservations from their title with the same queue (see
[Event labels](../data-model/events.md#event-labels)), then asks the Bilibili uploads
no rule settled for their form and tag, then tags the videos no entry
labels, each with what is left of the same per-run limit (see
[Video form and tags](../data-model/video-form.md#video-form-and-tags)); its result
counts them under `eventTags`, `bilibiliForms` (with `ruled`, the forms the
rules settled, and `uncertain` answers) and `videoTags`. A title that fails counts as
failed and the step goes on;
only a store failure or cancellation stops it. Its final failure does not
abort a run, so `reconcile` still runs after it. It reads
only the translation queue and records a disabled result while that queue
is empty. `POST /api/admin/translations/request` starts it alone as a
`translate` run after queueing operator requests.

The server process checks due source schedules every 30 seconds and submits
the one with the earliest planned run, so a frequent source cannot starve
another, through the same single-flight queue as manual and MCP jobs; a
schedule that finds the queue busy stays due for the next tick. Set
`WORKFLOW_SCHEDULER_ENABLED=0` to stop the source schedules and the
live-status probe.

## Live-status probe

The [live-status probe](../sources/youtube-live.md#live-status-probe) asks YouTube about
active videos and Bilibili about the account's
[live room](../sources/bilibili.md#live-room). It has its own timer and its own operator
settings, stored in `app_settings`: for YouTube `youtube_live_probe_enabled`
(default on) and `youtube_live_probe_interval_minutes` (default 15, 5–360),
for Bilibili `bilibili_live_probe_enabled` (default on) and
`bilibili_live_probe_interval_minutes` (default 30, 10–360). They are edited
in `/admin` (Sources → Live-status probe).
Each active video is due by its phase against its reserved start (its actual
start once on air), counted from its last successful check (a watch page or
a Data API answer) or probe request, whichever is newer:

| Phase      | When                                          | Next check                           |
| ---------- | --------------------------------------------- | ------------------------------------ |
| `prestart` | within 15 minutes before the start            | once in that lead, then at the start |
| `starting` | from the start for 45 minutes, not yet live   | every 3 minutes                      |
| `live`     | the check reported the broadcast live         | every operator interval              |
| `overdue`  | 45 minutes after the start and still not live | every operator interval              |

A check by the regular YouTube step counts as well, so a reservation it just
read is not asked again. The same server process checks every 30 seconds;
when a video is due and at least two minutes have passed since the previous
probe run was queued, it queues a run of the single `live` step
(`kind: "live"`, ad hoc workflow `@live`, labelled `live-probe`) through the
same single-flight queue. The run asks only about the due videos, earliest
first: with `YOUTUBE_API_KEY` and the YouTube chain's `api` method on, in one
Data API call, then the watch pages of
those the API could not settle (see
[Live-status probe](../sources/youtube-live.md#live-status-probe)). A busy queue or nothing
due waits for the next tick. Requests are recorded per video in `sync_state`
(source `youtube-live`), answered or not, so a video that keeps failing is
asked again on its phase's schedule rather than on every tick; entries older
than two days are dropped. A private or removed video, and any video an
operator stops, leaves the active set instead (see
[Stopped videos](../sources/youtube-live.md#stopped-videos)). Runs are recorded in `sync_runs` and
`/api/admin/runs` with `active`, `candidates` (the due videos), `requested`
(watch pages), `apiRequests` (Data API calls), `inspected`, `live`,
`changed`, `unavailable` (when a video was stopped as private or removed) and
fixed error codes; a run that recorded an error and answered none of its
videos is `failed`, one with any recorded failure (an API call that failed but
whose pages answered, for example) `partial`.

The probe may run more often than a source schedule (15 minutes) because one
run asks about at most five videos and runs only while something is due.
With a key it spends one quota unit per run, at most 720 a day.
One stream that starts on time costs a check before the start, a few in the
starting window and four an hour on air at the default interval; a waiting
room that never opens costs at most 15 in its starting window and then one
per interval until 24 hours after its start. At most five pages per run and
runs two minutes apart bound the worst case, several reservations starting
together, at 150 pages an hour; a regular YouTube step every 15 minutes (feed,
streams page and up to 12 detail pages) is about 56 requests an hour. Longer
than six hours would let a stale result outlive most streams. `live` is not
a catalog step: step runs and full syncs never select it.
`WORKFLOW_SCHEDULER_ENABLED=0` stops the probe; `SKIP_YOUTUBE=1` stops its
YouTube part and `SKIP_BILIBILI=1` its Bilibili part.
Page loads, refreshes and revision requests never start it.

The Bilibili part follows every active stream and members-only entry that
starts within the next 15 minutes or the last 24 hours. An entry shown on
Bilibili (its platforms, with those its merged entries lend it, include
`bilibili`; see [Event platforms](../data-model/events.md#event-platforms)) goes
through the same phases at the Bilibili interval. Every other one gets
sparse checks (phase `sparse`), about 5 and 20 minutes after its start and
then one per Bilibili interval while the room is live, so a broadcast on
Bilibili that only a YouTube reservation announced is seen as its
simulcast.
An entry whose broadcast the room already showed and ended, an ended entry
and a manually confirmed cancellation are left out. Without a due entry the
room is still asked about once per Bilibili interval after its last check
(phase `baseline`), so an unannounced stream is seen. A due Bilibili check
joins the same probe run as due YouTube videos and costs one request, never
more; it never runs during the live host's backoff. Its counts are under
`bilibili` in the run result (`targets`, `phase`, `requested`, `live`,
`changed`, `errors`). At the default interval the baseline costs 48
requests a day, one stream shown on Bilibili a few more around its start,
and any other scheduled stream about two.
