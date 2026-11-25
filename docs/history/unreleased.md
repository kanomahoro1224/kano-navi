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
> `WEB_PUSH_VAPID_PRIVATE_KEY` are set (a pair comes from
> `kano-go webpush-key`), and the bell stays hidden while they are empty. It adds
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

- `GET /api/posts/months` adds `accounts` (`x`, `bilibili`) and
  `accountAvatars`.
- New command `kano-go x-archive` (`import`, `check`, `apply`) imports X
  account exports and reviews the posts an earlier import added; see
  Database operations, X Account Exports. A first import of an account
  outside `X_HANDLES` adds the app setting `x_archive_accounts`.
- Startup adds the columns `posts.upstream_deleted_at`, `posts.upstream_kind`
  and `posts.shown_at` and moves the automatic hides `posts.hidden_at` held
  (an account-export import and its review, related-account demotion) into
  them once: a reviewed post takes its outcome as its kind and a `gone` one
  its deleted-on-X time, an unreviewed post becomes `unlisted`, each stored
  X answer gives its post's kind, and `hidden_at` keeps only operator hides.
  Every post keeps its visibility, except that a post whose stored X answer
  says it is a reply to another account, another author's status or a repost
  leaves the public pages, and a post a review found gone takes its schedule
  image with it. It rebuilds `posts_account_instant_idx`. A read-only
  instance refuses a snapshot without the columns, so start the writable
  service on the snapshot once to upgrade it. An earlier release reading the
  upgraded snapshot would show the posts an import or a review kept off the
  public pages, so roll back only by restoring the backup taken before the
  upgrade.
- The admin API's posts report `upstreamDeletedAt`, `upstreamKind`,
  `shownAt`, `demoted` and `visible` and take a `visibility` filter;
  `PUT /api/admin/posts/{id}/visibility` (`auto`, `shown` or `hidden`)
  replaces `PUT /api/admin/posts/{id}/hidden`.
- `kano-go x-archive check` reads every post still `unlisted`, those whose
  wording opens with another account's handle included.
- An event `view.state` may be `ongoing`, in the dashboard, calendar, MCP
  and every other read that carries the view.
- Startup adds the empty tables `alert_incidents` and `alert_deliveries`.
  Operator alerts are on by default and send nothing until a channel is
  added in `/admin` → Alerts; `ALERTS_ENABLED=0` turns them off. The
  outbound proxy pool gains the `notify` area, off until routed.
- The admin overview reports `alerts.open`; new routes under
  `/api/admin/alerts`.
- Startup adds `event_sources.unlisted_at` and the `schedule_asset_reviews`
  columns `previous_manual_status`, `previous_manual_reason`,
  `previous_manual_checked_at`, `check_status`, `check_reason`, `check_at`
  and `llm_image_sha256`, all empty, to existing snapshots.
- Startup adds the post indexes `posts_account_instant_idx` and
  `posts_bilibili_account_idx` to existing snapshots. A read-only instance
  serves an older snapshot without them, only more slowly.
- A post's public `label` (dashboard, post archive, MCP) is English text:
  `DAILY`, `SCHEDULE` or `ANNOUNCEMENT`. Startup rewrites the stored
  `DAILY / 近况` and `SCHEDULE / 日程` labels once. A configured extraction
  keyword that matched a post only through `近况` or `日程` in its old label
  no longer matches it.
- Admin adds `POST /posts/refresh` with `{ids}` (1–200 post IDs), which
  queues a refresh from the source for each own X post in one transaction
  and answers `{queued}`; unknown IDs, other sources and promoted statuses
  are skipped.
- `POST /posts/{id}/refresh` answers `400 invalid_input` for a promoted
  related-account status, which the own accounts' synchronization never
  reads.
- Startup adds `x_reposts.retweet_id` and its index, retains every distinct
  repost's own ID in `x_repost_ids`, backfills that table from existing
  repost links, and marks every pooled
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
  the article list and months, one article with its blocks, comment count
  and neighbours,
  and its archived comments, locale-neutral and with same-host media, served
  from data embedded in the release.
