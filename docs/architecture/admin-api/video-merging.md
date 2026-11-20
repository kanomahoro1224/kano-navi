# Video Merging and Corrections

Merging videos and recording corrections and their history.

## Video merging

A Bilibili upload merged into the YouTube video of the same work (see
[Video merging](../data-model/videos.md#video-merging)) reports `mergedInto`,
`mergedIntoTitle`, `mergeSource` (`rule` or `manual`), `mergeReason` and
`mergedAt`; `mergeSource: "manual"` with a null `mergedInto` is an undone
merge. Every video lists the uploads merged into it in `alternates`
(`{id, source, title, url, mergeSource, mergeReason, mergedAt}`).

`POST /videos/{id}/merge` merges a Bilibili upload into the YouTube video
`into` names, by ID or `youtube.com`/`youtu.be` link, as an operator decision
the rule never changes. A rule merge of another upload into that video is
released; another operator merge into it answers
`409 {"error":"merge_conflict"}`. An `{id}` that is not a Bilibili upload,
or an `into` that is not a stored YouTube video, answers `400`; an unknown
`{id}` answers `404`. `POST /videos/{id}/unmerge` undoes a merge, rule or
manual, and keeps the upload separate (`mergeReason: "unmerged_from:<id>"`);
an upload that is not merged answers `400`. Both advance the dashboard
revision and write or remove the target's Bilibili translation in the same
transaction. `POST /videos/{id}/separate` keeps an unmerged Bilibili upload
apart (`mergeSource: "manual"`, `mergeReason: "kept_separate"`), so the rule
never merges it; the board is unchanged, so the revision stays. A merged
upload answers `409 {"error":"merge_conflict"}` (undo it instead), an
`{id}` that is not a Bilibili upload `400` and an unknown one `404`.

`GET /videos/merges` lists, newest first and at most 50, the unmerged
Bilibili uploads the rule would merge (`status: "rule"`), or leaves to the
operator (`ambiguous`, or `unverified` when no candidate satisfies every
rule, or when only a video published more than six hours apart matches the
song title and length), without writing; the overview's `review.videoMerges` counts the
`ambiguous` and `unverified` uploads without the limit. Uploads an operator
kept separate are left out.
Each item has `bilibili` (the upload as listed) and up to five `candidates`:
`{youtube, complete, titleMatch, broadcast, durationDelta, gapMinutes}`,
where `durationDelta` is null when a length is unknown and `broadcast` marks
the stream archive of the broadcast a live replay replays (the rule's only
candidate for it). `GET /videos/{id}/merges`
describes one upload whatever its state, adding `merged`, `separate` and
`none`.

`GET /videos/works-conflicts` lists the stored Bilibili uploads whose merge
disagrees with the packaged Works list: the list pairs the upload with other
YouTube videos than the one it is merged into, or with stored ones while it
is unmerged. Each item is `{upload, bilibili, mergedInto, worksYoutube}`. The
list only reports; the Review queue's merge decisions show the items.

`GET /videos/works-recent` lists, newest first, at most 200 of the videos
dated after the packaged Works list's `updatedAt` (also returned) that the
Works page shows, and those the operator's Works decision keeps out, with
`total` counting them all, under the rules in
[Uploads after the collection date](../../development/works-archive.md#uploads-after-the-collection-date).
Each item is `{id, source, title, url, publishedAt, scheduledAt, kind, tag,
isShort, availability, work, origin, thumbnailUrl}`: `work` is the decision
(`true`, `false` or `null`), `origin` is `auto` or `operator` for a listed
video and `null` for an excluded one, and `thumbnailUrl` is the local URL of
the ready cached thumbnail or `null`. It reads the snapshot only. The Site
page lists them, excludes an automatic one and returns a decided one to the
rules; the video editor's Works page field sets the decision on any video.

## Video corrections and history

An operator correction replaces a platform value on the board without
changing it: `title`, `kind`, `isShort`, `availability` and `thumbnailUrl`
show the correction, while `sourceTitle`, `sourceKind`, `sourceIsShort` and
`sourceAvailability` keep what the platform reported and `manualTitle`,
`manualKind`, `manualIsShort`, `manualAvailability` and `manualThumbnail`
(`{mediaId, sha256}`) are the corrections themselves. `manualWork` is the
operator's Works decision (`true` includes the video among the Works page's
uploads after the collection date, `false` excludes it, `null` leaves it to
the rules).
`tag` is the shown content tag: `manualTag`, else `scheduleTag` (the label of
the video's schedule entry), else `inferredTag` (the title inference's), with
`inferredTagSource` set once the inference judged the video, with or without
a tag (see [Video form and tags](../data-model/video-form.md#video-form-and-tags)).
`PUT /videos/{id}` sets a field from a string, for `isShort` from a boolean
(`true` for a Short, `false` for any other upload), for `work` from a
boolean (`true` includes, `false` excludes), or for `thumbnail` from
one `{mediaId, sha256}` file listed in the history; `null` clears it and an
omitted field keeps it. A title is 1–300 characters after trimming; a kind is
`stream`, `premiere` or `upload`, a tag `game`, `talk` or `song`, and an
availability `public`, `unlisted`, `members`, `private` or `removed`, compared
without case. Other fields,
unknown files and invalid values answer `400`, an unknown video `404`. A
change advances the dashboard revision, and machine translations follow the
corrected title. The feed names the format of its newest uploads only (see
[Sources](../sources/youtube-videos.md#shorts)), so `sourceIsShort` is `null` for older ones and
the console's Format field is how an operator moves one into the board's
Shorts row.

`GET /videos/{id}/history` lists, newest first, the titles and thumbnails the
platform replaced (`revisions`, each with `origin` `sync` for a new title or
thumbnail URL and `media` for new bytes at the same URL) and the cached
`thumbnails` an operator may pin: the platform's current file (`current`),
earlier files of the same URL and files of earlier URLs. `selected` marks the
pinned one. Every URL is a local `/media/<id>?v=<sha256>` address.

`POST /videos/{id}/refresh` records `refreshRequestedAt` on a YouTube video
and starts the `youtube` and `media` steps; the YouTube step reads requested
pages before any other detail work. `started` is false while another run
holds the queue, and the request then waits up to 24 hours for the next run
that reads YouTube. A successful page read clears it. Each video also reports
`durationSeconds`, `checkedAt` (its last successful page read) and its number
of `revisions`.
