# Videos

Reading and editing YouTube videos, Bilibili uploads and the Bilibili form.

| Method and path              | Body / query                                                                                                    | Result                                       |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------- | -------------------------------------------- |
| `GET /videos`                | `page`, `pageSize` (≤ 200), `status`, `kind`, `thumbnail`, `source`, `search`, `merge`, `translation`, `queued` | list plus `featuredVideoId`, `featuredVideo` |
| `PUT /videos/featured`       | `{videoId}` (`null` restores automatic selection)                                                               | `{featuredVideoId}`                          |
| `GET /videos/{id}/history`   |                                                                                                                 | `{video, revisions, thumbnails}`             |
| `PUT /videos/{id}`           | `{title?, kind?, tag?, availability?, thumbnail?, isShort?, work?}`                                             | `{video}`                                    |
| `POST /videos/{id}/refresh`  |                                                                                                                 | `202 {video, started, run}`                  |
| `POST /videos/bilibili`      | `{input}` (BV ID or bilibili.com video link)                                                                    | `{video, started, run}`                      |
| `POST /videos/youtube`       | `{input}` (YouTube video ID or link)                                                                            | `{video, started, run}`                      |
| `GET /videos/merges`         |                                                                                                                 | `{items}` merge suggestions                  |
| `GET /videos/forms`          |                                                                                                                 | `{items}` Bilibili form decisions            |
| `GET /videos/works-recent`   |                                                                                                                 | `{updatedAt, items}`                         |
| `GET /videos/{id}/merges`    |                                                                                                                 | `{items}` for one Bilibili upload            |
| `POST /videos/{id}/merge`    | `{into}` (YouTube video ID or watch link)                                                                       | `{video}`                                    |
| `POST /videos/{id}/unmerge`  |                                                                                                                 | `{video}`                                    |
| `POST /videos/{id}/separate` |                                                                                                                 | `{video}`                                    |

