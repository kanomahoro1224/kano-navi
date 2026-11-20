# YouTube Live Status and WebSub

How live status is probed and how WebSub pushes reach the service.

## Live-status probe

Whether a stream is on air is read from YouTube rather than guessed from its
start time. A separate probe step (`live`) asks only about the videos it
checks, through the Data API when `YOUTUBE_API_KEY` is set and the YouTube
chain enables `api`, and otherwise their watch pages, with no feed, playlist or streams page, for at most five
(`ytsource.MaxLiveProbes`) stored videos per run:

- reservations whose scheduled start is within the next 15 minutes or has
  passed;
- broadcasts that started without a known end.

Both are limited to starts in the last 24 hours. Videos the page already
reported as over, events whose `ends_at` has passed, manually confirmed
cancellations and [stopped videos](#stopped-videos) are left out. Each active video is due by its phase: once before
its start, every 3 minutes from the start until the page reports it live (for
up to 45 minutes), then every operator interval (see
[Workflows](../workflows/steps-and-timers.md#live-status-probe)). A run requests only due videos,
the earliest due first and, among equals, never-checked videos before the
least recently checked, so failures cannot starve the rest. With nothing due,
the probe makes no request and queues no run.

`ParseWatch` keeps its completeness checks (matching ID, title, start time for
an upcoming page) and then reads `videoDetails.isLive` / `isUpcoming` and
`microformat.playerMicroformatRenderer.liveBroadcastDetails` (`isLiveNow`,
`startTimestamp`, `endTimestamp`). It exposes `LiveNow` (live and not yet
ended) and `LiveStatus`: `upcoming`, `live`, `ended` (an end time, or
`isLiveNow: false` on a broadcast that is not upcoming), or empty when the page
names none. A live or ended broadcast's `startTimestamp` is its actual start
and replaces the reservation time.

With a key and the chain's `api` method on, one `videos.list` call
(`snippet`, `contentDetails`,
`liveStreamingDetails`, `localizations`) asks about all of a run's due videos
before any watch page, and is read as in the [Data API](youtube.md#data-api) detail
check, including the [localized titles](youtube-videos.md#localized-titles).
`liveBroadcastContent` `upcoming` with a `scheduledStartTime` is `upcoming`;
`live` without an `actualEndTime` is `live` and sets `LiveNow`; an
`actualEndTime` is `ended` and writes the event's `ends_at`. The
`actualStartTime`, when present, is the actual start and replaces the
reservation time, as `startTimestamp` does. A length marks an upcoming or
live premiere, and an ended broadcast takes the verdict of the
per-format playlists, read as in the recent step, or else the stored
`is_premiere` verdict, which the probe loads with its targets. A video's
watch page is still read when the call fails (its code is recorded and
every video falls back), when the answer leaves the video out, when it
names no broadcast state, or when it cannot classify the video, such as an
ended broadcast with neither verdict or an upcoming one without a
start. If that page fails too, an ended broadcast with an API start is
written unclassified, as in the recent step. A page that asks to sign in
(`youtube_watch_login_required`) of a video the answer did not leave out ends
the run's page requests the same way: the remaining videos are left as if
their pages failed. `apiRequests` in the run
result counts the calls; `requested` counts watch pages. Each run costs one
quota unit whatever the number of due videos, two more when an ended
broadcast needs the per-format playlists, and nothing while nothing is
due; the probe's worst case of 30 runs an hour is 720 units a day, and
together with a regular YouTube step every 15 minutes (two units per run)
stays under a tenth of the default 10,000. Without either the probe reads
watch pages only.

The store writes `videos.live_status` and `live_checked_at`, the actual start,
`is_upcoming`, `ends_at` once reported and the status text through the same
upsert as the regular step, which keeps an ended broadcast
[ended](../data-model/event-precedence.md). It never adds a video, never moves the feed cursor
or detail queue, keeps manual locks and tombstones, and increments
`dashboard_revision` only when a public video or event field changed. A Data
API answer goes through the same upsert. In the same transaction it records
when each video was asked about, by the API or its page, in the
`youtube-live` `sync_state` row. A video neither answered keeps its
snapshot and is asked again when its phase next makes it due. Each request
uses the client's existing 10-second timeout, 4 MiB body limit and redirect
rejection; the key travels only in the `X-Goog-Api-Key` header.

### Stopped videos

A video the probe can learn nothing more about is stopped rather than asked
again until its 24 hours run out. It is unavailable when its failed page
tells it `private` or `removed` (see [Video availability](youtube-videos.md#video-availability)),
which also records that availability. A members-only page records `members`
without stopping the video, and a page that was not answered at all (a
timeout, a 5xx) or the bot check stops no video. An unavailable
video is not a run error: the run result counts it in `unavailable`, and a run
whose only due videos were unavailable succeeds. A later regular or probe
check that records the video `public`, `unlisted` or `members` lifts an
`unavailable` stop, so a video made watchable again is asked about again. An
operator can also stop any active video, or let a stopped one be asked again,
from `/admin`; only the operator lifts an `operator` stop.

Stops are kept in the same `youtube-live` `sync_state` row as the requests,
with when and why (`unavailable` or `operator`), and are forgotten with them
after two days. A stop leaves the video's stored broadcast state alone: a
`live` or `upcoming` result simply ages out of the board's freshness window,
and the video shows its start-time fallback. Letting a video be asked again
makes it a target again while its start is within the probe's window.

The public dashboard projects `liveStatus` (`live`, `upcoming`, `ended` or
null) on videos and on the events linked to them. `ended` is final; `live` and
`upcoming` count only while the check is younger than two probe intervals, so
stale results fall back to the start-time grace. A fresh `live` video stays
listed beyond that grace. Run timing is described in
[Workflows](../workflows/steps-and-timers.md#live-status-probe).

## WebSub Push

Polling remains the baseline. The operator turns push on in `/admin`
(Sources → YouTube → YouTube push) by entering the deployment's public HTTPS
address; the console adds `/api/websub/youtube` and saves the result as the
callback URL. The standalone process then subscribes the channel's
feed topic at Google's hub (`https://pubsubhubbub.appspot.com/subscribe`)
within seconds, without a restart, and again at every startup.
`backend/internal/source/youtube/websub` owns the protocol and its admin
module:

- `GET /api/admin/sources/youtube/push` returns the settings and runtime status:
  enabled, callback URL and required path, whether a secret exists,
  verification, lease expiry, next renewal, pending notification, last
  notification/trigger, the last fixed error code, and `available` (false
  while `SKIP_YOUTUBE=1`). `GET /api/admin/sources` carries the same object
  as `youtube.push`. Neither ever includes the secret.
- `PUT /api/admin/sources/youtube/push` accepts `{ "enabled": boolean, "callbackUrl":
string | null }`; omitted fields keep their value. The callback must be an
  absolute HTTPS URL of at most 2048 characters without credentials or a
  fragment. Enabling generates a 256-bit signing secret when none exists.
  `POST /api/admin/sources/youtube/push/rotate-secret` replaces it and resubscribes.
  Settings live in `app_settings` (`youtube_websub_enabled`,
  `youtube_websub_callback_url`, `youtube_websub_secret`); the secret is sealed
  with `LLM_SECRETS_KEY` like provider keys. A secret that no longer decrypts
  after a key change is replaced at startup.
- Disabling push or moving the callback sends an `unsubscribe` for the previous
  callback (at most three attempts; its lease expires anyway). While disabled,
  the callback route answers like an unknown API path, except to confirm that
  withdrawal once within an hour.
- The subscribe request asks for asynchronous verification, a five-day lease,
  and the HMAC secret. It has a 10-second timeout, rejects redirects, discards
  the response body after 64 KiB, and records only fixed codes
  (`websub_hub_timeout`, `websub_hub_http_error`, `websub_hub_denied`, and so on).
  A failed request is retried after 15 minutes.
- `GET /api/websub/youtube` echoes `hub.challenge` (at most 1 KiB) only for this
  channel's topic, and only for the subscription or the withdrawal this
  process requested within the last hour, while enabled for a subscription.
  Any other verification answers like an unknown path, so a stranger's
  subscribe request with this callback cannot replace the signing secret. The
  confirmed verification updates the lease, so renewal happens at half the
  granted lease but no more than hourly. A verification that arrives after a
  restart for the previous process's request is refused too, and the missing
  verification below requests the subscription again. When no verification arrives within an hour, the
  status records `websub_verification_missing` and the subscription is
  requested again.
- `POST /api/websub/youtube` reads at most 256 KiB and verifies
  `X-Hub-Signature` (`sha1`, `sha256`, `sha384` or `sha512`) in constant time.
  Unsigned, mis-signed, malformed, deleted-entry, or other-channel bodies are
  acknowledged with `202` and ignored, as WebSub requires.
- A verified notification for a valid video ID only marks a sync as pending.
  After one quiet minute (which also lets the public RSS list the entry), and at
  least two minutes after the previous push-triggered run, it starts the
  YouTube schedule's chain (`youtube`, `media`, `translate`, then
  `reconcile`), read with that schedule's sync setting, through the operator
  job path. The run is labelled
  `websub` and shares the single-flight queue; a busy queue keeps it pending and
  retries every 30 seconds.

The notification never writes snapshots; the RSS and stream-page reads above
do. Live start/end and reservation-time changes are not reliably pushed, so
reservation rechecks still depend on the YouTube schedule, and live start/end
on the [live-status probe](#live-status-probe). Runtime state is in
memory: a restart subscribes again, and a missed push is caught by the next
poll.
