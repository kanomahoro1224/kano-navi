# Runtime Configuration

## Environment Variables

| Variable                                  | Default                             | Purpose                                                                |
| ----------------------------------------- | ----------------------------------- | ---------------------------------------------------------------------- |
| `APP_MODE`                                | `production`                        | Explicit `development` bypasses login; production requires it          |
| `ADMIN_PASSWORD`                          | Empty                               | Production startup requires at least 12 non-padding characters         |
| `ADMIN_ORIGIN`                            | Empty                               | Optional explicit origin accepted for admin mutations                  |
| `PUBLIC_ORIGIN`                           | Empty                               | Optional public origin for absolute share-tag and feed links           |
| `LLM_SECRETS_KEY`                         | Empty                               | Required non-blank encryption master key at startup in every mode      |
| `PORT`                                    | `7657`                              | Shared frontend/API port for development, preview, and production      |
| `KANO_API_PORT`                           | `7658`                              | Development only: loopback port of the backend behind Vite             |
| `KANO_WATCH_POLLING`                      | `0`                                 | Development only: `1` makes Vite poll a mounted checkout               |
| `SCHEDULE_EXTRACTION_ENABLED`             | `1`                                 | Default for stage flags without a stored or environment value          |
| `SCHEDULE_KEYWORD_ENABLED`                | `1`                                 | Enable keyword candidate selection                                     |
| `SCHEDULE_VISION_ENABLED`                 | `1`                                 | Enable schedule-board LLM extraction                                   |
| `SCHEDULE_MESSAGE_ENABLED`                | `1`                                 | Enable single-message schedule detection                               |
| `SCHEDULE_MESSAGE_ALL_POSTS`              | `0`                                 | Send every post through the message route, skipping the heuristic      |
| `SCHEDULE_KEYWORDS`                       | `schedule,...`                      | Comma-separated schedule candidate keywords                            |
| `WORKFLOW_SCHEDULER_ENABLED`              | `1`                                 | Start the source timers in the server process                          |
| `MCP_ENABLED`                             | `1`                                 | Expose the stateless `/mcp` endpoint                                   |
| `MCP_CONTROL_TOKEN`                       | Empty                               | Bearer token for MCP mutation tools                                    |
| `X_HANDLES`                               | `kano_2525,_Kanotic`                | Comma-separated public X account names                                 |
| `X_API_BEARER_TOKEN`                      | Empty                               | Optional environment-only X API credential: fetches, chain method      |
| `YOUTUBE_API_KEY`                         | Empty                               | Optional environment-only Data API key: YouTube sync and fetches       |
| `YOUTUBE_CHANNEL_ID`                      | Main channel                        | YouTube channel identifier                                             |
| `YTDLP_BINARY`                            | `/app/yt-dlp` in production Compose | yt-dlp executable path; empty disables the method                      |
| `BILIBILI_UID`                            | Kano's account                      | Bilibili account (numeric mid) whose uploads and dynamics are read     |
| `BILIBILI_MIN_INTERVAL_MINUTES`           | `180`                               | Minimum minutes between Bilibili reads and timer runs (60–1440)        |
| `BILIBILI_COOKIE`                         | Empty                               | Optional signed-in browser cookies; wins over `BILIBILI_SESSDATA`      |
| `BILIBILI_SESSDATA`                       | Empty                               | Optional login cookie; a login is required for the dynamics            |
| `BILIBILI_APP_KEY`, `BILIBILI_APP_SECRET` | Empty                               | Optional official-client key pair; turns on the uploads' last fallback |
| `SKIP_X`, `SKIP_YOUTUBE`                  | `0`                                 | Set to `1` to skip that source's step and its timer                    |
| `SKIP_BILIBILI`                           | `0`                                 | Set to `1` to skip both Bilibili steps, timers and console reads       |
| `SKIP_SHOP`                               | `0`                                 | Set to `1` to skip the official shop's step, timer and fetch           |
| `SKIP_MEDIA`, `SKIP_LLM`                  | `0`                                 | Set a post-processing stage flag to `1` to skip it                     |
| `KANO_MEM_LIMIT`                          | `1g` in production Compose          | Production Compose: the container's memory limit (`mem_limit`)         |
| `GOMEMLIMIT`                              | `700MiB` in production Compose      | Go runtime soft memory limit, kept below the container's limit         |

