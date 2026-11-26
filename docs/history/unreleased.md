# Unreleased

<details>
<summary><strong>Upgrade notes</strong></summary>

- `GET /api/admin/sources` adds the Bilibili login fields `loginSource`,
  `loginVia`, `loginName`, `loginStoredAt`, `loginRefreshedAt`,
  `loginRefreshCheckedAt`, `loginRefreshError`, `loginStorable` and
  `loginUnreadable`, and the
  Bilibili methods' `requires` reads `a Bilibili login`.
- New admin routes `PUT` and `DELETE /api/admin/sources/bilibili/login` and
  `GET`, `POST` and `DELETE /api/admin/sources/bilibili/login/qr`.
- New alert rule `login_refresh_failed` (on, warning); `secrets_unreadable`
  adds the `bilibili` area.
- The server requests `passport.bilibili.com` once a day while a login is
  stored, and for an operator's QR sign-in or sign-out.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- The Bilibili tabs have a Login panel: sign in with a QR code or import a
  private window's login, which the server keeps and refreshes itself, and
  forget it, signing it out at Bilibili or not. The panel shows the
  signed-in account's nickname.

</details>

<details>
<summary><strong>Public pages</strong></summary>

- The Posts page opens on the posts the browser showed last while it checks
  the archive for changes, reads the newest posts alongside its month index,
  and going back to it from another page returns to where you were reading.
- The Posts page's All card is one line without a portrait; in wide windows
  it spans the side rail, with the accounts two a row below it.

</details>

<details>
<summary><strong>Runtime and development</strong></summary>

- The server keeps the post archive's month index and pages it has prepared
  until the dashboard revision changes, so repeated reads skip SQLite.
- A Bilibili login stored from the console wins over `BILIBILI_COOKIE` and
  `BILIBILI_SESSDATA`, is sealed with `LLM_SECRETS_KEY`, and is refreshed
  and confirmed once a day before the Bilibili reads.

</details>
