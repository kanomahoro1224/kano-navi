# Conventions, Session and Settings

Shared response conventions, the session, the overview read and site settings.

## Conventions

- Every response carries `Cache-Control: no-store, no-transform` and is
  never compressed.
- All routes except `GET /session`, `POST /session` and `DELETE /session`
  require the admin session (production) and every unsafe method passes the
  Fetch Metadata / origin guard, including login.
- Unsafe methods answer `503 {"error":"read_only"}` when the process opened
  SQLite read-only. A missing runtime dependency (no job queue) answers
  `503 {"error":"unavailable"}`.
- Errors are `{"error": "<code>", "message"?: "<safe text>"}`. Validation is
  `400 invalid_input` with an operator-facing message; malformed JSON is
  `400 invalid_json`; oversized bodies are `413 request_too_large`; unknown
  resources are `404 <resource>_not_found`; unknown routes are `404 not_found`
  and wrong methods are `405 method_not_allowed` with `Allow`.
- Lists are `{"items": [...], "page", "pageSize", "total", "totalPages"}`.
  `page` is 1-based and clamped to the last page; each list documents its
  maximum `pageSize`.
- Single-resource writes answer with the updated resource under its own name
  (`{"event": ...}`); deletes answer `204`.
- Timestamps are ISO 8601 strings; dates are Japan dates (`YYYY-MM-DD`).
- No response contains secrets, provider ciphertext, `raw_json`, absolute
  paths or raw upstream error bodies.

## Session

| Method and path   | Body / query       | Result                                                  |
| ----------------- | ------------------ | ------------------------------------------------------- |
| `GET /session`    |                    | `{authenticated, mode, requiresPassword, requiresCode}` |
| `POST /session`   | `{password, code}` | session payload; `401 invalid_credentials`, `429`       |
| `DELETE /session` |                    | `204`; the HttpOnly cookie is cleared                   |

## Overview

`GET /overview` is the console home. It is read-only, bounded and does no
external work:

```json
{
  "visitorRegionEnabled": false,
  "activeRun": Run | null,
  "recentRuns": [Run],            // newest 6
  "scheduler": {"enabled", "running", "tickSeconds"},
  "timers": {"scheduled", "nextRunAt"},
  "queue": [{"kind", "source"?, "nextRunAt", "due"}],
  "backlog": {
    "extraction",       // posts queued for re-extraction
    "postRefresh",      // X posts with a refresh from the source queued
    "postTranslation",  // posts with a machine translation requested
    "videoRefresh",     // YouTube videos with an open refresh request
    "videoTranslation"  // video titles with a machine translation requested
  },
  "sources": {
    "x":       {"skipped", "lastSuccessAt", "lastStep"},
    "x_related": {"skipped", "lastSuccessAt", "lastStep"}, // lastSuccessAt: the newest related account's read
    "youtube": {"skipped", "lastSuccessAt", "lastStep", "push": {"enabled", "verified"}},
    "bilibili": {"skipped", "lastSuccessAt", "lastStep"},
    "bilibili_dynamics": {"skipped", "lastSuccessAt", "lastStep"}, // lastSuccessAt: the listing's
    "shop": {"skipped", "lastSuccessAt", "lastStep"}
  },
  "review": {
    "pendingImages",          // unlabelled images with a URL the model has not settled (pending, uncertain, failed)
    "suspectedCancellations", // live events whose review is `suspected`
    "uncertainEvents",        // live events whose review is `uncertain`
    "failedPosts",            // posts with a failed extraction or translation
    "failedExtractions",      // posts whose last extraction failed
    "failedTranslations",     // X and Bilibili posts whose current wording failed machine translation and still lacks a translation
    "videoMerges",            // unmerged Bilibili uploads the merge rule leaves to the operator
    "bilibiliForms"           // unmerged Bilibili uploads waiting for the operator's form decision
  },
  "counts": {"upcomingEvents", "events", "posts", "videos"}
}
```

