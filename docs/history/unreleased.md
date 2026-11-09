# Unreleased

> [!NOTE]
> Model calls rise for a while after upgrading. The extraction prompt
> defines every event type and asks for sale periods, which changes the
> extractor version, so every stored X post that the detection settings
> select is extracted again, the ones processed longest ago first. Startup
> adds the empty `events.ends_on` column without rewriting events. Public
> events and MCP schedule reads add `endsOn`, and the event type `sale`
> joins the admin API's types.

> [!NOTE]
> Bilibili dynamics are listed only with a Bilibili login configured
> (`BILIBILI_COOKIE`, a signed-in browser's cookies, or `BILIBILI_SESSDATA`,
> [ADR-0007](../decisions/ADR-0007-bilibili-dynamics-need-a-login.md)).
> Startup adds the empty `events.platform` column and sets it to `youtube`
> on YouTube events, without rewriting other events. Japanese becomes a
> translation target:
> X posts and YouTube titles detected as Japanese still need only English
> and Chinese, so they are not translated again, but wording detected as
> English, Korean or another language now also gets a Japanese translation,
> within the per-run limit. Bilibili's `bilibili_risk_control` code is split
> into `bilibili_risk_control_http`, `_page`, `_352`, `_412` and `_voucher`.
> Public posts may have `source: "bilibili"`,
> public events add `platform` and full ones `liveUrl`, the dashboard adds
> `liveNow`, translations may include `ja`, the extraction schema adds
> `platform`, and the dynamics have their own `bilibili_dynamics` step and
> timer, whose chain adds the `schedule` step; the step catalog, the source
> schedules and the overview's sources add it. Startup gives a saved
> Bilibili timer a dynamics timer beside it with the same switch and
> interval, planned half an interval after the upload read; `SKIP_BILIBILI`
> covers both. Startup creates the empty `bilibili_statuses` table for the
> dynamics forwards forward. A forward's public `quote` is that dynamic, with
> a `null` `id` when the listing did not name it, the admin API adds
> `GET /bilibili-statuses`, and its translation routes accept the owner
> `bilibili_status`, which translation rows may also carry.

> [!IMPORTANT]
> Related X accounts
> ([ADR-0009](../decisions/ADR-0009-related-x-accounts-and-quoted-statuses.md))
> start empty: the `x_related_accounts` setting is empty after the upgrade
> and on a fresh install, so no account beyond `X_HANDLES` is read. To follow
> Kano's agency, add `Mil_Pro_` on `/admin` → Sources → X related. A new
> account starts with the keyword rules `鹿乃`, `@kano_2525` and `#鹿乃まほろ`.
> Related accounts are read on their own `x_related` timer, through
> FxTwitter only: startup gives a saved X timer an `x_related` timer beside it
> with the same switch, every 180 minutes with the default incremental read,
> planned half the X interval after the X timer's next run; a fresh install
> keeps it off. `SKIP_X` covers both X timers. The step catalog, the source
> schedules and the overview's sources add `x_related`.

> [!NOTE]
> Startup creates the `x_statuses` and `x_reposts` tables, adds
> `posts.promoted_at`, and fills the status pool from the quotes stored
> posts already hold, without a request or a rewritten row. Public posts add
> `promoted`; admin posts add `promotedAt` and admin events `hiddenByPosts`;
> the admin API adds `/sources/x/related` and `/x-statuses`, and its
> translation routes accept the owner `x_status`, which translation rows may
> also carry.

> [!NOTE]
> Schedule extraction skips hidden posts, and the automatic events that only
> hidden posts support leave the board, the calendar, the calendar
> subscription, the public API and MCP schedule reads. Posts hidden before
> the upgrade therefore stop being extracted and their automatic events
> leave the public pages. No row is changed: showing a post, or confirming
> an event in the console, shows the event again.

