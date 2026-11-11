# Unreleased

> [!IMPORTANT]
> Automatic schedule extraction selects only posts published within the
> extraction lookback, a new setting under `/admin` → Detection that is
> 30 days on upgrade and on a fresh install. Older posts are no longer sent
> to a model without a reprocess request: a changed input, a pending retry
> or a new extractor version leaves them as they are, and imported or
> backfilled history is not extracted. To keep extracting posts of any age
> automatically, set the lookback to `0`.

<details>
<summary><strong>Upgrade notes</strong></summary>

- `GET` and `PUT /api/admin/sync-settings` add `extractionLookbackDays`
  (0–3650, default 30, `0` for no limit), stored in `app_settings` as
  `sync_extraction_lookback_days`. It has no environment variable.
- `GET /api/admin/queue/schedule` adds `lookbackDays`; `postsRead` and
  `candidates` count only posts within the lookback.
- A manual fetch with `extract` on a `range` that ends before the extraction
  lookback answers `400 invalid_input`.
- Public dashboard, post archive and MCP posts add `rich`, and their quotes
  `quote.rich`, for a Bilibili dynamic with stickers: `{text}` and
  `{emoji, url}` segments whose `url` is a local `/media/` address or
  `null`.
- Bilibili sticker pictures are cached as `post-emoji` and `quote-emoji`
  media links. A stored dynamic gains its stickers on its next listing read,
  which advances the dashboard revision once; stickers change no stored
  text, so they cause no new translation or extraction.
- Dynamics stored from Bilibili listings without rich-text nodes may take
  new wording on their next read: plain text is HTML-unescaped, an album
  without a major type takes its opus summary (and its pictures) instead of
  `desc.text`, and a forward of a forwarded dynamic that has plain text only
  quotes it as available and adds it to the Bilibili status pool. Such
  dynamics, and those pooled dynamics, are translated and extracted once
  more.

</details>

<details>
<summary><strong>Public pages</strong></summary>

- In Safari on iPhone, picking a day with four or more entries in the month
  view keeps the month controls on screen instead of pushing them off the top
  and leaving a blank strip at the bottom.
- On the Works page, switching between Own releases, Featured on and
  Uploads keeps the tabs where they are on screen instead of jumping back to
  the top of the page while a section loads.
- On wide screens the On sale list moves into the top of the board: beside a
  ticket with artwork it stacks under the profile card, and under a compact
  date ticket it runs as a row. When more sales are current than it shows, an
  All button opens every one in a sheet.
- Works image captions, edition names, formats, packaging, bonuses, store
  bonuses, disc names, credits and track notes show their descriptive words
  in the interface language; names of works, people, stores, editions and
  bonus items stay as the releases write them. Journey cover captions follow
  the same rule.
- Stickers in a bilibili post show as small pictures in its original
  wording, or as their names until the picture is cached; translations
  leave them out.
- The Works page's feedback link opens a correction form that asks for the
  work, what is wrong and a source.
- In a Works release's Where to listen, the name of an extra item such as
  an instrumental sits above its store links instead of overlapping them.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- Bilibili listings without rich-text nodes give a dynamic its plain,
  HTML-unescaped text, and an older album without a major type keeps its
  pictures under `draw` or `opus`.
- Schedule extraction selects unrequested posts published within the
  extraction lookback only, so importing or backfilling years of posts no
  longer queues each of them for a model call or fills the calendar with
  past events. A reprocess request from the Posts page still extracts a post
  of any age, and a manual fetch with Extract refuses a date range that ends
  before the lookback.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- Detection's model limits add Extraction lookback (days), and the schedule
  queue preview shows the lookback it selected its candidates with.

</details>
