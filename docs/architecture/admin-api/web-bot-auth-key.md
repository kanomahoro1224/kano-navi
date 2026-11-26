# Web Bot Auth key

Managing the Web Bot Auth signing key.

`WebBotAuthStatus`, from `GET /sources/shop/web-bot-auth` and in
`shop.webBotAuth`, describes the [Web Bot Auth](../web-bot-auth.md) key that
signs the shop's requests:

```json
{"state", "source", "keyId", "createdAt", "origin", "originSetting", "publicOrigin",
 "directoryUrl", "previous": {"keyId", "until"} | null, "environmentManaged"}
```

`state` is `active`, `no_key`, `no_origin` or `key_unreadable` (see
[Signing state](../web-bot-auth.md#signing-state)); `source` is `stored`,
`environment` or null without a key. `keyId` is the key's JWK thumbprint and
`createdAt` when the console made it (null for an environment key).
`origin` is the origin requests name as their agent, `originSetting` the
operator's and `publicOrigin` the normalized `PUBLIC_ORIGIN`;
`directoryUrl` is the key directory's address while signing is active.
`previous` is the key a rotation replaced while the directory still lists
it. No answer carries a private key or its sealed form.

`PUT` takes `{origin}`: a bare `https` origin (scheme and host, with an
optional port and trailing slash), or `""` or null to clear it so
`PUBLIC_ORIGIN` applies; anything else, or another field, is
`400 invalid_input`. `POST …/key` generates a key, or rotates: the new key
signs at once and the replaced one stays listed for 24 hours. `DELETE …/key`
removes the key and any replaced one. Both answer
`409 environment_managed` while `WEB_BOT_AUTH_KEY` is set, and
`503 secrets_key_unavailable` when the key cannot be sealed. Every write
answers the new status, applies without a restart, and answers
`503 read_only` on a read-only snapshot.

The related accounts answer is `{accounts, defaultKeywords, status}`:
`accounts` lists `{handle, keywords}` (see
[Related accounts](../sources/x-accounts.md#related-accounts)), `defaultKeywords` the rules
the console offers a new account, and `status` each account's
`{handle, lastSuccessAt, lastError, statuses, promoted}`, from its
`x-related` row and the pool. `PUT` replaces the list: at most 10 accounts,
each a valid handle (a leading `@` is dropped) listed once and not one of
`X_HANDLES`, with up to 30 distinct trimmed keywords of at most 100
characters; anything else answers `400 invalid_input` naming the problem.
Saving evaluates every pooled status against the new rules in the same
transaction, promoting and demoting, and moves the dashboard revision when a
post changed. Neither route contacts X; the next `x_related` run, or a
manual fetch of one account, reads the accounts.

`FetchMethod` describes one manual fetch method in the order the console
offers them: `{id, modes, maxCount, pageSize, requires, refresh, extract,
available}`. `requires` names the credential the method needs
(`X_API_BEARER_TOKEN`, `YOUTUBE_API_KEY`, `YTDLP_BINARY` or `a Bilibili login`), never its value, and is absent
for public methods; `available` is false while
it, or the method's adapter, is missing. `refresh` and `extract` say which
options the method takes.

`schedules` lists the [source schedules](../workflows/source-schedules.md#source-schedules) of
`x`, `x_related`, `youtube`, `bilibili`, `bilibili_dynamics` and `shop`, in
that order.
`SourceSchedule` is `{source,
enabled, intervalMinutes, minIntervalMinutes, maxIntervalMinutes, sync,
chain, nextRunAt, lastRun, skipped}`, plus `reason: "environment"` while `SKIP_*`
disables the source. `sync` is `{mode: "incremental", max}` (default, `max` 50) or `{mode: "count", count}`, each 1–200; `x_related` takes the incremental form only, and `sync` is null for both Bilibili timers and the shop.
`chain` is the source's [method chain](../workflows/source-schedules.md#method-chains):
`{methods, fixed}`, where `methods` lists `{id, enabled, available,
requires}` in the order reads try them. `available` is false while the
method's credential is missing, and `requires` names it
(`X_API_BEARER_TOKEN`, `YOUTUBE_API_KEY`, `YTDLP_BINARY`,
`a Bilibili login` (stored or from the environment),
`BILIBILI_APP_KEY and BILIBILI_APP_SECRET`, or `a Web Bot Auth key` for the
shop's `signed`, available while the [key](<>) is active)
for methods that need one. `fixed` is true for the
related-accounts timer, whose one method, `timeline`, cannot change. The shop
chain orders `signed`, `new_arrivals` and `atom`. The Bilibili dynamics chain orders `desktop` and `dynamics`
(the WBI listing); both require the configured login. The X chain adds
`apiGate`, `apiDailyCap`, `apiUsage` (`{day, posts}`, the posts X API
answers returned on the current Japan day) and `timelineFailures`, each
account's current streak (see
[X API in the chain](../sources/x.md#x-api-in-the-chain)).
`lastRun` is `{id, status, at}` of the newest run of the source's chain, from
its timer or Run now
(durable run status; `at` is when it finished, started or was queued), or
null. The `minIntervalMinutes` of both Bilibili timers is
`BILIBILI_MIN_INTERVAL_MINUTES`, and the shop's 180.
`PUT /sources/{source}/schedule` takes any subset of `{enabled,
intervalMinutes, sync, chain}` and keeps omitted fields. `chain` takes any
subset of `{methods, apiGate, apiDailyCap}`: `methods` is `[{id, enabled}]`
in the new order and must list every method of the source once with at
least one enabled; `apiGate` (0–100) and `apiDailyCap` (1–10,000) apply to X
only. It answers `404 not_found` for another source and `400 invalid_input`
for other fields, an interval outside the bounds, an invalid `sync`, a
`count` sync for `x_related` or a `sync` for either Bilibili timer or the
shop, and for a chain with an unknown or repeated method, a missing method,
no enabled method, a gate or cap out of range or for a source other than X,
or any chain for the related-accounts timer. A method may be enabled while its
credential is missing; reads skip it until it is set. Enabling a
schedule or changing its interval plans the next run one interval later,
with the timer's [random offset](../workflows/source-schedules.md#source-schedules); a sync
change alone keeps the plan.

`POST /sources/{source}/schedule/start` starts a timer, as **Start now** on
the overview queue and **Run now** on the Sources page while the timer runs
do: it makes an enabled schedule due at once (a schedule already due keeps
its place) and asks the scheduler to submit the earliest due schedule
immediately. While a run holds the queue the schedule waits as **Run next**
and starts after the current run and the schedules due before it. The
submitted run is the timer's own, so its claim plans the following run (see
[Source schedules](../workflows/source-schedules.md#source-schedules)). It answers
`404 not_found` for another source and `400 invalid_input` while `SKIP_*`
disables the source, while timers do not run in this process or while the
schedule is off.

`POST /sources/{source}/run` is **Run now** while the timer is off or
timers do not run in the process: it submits the source's fixed chain once, triggered by the operator and read with the schedule's sync
setting and method chain, and answers `202 {run}` or `409 {error: "busy", run}` while the
queue is occupied. It does not move the schedule's `nextRunAt`. It answers
`404 not_found` for another source and `400 invalid_input` while `SKIP_*`
disables the source.

`PushStatus` is `{available, enabled, callbackUrl, callbackPath,
secretConfigured, verified, leaseExpiresAt, nextRenewalAt, pending,
lastNotificationAt, lastTriggeredAt, lastError}`. The signing secret is never
returned.

`LiveProbeStatus` is `{enabled, intervalMinutes, minIntervalMinutes,
maxIntervalMinutes, available, lastRunAt, nextRunAt, activeVideos,
maxVideos, videos, stopped}`. `available` is false while timers or YouTube
are disabled for the process; `activeVideos` counts the videos the probe is
following, and one run checks at most `maxVideos` of them. `videos` lists
them earliest due first as `{id, title, url, scheduledAt, phase, dueAt}`, and
`stopped` lists the [stopped videos](../sources/youtube-live.md#stopped-videos) still stored,
newest stop first, as `{id, title, url, scheduledAt, stoppedAt, reason}` with
`reason` `unavailable` or `operator`. `nextRunAt` is when the next run is
planned (the earliest due video, at least two minutes after `lastRunAt`, and
no earlier than now), or null while nothing is active or the probe is off or
unavailable. `PUT` accepts either field, keeps an omitted one, and rejects
other fields and intervals outside 5–360 minutes with `400`.

`PUT /sources/youtube/live-probe/videos/{id}` takes `{stopped}` (required,
boolean) for one stored YouTube video: `true` stops the probe asking about it,
`false` lets it ask again. It answers the new `LiveProbeStatus`, `404
not_found` for a video that is not stored and `400` for an ID that is not a
YouTube video ID or another body.

`/sync-settings` holds the [sync settings](../../operations/configuration/sync.md#sync-settings).
`settings` maps each field (`xMaxStatusRequests`, `xRefreshKnown`,
`xBootstrapDays`, `xDiscoveryLimit`, `youtubeMaxDetailRequests`,
`fetchMaxPages`, `fetchTimeoutMs`, `fetchRequestDelayMs`,
`extractionLookbackDays`, `translationScanLimit`, `translationLookbackDays`,
`llmModelAttempts`) to its
integer value, and
`fields` lists `{field, default, min, max}` for each. `PUT` keeps omitted
fields and answers `400 invalid_input` for an unknown field or a value that is
not an integer within its range; the next run reads the saved values.