> [!NOTE]
> A manual fetch accepts `source: "bilibili"` and `"bilibili_dynamics"` with
> the `space` method, admin `GET /sources` adds the Bilibili `methods` and
> `oldest` (also under `dynamics`), and the admin API adds
> `POST /videos/youtube`. Bilibili fetch continuations are new
> `bilibili-window` and `bilibili_dynamics-window` `sync_state` rows; no
> column is added.

> [!NOTE]
> The admin API adds `GET /queue/schedule`, `/queue/youtube`,
> `/queue/translate` and `/queue/x-refresh`, and the schedule step result
> adds `candidateMs`, `postsRead`, `candidatePosts`, `judged`, `settled` and
> `judgeMs`. It also adds `GET /events/{id}/evidence`, an entry's source
> posts and the same accounts' nearby posts.

> [!NOTE]
> Startup adds the empty `post_llm_states.settled_stamp` column. Until a
> post's next cache hit records its stamp, the schedule step judges it as
> every run did before, so the first runs after the upgrade cost what a run
> cost before; later runs count posts whose input has not changed as cache
> hits without loading them and leave unchanged states unwritten. A post
> whose cached image file was deleted from disk keeps its stored result
> instead of waiting for the image again.

> [!NOTE]
> Model calls rise a little after upgrading. Schedule extraction reads the
> wording a post quotes, which changes the input of every post with a shown
> quote, so those within the extraction lookback are extracted once more,
> and quoted posts within the translation lookback are translated within
> the translate step's per-run limit, after posts and videos.

> [!NOTE]
> The official shop is a new source, `shop`, with its own timer, which is
> off after the upgrade and on a fresh install: turn it on in `/admin` →
> Sources → Official shop (360 minutes by default, at least 180), and use
> **Read the whole catalog** once to pick up sales the newest 50 products no
> longer list. `SKIP_SHOP=1` disables it. Events may have `source: "shop"`,
> `meta.sourceFetchedAt` adds `shop`, the step catalog, the source schedules,
> the overview's sources and the outbound proxy areas add `shop`, admin
> `GET /sources` adds `shop`, and a manual fetch accepts `source: "shop"`
> with the `catalog` method. No column is added and no event is rewritten.

> [!NOTE]
> Startup adds the empty `events.platforms` and `events.platforms_inferred`
> columns without rewriting events; a stored entry reads as its single
> platform. The Bilibili extraction schema asks
> for a `platforms` array in place of `platform`; extraction identities do
> not name the schema, so no stored post is extracted again, and the X
> prompt and schema are unchanged. Public events and MCP `schedule_list`
> add `platforms`, full ones `links`, and `liveUrl` covers every stream
> shown on bilibili; MCP `schedule_list` and the calendar subscription read
> a stored live-room link as none, the subscription linking the platform
> instead; the admin
> API's events accept `platforms`, report `platforms`, `platformsInferred`
> and `shownPlatforms`, and refuse a live-room link as `url` with
> `event_live_room_url`. The live room gets about two more anonymous
> requests per scheduled stream.

> [!NOTE]
> The new Works page shows its covers from `data/works/`. Run
> `npm run works:package -- --from /path/to/kano_official` on the instance
> to package them, then `npm run variants` (or a copies run from the
> console) to make their smaller copies; until packaged, the page shows each
> cover as unavailable. The Journey page's release milestones show the same
> covers, so they are unavailable there too until packaged. The Journey
> page's own images are packaged the same way into `data/journey/`: run
> `npm run history:package -- --from /path/to/kano_official` before
> `npm run variants`. The variants command no longer takes `-static`. The
> uploads' video thumbnails need no packaging: the media step saves them over
> its next runs, and `/admin` → Site → Works upload covers shows which are
> still missing and starts a media step to fill them. The admin API adds
> `GET /media/works-covers`.

> [!NOTE]
> Startup adds the empty `videos.manual_work` column without rewriting
> videos. `GET /api/works/uploads` adds `recent`, the uploads after the Works
> collection date (the newest 48, with `recentTotal`). Admin
> videos add `manualWork`, `PUT /videos/{id}` accepts `work`, and the admin
> API adds `GET /videos/works-recent`.

