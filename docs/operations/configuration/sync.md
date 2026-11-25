# Sync Settings and Source Timers

Stored sync settings, related X accounts and source timers.

## Sync Settings

The read limits of source steps, manual fetches and the translate step, and
the attempts each model gets, are operator settings in `app_settings`, edited
on `/admin` → Sources (the X and YouTube read limits) and `/admin` → Detection
(model limits: the attempts, the extraction lookback and the translation
limits). Each run reads the
limits when it
starts, so a saved value applies from the next run; the attempts per model
are read for each post, image or translation a model is called for.

| Setting                      | Default | Range      | Purpose                                                                   |
| ---------------------------- | ------- | ---------- | ------------------------------------------------------------------------- |
| X status requests per run    | `12`    | 0–50       | X detail requests shared by all accounts; manual X fetch page budget      |
| X known posts refreshed      | `1`     | 0–5        | Stored X posts read again per run                                         |
| X first-run age limit (days) | `7`     | 1–30       | Age limit while an X account has no cursor yet                            |
| X full-sync post cap         | `50`    | 1–200      | Full-sync cap of unstored X statuses per account; pages of 20             |
| YouTube video pages per run  | `12`    | 1–50       | Watch pages inspected per run for reservations and details                |
| Manual fetch pages           | `4`     | 1–20       | Pages of one fetch per X account, the channel or a Bilibili listing       |
| Request timeout (ms)         | `7000`  | 1000–60000 | Per request of manual fetches and the X API method                        |
| Request spacing (ms)         | `0`     | 0–10000    | Wait between X and YouTube fetch pages; Bilibili pages wait 15 s          |
| Extraction lookback (days)   | `30`    | 0–3650     | Age of posts schedule extraction selects unrequested; `0` sets no limit   |
| Translation items per run    | `20`    | 1–200      | Posts and titles the translate step sends to a model                      |
| Translation lookback (days)  | `30`    | 1–3650     | Age of posts and videos the translate step covers unrequested             |
| Attempts per model           | `3`     | 1–10       | Calls one model gets for an input before the next queued model takes over |

On startup, each setting without a stored value takes the integer value of
its environment variable, if set: `X_MAX_STATUS_REQUESTS`, `X_REFRESH_KNOWN`,
`X_BOOTSTRAP_DAYS`, `X_DISCOVERY_LIMIT`, `YOUTUBE_MAX_DETAIL_REQUESTS`,
`SOURCE_HISTORY_MAX_PAGES`, `SYNC_TIMEOUT_MS`, `SYNC_REQUEST_DELAY_MS`,
`TRANSLATION_SCAN_LIMIT` and `TRANSLATION_LOOKBACK_DAYS`, in table order;
the extraction lookback and the attempts per model have no variable.
Manual fetch pages, the timeout, the spacing and the translation limits take
an out-of-range value at the nearest bound; the others ignore it. A stored
value is never replaced from the environment, so these variables have no
effect once stored and need not be set.

## Related X accounts

Related X accounts are not environment variables: they live in the app
setting `x_related_accounts` and are edited on `/admin` → Sources → X
related, beside their timer. Each account is a handle that is not one of
`X_HANDLES`, with keyword rules (`鹿乃`, `@kano_2525` and `#鹿乃まほろ` for a
newly added account). Each run of the related-accounts timer reads them
through the FxTwitter timeline only; see
[Related accounts](../../architecture/sources/x-accounts.md#related-accounts). The setting
starts empty, and seeding leaves it as it is.

## Source timers

`kano-go serve` starts the timer process unless
`WORKFLOW_SCHEDULER_ENABLED=0`, which stops the source schedules and the
live-status probe. Manual runs from /admin, including each source's Run now,
work either way. Page requests and admin reads never start a
timer or a run. Each source's timer is an operator setting, not an
environment variable: `/admin` → Sources turns it on or off and sets its
interval and read mode, stored in the `source_schedules` table (X 30
minutes, its related accounts 180 and YouTube 60 by default, 15 minutes
through seven days; the Bilibili
uploads and dynamics timers 180 each by default, never below
`BILIBILI_MIN_INTERVAL_MINUTES`). Both Bilibili timers reach the same
address, so their requests add up: keep the dynamics timer no shorter than
the uploads timer until the account has read without a captcha for a while.
The [official shop](../../architecture/sources/shop.md#official-shop)'s timer is 360
minutes by default and never below 180. `SKIP_X`, `SKIP_YOUTUBE`,
`SKIP_BILIBILI` and `SKIP_SHOP` keep that source's timers from running
(`SKIP_X` both X timers).
Per-source timers are the only timed sync
([source schedules](../../architecture/workflows/source-schedules.md#source-schedules)). See
[Workflows](../../architecture/workflows/source-schedules.md#source-schedules).

The YouTube live-status probe is an operator setting, not an environment
variable: `/admin` → Sources → Live-status probe turns it on or off and sets
its interval (default 15 minutes, 5–360), stored in `app_settings` as
`youtube_live_probe_enabled` and `youtube_live_probe_interval_minutes`. The
same fold stops or resumes checking one video (see
[Stopped videos](../../architecture/sources/youtube-live.md#stopped-videos)). The
interval paces streams on air and waiting rooms that stay closed past their
start; around the start itself the probe checks every 3 minutes. With
`YOUTUBE_API_KEY`, and unless the YouTube timer's method chain turns the Data
API off, it asks the Data API first, one quota unit per run. It
uses the same timer process, so `WORKFLOW_SCHEDULER_ENABLED=0` stops it and
`SKIP_YOUTUBE=1` its YouTube part. `/admin` → Sources → Bilibili uploads →
Bilibili live room turns the Bilibili live check on or off and sets its
baseline interval (default 30 minutes, 10–360),
stored as `bilibili_live_probe_enabled` and
`bilibili_live_probe_interval_minutes`; `SKIP_BILIBILI=1` stops it. See
[Workflows](../../architecture/workflows/steps-and-timers.md#live-status-probe).

How long finished runs stay in the run history is an operator setting as
well: `/admin` → Runs → History cleanup, stored in `app_settings` as
`run_retention_succeeded_days` (default 30) and `run_retention_failed_days`
(default 90). See
[Workflows](../../architecture/workflows/run-execution.md).
