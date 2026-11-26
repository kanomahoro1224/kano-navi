# Environment Variables

Each environment variable and the admin one-time code.

| Variable                                  | Default                             | Purpose                                                               |
| ----------------------------------------- | ----------------------------------- | --------------------------------------------------------------------- |
| `APP_MODE`                                | `production`                        | Explicit `development` bypasses login; production requires it         |
| `ADMIN_PASSWORD`                          | Empty                               | Production startup requires at least 12 non-padding characters        |
| `ADMIN_ORIGIN`                            | Empty                               | Optional explicit origin accepted for admin mutations                 |
| `ADMIN_TOTP_SECRET`                       | Empty                               | Optional one-time code secret; empty keeps password-only login        |
| `PUBLIC_ORIGIN`                           | Empty                               | Optional public origin for absolute share-tag and feed links          |
| `CDN_PUBLIC_BASE`                         | Empty                               | Optional CDN base for the build's assets and cached media             |
| `LLM_SECRETS_KEY`                         | Empty                               | Required non-blank encryption master key at startup in every mode     |
| `PORT`                                    | `7657`                              | Shared frontend/API port for development, preview, and production     |
| `KANO_API_PORT`                           | `7658`                              | Development only: loopback port of the backend behind Vite            |
| `KANO_WATCH_POLLING`                      | `0`                                 | Development only: `1` makes Vite poll a mounted checkout              |
| `SCHEDULE_EXTRACTION_ENABLED`             | `1`                                 | Default for stage flags without a stored or environment value         |
| `SCHEDULE_KEYWORD_ENABLED`                | `1`                                 | Enable keyword candidate selection                                    |
| `SCHEDULE_VISION_ENABLED`                 | `1`                                 | Enable schedule-board LLM extraction                                  |
| `SCHEDULE_MESSAGE_ENABLED`                | `1`                                 | Enable single-message schedule detection                              |
| `SCHEDULE_MESSAGE_ALL_POSTS`              | `0`                                 | Send every post through the message route, skipping the heuristic     |
| `SCHEDULE_KEYWORDS`                       | `schedule,...`                      | Comma-separated schedule candidate keywords                           |
| `WORKFLOW_SCHEDULER_ENABLED`              | `1`                                 | Start the source timers in the server process                         |
| `ALERTS_ENABLED`                          | `1`                                 | Evaluate [operator alerts](../alerts.md); `0` turns them off          |
| `MCP_ENABLED`                             | `1`                                 | Expose the stateless `/mcp` endpoint                                  |
| `MCP_CONTROL_TOKEN`                       | Empty                               | Bearer token for MCP mutation tools                                   |
| `X_HANDLES`                               | `kano_2525,_Kanotic`                | Comma-separated public X account names                                |
| `X_API_BEARER_TOKEN`                      | Empty                               | Optional environment-only X API credential: fetches, chain method     |
| `YOUTUBE_API_KEY`                         | Empty                               | Optional environment-only Data API key: YouTube sync and fetches      |
| `YOUTUBE_CHANNEL_ID`                      | Main channel                        | YouTube channel identifier                                            |
| `YTDLP_BINARY`                            | `/app/yt-dlp` in production Compose | yt-dlp executable path; empty disables the method                     |
| `BILIBILI_UID`                            | Kano's account                      | Bilibili account (numeric mid) whose uploads and dynamics are read    |
| `BILIBILI_MIN_INTERVAL_MINUTES`           | `180`                               | Minimum minutes between Bilibili reads and timer runs (60–1440)       |
| `BILIBILI_COOKIE`                         | Empty                               | Optional signed-in browser cookies; a stored login wins over them     |
| `BILIBILI_SESSDATA`                       | Empty                               | Optional login cookie, read without `BILIBILI_COOKIE`                 |
| `BILIBILI_APP_KEY`, `BILIBILI_APP_SECRET` | Empty                               | Optional official-client key pair; lets the uploads' `app` method run |
| `HUAWEI_PUSH_SERVICE_ACCOUNT`             | Empty                               | AGC service account JSON file; empty leaves Huawei push off           |
| `HUAWEI_PUSH_CATEGORY`                    | `SUBSCRIPTION`                      | AGC message category the App is approved for                          |
| `HUAWEI_PUSH_TEST`                        | `false`                             | `true` sends test messages the App handles without showing them       |
| `HUAWEI_PUSH_RECEIPT_SECRET`              | Empty                               | AGC receipt callback key; empty serves no receipt route               |
| `HUAWEI_PUSH_RECEIPT_USER`                | Empty                               | AGC receipt callback user name the signature covers                   |
| `WEB_PUSH_VAPID_PUBLIC_KEY`               | Empty                               | VAPID public key; empty leaves browser notifications off              |
| `WEB_PUSH_VAPID_PRIVATE_KEY`              | Empty                               | VAPID private key, printed by `kano-go webpush-key`                   |
| `WEB_PUSH_VAPID_SUBJECT`                  | `mailto:kano@example.com`           | Contact a push service may use about this sender                      |
| `WEB_PUSH_ICON`                           | `/assets/kano-avatar.jpg`           | Avatar shown beside every browser notification                        |
| `WEB_PUSH_TTL_SECONDS`                    | `86400`                             | How long a push service keeps an undelivered message                  |
| `WEB_PUSH_ENDPOINT_HOSTS`                 | Built-in allowlist                  | Comma-separated push-service hosts accepted at registration           |
| `BAMBINO_INGEST_SECRET`                   | Empty                               | HMAC secret for `/internal/ingest/bambino`; empty leaves it absent    |
| `BAMBINO_INGEST_TOLERANCE_SECONDS`        | `300`                               | Accepted clock skew of an ingest timestamp, in seconds                |
| `BAMBINO_INGEST_CATCHUP_SECONDS`          | `180`                               | Ticker that re-submits post-processing a busy queue dropped           |
| `WEB_BOT_AUTH_KEY`                        | Empty                               | Optional override of the console's Web Bot Auth signing key           |
| `SKIP_X`, `SKIP_YOUTUBE`                  | `0`                                 | Set to `1` to skip that source's step and its timer                   |
| `SKIP_BILIBILI`                           | `0`                                 | Set to `1` to skip both Bilibili steps, timers and console reads      |
| `SKIP_SHOP`                               | `0`                                 | Set to `1` to skip the official shop's step, timer and fetch          |
| `SKIP_MEDIA`, `SKIP_LLM`                  | `0`                                 | Set a post-processing stage flag to `1` to skip it                    |
| `KANO_MEM_LIMIT`                          | `1g` in production Compose          | Production Compose: the container's memory limit (`mem_limit`)        |
| `GOMEMLIMIT`                              | `700MiB` in production Compose      | Go runtime soft memory limit, kept below the container's limit        |