`timers` counts the enabled [source schedules](sources.md#sources) this process runs
(not those disabled by `SKIP_*`) and the earliest planned run among them.
`queue` lists the timed work in the order it reaches the single-flight
queue: each of those schedules (`kind: "source"` with its `source`) and the
[live-status probe](../workflows/steps-and-timers.md#live-status-probe) when it has a next run
(`kind: "live-probe"`), by planned run and then by source, as the scheduler
picks due schedules. `due` is true once `nextRunAt` has passed; a due entry
waits for the current run and the entries due before it.

`backlog` counts the operator requests that wait for the step that does
the work: schedule extraction takes a re-extraction, the X step a post
refresh, the YouTube step a video refresh and the translate step a machine
translation. A video refresh request counts while it is open, for a day
after it was made. Each count matches the `queued` filter of
[`GET /posts`](posts.md#posts) or [`GET /videos`](videos.md).

`lastSuccessAt` is the newest successful incremental read of the source
(`sync_state.last_success_at`). `lastStep` is `{runId, status, finishedAt}` of
the source's step in the newest finished run among the last 20 that included
it, or `null`.

The overview's `visitorRegionEnabled` flag controls the public pages' optional
mainland China site suggestion. It defaults to `false`.

## Site settings

| Method and path                | Body                     | Result                   |
| ------------------------------ | ------------------------ | ------------------------ |
| `GET /settings/visitor-region` |                          | `{ "enabled": boolean }` |
| `PUT /settings/visitor-region` | `{ "enabled": boolean }` | `{ "enabled": boolean }` |

The setting is stored in `app_settings` as `visitor_region_enabled`,
`visitor_region_provider_name`, `visitor_region_provider_url` and
`visitor_region_mirror_url`. Enabling it requires a provider name and absolute
HTTP(S) URLs for the provider website and mirror site. It publishes the site
and the same-origin country-code check described in the [public API](../public-api/snapshot.md); it never sends an IP to a geolocation service.

### Image copies

| Method and path       | Query          | Result                |
| --------------------- | -------------- | --------------------- |
| `GET /media/variants` | `refresh` (=1) | `{groups, checkedAt}` |

`groups` is the [status](../media-cache/resized-copies.md#copy-groups) of the `lineblog`,
`works`, `history` and `media` groups in that order, each `{group, originals, ready,
unhashed, widths: [{width, copies, kept, missing}]}`, and `checkedAt` is when
it was listed. The listing is answered again until another run finishes, or
made anew with `refresh=1` ([Copies Run](../media-cache/resized-copies.md#copies-run)). A
process without the service's data directory and built site answers
`503 unavailable`. The copies themselves are made by
`POST /runs {"kind": "variants"}`.

### Works upload covers

| Method and path           | Query | Result                                                                   |
| ------------------------- | ----- | ------------------------------------------------------------------------ |
| `GET /media/works-covers` | none  | `{total, covered, due, waiting, uncovered, uploads, skipped, checkedAt}` |

The counts sort the packaged upload list's uploads by the
[cover rules](../media-cache/packaged-media.md#works-upload-covers): `covered` have a ready
cover, `due` are those the next media step looks up or queues, `waiting`
have an image or lookup on its way or waiting to retry, and `uncovered` have
every video gone. `uploads` lists every upload without a cover, newest first,
as `{id, date, state, videos}` with `state` `due`, `waiting` or `uncovered`;
each video, in cover order, is `{platform, id, members, state, code,
retryAt}` with `state` `due`, `waiting` or `gone`, the error code of its
last failed lookup or download and when that failure is tried again.
`skipped` is true while `SKIP_MEDIA` disables the media step, and `checkedAt`
is when the snapshot was read. The route reads the snapshot only and makes
no lookup; a process without the packaged list answers `503 unavailable`.
The covers are filled by `POST /runs {"kind": "workflow", "steps":
["media"]}`.
