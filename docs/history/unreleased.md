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
<summary><strong>Upgrade notes</strong></summary>

- The shop's requests can be signed with Web Bot Auth. The operator
  generates, rotates and removes the key in `/admin` → Sources → Official
  shop; it is stored sealed with `LLM_SECRETS_KEY` and served publicly at
  `GET /.well-known/http-message-signatures-directory`. Signing needs an
  `https` origin: the panel's signing origin, or an `https` `PUBLIC_ORIGIN`.
  The optional `WEB_BOT_AUTH_KEY` (make one with
  `kano-go web-bot-auth-key`) overrides the stored key; a malformed value
  stops startup.
- Admin adds `GET`/`PUT /sources/shop/web-bot-auth` and
  `POST`/`DELETE /sources/shop/web-bot-auth/key`, and `GET /sources` adds
  `shop.webBotAuth`.
- The shop timer has a method chain: `signed`, `new_arrivals` and `atom`,
  all on. A stored shop chain gains `signed` before `new_arrivals` and `atom`
  after it. Admin `GET /sources` reports it with `fixed: false` and
  `a Web Bot Auth key` for `signed`, and `PUT /sources/shop/schedule` accepts
  its `chain`.
- The `shop` step result adds `listing`, `tried`, `signed` and, after a
  fallback, `fallback`; a fallback makes the run `partial`. New codes:
  `shop_no_signing_key`, `shop_no_method`, `shop_invalid_atom` and
  `shop_signing_failed`.

</details>

<details>
<summary><strong>Public pages</strong></summary>

- On phones, Month view, Past posts and Past videos open straight onto their
  controls: the month row sits beside the close button, without the title or
  the month's count above it. The account filter of Recent Posts stays on one
  row beside the Past posts button and scrolls sideways when it is too long.
- Month view always lists the picked day's entries; the button that folded
  the day details away is gone. On phones the list appears once a day is
  picked.
- Videos in Streams and works no longer repeat their form (video, stream
  archive, Short) where the section already names it; Past videos keeps it.
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

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- The official shop read can identify itself with a Web Bot Auth signature,
  which the storefront rate-limits less than anonymous automated reads, and
  can fall back to the catalog's Atom feed. A 429 still ends the read at
  once, and a read makes at most three requests.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- The shop's timed reads show its method chain for ordering and switching,
  with the signed read greyed until a Web Bot Auth key is active.
- Sources → Official shop has a Web Bot Auth panel: the key's state, keyid,
  age and directory, the signing origin, and Generate, Rotate and Remove.

</details>
