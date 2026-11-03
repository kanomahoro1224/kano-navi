# Unreleased

> [!NOTE]
> The extraction prompt asks for a calendar label, which changes the extractor
> version. After upgrading, every known X post within the schedule lookback is
> extracted again on the following `schedule` runs, so model calls rise for a
> while until the backlog clears. Operator edits, locks and tombstones are kept.
> Startup adds `events.tag`, `events.tag_source` and `events.tag_attempted_at`
> to an existing database; no manual migration is needed. The admin event
> list (`GET /api/admin/events`) reports `tagSource` as `manual`, `llm` or
> `title`, and the `reconcile` result adds a `tagged` count.
>
> A video's `kind` is its form (`stream`, `premiere`, `upload` or `unknown`)
> and its content is a separate tag (ADR-0005). Startup adds
> `videos.manual_tag`, `videos.tag`, `videos.tag_source` and
> `videos.tag_attempted_at` and rewrites stored kinds once: an operator's
> `GAMEPLAY`, `TALK` or `MUSIC` correction becomes the matching manual tag,
> and every other kind is derived from the stored broadcast facts. A YouTube
> upload never read by a detail check becomes `unknown` until its next check
> or an operator's refresh. Clients that read `kind` values such as
> `LIVE ARCHIVE` or `MUSIC` must switch to the new forms and `tag`. Uploads
> have no tag until the translation queue judges them, a few per run, so the
> past-video categories fill in gradually and model calls rise until that
> backlog clears.
>
> Startup adds `videos.availability` and `videos.manual_availability` to an
> existing database, empty for every row; each video fills in at its next
> YouTube check, so nothing is hidden after the upgrade.

<details>
<summary><strong>Public pages</strong></summary>

- Every video projection adds `availability` (`public`, `unlisted`,
  `members`, `private`, `removed`, or `null` until a check could tell). A
  video that not everyone can watch shows Members only (in green, as on
  YouTube), Private or Removed beside its labels, and a private or removed
  video no longer links to YouTube; without another copy its cover fades.
  The stage's latest video passes over private and removed videos.
- Every video cover marks the platforms it is on, a single YouTube or
  bilibili mark included, not only videos on both.
- Members-only schedule entries take the green of the Members only chip, and
  video entries move to lavender.

- A calendar entry can carry a label (`live`, `game`, `talk`, `song` or
  `video`) in its `tag` field, so a client can show what kind of activity an
  entry is instead of guessing from its title. The label belongs to the event,
  so a notice without a video link can carry one too. Each read combines it
  with the entry's videos: an operator's label on the entry decides, then an
  operator's video tag, the entry's content label, the video's inferred tag
  and the entry's form label, so an entry and its video name the same
  content.
- The weekly guide and the month view show an entry's label with an icon. An
  activity label replaces a plain stream type, while members-only, event and
  release types stay beside it; colors still follow the type. A `song` label
  reads as music with a note on a release or video entry and as singing with
  a vocal microphone elsewhere. The Next Up ticket names gameplay, talk and
  singing entries by their label, and a members-only entry keeps its Members
  only chip beside it.
- Past videos shows four columns of covers on wide screens, three on narrow
  ones and two on phones. A month long enough to scroll keeps the same columns
  as a short one.
- Past videos filters a month by games, talk, songs or other, with a count on
  each button. The video's tag decides: the operator's, else the calendar
  label of its entry, else the title inference's. `GET /api/videos` takes the
  matching `category` filter and reports `categoryCounts`.
- Video `kind` is the form YouTube reports: `stream`, `premiere`, `upload`,
  or `unknown` before a detail check. Every video projection adds `tag`
  (`game`, `talk`, `song` or `null`), and the Atom feed names the form and
  the tag as categories. The video shelf sorts streams and works by the form
  alone, so an upload whose title mentions a stream stays a work.
- Every video shows its form and, beside it, its tag with an icon, so stream
  archives also say whether they were gameplay, talk or singing. A stream
  that may still be on air reads as a stream, not an archive, and one whose
  reported end has passed reads as an archive. Singing
  takes a vocal microphone and a music video or cover a note. Past videos'
  category buttons carry the same icons, and the Next Up ticket names a
  video's tag ahead of its entry's label.
