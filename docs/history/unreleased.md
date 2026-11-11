# Unreleased

> [!WARNING]
> An X detail answer that shows a stored own post to be the account's
> repost of another status (one an archive import stored, say) deletes that
> post with its media links, schedule-image candidate, extraction state and
> history, translations and refresh request, and retires the automatic,
> unlocked events only it supported; manual and locked events stay. The
> repost's ID is then never stored as a post again. Back up `./data` before
> queuing refreshes of imported posts.

<details>
<summary><strong>Upgrade notes</strong></summary>

- Admin adds `POST /posts/refresh` with `{ids}` (1–200 post IDs), which
  queues a refresh from the source for each own X post in one transaction
  and answers `{queued}`; unknown IDs, other sources and promoted statuses
  are skipped.
- `POST /posts/{id}/refresh` answers `400 invalid_input` for a promoted
  related-account status, which the own accounts' synchronization never
  reads.
- Startup adds `x_reposts.retweet_id` and its index, and marks every pooled
  X status an `x_reposts` row names as reposted (`reposted_at` from the
  earliest sighting), without a request.
- A pending X status whose detail answer is the account's repost leaves the
  queue with the new code `x_status_repost` in `pendingDropped`, without a
  failure or retry.
- Every own-account repost a listing or detail answer carries enters the X
  status pool under the `reposted` reason, whatever its author. Only a
  related account's reposted status is translated; another author's is
  stored only and is never promoted. An operator translation request for it
  queues nothing.
- Public dashboard, post archive and MCP post media and quote media add
  `kind`: `photo`, `video` or `gif`.
- Startup adds `media_links.kind` and, once, marks the video and GIF links of
  stored X posts whose `raw_json` types their images, without a request.
  Other links read as photos until their post is refreshed.

</details>

<details>
<summary><strong>Public pages</strong></summary>

- Past posts and Past videos keep one header height as you change months or
  scroll through years: the month controls and the filters stay on one row
  wherever the window is wide enough for both. Past posts names its platforms
  above the title instead of every account, and both dialogs show their total
  only on the filter's buttons.
- Changing months in Month view and Past posts no longer collapses the
  dialog while the month loads: the calendar keeps its dates in place and
  Past posts shows outlines of its cards, on phones as well. A month without
  entries says so beside the calendar (below it on phones) rather than above
  it.
- Month view jumps to any month: select the month name to pick a year and
  month, as in Past posts.
- After a jump to an earlier month in Past videos, Show newer videos loads
  the next newer videos above the list, keeping what you were looking at in
  place, instead of returning to the newest.
- A post's or quoted post's video or GIF shows its still with a play mark or
  a GIF tag, on the board, in the post archive and in the image viewer, and
  opens the post on X instead of the viewer.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- Every own X post in Posts has Refresh from source in its actions, with a
  Refresh queued badge, whether or not it quotes a status; the quote card
  keeps only its hide and open actions.
- Posts selects own X posts on a page and queues their refresh from the
  source in one request; Review's extraction lists offer the same bulk
  refresh beside Process again.
- External posts lists every status an own account reposted under Reposted,
  with the reposting accounts.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- A vxtwitter detail answer that carries `retweet` is recorded as the
  account's repost of the original status and never stored as a post, on
  the pending queue, known-post refreshes and operator refreshes alike.
- X API answers record each repost's own status ID, so a stored post under
  it is removed in the same way.
- Each X image records whether it is a photo or a video's or GIF's
  thumbnail, from the detail answer, the timeline and the X API alike. Video
  and GIF files are never downloaded.

</details>