- Every LINE BLOG archive video is H.264 with AAC audio: thirteen VP9 and
  AV1 copies are re-encoded under their names, so the media catalog's sizes
  and hashes change. Update `data/lineblog/` from the recovery archive (pull
  it, or copy its `media/` again) before deploying; it no longer splits any
  video.

</details>

<details>
<summary><strong>Public pages</strong></summary>

- Every page and the operator console sit on warm ivory paper, or warm
  charcoal in the dark theme, with near-neutral ink and a softer strawberry
  accent; the status colours share one lightness and strength. Text on a
  coloured fill stays readable in the dark theme, and a system setting for
  more contrast darkens secondary text and rules.
- Posts get a page of their own, `/posts`, in the site menu: every stored
  post in one timeline by month and day, with a card per account and a map
  of every year's months and totals to jump through, in an address that
  names the month and account. The board's **Past posts** opens it in place of the
  month dialog.
- The post archive offers MKLNtic, the unit account Kano shares, as an
  account of its own; its posts stay off the board's wall and latest post.
- A post deleted on X leaves the wall, Past posts, the API and MCP, and its
  schedule image leaves the board and the calendar, as do the automatic
  events it is the only source of. They return when X lists it again.
- A video confirmed on air is the board's next stream and stage ticket even
  when a waiting room without a start time was published earlier; such a
  waiting room comes after every scheduled video.
- A Featured Short reports that it may be the stage's latest video in its
  view, as the stage already treats it.
- MCP `schedule_list` reports the same event state as the board: an ended
  stream is not live in the grace after its start, a waiting room the
  probe saw stays upcoming, and a YouTube premiere on air is live.
- The board shows only snapshots with projection version 1 and takes the
  stage, the on-sale list, the video shelf and every event and video state
  from it, without a local fallback. A snapshot stored by a board from
  before v0.7.0, or one served with another projection version, is not
  shown; the board reads a fresh one.
- An offline event between its start and end is in progress: the week
  keeps its card whole with an **In progress** chip, and the stage shows it
  as happening now, without a countdown or On air, until its end or a
  stream about to start.
- A stream confirmed on air before its scheduled start is live on the board
  and the stage.
- A date-only stream or membership is no longer shown live from midnight to
  3:00 the next day, and the board refreshes when that day ends.
- A premiere past its start with neither an end nor a confirmed broadcast
  leaves the stage as an aired video.
- An event with a cancelled status stays cancelled when a model also
  suspects its cancellation.
- A sale with only an opening date shows its opening on that date rather
  than a closing.
- The board's Links section is one row of the resources the profile card
  does not already link, each naming what it is on hover, then the common
  hashtags; the milestone card leaves the board for the Journey page in the
  site menu.
- Phones list the newest three posts on the board; Past posts has the rest.
- The video shelf's Coming up leaves out the reservation the stage already
  shows as next up, and the stream archives preview is one row.
- The week's day list folds two or more empty days in a row into one line
  naming the first and last of them.
- On phones the Journey page shows a smaller portrait, sets the identities
  two to a row, and shows a milestone's images as a row of small
  thumbnails.
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
- Every video in the LINE BLOG archive plays in Safari and on iPhone, not
  only those YouTube had kept as H.264.
- In Safari and on iPhone, the weekly schedule folds past entries to two
  lines of title and cuts other entries' titles at three lines, as in other
  browsers, instead of showing every title whole.
- A reposted post the operator picks shows on the wall and in Past posts as
  its author's post, marked Reposted by and the account of Kano's that
  reposted it, under All only.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- A new **Alerts** page sends the operator a message through ntfy,
  Telegram or a signed webhook when runs keep failing, a source goes
  unread, the Bilibili login expires or stored secrets cannot be decrypted,
  and another when it recovers. A heartbeat pings an external monitor such
  as Uptime Kuma, so a stopped server is noticed too. Destinations must be
  public addresses, and development mode never sends. Acknowledging an
  open alert takes it off the nav badge until the problem returns.
