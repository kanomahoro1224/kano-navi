# Feeds, MCP and Other Endpoints

Public feeds, the MCP boundary, the WebSub callback and the Web Bot Auth key directory.

## Public Feeds

`GET /api/calendar.ics` is an iCalendar (RFC 5545) subscription of the events
dated 30 days before to 90 days after today's Japan date, at most 500 events,
each once: events from today on fill the cap first and the most recent past
events take what is left. They are read through the same bounded calendar
pages and event projection as `/api/calendar`. The calendar names no time
zone, so it reads the same in every viewer's zone: times are UTC, and an event
without a time, or of `unknown` precision, is a floating all-day entry
(`DTSTART;VALUE=DATE` with an exclusive `DTEND` the next day) on its Japan
date. All-day entries also carry `X-MICROSOFT-CDO-ALLDAYEVENT:TRUE`, which
keeps Outlook from drawing them across two days; timed entries do not. A
timed event without a stored end after its start lasts an hour when it is a
stream (the `stream` and `member` types the board treats as streams) and half
an hour otherwise, since RFC 5545 would make it zero-length. A sale period
is written at its ends, as the board shows it: its opening (titled "Sales
open:" in the feed's language) and, when it closes on a later date, its close
("Sales close:"), or the close alone for a deadline (a close on the opening
date without a stored opening time; a stored time of `unknown` precision
makes an all-day opening, as on the board). A closing entry with a
time ends at that time and starts half an hour earlier. A UID is the
public event ID with `@kano-navi`; the closing entry of a period adds
`-close` to the ID. `LAST-MODIFIED` is the event's stored
`updated_at`, and `SEQUENCE` is that time in whole seconds since 2020-01-01
UTC, so it grows whenever the event is written and clients replace a changed
entry; an event without a parseable update time has neither. `STATUS` is
`CANCELLED` only for a cancelled status or an operator's confirmation
(`manual_confirmed`), `TENTATIVE` for a suspected cancellation or a
`tentative` status, and `CONFIRMED` otherwise. The description carries the
event's detail, its provenance and its public link; only absolute HTTP(S)
links are written. Text is escaped and lines are folded at 75 octets without
splitting a character.

`GET /api/feed.xml` is an Atom 1.0 feed of at most 50 public videos (YouTube
videos and reservations and unmerged Bilibili uploads, as the video archive
lists them) dated 30 days before to 11 days after today, newest first. An
entry is dated by its publication (a future time is dated at the start of
today's Japan day until it passes, so the entry does not move between polls),
says when a reservation is scheduled, and links the video and any
merged platform copy. Its categories name the form (`stream`, `premiere`,
`upload`, `short` for a Short, or `video` while the form is unknown) and
then the video's tag, when it has one. Entry and feed IDs are `urn:kano-navi:` URNs. The
server never builds links from the request's `Host`: with `PUBLIC_ORIGIN` the
feed links the site absolutely and adds a `rel="self"` link to
`/api/feed.xml`; without it the site link stays `/` and there is no self link.

Both take an optional `lang` query, `en`, `ja` or `zh-CN`; a missing or
unknown value is the English base, so the bare paths stay English. Titles
resolve [translations](../data-model/translations.md) the way the board does:
each reader gets the locale's translation when one exists, except that
Japanese readers keep wording detected as Japanese (an event only through its linked
video's title). The fixed wording the server writes — the calendar and feed
names (the board's `site.name`), the event descriptions' provenance and
unofficial notes, the generated YouTube details (`YouTube Live`,
`YouTube Premiere` and the older stored forms it still recognizes) and the
Atom subtitle and summaries — comes from a small table in
`backend/internal/feeds/locale.go`. Stored data is unchanged: only
rendering picks the language. A non-English Atom feed declares `xml:lang`, has
the feed ID `urn:kano-navi:feed:<lang>` and its self link keeps the `lang`
query.

Both answer `GET` and `HEAD`, never fetch platforms or models, negotiate gzip
like the [public JSON reads](../backend.md#text-transfer-and-static-caching), and
send `Cache-Control: public, max-age=600` with a weak ETag that validates both
encodings. The ETag names the language, the build version (so an upgrade's
rewording is not hidden by a cached copy) and a hash of the rendered body.
Every calendar `DTSTAMP` is the start of today's Japan date, so the
calendar changes only with the snapshot, the date and the build; its ETag
also names the dashboard revision. The Atom feed also follows the clock,
since a reservation turns into an aired video when its time passes. A
matching `If-None-Match` answers 304. A database failure answers 503 without
an ETag.
The routes sit under `/api/`, so the development proxy and the
[read-only mode](../backend.md#read-only-mode) serve them unchanged.

## MCP Boundary: `/mcp`

`/mcp` uses stateless Streamable HTTP. `dashboard_read`, `schedule_list`,
`posts_list`, `health_read` and `sync_status` are public read-only tools and
do not require a credential. Public `sync_status` lists the recent jobs' kind,
status and times only: it carries no job ID and takes no `jobId`. Control
tools are only registered for requests carrying
`Authorization: Bearer <MCP_CONTROL_TOKEN>`: they can advance a dashboard
revision, start an asynchronous sync, or run the automatic schedule scan. For
those requests `sync_status` also reports each job's `id` and looks a job up
by `jobId`, so a started job can be followed. The token is independent of
`ADMIN_PASSWORD` and browser sessions.

A JSON-RPC batch holds at most 100 messages and runs at most 10 tool calls,
since each can read a whole snapshot. Later `tools/call` requests in the same
batch are answered with the tool error `Too many tool calls in one batch (at
most 10)` without running; other requests in the batch are answered as usual.

The endpoint deliberately has no tool for manual event confirmation, editing,
deletion, bulk approval, profile-media selection/upload, SQL, file paths, DOM
commands, or arbitrary URL proxying. Human confirmation remains a human-only
workflow at `/admin`.

## YouTube WebSub Callback: `/api/websub/youtube`

The route is active only while YouTube push is enabled in `/admin`
(`/api/admin/sources/youtube/push`); otherwise it answers like an unknown API
path. It answers Google's hub verification for this channel's topic and
accepts signed Atom notifications, which only wake a queued YouTube sync. See
[Sources](../sources/youtube-live.md#websub-push) for the settings, verification, signature,
debounce and queue rules.

## Push Device Registration: `/api/push/device`

`POST /api/push/device` upserts one HarmonyOS device for Huawei Push Kit
delivery: the token, a device id, optional platform, locale and app version,
and the switches `enabled`, `live`, `video` and `post`, which default to on
when the App omits them. A registration retires the same device's previous
token, so a rotated token cannot deliver a second copy of every update, and
repeating one is harmless. The route answers `{ "ok": true }`; a missing or
oversized token or device id answers `400`, and a read-only snapshot answers
`503 push_unavailable`. It carries no session because a fresh install has none;
the token it stores is what a delivery targets. See
[Configuration](../../operations/configuration/environment.md#huawei-push-and-bambino-ingest) for the service account that
lets the process send and [Data model](../data-model/sqlite-snapshot.md) for `push_devices`.

## Browser Notifications: `/api/push/web/*`

`GET /api/push/web/key` reports `{enabled, publicKey}` and is `no-store`. An
empty VAPID pair answers `{ "enabled": false }` rather than an error, so the
site hides its bell instead of retrying.

`POST /api/push/web/subscribe` stores one browser: the endpoint, the
`p256dh` and `auth` keys, an optional locale and the switches `live`, `video`
and `post`, which default to on. The route is idempotent, and an omitted
switch keeps the stored value. The endpoint must be `https` on a known push
service: the server posts to whatever it stores, so an unchecked URL would be
a request forgery primitive. Both write routes are limited to 30 calls per
address per hour, answer `204`, and reject an unsafe endpoint, a wrong key
length or a malformed body with `400`. `POST /api/push/web/unsubscribe` takes
the endpoint alone; knowing it is the capability. A read-only snapshot answers
`503`.

## Bambino Ingest: `/internal/ingest/bambino`

BambinoStatus posts normalized X updates here so the board can carry a post
before the next source read. A body holds at most 50 events and 2 MiB, and is
authenticated by `X-Bambino-Timestamp` (Unix seconds inside the tolerance
window) and `X-Bambino-Signature`, the HMAC-SHA256 of that timestamp, a
newline, and the body under `BAMBINO_INGEST_SECRET`. The route answers
`404 ingest_disabled` while the secret is empty and rejects a stale or
mismatched request before reading it as data. Accepted batches queue the
post-processing chain the sync steps use, and a catch-up ticker re-submits it
when the single-flight queue was busy.

## Web Bot Auth Key Directory

`GET /.well-known/http-message-signatures-directory` (and `HEAD`) serves the
public key that signs the shop source's outbound requests, as
`application/http-message-signatures-directory+json`: a JWKS with one Ed25519
key (`kty`, `crv`, `x`) and never a private member. The response carries its
own `Signature-Input` and `Signature` over `("@authority";req)`, tagged
`http-message-signatures-directory`. It answers `404 {error: "not_found"}`
while Web Bot Auth signing is not active and 405 to other methods. See
[Web Bot Auth](../web-bot-auth.md#key-directory).