> [!NOTE]
> Bilibili uploads that are not merged into a YouTube video become public
> videos: the board, `/api/videos` with its month index and counts, MCP and
> the Atom feed list them with `source: "bilibili"` once their form is
> settled. Startup adds the empty `videos.kind_source`, `kind_review`,
> `kind_attempted_at` and `kind_attempts` (0) columns without rewriting
> videos, so every stored Bilibili upload starts unsettled and stays off
> public readers until the next `translate` run settles it by rule or model
> or sends it to `/admin` → Review queue → Bilibili form. That run asks the
> translation queue once per unmerged upload no rule settles, and the
> translation backlog of the newly public titles follows, so model calls
> rise for a while. The list read no longer rewrites a stored upload's form.
> `reconcile` adds `rule:broadcast` merges of live replays. The admin API
> adds `GET /videos/forms`, `review.bilibiliForms`, the videos'
> `formSource`, `formReview` and `public`, and merge candidates'
> `broadcast`; `PUT /videos/{id}` accepts only `upload` or `stream` as a
> Bilibili upload's `kind`. The `translate` result adds `bilibiliForms`.

<details>
<summary><strong>Public pages</strong></summary>

- Bilibili-only uploads are videos on the board and in Past videos, with the
  bilibili mark and link; a bilibili live replay is a stream archive, or the
  bilibili link of its YouTube archive when the stream was simulcast.
- Pages keep the same width when a scrollbar appears or disappears: a page
  that grows or shrinks, an image viewer or sheet, or the language and
  calendar menus no longer shift the layout sideways on desktops with
  space-taking scrollbars. The operator console keeps its width the same way.
- Merchandise orders, pre-orders, ticket sales and lotteries are their own
  Sale entries on the schedule, shown on the day they open and the day they
  close (or on the deadline alone) with an On sale status. They never take
  the next-up ticket, and the calendar subscription lists their opening and
  closing ([ADR-0006](../decisions/ADR-0006-sale-periods.md)). Reminders of
  a sale, such as a last-day post, fold into the announced period instead of
  adding another entry. An On sale row below the next-up ticket lists up to
  three sales on sale now or opening within a week, with their close or
  opening time.
- On phones the Now section is shorter, so the On sale row and the schedule
  come up sooner. The profile card leads the section as a compact identity
  row: a short banner strip with the avatar overlapping its lower left
  edge, beside the name, and the catchphrase below in line with the links; it
  leaves out the role line above the name and the counts, and its last row holds the X, YouTube and
  bilibili read times, the run's state dot and the reload button. The
  countdown sits with the ticket's time, the latest video card puts its
  labels above the thumbnail, and the On sale cards scroll sideways in one
  row. On wider screens the profile counts open the schedule, post wall and
  video shelf.
- The About page shows each code contributor's GitHub avatar with the name
  below it, loaded from GitHub, and its technology list adds yt-dlp and MCP.
- The post wall is **Recent Posts** and lists Kano's bilibili dynamics with
  her X posts, with the bilibili mark and a link to the dynamic; bilibili
  posts show no engagement counts. A bilibili forward shows the post it
  forwards as a nested card with its author's name, translated like a
  quoted X post, and links to it on bilibili.
- Japanese content shows a Japanese translation of wording that is not
  Japanese, such as a Chinese bilibili post, with the AI mark; the calendar
  and video feeds pick Japanese wording the same way.
- A bilibili stream links to Kano's live room and shows as live while the
  room is on air; an unannounced bilibili stream shows on the stage as live
  now without adding a schedule entry.
- A stream on YouTube and bilibili at once is one card with a link to each
  platform, on the stage, in the week and in the calendar, and the platform
  that is live is marked and opened first; the card reads live while either
  is. A link is named after a platform only when it opens that platform;
  the post that announced the entry keeps the open arrow. A bilibili
  broadcast during a YouTube stream that has started shows on that
  stream's card instead of as a separate live-now ticket.
