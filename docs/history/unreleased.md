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
> Startup also adds the `push_devices` table, which holds the HarmonyOS push
> registrations and their switches, and `web_push_subscriptions`, which holds
> the browsers that subscribed from the site; nothing is migrated. Browser
> notifications stay off until `WEB_PUSH_VAPID_PUBLIC_KEY` and
> `WEB_PUSH_VAPID_PRIVATE_KEY` are set (a pair comes from `kano-go
webpush-key`), and the bell stays hidden while they are empty. It adds
> `push_pending`, which holds an accepted X post until the translate step
> settles its wording and the other languages are notified.

<details>
<summary><strong>Public pages</strong></summary>

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
  singing entries by their label.
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
  that may still be on air reads as a stream, not an archive. Singing
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
- A bell in the X section's header subscribes the browser to notifications.
  The click asks for permission, registers the browser and shows three
  switches (streams, videos, posts); the message carries the site avatar, a
  title and the post or video text, and a tap opens the page. A blocked
  browser, or an iPhone that has not added the site to its Home Screen, gets
  an instruction instead of a prompt that cannot succeed.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- The Videos page gains a Calendar label, one of `live`, `game`, `talk`, `song`
  or `video`. The App's calendar shows it for the video's event instead of
  reading the title, so a label set in the console needs no App release; an
  empty choice reads the title again.
- The Videos page's live filter reads the broadcast state, so it lists the
  streams that are on air instead of answering with an empty list.
- The console says Edit where it used to say Correct, for the same fields.
- The schedule editor sets an entry's label, and an empty choice clears it. A
  saved label is the operator's decision, outranks the automatic one and keeps
  the entry from being relabelled later. Saving an entry for another reason,
  such as a cancellation decision, keeps an automatic label automatic.
- The video editor corrects the form (`stream`, `premiere` or `upload`) and
  sets a content tag, which outranks the entry's label. The video list and
  `GET /api/admin/videos` show the tag with where it came from
  (`manualTag`, `scheduleTag`, `inferredTag`, `inferredTagSource`).
- The Push page lists the browsers that subscribed from the site beside the
  App's devices, with their locale, switches and failure count. A test goes to
  every browser or to one, and removing a row stops its notifications.

</details>

<details>
<summary><strong>Push notifications</strong></summary>

- `POST /api/push/device` registers a HarmonyOS device with its Huawei Push
  Kit token and four switches (all notifications, live, video, post); the App
  calls it on launch, after a token rotation and whenever a switch changes.
  Registering again retires the same device's previous token. The device's
  `locale` is the App's push language setting, which decides both the
  notification wording and the body language of a deferred post.
- An accepted X post notifies the devices reading Japanese at once with the
  original wording. Every other language waits in `push_pending` until the
  translate step records a settled translation for the post, and the step's
  completion then sends one notification per language with the translated
  body; Traditional Chinese readers receive the Simplified body. A post whose
  translation never settles is sent with its own wording after a ten-minute
  wait.
- Huawei Push Kit delivery needs an AGC service account JSON file in
  `HUAWEI_PUSH_SERVICE_ACCOUNT`; while it is empty the process starts without
  push. `HUAWEI_PUSH_CATEGORY` names the category approved for the app, and
  `HUAWEI_PUSH_TEST=true` marks test messages.
- BambinoStatus posts normalized X updates to `/internal/ingest/bambino`,
  signed with `BAMBINO_INGEST_SECRET` and accepted inside
  `BAMBINO_INGEST_TOLERANCE_SECONDS`; accepted batches queue the
  post-processing chain, and `BAMBINO_INGEST_CATCHUP_SECONDS` re-submits it
  when the queue was busy.
- A post accepted from that route reads the same type and label as one the
  timeline parser stored, so a schedule notice looks the same either way.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- A chat-completions provider that takes only `max_tokens` and
  `response_format: json_object` (DeepSeek) no longer fails its request with
  HTTP 400: the token limit keeps its portable name, and the JSON schema
  travels in the system prompt when the provider cannot take it in
  `response_format`.
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
  going and is asked again after the titles not yet tried, so an outage
  marks nothing as decided. The request names whether the entry is a stream
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

</details>
