# ADR-0006: Web Push Notifications for the Public Site

- Status: Accepted
- Date: 2026-11-03

## Context

The board already notifies HarmonyOS devices when a stream starts, a video is
published or an X post is accepted. `push_devices` holds one row per App
install, `push_pending` holds a post until its translation settles, and
`push.Dispatcher` decides the wording per language and honours each device's
`live`, `video` and `post` switches. Nothing of that reaches a reader who
only has the site open in a browser: the revision stream updates an open tab
and nothing else, so someone who wants to know has to keep the page in front
of them or install the App.

Web Push is the browser standard for the same job: a service worker receives
a message from the push service even when the site is closed, and shows it
through the operating system. It needs three things the project does not have
yet: a VAPID key pair, a service worker served from the site root, and one
stored subscription per browser.

What already fits:

- The site is served by the same Go process (`httpapi.staticSite`) from
  `dist`, so a root-level `sw.js` is one file in `public/`.
- Notifications originate in the Bambino ingest path, which already builds
  `push.Update` values and already waits for a post's translation.
- The wording per language, the category rules and the per-device switches
  exist in `internal/push` and can be shared rather than duplicated.

What does not fit:

- Web Push requires HTTPS (except `localhost`); a LAN address cannot subscribe.
- The browser chooses the endpoint and may rotate it; the server never
  addresses a device directly.
- iOS and iPadOS deliver Web Push only to a web app added to the Home Screen,
  from 16.4 on.
- A payload is at most 4 KB, so a notification is a title, a short body and a
  URL, never media.

## Decision

Adopt Web Push with VAPID (RFC 8292) and `aes128gcm` payload encryption
(RFC 8291, RFC 8030 transport). Keep the App's vocabulary: the same three
categories, the same wording, the same rule that a push failure never fails
the ingest that produced it.

### Subscription storage

```sql
CREATE TABLE web_push_subscriptions (
    endpoint TEXT PRIMARY KEY,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    locale TEXT,
    user_agent TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    notify_live INTEGER NOT NULL DEFAULT 1,
    notify_video INTEGER NOT NULL DEFAULT 1,
    notify_post INTEGER NOT NULL DEFAULT 1,
    failure_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);
```

The endpoint is the identity: one row per browser profile, not per person.
There is no account, so two browsers of the same reader are two rows and
both are notified, exactly like two App installs today.

`locale` follows the same tags as `push_devices` (`ja`, `zh-Hans`,
`zh-Hant`, `en`), so `push.wording` is reused unchanged. `user_agent` is
stored clipped to 128 runes for the admin list only; no IP address is kept.

### Public endpoints

| Method | Path                        | Body                                                            | Answer                 |
| ------ | --------------------------- | --------------------------------------------------------------- | ---------------------- |
| `GET`  | `/api/push/web/key`         | –                                                               | `{enabled, publicKey}` |
| `POST` | `/api/push/web/subscribe`   | `{endpoint, keys:{p256dh,auth}, locale?, live?, video?, post?}` | `204`                  |
| `POST` | `/api/push/web/unsubscribe` | `{endpoint}`                                                    | `204`                  |

Rules:

- `GET /api/push/web/key` is `no-store`, because a browser may ask before
  every subscribe. It answers `{enabled:false}` instead of `404`, so the
  client can hide the button without treating it as an error.
- `subscribe` is idempotent: the same endpoint updates the keys, locale and
  switches and refreshes `last_seen_at`. Omitting a switch keeps its stored
  value, as the App registration does with pointers.
- Validation: `endpoint` must be `https`, at most 2048 runes, and its host
  must be in an allowlist (`fcm.googleapis.com`, `*.push.services.mozilla.com`,
  `*.push.apple.com`, `*.notify.windows.com`, `*.push.samsungosp.com`).
  This is not cosmetic: the server makes an outbound request to whatever is
  stored, so an unchecked endpoint is an SSRF primitive. `p256dh` must decode
  to 65 bytes and `auth` to 16.
- `unsubscribe` only needs the endpoint; knowing it is the capability.
- Both writes are rate limited per IP (30 per hour, in memory) and the body
  is capped at 8 KB, matching the App registration.

### Service worker

`public/sw.js`, served at `/sw.js` so its scope is the whole site.

- `push`: read `event.data.json()` and call `showNotification` with the
  title, body, icon, `tag` and `data.url`.
- `notificationclick`: focus an open tab on that URL, else `clients.openWindow`.
- `pushsubscriptionchange`: re-subscribe with the key fetched from
  `/api/push/web/key` and re-post; the locale is read from an IndexedDB copy
  written at subscribe time, and the next page visit corrects it if absent.
- No `fetch` handler, so the worker does not cache anything and cannot
  change how the SPA or its hashed assets load today.

`httpapi.staticSite` serves `sw.js` with `public, max-age=0, must-revalidate`
today; the route gets `Cache-Control: no-cache` explicitly so an updated
worker is picked up on the next visit.

### Sending

`push.Dispatcher` gains a web sender beside the Huawei client; both read the
same `Update` values at the same two call sites (`Dispatch` from ingest,
`FlushPending` after translation). The per-run fanout limit, the author
dedupe and the pending-post grace stay as they are and apply to both
transports together, so one ingest still produces at most eight
notifications in total.

Per subscription:

- Payload `{title, body, url, tag, icon}` built by the existing `wording`
  function for that row's locale; `tag` is the update's category so a newer
  live notification replaces an undelivered older one.