- A premiere carries its Premiere label only while it waits or airs; once
  it has aired it is an ordinary video and shows no form label, as a
  stream archive on the shelf shows none.
- English pages write Kano's group as ミリプロSONA, as the agency does, and
  name the agency's shop Million Production shop after its official English
  name.
- A quoted post of any account is translated into the content language,
  with its own AI mark on the quoted card.
- The translation mark shows where a translation came from: the translate
  icon with AI for an AI translation, with the YouTube or bilibili logo for
  a translation the platform provides, and alone for the editor's own.
- The LINE BLOG archive's header art is a picture only; the menu bar's Home
  link returns to the archive's front page.
- Posts of the agency's account about Kano appear on the post wall, with the
  account's initial rather than Kano's portrait. A post Kano quotes shows
  only inside her post, and the agency never gets a tab of its own.
- Sales of goods Kano takes part in reach the schedule and the On sale row
  from the official shop, even when no post announces them, such as a
  group collection where she is one of many voices. Each collection and
  period is one entry named after the collection, opening at the time the
  shop put it on sale and closing on its last day, and a post about the
  same sale folds into it. The profile card adds the shop's last read.

- A Works page lists Kano's discography, led by the acoustic cover trilogy
  one, two and three: her own releases as a cover shelf with each edition's
  covers, tracklist with photos of its discs, package contents and store
  bonuses and links from album tracks to the same recording's uploads, the works she is featured on with
  the tracks she sang or wrote, and her niconico, YouTube (members-only
  included) and bilibili uploads as a cover grid with filters and search.
  Each instance saves the uploads' video thumbnails itself; an upload the
  site knows shows its length and whether it can still be watched, a disc
  beside an upload's title lists the albums that later included it. The page
  header names the date the whole collection covers.
- Uploads posted after the Works collection date appear above the uploads
  list as their own block: song uploads and premieres the site finds by
  their content tag, Bilibili-only ones included, and those the operator
  adds, each marked as found automatically or added by hand, with its
  cached thumbnail and site links. Two rows show until the reader opens the
  rest, up to the newest 48. The block takes no part in the filters or
  counts, and a release that moves the date leaves it to the list.
- A Works release's sheet and the Works and Journey image viewers keep one
  size whatever the release or image; on a wide screen the release's cover
  and figures stay on the left while its tracklist and details scroll beside
  them, in columns of their own on the widest screens. A release's details
  link the official streaming and download stores that offer it. An edition with
  photos of several discs can put any of them on the record that turns in
  its header. Each edition, such as
  初回限定盤 or 通常盤, is a card with its own cover and format; switching
  editions fades from one cover to the next without a blank in between.
- The Journey page's address is `/journey`; `/history` links redirect to it.
  Its Chinese name is 足迹.
- Journey's release milestones show their release's covers from the Works
  page, each linking to the release there; the doujin album one is dated
  December 29, 2017, and the ルル MV August 13, 2026.
- Journey shows both BOOTH artworks of the Hanayori anniversary acrylic
  stand, dated August 1, 2020, a frame of the SONA teaser video for the
  teaser milestone, the head and shoulders of the full-length 2021 singer
  illustration in its timeline thumbnail, the Pixio ambassador illustration
  in its gallery, the full-size SONA joining merchandise image, and a
  milestone for the March 29, 2020 VR concert 「ReBirth」 with its rehearsal
  photo.
- A video only YouTube channel members can watch adds YouTube's members
  star beside the YouTube mark on its cover, and its Members only chip uses
  the same star.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- Bilibili uploads have a form, an upload or a live replay: a replay title,
  a long upload soon after a Bilibili broadcast or a short upload settles it
  by rule, and otherwise the title inference answers it with the content
  tag, sending an upload it is unsure of to the Review queue; a rule that
  applies once the broadcast is known replaces the model's answer. The
  title inference and machine translation cover every public Bilibili
  upload, a live replay merges into the YouTube archive of the same
  simulcast (never into an earlier one), and a copy posted more than six
  hours from its YouTube video is put to the operator rather than listed
  twice unnoticed.
