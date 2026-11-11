# Unreleased

<details>
<summary><strong>Upgrade notes</strong></summary>

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

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- Bilibili listings without rich-text nodes give a dynamic its plain,
  HTML-unescaped text, and an older album without a major type keeps its
  pictures under `draw` or `opus`.

</details>
