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

</details>

<details>
<summary><strong>Operator console</strong></summary>

- Sources → Bilibili uploads shows the upload read's three methods in its
  method chain, the upload list, the signed-in dynamics listing and the
  official client's listing, each with its switch and up/down moves, and a
  method the server lacks the credential for greyed with the variable it
  needs.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- Bilibili upload reads try the enabled methods of the timer's chain in its
  order, so the official client's listing can come first or a fallback can
  be turned off; during a captcha backoff only the official client's listing
  is read, and only when the chain enables it.

</details>
