# Event Precedence

Which writer wins when manual edits, locks, tombstones and automatic updates meet.

`events.source` names the writer of an entry: `x` or `bilibili` for post
extraction, `youtube` for reservations and broadcasts, `shop` for the
[official shop](../sources/shop.md#official-shop)'s sale periods and `manual` for an
entry an operator created. Automatic X extraction, YouTube reservations and
shop reads use `provenance = automatic`.
Creating, editing, or confirming an event through `/admin` changes it to
`provenance = manual` and sets `manual_locked = 1`. Automatic upserts include a
database-level lock condition and cannot replace those rows. Manual deletion is
a soft-delete tombstone with the same lock, so synchronization cannot resurrect
it. An automatic retirement stays unlocked: new X evidence, a YouTube
reservation, or the reconciler's event for an ended broadcast of the same
event restores it. Manual operators can mark an event `manual_confirmed` with a required reason;
that decision is separate from the LLM overlay and is what the public dashboard
uses as a definitive cancellation. Editing or confirming an event that carries
an `llm_suspected` overlay keeps the overlay and its evidence unless the
operator explicitly dismisses it (`none`) or replaces it with a manual decision.
LLM matching compares start instants, so `Z` and `+09:00` offsets refer to the
same event. The dashboard hides tombstones, keeps cancelled rows visible with
their reason/evidence, and labels visible automatic/manual events.

A duplicate of the same activity is hidden by a merge decision rather than
deleted, with this precision rank: manual > YouTube reservation > X exact time

> X approximate time > date-only. Manual or locked events are never merged
> away but can receive merges, so an operator edit always wins; editing or
> confirming a rule-merged event shows it again. See
> [Duplicate merging](duplicate-merging.md).

X event IDs always include a hash of the date, time (or unknown), and normalized
title, independently of the number of events in a result. Matching evidence in
`event_sources` and the unique source tuple reuse existing IDs, including legacy
date IDs, manual locks, and tombstones. Identical activities across X posts share
an event; evidence attached by an old date-ID collision is reused only when it
matches the surviving row's source key, so re-extraction can separate those
activities. YouTube URLs use the video reservation identity. Replacing a source's
events and retiring its missing evidence happen in one transaction. An automatic
event retires only when no active source evidence remains.

Event and video ordering compares actual ISO instants rather than timestamp
strings. Date-only events follow timed events on the same Japan date without
inventing a start time; equal instants use event/video ID as the stable tie break.
The dashboard and paginated admin schedule apply this ordering. A linked YouTube
reservation with `manual_confirmed` cancellation is excluded from `nextStream`,
including after source refresh. An `llm_suspected` overlay remains review evidence
and does not definitively cancel its video.

Videos have additive `live_status` (`upcoming`, `live`, `ended` or null) and
`live_checked_at` columns, written by every successful watch-page check,
including the [live-status probe](../sources/youtube-live.md#live-status-probe). The
dashboard projects only a sanitized `liveStatus`: `ended` always, `live` and
`upcoming` while younger than two probe intervals. Startup adds the columns
empty to existing snapshots.

Selected profile-media rows have additive `active_sha256` and
`active_mime_type` columns describing the selected file independently of the
latest candidate download. `download_status` and `last_download_at` record the
latest attempt; `last_error` remains diagnostic. Every startup reconciles active
hash/MIME from the actual selected file, whatever the stored values. It preserves candidate metadata, the manual selection,
and unrelated snapshots/settings. Missing, unsafe, oversized, or unsupported
files leave active hash/MIME unavailable and cannot be served; a subsequent
startup can recover them after a valid selected file is restored. When active
hash/MIME changes, the repair increments the persistent `dashboard_revision`
once in the same transaction as all selected metadata updates. An unchanged
startup does not increment it. Open boards therefore follow the revision and
reload the repaired public profile snapshot, including its versioned avatar
URL.

`account_avatars` stores one state row per `(source, account_key)` (lowercase
X handle without `@`), with `mode` (`auto`, `manual`, or `hidden`),
`active_candidate_id`, and `updated_at`. `account_avatar_candidates` stores
candidate history (`sync`, `discover`, `url`, or `upload`) linked to either
`media_assets` for remote X downloads or a local file path under `data/avatar/`
for manual uploads. Each reported URL or upload advances its candidate's
per-account `observation_order`; download completion and failure do not change
that order. `auto` selects the most recently observed ready candidate, including
a previously cached URL that the source reports again. `manual` preserves the
operator's selection, and `hidden` suppresses public portraits. Startup adds a
missing `observation_order` column and initializes it from the stored update and
creation order without changing account selections. Download synchronization and
public revision rules are in [Account avatars](../media-cache/lifecycle.md#account-avatars).
Schedule image reviews add `llm_input_fingerprint` for input cache validation
(see [Schedule images](../sources/schedule-images.md)); a review without
one is rechecked when next scanned. Automatic
review writes update LLM columns only. Image responses compare the captured
`assets.asset_version`, URL and media hash inside a transaction; replacement,
including replacing and restoring the same URL, invalidates a waiting response.
Manual verdicts, reasons and decision timestamps survive concurrent model work.

Schedule assets keep publication in `updated_at` separately from inclusive
`period_start` / `period_end`, with `period_basis` (`explicit`, `relative`,
`unknown`, or `legacy`). `week_start` is the Monday of the applicable start,
not the publication week. When startup adds the period columns, it copies
each schedule asset's `week_start` into a seven-day `legacy` period, which does
not assert that those dates were verified.
Manual correction stores separate `manual_period_start`, `manual_period_end`,
and `manual_period_reason`; it preserves the inferred values, source and review.
Registration of the same image retains that correction. A different image URL
resets its review and correction. `manual_image_at` records when an operator
picked one of the post's images as the schedule image; synchronization then
leaves the asset's image, review and correction alone. Readers derive the
period of an asset with no stored or corrected one from its source post's
events (basis `events`, see [Schedule images](../sources/schedule-images.md));
nothing stores it. A period that is still unknown stays null until corrected
and is not assigned to a calendar week.

Complete post `media_urls` snapshots reconcile `post-image` ownership in the
same transaction as the post write. An empty list removes all display links;
omitted media fields preserve the known declaration. Failed source reads never
call this writer. Cache rows/files may remain after a link is removed.

An X post's `post-image` and `quote-image` links carry `kind`: `photo`, or
`video` or `gif` when the image is the thumbnail of one (see
[Media cache](../media-cache/layout-and-model.md#database-model)); other links and links the
source did not type keep `NULL`, which every reader takes as `photo`. When
startup adds the column, it marks the video and GIF links of every stored X
post whose `raw_json` types its images (`media_extended`, FxTwitter's
`media` or `mediaKinds`) once, without a request; a post whose payload holds
no types keeps photo links until its next refresh.