Production Docker Compose limits the container to `KANO_MEM_LIMIT` and passes
`GOMEMLIMIT` to the server. `GOMEMLIMIT` is the Go runtime's own setting,
read from the process environment when `kano-go` starts: Compose sets it in
the container, while a `.env` file the binary reads itself comes too late for
it, so a process started without it has no soft limit. Near the value the
garbage collector works harder instead of letting the heap grow until the
container limit stops the process; it is a soft limit, and the process is not
stopped when the heap exceeds it. The defaults keep about 300 MB of the 1 GiB
container for yt-dlp, its JavaScript runtime and memory the Go runtime does not
count. [Making resized copies](../../architecture/media-cache/resized-copies.md#making-copies)
needs up to about 350 MB at once for two 40-megapixel originals, which fits
under the soft limit. On a larger host raise both values together, keeping
`GOMEMLIMIT` about 300 MB below `KANO_MEM_LIMIT`; `GOMEMLIMIT=off` removes
the soft limit.

`SKIP_LLM=1` skips both model steps, `schedule` and `translate`. The
translate step reads only the translation queue set in `/admin` → Detection, so
it does nothing until that queue holds a text model.

`CDN_PUBLIC_BASE` is the public base of an optional CDN, such as
`https://cdn.example.invalid` or `https://cdn.example.invalid/site` (an http
or https URL whose path is an optional prefix; a query, credentials, escaped
segments, empty or `.`/`..` segments fail startup, and fail `vite build` the
same way; the host is lowercased). The frontend build also
reads it, so pass it to `npm run build` or `docker build`; the service then
writes the build's fixed files, the share-tag banner and every public media
address to that base, and empty keeps them on this origin. See
[CDN](../cdn.md).

`PUBLIC_ORIGIN` is a bare `http` or `https` origin such as
`https://kano.example.invalid` (a trailing `/` is accepted and the host is
lowercased; a path, query, credentials, or a host that is not a DNS name or
IP address with an optional port fail startup). It only completes the link-preview tags of
`index.html` (see [Share tags](../../architecture/backend.md#share-tags)) and the
Atom feed's site and self links (see
[Public feeds](../../architecture/public-api/feeds-and-endpoints.md#public-feeds)), and, when it is
`https` and no signing origin is set, names the Web Bot Auth signing agent
(see [Web Bot Auth](web-bot-auth.md)); it grants no access and is not an admin
origin.

## Admin one-time code

`ADMIN_TOTP_SECRET` adds a one-time code to production admin login: the
six-digit RFC 6238 code (HMAC-SHA1, 30-second step) that password managers
such as 1Password and authenticator apps generate. It is off while the
variable is empty or whitespace, and `APP_MODE=development` never asks for
it. The value is base32 of at least 16 bytes, in either case, with spaces,
hyphens and padding ignored; any other value, including one of only those
characters, is malformed and stops `kano-go serve` in every mode before
SQLite opens. Make a value with:

```bash
kano-go totp-secret
```

It prints the `ADMIN_TOTP_SECRET=` line and, as a comment, the
`otpauth://totp/...` address. Import the address (or paste the secret) into
the password manager's one-time password field for the console's login, then
restart; the login page asks for the code once the service runs with it.
Losing the authenticator is recovered by replacing or clearing the variable
and restarting. The secret is a credential like the others below.

Setting it is recommended for any deployment whose `/admin` is reachable
from the internet; see
[Deployment security](../security.md#admin-authentication).

## Huawei push and Bambino ingest

Huawei push stays off until `HUAWEI_PUSH_SERVICE_ACCOUNT` names an AGC service
account JSON file whose `project_id`, `key_id`, `sub_account` and private key
are present; a path that cannot be read or parsed refuses startup rather than
running half configured. The file holds a signing key, so keep it outside the
image and the checkout. `HUAWEI_PUSH_CATEGORY` must be a category AGC approved
for the app, and `SUBSCRIPTION` is the only one a self-classified message may
use. Devices subscribe through `POST /api/push/device` (see
[Public API](../../architecture/public-api/feeds-and-endpoints.md#push-device-registration-apipushdevice));
a delivery skips a device that turned its master switch or that update's
category off. `HUAWEI_PUSH_RECEIPT_SECRET` and `HUAWEI_PUSH_RECEIPT_USER` turn
on `POST /api/push/receipt`: a signed receipt batch retires the tokens it
reports as uninstalled or reissued, and an empty secret leaves the route
absent.

## Web push

Browser notifications stay off until `WEB_PUSH_VAPID_PUBLIC_KEY` and
`WEB_PUSH_VAPID_PRIVATE_KEY` both hold a pair from `kano-go webpush-key`; a
malformed pair refuses startup instead of failing every delivery. The public
key is handed to the browser, the private key signs the VAPID token and stays
in the environment. `WEB_PUSH_VAPID_SUBJECT` is the contact a push service may
use, `WEB_PUSH_ICON` is the avatar beside the message, and
`WEB_PUSH_TTL_SECONDS` bounds how long an undelivered message may wait.

A browser may only register an `https` endpoint on a push service the server
will post to; `WEB_PUSH_ENDPOINT_HOSTS` replaces the built-in list
(`fcm.googleapis.com`, `*.push.services.mozilla.com`, `*.push.apple.com`,
`*.notify.windows.com`, `*.push.samsungosp.com`) for a self-hosted service.
The site needs HTTPS for any of this; `localhost` is the browser's only
exception.

`BAMBINO_INGEST_SECRET` turns on `/internal/ingest/bambino`, the bridge's entry
for normalized X updates; the same value must be configured in BambinoStatus,
which signs `timestamp + "\n" + body` with it. An empty secret leaves the
route answering `404` instead of trusting an unauthenticated write.
`BAMBINO_INGEST_TOLERANCE_SECONDS` bounds how old a signature may be, and
`BAMBINO_INGEST_CATCHUP_SECONDS` re-submits the post-processing chain when the
single-flight queue was busy at ingest; `0` stops that ticker.
