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
> Bilibili dynamics are listed only with `BILIBILI_SESSDATA` configured
> ([ADR-0007](../decisions/ADR-0007-bilibili-dynamics-need-a-login.md)).
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
> a `null` `id` when the listing did not name it, and the admin API's
> translation routes accept the owner `bilibili_status`, which translation
> rows may also carry.

> [!IMPORTANT]
> Related X accounts
> ([ADR-0009](../decisions/ADR-0009-related-x-accounts-and-quoted-statuses.md))
> start empty: the `x_related_accounts` setting is empty after the upgrade
> and on a fresh install, so no account beyond `X_HANDLES` is read. To follow
> Kano's agency, add `Mil_Pro_` on `/admin` → Sources → X. A new account
> starts with the keyword rules `鹿乃`, `@kano_2525` and `#鹿乃まほろ`, and the
> next X run reads its timeline through FxTwitter only.

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

<details>
<summary><strong>Public pages</strong></summary>

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
- Posts of the agency's account about Kano appear on the post wall, with the
  account's initial rather than Kano's portrait. A post Kano quotes shows
  only inside her post, and the agency never gets a tab of its own.
- Sales of goods Kano takes part in reach the schedule and the On sale row
  from the official shop, even when no post announces them, such as a
  group collection where she is one of many voices. Each collection and
  period is one entry named after the collection, opening at the time the
  shop put it on sale and closing on its last day, and a post about the
  same sale folds into it. The profile card adds the shop's last read.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

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
- Each X run also reads related accounts, such as the agency, in full into
  the pool through the FxTwitter timeline only, with their own cursor.
  Statuses matching the account's keyword rules are promoted onto the board
  without schedule-image candidates; the rest stay in the pool.
- A `shop` step reads the official shop's public product JSON on
  `shop.milpr.com` as `kano-navi-go/shop`: the newest 50 products on each
  timed run, the collection list only for products no stored sale holds,
  and the whole catalog when the operator asks. Products tagged 鹿乃まほろ
  with a dated order or sale period become one automatic sale per
  collection or title prefix and period; permanent items are skipped. A
  429 stores its `Retry-After`, and no read requests the shop before it
  passes. Entries stay when products leave the listing, and operator edits
  and deletions win.

</details>

<details>
<summary><strong>Operator console</strong></summary>

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
  its timer and Run now and, below the timed reads, its read's state: the
  account, the last read and its error, whether the login cookie signs in
  and the captcha backoff. Bilibili uploads also holds the live room check
  (its switch, interval, followed streams and the room's last check) in the
  main column, as YouTube holds its probe. Beside them each tab keeps only
  its manual action, adding one upload or one dynamic by its link, as X and
  YouTube keep their manual fetch. During a captcha backoff, adding or refreshing
  one upload or dynamic offers **Try once anyway**. Translation views show
  the Japanese locale, and bilibili posts offer no X-only actions. Posts
  shows a bilibili forward's forwarded dynamic as its quote, which can be
  hidden from the public pages like an X quote.
- Site → Image copies shows its panel and group meters at once and fills in
  the counts when the listing arrives.
- Source tabs that overflow a phone screen scroll within their row instead
  of widening the page.
- The schedule marks an automatic entry whose every source post is hidden
  as Hidden by its posts and dims it in the list and the calendar, since the
  public pages leave it out.
- Sources → X lists related accounts with their keyword rules and last read.
- External posts, in the Data group, lists the status pool with its reasons
  and state, and promotes or demotes a status with a reason, as a lock the
  keyword rules never override. Posts marks promoted posts.
- Sources has an Official shop tab with its timer and Run now, the shop
  read's state (sale entries, last read and error, a 429's wait) and
  **Read the whole catalog** beside it; the outbound proxy areas add the
  shop.

</details>