- The extraction prompt defines each event type and returns one entry per
  sale period with its opening and its close, instead of separate entries
  of mixed types for the opening and the deadline.
- A `bilibili_dynamics` step reads the account's dynamics with a login
  cookie, signed with Bilibili's WBI keys, on its own timer and interval and
  under the captcha backoff it shares with the upload read, so dynamics can
  be read on a schedule apart from the uploads and their runs and failures
  stay apart; uploads and live notices are left out, and emoji are
  dropped from the stored text. A forward is kept as a post that quotes the
  forwarded dynamic, read from the listing itself without another request,
  and extraction reads the forwarded wording as context, with its times in
  Beijing time.
- Bilibili dynamics go through schedule extraction with a Chinese prompt.
  The model gives times as written with their zone, and code converts
  Beijing time to Japan time, including the next-day rollover
  ([ADR-0008](../decisions/ADR-0008-source-time-zones-in-code.md)). Every
  entry records its platform, and a bilibili release links to the upload it
  announced once that is read. An X notice of a bilibili stream merges into
  the dynamic's entry of it.
- Bilibili requests read like the site's own pages: a desktop browser user
  agent and language, the page they come from, and the `b_nut` and
  `bili_ticket` device cookies. The login cookie is verified once a day and
  not sent while expired, and captcha answers name how Bilibili gave them
  (`bilibili_risk_control_http`, `_page`, `_352`, `_412`, `_voucher`), all
  with the same backoff.
- `BILIBILI_COOKIE` takes a signed-in browser's cookies: signed-in requests
  send its `SESSDATA` with the account's other login cookies and the
  browser's own device identity, which Bilibili accepts more readily than
  `SESSDATA` beside an anonymous identity. Other cookies are dropped.
- When the Bilibili upload list fails, the read falls back to the signed-in
  dynamics listing (with a login) and then, with an official client's app
  key pair in `BILIBILI_APP_KEY` and `BILIBILI_APP_SECRET`, to that client's
  space listing on `app.bilibili.com`, and stores the uploads the first
  success shows; the dynamics read stores the uploads its video dynamics
  show too, keeping the list's publication times. The client's listing,
  another service, is the only one read while the captcha backoff lasts and
  never lifts it, and it skips co-created uploads, which it does not
  attribute.
- The live-status probe checks the Bilibili live room, one request per run,
  around announced streams and once per operator interval (30 minutes by
  default) otherwise, and learns the room from the answer.
- Entries record every platform they are on. A bilibili dynamic lists the
  platforms it names, and X posts that announce a simulcast (同時配信, or
  both platforms with 同時) name both; a dynamic's plain stream merges into
  the YouTube stream at the same time, while one stated bilibili-only
  (B站限定) stays apart and never joins that day's YouTube stream. The live room is
  also checked about 5 and 20 minutes into every other scheduled stream, so
  a bilibili broadcast that only a YouTube reservation announced is seen as
  its simulcast.
- Translation adds Japanese, and translates Bilibili dynamics into English
  and Japanese, keeping their times as written. A forwarded dynamic is
  translated once however often it is forwarded, after posts, videos and
  quoted X posts.
- Manual X timeline fetches count FxTwitter's fixed pages of 20 statuses, so
  a large `count` fetch stops at the page budget and continues when repeated
  instead of reading fewer statuses than it reports.
- Schedule extraction skips hidden posts, including operator reprocess
  requests for them, which wait until the post is shown again.
- Schedule extraction reads the wording a post quotes as marked context, so
  a post that only points at a quoted announcement gets its schedule; a
  quote hidden from the board adds nothing.
- Quoted statuses are kept in a status pool that the public pages never
  read, and own accounts' reposts are recorded as links; neither becomes a
  post.