- The nav groups pages by task: Today, Content, Pipeline and System.
  Queue becomes the Runs page's **Up next** tab, and the Works covers and
  recent uploads move from Site to a **Works** page in System. Older
  `#queue` links open Up next.
- Panels, fields and switches drop their explanatory text and question-mark
  help; the docs describe how each feature works.
- Posts shows what X says about each post (**Deleted on X**, **Thread**,
  **Reply**, **Other author**, **Repost**, **Unlisted**) and a promoted
  post's **Demoted**, filters by whether the public pages show a post, and
  replaces the hide switch with **Public pages**: Auto, Show or Hide. Show
  keeps a post on the public pages whatever X or External posts say.
- The model provider dialog drops its retries field, which never took
  effect; calls follow the attempts per model sync setting.
- The Review queue lists entries their posts no longer list, with Keep entry
  and Delete entry, and the overview counts them.
- A schedule image card shows its last check and, after an image change, the
  label on the previous image.
- Admin API: events carry review kind `unlisted`; the overview's review
  counts add `unlistedEvents`; schedule images add `needsReview`,
  `previousDecision` and `lastCheck`.
- Admin API: the event an edit, confirmation, merge, unmerge or creation
  returns, and the evidence read's event, carry their `extractionId` as the
  events list does instead of `null`.
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

- X account exports fill in Kano's own posts with their wording, links,
  quotes and photos, and keep the replies and other people's posts an
  earlier import filed as hers off the public pages; a slow local check
  tells what each is and shows her own posts and thread replies again.
- Every X answer that can tell records what a post is (her own post, a
  thread reply, a reply to someone else, another author's post or a
  repost), and the public pages follow it: a reply to someone else, another
  author's post or a repost stays off the wall, Past posts, the API and MCP
  with the entries only it announced. A post the public pages leave out is
  not extracted, and translated only when a post quotes it.
- A run whose lease expired after a source failure it tolerated resumes with
  its remaining steps and finishes partial instead of failing.
- An X `count` sync setting and a timeline `count` fetch read the newest posts
  from the head every run instead of continuing from older pages; an `all`
  fetch still continues from its checkpoint.
- An X `count` sync read that no account answers fails with the retryable
  `x_source_failed`, so it is tried again and can be retried like an
  incremental read.
- Retrying a run whose source step failed also re-runs the completed media,
  schedule, translation and reconcile steps after it, since the retried read
  can store new posts with media.
- Finishing or cancelling a run no longer advances the dashboard revision; the
  writes that change public data advance it, including schedule extraction
  that changes a post's events and approved schedule images, so open boards
  reload only for a change.
- An X account whose listing served records its last success even when it
  found nothing new, so a quiet timeline or X API read keeps the source
  fresh.
- A failed refresh of a stored X post no longer fails a quiet run: it is a
  note that leaves the run partial, and the post is not queued.
- An X status stored under another account's handle no longer fails the
  account's run. The post keeps its stored row and is skipped with
  `x_post_owner_conflict`; the rest of the run is written.
- A stored X post whose status answers 404 or 410 twice is marked deleted
  on X instead of staying public; its data, media and events are kept, and
  a later answer that carries it clears the mark.
- An event whose post's evidence moves to another event, such as an entry
  that gains or drops a YouTube link, is retired when it has no other source
  instead of staying on the board without one. Evidence attached to an
  operator's edited or deleted entry stays with it, so a deleted activity no
  longer returns as the video's event.
- A post's entry that links a YouTube video only attaches to the event the
  YouTube sync wrote for that video, adding its stated platforms and label;
  the video's title, times and type stay YouTube's.
- A post read again as no schedule keeps its events and sends them to the
  Review queue instead of leaving them unnoticed.
- A schedule image check that is skipped (an image still caching, an empty
  image queue), running or failed keeps the image's verdict and its place on
  the board, and records the check apart, when it checks the image the
  verdict judged; a different image under the same URL never shows on an
  earlier approval.
