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
>
> Startup adds `posts.repost_json`, the original status of a repost. It fills
> the column for every stored X post whose `raw_json` carries a nested
> `retweeted_tweet`, so a repost stored earlier gains its author without a
> re-fetch; no manual migration is needed.

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
  related account's reposted status is translated in the pool; another
  author's is stored untranslated, and an operator translation request for
  it queues nothing, until an operator promotes it.
- `PUT /x-statuses/{id}/decision` accepts `promote` for any reposted status,
  whatever its author; keyword rules still promote related accounts'
  statuses alone.
- Public dashboard, post archive and MCP posts add `repostedBy`: the own X
  accounts that reposted a promoted post, first sighting first, and `[]` for
  every other post.
- Public dashboard, post archive and MCP post media and quote media add
  `kind`: `photo`, `video` or `gif`.
- Startup adds `media_links.kind` and, once, marks the video and GIF links of
  stored X posts whose `raw_json` types their images, without a request.
  Other links read as photos until their post is refreshed.
- Public API adds `GET /api/journey`, `GET /api/works` and
  `GET /api/works/upload-list`: the Journey and Works pages' data with every
  language and same-host image URLs, from the build and without a revision.
  `GET /api/works/uploads` is unchanged.
- Public API adds the LINE BLOG archive reads `GET /api/lineblog`,
  `GET /api/lineblog/posts/{id}` and `GET /api/lineblog/posts/{id}/comments`:
  the article list and months, one article with its blocks and neighbours,
  and its archived comments, locale-neutral and with same-host media, served
  from data embedded in the release.

</details>

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
- A repost card says Reposted above it, names the original author with the
  author's cached avatar instead of the reposting account, and shows the
  original wording and images. The open link goes to the original post. An
  older repost without a cached avatar shows a letter placeholder until the
  collector supplies one.
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
- A reposted post the operator picks shows on the wall and in Past posts as
  its author's post, marked Reposted by and the account of Kano's that
  reposted it, under All only.

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
- The console has a Browser push page of its own, separate from the App's
  Push Kit page: subscriptions that came from the site, with their locale,
  switches and failure count. A test goes to every browser or to one, and
  removing a row stops its notifications. Test mode sends that test as an
  important notification, which is what Windows shows while Do Not Disturb is
  on; every real notification is important regardless.
- A console page reloads its data when its tab becomes visible again, so a
  switch a reader flips on the site shows up without a manual refresh.
- The operator console uses the same labels in English, Japanese and
  Simplified Chinese, including Site, source timers, works covers,
  thumbnails, external posts and the queue.
- Every own X post in Posts has Refresh from source in its actions, with a
  Refresh queued badge, whether or not it quotes a status; the quote card
  keeps only its hide and open actions.
- Posts selects own X posts on a page and queues their refresh from the
  source in one request; Review's extraction lists offer the same bulk
  refresh beside Process again.
- External posts lists every status an own account reposted under Reposted,
  with the reposting accounts, and promotes or demotes any of them by hand
  with a reason, as a lock. A promoted repost is translated as a post.

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
  timeline parser stored, so a schedule notice looks the same either way. A
  repost notification carries the original wording and opens the original
  post.

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
- A vxtwitter detail answer that carries `retweet` is recorded as the
  account's repost of the original status and never stored as a post, on
  the pending queue, known-post refreshes and operator refreshes alike.
- X API answers record each repost's own status ID, so a stored post under
  it is removed in the same way.
- Each X image records whether it is a photo or a video's or GIF's
  thumbnail, from the detail answer, the timeline and the X API alike. Video
  and GIF files are never downloaded.

</details>

<details>
<summary><strong>Runtime and development</strong></summary>

- `npm run lineblog:export` writes the server's copy of the LINE BLOG
  archive from the tracked page data, and the frontend tests fail when the
  committed copy differs.
- Profile images load when the data directory is reached through a symbolic
  link, as macOS reaches its temporary directories; a symbolic link in place
  of the avatar directory is still refused.

</details>
