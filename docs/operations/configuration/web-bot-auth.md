# Web Bot Auth

Configuring the Web Bot Auth signing key and origin.

The [official shop](../../architecture/sources/shop.md#official-shop)'s requests
carry a [Web Bot Auth](../../architecture/web-bot-auth.md) signature once the
operator generates a key in `/admin` → Sources → Official shop → Web Bot
Auth; no environment variable is needed. The key is sealed with
`LLM_SECRETS_KEY` in SQLite, so a backup of the data directory keeps it and
a changed `LLM_SECRETS_KEY` makes it unreadable until a new one is
generated. Signing also needs an `https` origin that reaches this site, where
verifiers fetch `/.well-known/http-message-signatures-directory`: the
panel's signing origin, or `PUBLIC_ORIGIN` when it is `https` and no signing
origin is set. Generating, rotating, removing and the origin apply without a
restart; a rotation keeps the replaced public key in the directory for 24
hours.

`WEB_BOT_AUTH_KEY` overrides the console's key for a deployment that
manages it outside SQLite: a 32-byte Ed25519 seed in base64url. While it is
set the panel shows the key as environment-managed and cannot generate,
rotate or remove one. A malformed value stops `kano-go serve` and
`kano-go sync` before SQLite opens. Make a value with:

```bash
kano-go web-bot-auth-key
```

It prints the `WEB_BOT_AUTH_KEY=` line and, as a comment, the key's keyid
(its RFC 7638 JWK thumbprint). A changed value takes effect after a restart,
and the directory then lists only the new public key. The value is a
credential like the others below.

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
anonymous reads, the upload read's `dynamics` method can run, and the
`bilibili_dynamics` step reads the account's dynamics, which
it skips without one
([Bilibili dynamics](../../architecture/sources/bilibili-dynamics.md#dynamics)). The
login cookies are sent only to `api.bilibili.com`, never stored in SQLite,
logged, or returned.

`BILIBILI_APP_KEY` and `BILIBILI_APP_SECRET` are an official Bilibili
client's app key pair (16 and 32 lowercase hex digits). With both set, the
upload read's `app` method can run: that client's space listing on
`app.bilibili.com`, signed with the pair, tried where the Bilibili timer's
method chain places it (see [Bilibili](../../architecture/sources/bilibili.md));
a malformed or missing pair keeps the method from running. Requests signed with it read as the official
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

`.env.example` is the complete non-secret inventory. `ADMIN_PASSWORD`,
`ADMIN_TOTP_SECRET`, `LLM_SECRETS_KEY`, `MCP_CONTROL_TOKEN`, `BILIBILI_COOKIE`, `BILIBILI_SESSDATA`, `BILIBILI_APP_SECRET`, `WEB_BOT_AUTH_KEY`, and provider API keys are credentials;
never place real values in source code, SQLite plaintext, URLs, API responses,
or logs. `OPENAI_API_KEY` is accepted only by the one-time
`kano-go migrate-llm` command.

Configure local values in the ignored .env or deployment environment. Changes
to environment-only credentials require a server restart. Provider hosts,
models, encrypted keys, routes, source accounts, and each source schedule's
sync setting are operator settings managed through /admin, as are the
[sync settings](sync.md#sync-settings). How much a run reads and the timer interval
are independent. A source schedule's incremental cap replaces the full-sync
post cap for every run that reads X with it (timer, Run now, YouTube push,
one-off step run); full syncs (`kano-go sync`, the console's Sync
everything, MCP) keep that setting for X and read 50 YouTube uploads at most. Each schedule's
[method chain](../../architecture/workflows/source-schedules.md#method-chains) decides whether
its runs use `X_API_BEARER_TOKEN` and `YOUTUBE_API_KEY`: the X API only as a
fallback behind its gate and daily cap, the YouTube Data API only while the
chain enables it ([method chains](../../architecture/workflows/source-schedules.md#method-chains)).
Full syncs read through the default chains, which leave the X API off, and
no timed read turns a credential into a date search
([source schedules](../../architecture/workflows/source-schedules.md#source-schedules)).

The backend (`kano-go`) reads `.env` from its working directory; variables
already present in the environment win. The database path is a command-line
option (`-database`, default `data/database/kano.sqlite`) rather than an
environment variable, as are `-static` and `-listen`. `KANO_API_PORT` is read
only by `make dev`/`make preview` and Vite; production serves everything on
`PORT`.

Run with flags only and no command, the binary serves public reads from an
existing snapshot without writing it; only the container smoke
(`make go-container-check`, part of `make go-check`) uses this mode. See
[Read-only mode](../../architecture/backend.md#read-only-mode).

Set `LLM_SECRETS_KEY` before starting development, preview, or production, even
when no provider has been added. Missing, empty, or whitespace-only values
refuse startup before SQLite opens. Use a random master key and retain it
across restarts; changing it makes previously stored provider keys unreadable.
For example, generate a value locally with
`node -e "console.log(require('node:crypto').randomBytes(32).toString('base64'))"`
and save it only in the ignored environment file or deployment secret store.