Each video's `thumbnailUrl` is the local `/media/<id>?v=<sha256>` URL of its
cached thumbnail, or `null` until that file is ready; the remote CDN address is
never returned. `thumbnailStatus` is `ready`, `pending`, `failed` or `missing`.
A video's `translation` is its title's translation summary (see
[Translations](models.md#translations)) when the translate step covers it, a YouTube
video or a public Bilibili upload, and `null` for a merged or unsettled
Bilibili upload.

`featuredVideo` is the pinned video in the same shape, whatever page or filter
is listed, or `null` while the board chooses automatically.

Changing the featured video advances the dashboard revision.

`status` is `published`, `upcoming` or `live`; `kind` is the shown form,
`stream`, `premiere`, `upload` or `unknown`; `thumbnail` is `ready`,
`present` or `missing`; `source` is `youtube` or `bilibili`. `search` matches
the shown title, the platform title or the video ID. `merge=visible` excludes
uploads merged into another video and `merge=merged` lists only them;
`translation` is `incomplete`, `complete` or `requested`, as for posts, over
the videos the translate step covers. `queued` keeps the videos with an open
operator request: `refresh` (a YouTube refresh request made within the last
day, which the YouTube step still takes first), `translation` (a machine
translation) or `any` of them.

## YouTube videos

`POST /videos/youtube` reads one video of the configured channel now and
stores it as a manual fetch stores a listed video and its details (see
[YouTube methods](../workflows/fetch-methods.md#youtube-methods)), so manual locks and
deletion tombstones hold; a stored video is read again and keeps a replaced
title as history. The input is a video ID or a `youtube.com` watch,
`/shorts/`, `/live/` or `/embed/` link or a `youtu.be` link. With
`YOUTUBE_API_KEY` the Data API describes it under the recent step's rules
(one `videos.list` call, plus the newest page of each format playlist for an
ended broadcast that needs a verdict); without one, or when the answer
cannot classify an ended broadcast, its watch page does. It then starts the
`media` step for the thumbnail (`started` is false while another run holds
the queue). Another input or field, a video of another channel, or a
process that skips YouTube answers `400 invalid_input`; a video YouTube does
not show publicly answers `404 {"error":"video_not_found", code}` with the
watch page's `youtube_watch_*` code, and other upstream failures `502` with
their `youtube_*` code.

## Bilibili uploads

`POST /videos/bilibili` reads one upload of the configured Bilibili account
now and stores it; `POST /videos/{id}/refresh` on a Bilibili video reads it
again the same way and answers `200`. Both start the `media` step for the
cover (`started` is false while another run holds the queue). An input that
is neither a BV ID nor a `bilibili.com` video link, or an upload of another
account, answers `400`; a video Bilibili does not list answers `404`. A
captcha answer, and any later read while the backoff lasts, answers `503
{"error":"bilibili_blocked", code, blockedUntil, force: true, message}`,
where `code` names the captcha answer or `bilibili_blocked` for the waiting
backoff; nothing is sent while it waits. Both reads accept `{force: true}`
in the body, which sends that one read anyway: a success lifts the backoff
and a failure extends it. Other upstream failures answer `502` with their
`bilibili_*` code. `bilibili` in
`GET /sources` reports the uploads' manual fetch `methods` and the
`oldest` stored upload's publication time, the account, the stored uploads, whether a login cookie
is configured (`signedIn`, never the cookie), `login` (`signed_in`,
`expired`, `unchecked` or `not_configured`) with `loginCheckedAt`, the read
interval and the
latest attempt, success, backoff and error. Its `dynamics` reports the stored
dynamics, their manual fetch `methods` and `oldest` stored dynamic, and the listing's latest attempt, success and error
(`bilibili_login_expired` when the cookie no longer signs in), and `skipped`:
`login_required` without a cookie, `login_expired` while the latest login
check found it expired, `blocked` during the backoff, otherwise null. Its `live` is the room as the last check saw it (`liveStatus` 0 off
air, 1 live, 2 rotation; `roomId` null until the first check) and
`lastLiveAt`, the last check that saw it live: the approximate end of the
last broadcast once the room is off air.

`POST /posts/bilibili` with `{input}`, a `bilibili.com/opus/<id>` or
`t.bilibili.com/<id>` link, reads that dynamic of the configured account from
its opus page now, stores it and starts the `media` step for its pictures,
answering `{post, started, run}` with the post in the list shape. Another
input or a dynamic of another account answers `400 invalid_input`, a
video or another non-post dynamic `400 {"error":"bilibili_unsupported_dynamic"}`,
one Bilibili does not show `404 {"error":"post_not_found"}`, and a captcha,
`force` and other upstream failures work as for uploads. The read takes the
dynamic as its page shows it, with no quote: forwards and their quotes come
from the listing, and the read never rewrites a stored forward (see
[Sources: Bilibili dynamics](../sources/bilibili-dynamics.md#dynamics)).

The live probe status adds `bilibili`: `{enabled, intervalMinutes,
minIntervalMinutes, maxIntervalMinutes, available, nextCheckAt, targets}`,
each target `{eventId, title, startsAt, phase, dueAt}` with `phase` one of
the YouTube phases or `baseline`.

## Bilibili form

Each video reports `formSource`: `manual` when `manualKind` is set, `rule` or
`llm` for a Bilibili upload whose form a rule or the model settled, and
`null` for an unsettled upload or a YouTube video's platform facts (see
[Bilibili form](../data-model/video-form.md#bilibili-form)). `formReview` is `uncertain`
or `failed` while an unsettled upload waits for the operator, and `public`
says whether public readers list the video. `PUT /videos/{id}` with
`{kind}` answers the form; a Bilibili upload accepts only `upload` or
`stream`, and anything else answers `400`.

`GET /videos/forms` lists, newest first and at most 100, the unmerged
Bilibili uploads waiting for the operator's form decision that nobody marked
private or removed. Each item is `{video, review, broadcast}`: `review` is
`uncertain` or `failed`, and `broadcast` is the broadcast the replay rule
read, `{source, eventId, title, startsAt, endsAt}` with `source` `entry` or
`room`, or `null`. The overview's `review.bilibiliForms` counts them without
the limit.