- A schedule image that changes under an operator's label keeps that label
  as the previous decision and waits for the operator; a `schedule` label no
  longer approves the new image, a `not_schedule` one keeps hiding it, and
  the period correction stays.
- An ended YouTube broadcast stays ended when a later check reports it
  upcoming or names no state.
- An upcoming yt-dlp entry that names only its release date becomes a
  date-only reservation instead of starting at its publication time.
- A corrupt file at a media content address is replaced by the verified
  download, and a file the media step cannot write records a retryable
  failure instead of stopping the step.
- An X timeline gap keeps the ceiling its opening read reached, so a post
  pinned or fetched inside the gap no longer lets an X API read close it
  before the posts between them are read.
- Schedule images that share one image are checked through a post that
  passes the board-notice gate, so a sibling whose post fails it no longer
  keeps the image from ever being checked.
- A YouTube thumbnail whose revalidation was rejected once (an invalid image,
  an oversized body) is checked again after its video's next watch-page read,
  such as a refresh request, instead of never.
- A post whose input returns to one read before applies that stored result
  to its events again, so its events follow its current input.
- Entries of one post that name the same activity on different platforms
  are one event carrying every platform, not only the last line's.
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
- A vxtwitter detail answer that carries `retweet` and explicitly matches
  the requested outer status ID is recorded as the
  account's repost of the original status and never stored as a post, on
  the pending queue, known-post refreshes and operator refreshes alike.
- X API answers record each repost's own status ID, so a stored post under
  it is removed in the same way. Repeated reposts of one original retain
  every own ID, including when one API page carries several, so later
  synchronization and imports cannot restore them as posts.
- Each X image records whether it is a photo or a video's or GIF's
  thumbnail, from the detail answer, the timeline and the X API alike. Video
  and GIF files are never downloaded.

</details>

<details>
<summary><strong>Runtime and development</strong></summary>

- `/posts` loads only the board's post wording and its post and month
  stylesheets instead of the whole board copy and `board.css`.
- The board snapshot reads each account's newest posts, the Bilibili
  accounts and the schedule images of posts deleted on X through indexes
  instead of walking every stored post on each read, and looks up the events
  of its active and cancelled videos by identity instead of walking the
  events for every video.
- `/api/dashboard` answers a request at an unchanged revision, a 304
  included, from a prepared response that lasts until the next projection
  transition, a trusted broadcast state going stale or the next minute;
  requests that miss together compute it once. `meta.generatedAt` is the
  time that response was computed.
- The agent guide is a short root `AGENTS.md` with area guides for
  `backend/`, `src/`, `scripts/` and `docs/`, plus `merge-to-main` and
  `release` skills in `.claude/skills/`.
- The large documents (sources, data model, admin API, frontend, public
  API, workflows, media cache, testing, configuration and LINE BLOG archive
  maintenance) are short indexes of topic pages, and `make docs-check` also
  validates heading anchors.
- A pull request that changes `backend/go.mod`, `backend/go.sum`,
  `package.json` or `package-lock.json` also runs the matching Go or npm
  advisory audit, and `PR / Gate` requires it to pass.
- The Release workflow builds the published image without a GitHub Actions
  layer cache, which restored no layer between releases and took about half
  of the image job exporting.
- `npm run lineblog:export` writes the server's copy of the LINE BLOG
  archive from the tracked page data, and the frontend tests fail when the
  committed copy differs, allowing Git checkout line endings.
- HTML revalidates with an ETag over its rendered content, including page
  tags and runtime CDN/public-origin configuration. A build timestamp alone
  cannot produce a 304 that keeps an earlier configuration's addresses.
- LINE BLOG packaging refuses a video that is not H.264 with AAC audio.
- Profile images load when the data directory is reached through a symbolic
  link, as macOS reaches its temporary directories; a symbolic link in place
  of the avatar directory is still refused.
- `CDN_PUBLIC_BASE` optionally serves the build's assets and cached media
  from a CDN. It is empty by default, changes no stored data, and must be
  passed to the frontend build as well; see [CDN](../operations/cdn.md).

</details>
