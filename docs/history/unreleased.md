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

<details>
<summary><strong>Public pages</strong></summary>

- A calendar entry can carry a label (`live`, `game`, `talk`, `song` or
  `video`) in its `tag` field, so a client can show what kind of activity an
  entry is instead of guessing from its title. The label belongs to the event,
  so a notice without a video link can carry one too.
- The weekly guide and the month view show an entry's label with an icon. An
  activity label replaces a plain stream type, while members-only, event and
  release types stay beside it; colors still follow the type. The Next Up
  ticket names gameplay, talk and singing entries by their label.
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
  archives also say whether they were gameplay, talk or singing. Singing
  takes a vocal microphone and a music video or cover a note. Past videos'
  category buttons carry the same icons, and the Next Up ticket names a
  video's tag ahead of its entry's label.
- Closing the image viewer leaves the page where it was instead of scrolling
  back to the picture. On phones, LINE BLOG's monthly archive opens on a tap,
  so a scroll that starts on it leaves it shut.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- The schedule editor sets an entry's label, and an empty choice clears it. A
  saved label is the operator's decision, outranks the automatic one and keeps
  the entry from being relabelled later.
- The video editor corrects the form (`stream`, `premiere` or `upload`) and
  sets a content tag, which outranks the entry's label. The video list and
  `GET /api/admin/videos` show the tag with where it came from
  (`manualTag`, `scheduleTag`, `inferredTag`, `inferredTagSource`).

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
  queue's retries and provider status. A title the model cannot classify is
  recorded rather than asked again; a title whose request fails counts as
  failed, leaves the run going and is asked again after the titles not yet
  tried, so an outage marks nothing as decided.
- A rule merge lends an X notice's label to the event that survives, including
  merges made before the notice had a label, and replaces a label guessed from
  the title. Operator and locked entries keep their own label.
- YouTube and Bilibili titles no longer decide a video's kind. Listings store
  `unknown` and the detail check writes the form from the broadcast start
  and premiere verdict; Bilibili uploads are `upload`.
- After the entries, the same translation queue tags videos that no entry
  labels, newest first, within the same per-run limit (`videoTags` in the
  run result). The title prompt now counts covers, originals and music videos
  as `song`.

</details>
