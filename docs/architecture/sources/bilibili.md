# Bilibili

How Bilibili uploads and the live room are read. The login the reads send
is described in [Bilibili login](bilibili-login.md).

The `bilibili` step reads the uploads and the `bilibili_dynamics` step the
dynamics (动态) of `BILIBILI_UID` (Kano's account by default), each on its
own [timer](../workflows/source-schedules.md#source-schedules); the [live-status
probe](../workflows/steps-and-timers.md#live-status-probe) checks its live room. Every request
goes to one of five fixed hosts: `api.bilibili.com`, `www.bilibili.com`
(the opus page of one dynamic and the login refresh's correspond page only),
`api.live.bilibili.com`, `app.bilibili.com` (the uploads' official-client
method, below) and `passport.bilibili.com` (the
[login](bilibili-login.md)'s refresh, QR sign-in and sign-out). The upload list
uses only endpoints that need no request signature; its fallbacks are
signed (the dynamics listing with WBI, the official client's listing with
its app key):

- `/x/frontend/finger/spi` returns the anonymous device identity
  (`buvid3`/`buvid4`) Bilibili's own pages send; its `b_nut` is when it was
  issued. It is reused for seven days and kept in `sync_state` (source
  `bilibili`).
- `POST /bapis/bilibili.api.ticket.v1.Ticket/GenWebTicket` returns the
  `bili_ticket` cookie (a JWT valid about three days), signed as Bilibili's
  web script signs it (`key_id` `ec02`, `hexsign` an HMAC-SHA256 of
  `ts<unix seconds>`, `context[ts]`, an empty `csrf`). It is kept with the
  device identity and renewed six hours before it expires, and its answer
  also names the current WBI keys. A failed ticket request is not a
  failure: the read goes on without it.
- `/x/series/recArchivesByKeywords` lists the account's uploads newest
  first, 20 a page, with title, cover, publication time, length and
  description; the timed read takes the first page and a
  [manual fetch](../workflows/fetch-methods.md#bilibili-methods) pages on. Entries of
  another uploader are skipped.
- `/x/web-interface/view` reads one upload for an operator, and the cover
  address of an upload on the Works page.

Every request reads like Bilibili's own pages: one desktop browser user
agent (`BrowserUserAgent` in `backend/internal/source/bilibili/client.go`,
updated deliberately together with this page), `Accept-Language:
zh-CN,zh;q=0.9`, the page it comes from as `Referer` (the space for the
upload list and the listing, the video page for one upload, the home page
for the device identity, ticket and `nav`, `https://live.bilibili.com/` for
the live host) and, on the API hosts, that page's `Origin`. The device
cookies (`buvid3`, `buvid4`, `b_nut`, `bili_ticket` and
`bili_ticket_expires`) go to `api.bilibili.com` and the opus page, never to
the live host; the device-identity request sends none.

Bilibili still puts an address behind a captcha for hours at times. The
client never retries and names how Bilibili answered:
`bilibili_risk_control_http` (HTTP 412), `bilibili_risk_control_page` (an
HTML captcha page, or an opus page without its state),
`bilibili_risk_control_352` and `bilibili_risk_control_412` (the JSON codes)
and `bilibili_risk_control_voucher` (a `v_voucher` instead of the listing);
`-799`/`-509`/429 are `bilibili_rate_limited`. Each starts the same backoff:
the adapter waits 6 hours, doubled for each consecutive block up to a day,
and neither step makes an `api.bilibili.com` or `www.bilibili.com` request
until then (the upload read may still ask `app.bilibili.com`, below), and
only an `api.bilibili.com` success clears it; independently, upload reads are at least
`BILIBILI_MIN_INTERVAL_MINUTES` (180 by default) apart, which is also the
shortest interval of either Bilibili timer. A skipped read records
`{ skipped: true, reason: "interval" | "blocked" }`; a failed one records its
code as a failed step without a retry, keeps every stored upload and post,
and lets the rest of the run continue.

An optional [login](bilibili-login.md), the one the server stores or the
environment's (`BILIBILI_COOKIE` or `BILIBILI_SESSDATA`), signs the
`api.bilibili.com` requests in (never the device-identity or ticket request,
the opus page or the live host). Signed-in reads meet the captcha far less
often, and both dynamics methods require the login. It is verified at most
once a day through `nav` and once right after a QR sign-in, and a stored
login is refreshed once a day at the start of either Bilibili step.

The upload read tries the enabled methods of the Bilibili timer's
[method chain](../workflows/source-schedules.md#method-chains) in its order, by default `list`,
`dynamics`, then `app`. When one fails (a captcha included), or the
device-identity request the web API methods share fails, the same read goes
on at once to the next. Besides the upload list (`list`, above), they are:

1. `dynamics`, the first page of the signed-in
   [dynamics listing](bilibili-dynamics.md#dynamics), when a login is configured that the latest
   check did not find expired and the device identity was read, storing
   the uploads its video dynamics show: one listing request, plus `nav` when
   the login check or the WBI keys are due. It leaves the dynamics read's own
   clock and the dynamics themselves to the `bilibili_dynamics` step;
2. `app`, the official client's space listing,
   `app.bilibili.com/x/v2/space/archive/cursor?vmid=<mid>&ps=20&order=pubdate`,
   when an app key pair is configured (`BILIBILI_APP_KEY` and
   `BILIBILI_APP_SECRET`, see [Configuration](../../operations/configuration.md)):
   one request without cookies or Referer,
   signed as Bilibili's apps sign (`appkey` and `ts` added, `sign` the MD5 of
   the key-sorted query followed by the app secret). It answers a different
   service from the web API that shows the captcha. Its ordinary
   uploads (`goto` `av`) with a BV ID, title and publication time (`ctime`)
   are stored with their cover and length; it carries no description, so a
   stored one is kept. The listing names no uploader, so a co-created upload
   (`is_cooperation`), which may be another account's, is left to the other
   listings.

A method without its credential (the login, or the key pair) is passed
over without a request, and a read none of whose enabled methods can run
fails with `bilibili_no_method` without a request or a backoff. The step
result names the method that answered as `listing`. When an earlier method
failed, the step is `partial` with that method as `fallback`, the first
failure's code as `listError` and each later failure in `fallbackErrors`.
A `list` or `dynamics` success is an `api.bilibili.com` answer and clears
the backoff an earlier failure started; an `app` success keeps it. While a
backoff lasts, an upload read due by its interval asks only the official
client's listing when the chain enables it and a key pair is configured
(`listError` `bilibili_blocked`) and is skipped as `blocked` otherwise; that
read never extends the backoff. When every method fails, the step keeps the
first failure's code and adds each later one as `<method>: <code>` to its
errors; the backoff starts once, at the first captcha among them.

The dynamics and the official client's listing date an upload by their own
clocks and the latter omits its `aid`, so an upload either of them reads is
partial: it keeps a stored upload's `published_at` and `aid` (the upload
list still corrects them), and a missing `aid` never replaces a known one.

Signing with an official client's key makes those requests read as that
client rather than as this service. That is a trade-off each deployment
accepts by configuring the pair, so the repository carries none: the pair
grants no account access, and when Bilibili changes it the `app` method fails
with a controlled code until the operator configures the new one.

An operator's single read (one upload, one dynamic) answers
`bilibili_blocked` during a backoff without a request, unless the operator
forces it once from the console (`force`, see
[Admin API](../admin-api/videos.md#bilibili-uploads)). A forced read that succeeds
lifts the backoff; one that fails extends it as usual. Timer reads and
[manual fetches](../workflows/fetch-methods.md#bilibili-methods) are never forced.

Uploads are `videos` rows with `source = 'bilibili'` and the BV ID as their
ID, linked to `https://www.bilibili.com/video/<BV>`. The list and view
endpoints carry no live-replay flag, so a new upload is stored as an
unsettled `upload` and its form (`upload` or `stream`) is settled later by
rule, model or operator (see [Bilibili form](../data-model/video-form.md#bilibili-form));
Bilibili's categories say nothing about the content, which the
[video tag](../data-model/video-form.md#video-form-and-tags) carries. The description and
`aid` stay in `raw_json`. Every list read replaces the other platform fields
and keeps a replaced title or cover in `video_revisions`, as YouTube does;
the form and operator corrections survive. Covers and dynamic pictures come
only from `*.hdslb.com`, upgraded to HTTPS, and are cached under
`data/bilibili/` by the media step.

An upload whose form is settled is a public video like a YouTube one, on the
board, the video archive, MCP and the Atom feed, until it is merged. The
`reconcile` step merges an upload into the YouTube video of the same work
(a live replay into the stream archive of the same broadcast), which then
lists it in `alternates` and shows its title as the video's Chinese platform
translation, so one work is never listed twice; operators confirm, add or
undo merges in `/admin` (see
[Video merging](../data-model/videos.md#video-merging)). Bilibili answers no
availability for a listed upload; an operator marks a deleted or hidden one
`removed` or `private` through `manual_availability`. Operators can
add an upload by BV ID or `bilibili.com` video link and read one again at
once from `/admin` (see [Admin API](../admin-api/videos.md#bilibili-uploads)); those
reads respect the same backoff and accept only uploads of the configured
account. They run outside the workflow queue, so the state write keeps a
block that a concurrent read stored meanwhile (one still active and later
than the writer's), with its failure count and code: a captcha backoff is
never lost. A read that changes a merged upload's title rewrites the
translation in the same transaction.

## Bilibili login boundary

The dynamics methods require a signed-in cookie because anonymous requests depend on browser fingerprint parameters and encounter captcha responses. The server therefore skips the methods without a login and keeps the cookie in memory for the Bilibili API only. An operator can still add one dynamic through its public opus page, which does not require the listing credential.

## Live room

The account's live room is not configured: each live check learns it.
The [live-status probe](../workflows/steps-and-timers.md#live-status-probe) asks
`api.live.bilibili.com/room/v1/Room/get_status_info_by_uids?uids[]=<mid>`,
which needs no cookie, about the account's room in one request, and keeps
the answer's `room_id` with the rest of the state. Only `live_status` 1 is a
broadcast (0 is off air and 2 a rotation of recordings, which is not live);
`live_time` is its actual start. A live answer without `live_time` is live
with an unknown start: the broadcast already seen, or one that started by
that check; it never ends a broadcast. The live host has its own captcha backoff,
with the same 6-hour doubling. The state (room, title, status, start,
checked and attempted times, backoff) is kept in `sync_state` (source
`bilibili-live`, the mid as account), together with the start of the last
broadcast seen live, the last check that saw it live and the first later
check that saw the room off air, and the start and observed end of the
broadcast before it when a check saw that one end.

The same answer names the account's avatar (`face`). An operator's
[profile media discovery](../admin-api/profile-media.md#profile-media) asks
for it once, on request, and keeps it only on Bilibili's image CDN
(`*.hdslb.com`, upgraded to HTTPS); that request leaves the live check's
state and backoff alone.

Entries are tied to the room by their [platforms](../data-model/events.md#event-platforms),
never by a link: the room's link is the same for every broadcast, while
readers treat an entry's link as the identity of one broadcast. Public reads
add the room as `liveUrl` and as the bilibili link to every `stream` or
`member` entry shown on Bilibili once a check learned the room, so entries
extracted before the first check get it too and a changed room needs no
rewrite. A broadcast starting at L is matched against the active, timed
stream and members-only entries that are not merged away or a confirmed
cancellation, all of them rather than one read's, so every page agrees:

1. the entry shown on Bilibili (its platforms, with those its merged
   entries lend it, include `bilibili`) starting at S when
   S−2h ≤ L ≤ S+3h and not ended before L (its `ends_at`), the nearest of
   several;
2. otherwise the YouTube event whose video a check saw `live` or `ended`,
   whose broadcast started within 30 minutes of L and had not ended before
   L, the nearest of several: the broadcast is its simulcast, and its shown
   platforms gain an observed `bilibili`. A reservation that has not started,
   or that no check has seen, never takes it.

One broadcast matches one entry at most. The entry the previous broadcast
matched takes no broadcast that starts more than 30 minutes after a check
saw that one end: it is another broadcast, not a restart. That entry is `live` while the
check is younger than two Bilibili check intervals, and `ended` once a
later check saw the room off air, unless its YouTube broadcast already
reports a state; a live room always makes it `live`. The last check that saw the
broadcast live is its approximate end, kept in the state and shown in the
console; it is never written as the entry's `ends_at`. Bilibili has no
reservation state, so no entry is `upcoming` from the room; an entry's own
start drives that. A live broadcast that matches no stored entry creates no
entry: the dashboard reports it as `liveNow` (see
[Public API](../public-api/snapshot.md)). Raw answers, counters
and errors stay in the console.
