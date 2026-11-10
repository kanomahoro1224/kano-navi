# Unreleased

<details>
<summary><strong>Upgrade notes</strong></summary>

- The Bilibili upload timer's method chain lists `list`, `dynamics` and
  `app`, all on in that order, which is how every upload read already went;
  a chain stored for it before reads as this default. Admin `GET /sources`
  reports it in `schedules` with `fixed: false` and the variables
  the `dynamics` and `app` methods need, `PUT /sources/bilibili/schedule`
  accepts its `chain`, and the `bilibili` step result adds `listing`. A read
  none of whose enabled methods can run fails with `bilibili_no_method`.
- Admin `GET /videos` gives a public Bilibili upload its title `translation`
  summary, as for a YouTube video.
- The translation prompt tells the model to judge a text's language by its
  own wording rather than a quoted song title, so a Chinese Bilibili title
  around a Japanese song title is `zh`. A Bilibili title already detected as
  Japanese keeps that until its translation is requested again in Videos.

</details>

<details>
<summary><strong>Public pages</strong></summary>

- The video shelf's Latest work is the board's newest cover or premiere on
  any platform, a Bilibili-only cover included, rather than the next older
  one.
- Hovering a video or schedule title that its card cuts to two lines shows
  the whole title.
- The profile card drops the latest run's outcome and time and the reload
  button. A row of platform icons shows each platform's last successful read
  on hover, and an icon, like the header dot, turns amber only when that
  platform's read is a day old, not when a single collection run fails.
- The profile card's counts become small icons with numbers on the same row
  as the platform icons, on phones as well.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- Sources → Bilibili uploads shows the upload read's three methods in its
  method chain, the upload list, the signed-in dynamics listing and the
  official client's listing, each with its switch and up/down moves, and a
  method the server lacks the credential for greyed with the variable it
  needs.
- Videos shows the title translations of a public Bilibili upload, which
  can be corrected or requested again like a YouTube video's.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- Bilibili upload reads try the enabled methods of the timer's chain in its
  order, so the official client's listing can come first or a fallback can
  be turned off; during a captcha backoff only the official client's listing
  is read, and only when the chain enables it.

</details>