- Related accounts, such as the agency, are read in full into the pool
  through the FxTwitter timeline only, with their own cursor, on their own
  `x_related` timer (every three hours by default), so a failed read of
  Kano's accounts never stops them and their failures never make an X run
  partial. Statuses matching the account's keyword rules are promoted onto
  the board without schedule-image candidates, then extracted and translated
  in the same run; the rest stay in the pool.
- A `shop` step reads the official shop's public product JSON on
  `shop.milpr.com` as `kano-navi-go/shop`: the newest 50 products on each
  timed run, the collection list only for products no stored sale holds,
  and the whole catalog when the operator asks. Products tagged 鹿乃まほろ
  with a dated order or sale period become one automatic sale per
  collection or title prefix and period; permanent items are skipped. A
  429 stores its `Retry-After`, and no read requests the shop before it
  passes. Entries stay when products leave the listing, and operator edits
  and deletions win.
- Each YouTube recent run reads the channel's members-only playlist and
  marks every stored video it names as members only, past streams included:
  its newest 50 entries through the Data API with a key (one more quota
  unit), otherwise, or when that call fails, the newest 15 from the
  playlist's public feed. An operator's YouTube fetch ends with the same
  read, and both report `members` and `membersFrom` in the step result.
- The console's video rows give the translation marks their own width, so
  they no longer run under the row's buttons on a wide screen, and on a
  phone the marks and the buttons each take a row.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- Posts, External posts and Videos switch platforms with one icon tab row
  at the top of the page, kept in the link (`#posts/bilibili`,
  `#videos/youtube/queued`). Posts adds All, X and Bilibili tabs, and its
  Bilibili tab lists the dynamics without an account filter. On a platform
  tab the count of posts or videos lacking a translation, and **Translate
  missing**, cover that platform only.
- Videos has platform tabs (All, YouTube, Bilibili) above its task tabs
  instead of a Bilibili tab, a merge badge opens the other
  side of the merge, and a Bilibili row shows its form and who settled it,
  or that it is not public yet. The Review queue adds a Bilibili form tab
  with Upload and Live replay buttons for the uploads the model could not
  settle, and a public Bilibili upload can be featured.
- Editing a schedule entry shows the posts it was read from beside the
  form, between the same accounts' nearby posts, and a suspected
  cancellation in the Review queue opens the same timeline.
- The schedule has a calendar and a list. The calendar works like a phone
  calendar: a month grid with entry dots and week image marks beside the
  selected week's days, with that week's schedule images under the grid
  and buttons that move an image's dates a week earlier or later or fit
  them to the week. Suspected cancellations and uncertain additions stand
  out in their rows with the decision and source posts at hand, and merged
  duplicates sit under the entry they joined. The list shows the entries
  or every schedule image, with an Undated tab, and an image card's Show
  its week opens the calendar on its week.
- The Review queue's schedule tab replaces its images and cancellations
  tabs: what needs a look, uncertain additions included, grouped by week
  with Open in calendar, which shows that week with only what needs a look
  and leads on to the next such week. The overview counts uncertain
  additions as well.
- The Review queue's posts are cards coloured by state, with the post's
  images and quote beside its wording and one verdict line naming the
  state, error, last try and retry.
- The schedule editor offers the Sale type, with a close date and time in
  place of the length.
- The schedule editor chooses an entry's platforms (YouTube, bilibili,
  Other; both for a simulcast). A pasted bilibili live-room link selects
  bilibili instead of becoming the entry's link, unless Other is chosen for
  someone else's room. Schedule rows show the platforms, dashed when a
  merged entry or the live room adds one and italic when a source default
  gave it.
- A timed entry's **Type default** length saves its end time: an hour after
  the start for a stream or membership, half an hour otherwise, so the board
  stops showing the stream as live at that time. **No end time** leaves the
  end open, and an entry saved without an end opens on it.
