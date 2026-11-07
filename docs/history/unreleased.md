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
> covers both.

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
  posts show no quote or engagement counts.
- Japanese content shows a Japanese translation of wording that is not
  Japanese, such as a Chinese bilibili post, with the AI mark; the calendar
  and video feeds pick Japanese wording the same way.
- A bilibili stream links to Kano's live room and shows as live while the
  room is on air; an unannounced bilibili stream shows on the stage as live
  now without adding a schedule entry.
- A premiere carries its Premiere label only while it waits or airs; once
  it has aired it is an ordinary video and shows no form label, as a
  stream archive on the shelf shows none.
- English pages write Kano's group as ミリプロSONA, as the agency does, and
  name the agency's shop Million Production shop after its official English
  name.

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
  stay apart; reposts, uploads and live notices are left out, and emoji are
  dropped from the stored text.
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
- Translation adds Japanese, and translates Bilibili dynamics into English
  and Japanese, keeping their times as written.
- Manual X timeline fetches count FxTwitter's fixed pages of 20 statuses, so
  a large `count` fetch stops at the page budget and continues when repeated
  instead of reading fewer statuses than it reports.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- The schedule editor offers the Sale type, with a close date and time in
  place of the length.
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
  the Japanese locale, and bilibili posts offer no X-only actions.
- Site → Image copies shows its panel and group meters at once and fills in
  the counts when the listing arrives.
- Source tabs that overflow a phone screen scroll within their row instead
  of widening the page.

</details>