- `TTL` 24 hours; `Urgency: high` for `live_start`, `normal` otherwise.
- `201`/`200` is success and resets `failure_count`. `404` and `410` delete
  the row: the browser is gone. `413` is logged and the row kept (payload
  bug, not a dead subscription). `429` and `5xx` increment `failure_count`
  and are retried on the next run; a row is disabled at 10 consecutive
  failures, which keeps a broken push service from consuming every run.
- Sends run with a small concurrency (4) and a 10 second timeout each, so a
  slow push service cannot hold the ingest.

### Consent and switches

The site gets one bell button in the header and a small sheet with the three
switches, mirroring the App's settings:

- The button only renders when `serviceWorker` and `PushManager` exist.
- The click itself calls `Notification.requestPermission()`, then
  `pushManager.subscribe({userVisibleOnly:true, applicationServerKey})`, then
  posts the subscription. A permission prompt outside a gesture is ignored by
  Safari and discouraged everywhere else.
- `denied` replaces the button with the instruction to allow notifications
  in the browser's site settings; `granted` plus an existing subscription
  shows the switches directly.
- On iOS, `navigator.standalone !== true` shows "add to Home Screen first"
  instead of a prompt that cannot succeed.
- Unsubscribing calls `subscription.unsubscribe()` and then posts the
  endpoint, so the row disappears even if the write fails.

### Configuration

```
WEB_PUSH_VAPID_PUBLIC_KEY=
WEB_PUSH_VAPID_PRIVATE_KEY=
WEB_PUSH_VAPID_SUBJECT=mailto:kano@example.com
WEB_PUSH_TTL_SECONDS=86400
WEB_PUSH_ENDPOINT_HOSTS=
```

Empty keys leave the feature off: `/api/push/web/key` answers
`{enabled:false}`, the subscribe routes answer `503`, and the UI hides the
button. `WEB_PUSH_ENDPOINT_HOSTS` replaces the default allowlist, for a
self-hosted push service. A `kano-go webpush-key` subcommand prints a fresh
pair in base64url; the private key stays in the environment and is never
written to the database or the admin API.

### Admin

The existing push panel (`GET /api/admin/push`) gains a `web` section: row
count, enabled count, the 20 most recent subscriptions with a masked endpoint
and locale, and `DELETE /api/admin/push/web/{endpoint}`. The test action
takes a transport (`{"transport":"web","locale":"ja"}`) and sends the same
sample notification the App test uses.

### Upgrade

`web_push_subscriptions` is added to `bootstrap.sql`; `upgradeSchema` creates
missing tables from the reference schema, so an existing database gains it on
startup with no migration and no backfill.

## Consequences

- A reader can be notified without installing the App, and the same ingest
  produces both transports from one code path.
- Two new public write routes and one outbound HTTP client exist; both are
  bounded (allowlist, size cap, rate limit, timeout) and both fail softly.
- The site now carries a service worker. It handles push only, so it cannot
  break the SPA's caching, but it is a permanent part of the public surface
  and must keep working when the build changes.
- Dead subscriptions are retired by `410`; a browser whose push service
  silently stops accepts rows that stay until they fail ten times.
- Notification wording is shared with the App, so a wording change reaches
  both at once and needs no second translation table.
- iOS readers must install the site to the Home Screen; the UI says so
  instead of failing quietly.

## Alternatives

- **Server-sent events only.** `/api/dashboard/events` already exists, but it
  reaches an open tab only. It stays as the live-update channel; it does not
  answer "tell me when it starts".
- **A WebSocket.** Same limitation as SSE, plus a second long-lived protocol
  to operate behind the reverse proxy.
- **Email or a bot message.** Needs an address or an account, and the project
  has neither for public readers.
- **A hosted service (OneSignal, FCM for web).** Adds a third party that sees
  the subscriptions, a script on the public site and a second set of
  credentials; the standard is enough for one sender.
- **Hand-rolled RFC 8291 encryption.** `crypto/ecdh` makes it possible in
  about 200 lines, but the failure mode is silent (a payload the browser
  cannot decrypt) and the test vectors are the only guard. The plan uses
  `github.com/SherClockHolmes/webpush-go` for encryption and the VAPID JWT;
  if the dependency is rejected, this section is the fallback and the tests
  below are the same.

## Implementation plan

1. `bootstrap.sql` table; `web_push_subscriptions.go` (upsert, list, delete,
   failure accounting) with store tests.
2. `internal/push/web.go`: sender, VAPID configuration, payload builder;
   dispatcher wiring in `standalone.go` and the ingest path.
3. `httpapi` routes, validation, allowlist, rate limit; tests for
   `404`/`400`/`503`/`204`, `no-store` on the key, and a fake push service
   that asserts the encrypted body decrypts to the expected JSON.
4. Admin panel section and test action.
5. `public/sw.js`, `src/lib/web-push.ts`, header button, sheet, i18n in
   `ja`/`zh-Hans`/`zh-Hant`/`en`.
6. Docs: `.env.example`, `public-api.md`, `operations/configuration.md`,
   `architecture/backend.md`, `history/unreleased.md`.
7. Manual matrix: Chrome, Edge, Firefox, Safari 17 (macOS), Safari on iOS
   with the site installed, and one browser with notifications blocked.

## Open questions

- Should a notification body quote the post text, or stay generic on the lock
  screen? The App quotes it today; matching that is the default.
- Quiet hours: the App has none, so neither does this. If added, both
  transports should share the rule rather than the web alone.
- Do we want a per-subscription "test" in the admin list, or is the global
  test action enough for debugging one browser?