Production Docker Compose limits the container to `KANO_MEM_LIMIT` and passes
`GOMEMLIMIT` to the server. `GOMEMLIMIT` is the Go runtime's own setting,
read from the process environment when `kano-go` starts: Compose sets it in
the container, while a `.env` file the binary reads itself comes too late for
it, so a process started without it has no soft limit. Near the value the
garbage collector works harder instead of letting the heap grow until the
container limit stops the process; it is a soft limit, and the process is not
stopped when the heap exceeds it. The defaults keep about 300 MB of the 1 GiB
container for yt-dlp, its JavaScript runtime and memory the Go runtime does not
count. [Making resized copies](../architecture/media-cache.md#making-copies)
needs up to about 350 MB at once for two 40-megapixel originals, which fits
under the soft limit. On a larger host raise both values together, keeping
`GOMEMLIMIT` about 300 MB below `KANO_MEM_LIMIT`; `GOMEMLIMIT=off` removes
the soft limit.

`SKIP_LLM=1` skips both model steps, `schedule` and `translate`. The
translate step reads only the translation queue set in `/admin` → Models, so
it does nothing until that queue holds a text model.

`PUBLIC_ORIGIN` is a bare `http` or `https` origin such as
`https://kano.example.invalid` (a trailing `/` is accepted and the host is
lowercased; a path, query, credentials, or a host that is not a DNS name or
IP address with an optional port fail startup). It only completes the link-preview tags of
`index.html` (see [Share tags](../architecture/backend.md#share-tags)) and the
Atom feed's site and self links (see
[Public feeds](../architecture/public-api.md#public-feeds)); it grants no access
and is not an admin origin.

`YTDLP_BINARY` enables the optional YouTube yt-dlp method. The production
Compose deployment points it at the bundled `/app/yt-dlp`; other deployments
can set a fixed executable name on `PATH` or an absolute path. The process
never accepts a command path from an HTTP request; the Sources page only
reports its version and asks that configured executable to run its own release
update.

Docker Compose also uses `KANO_IMAGE` (default
`yexca/kano-navi:latest` from Docker Hub) for the published image and `KANO_PORT`
(default `7657`) for the host port. The API also defaults to `7657` inside the container.

`BILIBILI_COOKIE` is the `Cookie` request header of a browser signed in to
Bilibili (developer tools → Network → any `api.bilibili.com` request →
Request Headers → `cookie`). The server keeps `SESSDATA` and the known login
cookies of that browser (`bili_jct`, `DedeUserID`, `DedeUserID__ckMd5`,
`sid`, and the device cookies `buvid3`, `buvid4`, `b_nut`, `_uuid`,
`buvid_fp`) whose values are well formed, and drops every other cookie. A
signed-in request sends the browser's own device cookies in place of the
anonymous identity, so the account and its device match as they do in the
browser. `BILIBILI_SESSDATA` is the `SESSDATA` value alone (Application →
Cookies → `.bilibili.com`); it is read only when `BILIBILI_COOKIE` carries no
`SESSDATA`, and Bilibili accepts it beside an anonymous identity less
readily. With either, the `bilibili` step and the console's reads are sent
as that account, which Bilibili's captcha blocks far less often than
anonymous reads, the upload list falls back to the dynamics listing when it
fails, and the `bilibili_dynamics` step reads the account's dynamics, which
it skips without one
([ADR-0007](../decisions/ADR-0007-bilibili-dynamics-need-a-login.md)). The
login cookies are sent only to `api.bilibili.com`, never stored in SQLite,
logged, or returned.

`BILIBILI_APP_KEY` and `BILIBILI_APP_SECRET` are an official Bilibili
client's app key pair (16 and 32 lowercase hex digits). With both set, an
upload read whose list and dynamics fallback failed tries that client's
space listing on `app.bilibili.com`, signed with the pair (see
[Bilibili](../architecture/sources.md#bilibili)); a malformed or missing
pair keeps that fallback off. Requests signed with it read as the official
client rather than as this service, which is the trade-off an operator
accepts by setting it. Open-source Bilibili clients publish such pairs; the
repository carries none. The pair is kept, sent and protected like the
other source credentials: only in the environment and memory, never stored,
logged, or returned.
"Signed in" in the console means Bilibili accepted the cookie at the latest
daily check (`nav` `isLogin`); before the first check it reads as not
checked yet, and an expired cookie is no longer sent until a check accepts a
renewed one. Prefer a secondary account,
and replace the value when it expires or the account signs out.

`.env.example` is the complete non-secret inventory. `ADMIN_PASSWORD` and
`LLM_SECRETS_KEY`, `MCP_CONTROL_TOKEN`, `BILIBILI_COOKIE`, `BILIBILI_SESSDATA`, `BILIBILI_APP_SECRET`, and provider API keys are credentials;
never place real values in source code, SQLite plaintext, URLs, API responses,
or logs. `OPENAI_API_KEY` is accepted only by the one-time
`kano-go migrate-llm` command.

Configure local values in the ignored .env or deployment environment. Changes
to environment-only credentials require a server restart. Provider hosts,
models, encrypted keys, routes, source accounts, and each source schedule's
sync setting are operator settings managed through /admin, as are the
[sync settings](#sync-settings). How much a run reads and the timer interval
are independent. A source schedule's incremental cap replaces the full-sync
post cap for every run that reads X with it (timer, Run now, YouTube push,
one-off step run); full syncs (`kano-go sync`, the console's Sync
everything, MCP) keep that setting for X and read 50 YouTube uploads at most. Each schedule's
[method chain](../architecture/workflows.md#method-chains) decides whether
its runs use `X_API_BEARER_TOKEN` and `YOUTUBE_API_KEY`: the X API only as a
fallback behind its gate and daily cap, the YouTube Data API only while the
chain enables it ([ADR-0004](../decisions/ADR-0004-method-fallback-chains.md)).
Full syncs read through the default chains, which leave the X API off, and
no timed read turns a credential into a date search
([ADR-0002](../decisions/ADR-0002-count-based-source-sync.md)).

The backend (`kano-go`) reads `.env` from its working directory; variables
already present in the environment win. The database path is a command-line
option (`-database`, default `data/database/kano.sqlite`) rather than an
environment variable, as are `-static` and `-listen`. `KANO_API_PORT` is read
only by `make dev`/`make preview` and Vite; production serves everything on
`PORT`.

Run with flags only and no command, the binary serves public reads from an
existing snapshot without writing it; only `make go-check` uses this mode. See
[Read-only mode](../architecture/backend.md#read-only-mode).

Set `LLM_SECRETS_KEY` before starting development, preview, or production, even
when no provider has been added. Missing, empty, or whitespace-only values
refuse startup before SQLite opens. Use a random master key and retain it
across restarts; changing it makes previously stored provider keys unreadable.
For example, generate a value locally with
`node -e "console.log(require('node:crypto').randomBytes(32).toString('base64'))"`
and save it only in the ignored environment file or deployment secret store.

## Sync Settings

The read limits of source steps, manual fetches and the translate step, and
the attempts each model gets, are operator settings in `app_settings`, edited
on `/admin` → Sources (the X and YouTube read limits) and `/admin` → Detection
(model limits: the attempts and the translation limits). Each run reads the
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
| Translation items per run    | `20`    | 1–200      | Posts and titles the translate step sends to a model                      |
| Translation lookback (days)  | `30`    | 1–3650     | Age of posts and videos the translate step covers unrequested             |
| Attempts per model           | `3`     | 1–10       | Calls one model gets for an input before the next queued model takes over |

On startup, each setting without a stored value takes the integer value of
its environment variable, if set: `X_MAX_STATUS_REQUESTS`, `X_REFRESH_KNOWN`,
`X_BOOTSTRAP_DAYS`, `X_DISCOVERY_LIMIT`, `YOUTUBE_MAX_DETAIL_REQUESTS`,
`SOURCE_HISTORY_MAX_PAGES`, `SYNC_TIMEOUT_MS`, `SYNC_REQUEST_DELAY_MS`,
`TRANSLATION_SCAN_LIMIT` and `TRANSLATION_LOOKBACK_DAYS`, in table order;
the attempts per model have no variable.
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
[Related accounts](../architecture/sources.md#related-accounts). The setting
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
The [official shop](../architecture/sources.md#official-shop)'s timer is 360
minutes by default and never below 180. `SKIP_X`, `SKIP_YOUTUBE`,
`SKIP_BILIBILI` and `SKIP_SHOP` keep that source's timers from running
(`SKIP_X` both X timers).
Per-source timers are the only timed sync
([ADR-0003](../decisions/ADR-0003-per-source-sync-schedules.md)). See
[Workflows](../architecture/workflows.md#source-schedules).

The YouTube live-status probe is an operator setting, not an environment
variable: `/admin` → Sources → Live-status probe turns it on or off and sets
its interval (default 15 minutes, 5–360), stored in `app_settings` as
`youtube_live_probe_enabled` and `youtube_live_probe_interval_minutes`. The
same fold stops or resumes checking one video (see
[Stopped videos](../architecture/sources.md#stopped-videos)). The
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
[Workflows](../architecture/workflows.md#live-status-probe).

How long finished runs stay in the run history is an operator setting as
well: `/admin` → Runs → History cleanup, stored in `app_settings` as
`run_retention_succeeded_days` (default 30) and `run_retention_failed_days`
(default 90). See
[Workflows](../architecture/workflows.md#run-execution).

## Related Docs

- [Local development](../development/local-dev.md)
- [Docker deployment](docker.md)
- [Deployment security](security.md)
- [Manual source fetches](../architecture/workflows.md#manual-fetches-and-historical-backfill)