- Clicking a schedule entry opens its details with the whole title, the
  visitor's own time outside Japan, and an Open link button, instead of
  following the link at once. In the month view an entry picks its day and is
  marked in the day list, which shows whole titles. Entries show
  Auto-detected and Confirmed as icons, and an arrow in their bottom corner
  still opens the link directly. The legend under the schedule names these
  marks and each activity icon.
- On wide screens the month view fills its window, and a day with few entries
  wraps their titles onto several lines.
- Closing the image viewer leaves the page where it was instead of scrolling
  back to the picture. On phones, LINE BLOG's monthly archive opens on a tap,
  so a scroll that starts on it leaves it shut.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- The schedule editor sets an entry's label, and an empty choice clears it. A
  saved label is the operator's decision, outranks the automatic one and keeps
  the entry from being relabelled later; a cleared entry shows no label and
  does not send its videos to the title inference. Saving an entry for another reason,
  such as a cancellation decision, keeps an automatic label automatic.
- The video editor corrects the form (`stream`, `premiere` or `upload`) and
  sets a content tag, which outranks the entry's label. The video list and
  `GET /api/admin/videos` show the tag with where it came from
  (`manualTag`, `scheduleTag`, `inferredTag`, `inferredTagSource`).
- The live-status probe lists the videos it is following and the ones it
  stopped, so an operator can stop checking one video or let it be checked
  again without turning the whole probe off
  (`PUT /api/admin/sources/youtube/live-probe/videos/{id}`).
- The video editor corrects who can watch a video (public, unlisted, members
  only, private or removed) over what the checks recorded, which they keep
  writing beneath it. `PUT /api/admin/videos/{id}` takes `availability`, and
  the video list and `GET /api/admin/videos` show `availability`,
  `sourceAvailability` and `manualAvailability`.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- The extraction prompt may return a label when the post or image states the
  activity. An unknown label is dropped without losing the entry, and a post
  that states no label keeps the label already on the entry. It shares the
  title prompt's label definition: `game`, `talk` and `song` win whenever the
  content shows, `live` and `video` only name the form otherwise, and a song
  premiere is `song` on either path.
- YouTube reservations never pass through that prompt, so the translation
  queue also labels them from their title (`tagSource = title`), with the
  queue's retries and provider status. A title the model cannot classify,
  or answers with a label outside the five codes, is recorded rather than
  asked again; a title whose request fails counts as failed, leaves the run
  going and is asked again a day later, after the titles not yet tried, so an
  outage marks nothing as decided and a title that keeps failing costs one
  attempt a day. The request names whether the entry is a stream
  or a premiere, and a creator's rename asks again about a title that gave
  no content.
- A rule merge lends an X notice's label to the event that survives, including
  merges made before the notice had a label, and replaces a label guessed from
  the title. Operator and locked entries keep their own label. Among
  automatic labels a form (`live`, `video`) never replaces content, on a merge
  or a new extraction, and a notice's content replaces an inherited form.
- YouTube and Bilibili titles no longer decide a video's kind. Listings store
  `unknown` and the detail check writes the form from the broadcast start
  and premiere verdict; a check that cannot tell keeps a stored stream or
  premiere. Bilibili uploads are `upload`.
- After the entries, the same translation queue tags public YouTube videos
  whose entries give no content, including entries that only carry a form
  from a notice and operator entries without a label, newest first, within
  the same per-run limit (`videoTags` in the run result). The title prompt now counts covers, originals and music videos
  as `song`.
- A stream made private after it ends no longer keeps the live-status probe
  failing every interval until 24 hours after its start. A watch page that
  reports a private video reads as private instead of passing for YouTube's
  bot check, Japanese reasons included, and a video the Data API leaves out
  whose page answers without details reads as private or removed. The probe
  stops asking about such a video and counts it in `unavailable` rather than
  failing the run, and asks again once a later check records it watchable.
- YouTube checks record who can watch each video: the Data API's privacy
  status (the `status` part, at no extra quota), the watch page's playability
  (a public waiting room included), and private or removed for a failed page only when YouTube answered it. A
  private or members-only page is no longer a run error, where a members-only
  page used to record `youtube_watch_details_incomplete`. A members-only video
  is a member entry whatever its title says.

</details>