- Sources has a Bilibili uploads and a Bilibili dynamics tab, each with
  its timer and Run now, whether the login cookie signs in and the captcha
  backoff in its summary. Bilibili uploads also holds the live room check
  (its switch, interval, followed streams and the room's last check) as a
  fold in the main column, as YouTube holds its probe. Beside them each tab has a manual
  fetch, as X and YouTube do: the account's newest uploads or dynamics, or
  the whole listing, 15 seconds a page within the manual fetch page budget
  and resumed when repeated; the dynamics fetch needs the login cookie and
  can go on to extract schedules. A fetch is never sent during a captcha
  backoff. Below it each tab adds one upload or one dynamic by its link;
  during a backoff, adding or refreshing one upload or dynamic offers **Try
  once anyway**.
- YouTube push takes the deployment's address and adds the callback path
  itself.
- Sources → YouTube adds one video of the channel by its ID or link below the
  manual fetch and reads it at once: through the Data API with a key,
  otherwise from its watch page. A video of another channel is refused. Translation views show
  the Japanese locale, and bilibili posts offer no X-only actions. Posts
  shows a bilibili forward's forwarded dynamic as its quote, which can be
  hidden from the public pages like an X quote.
- Site leads with the Works panels, then Image copies, and ends with the
  mainland China site suggestion.
- Site → Image copies shows its panel and group meters at once and fills in
  the counts when the listing arrives.
- Site → Works upload covers counts the uploads with and without a cover,
  lists each upload still without one with its videos (gone, waiting with
  the last error and retry time, or still to try), and **Fill missing
  covers** runs the media step once.
- Site → Works uploads after the collection date (titled with that date)
  lists the uploads the Works page shows after it and those kept off, with
  their origin, the newest 200 with the count of all; **Exclude** takes an automatic one off and **Follow the rules**
  clears a decision. The video editor's **Works page** field includes or
  excludes any video.
- Source tabs that overflow a phone screen scroll within their row instead
  of widening the page.
- The schedule marks an automatic entry whose every source post is hidden
  as Hidden by its posts and dims it in the list and the calendar, since the
  public pages leave it out.
- Sources has an X related tab laid out like X and YouTube: the related
  accounts' timer (switch, interval, most new statuses per account, next and
  last run, Run now) and its one method, the related accounts with their
  keyword rules entered as chips (Enter or a comma adds one) and last read,
  and beside them a manual fetch that reads one picked related account now,
  as its timer would, with optional schedule extraction and translation.
- The X manual fetch reads every own account or only the one picked.
- External posts, in the Data group, lists the status pool, without Kano's
  own accounts' statuses, with its reasons and state, and promotes or demotes a status with a reason, as a lock the
  keyword rules never override. Posts marks promoted posts.
- External posts has an X and a Bilibili tab. The Bilibili tab lists the
  dynamics Kano's Bilibili posts forward from other accounts, with whether a
  forward shows them, links to the forwards in Posts and their
  translations, which can be edited there; they are never promoted, since
  the forward's quote shows them.
- Sources has an Official shop tab with its timer and Run now, the shop
  read's state (sale entries, last read and error, a 429's wait) and
  **Read the whole catalog** beside it; the outbound proxy areas add the
  shop.
- Queue, in the System group, shows what each step would work through if a
  run started now: schedule extraction's posts in its real order with why
  each is processed, its route and source, and counts of what waits for
  later runs, sits in backoff or stopped retrying; the YouTube detail queue
  with each run's budget; the translate step's candidates; and the X posts
  waiting to be read again. The Overview links to it, and schedule runs
  record how long choosing and judging their candidates took.
- Detection holds the model setup: the detection mode and keywords, then
  the model queues, providers, model limits and recent failures. The
  separate LLM page and the models-in-use summary are gone, and old LLM
  links open Detection.
- The video correction dialog is simpler and calmer: the field hints, intro
  and refresh schedule are gone; the board row sits above the corrections,
  beside one soft panel with the merge and what the platform replaced; a
  corrected control is tinted with an icon reset; the thumbnail picker and the
  replaced history appear only when they have entries; and the refresh is a
  quiet button.

</details>
