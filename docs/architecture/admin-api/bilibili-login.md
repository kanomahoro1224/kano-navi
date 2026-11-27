# Bilibili Login Endpoints

Importing, signing in by QR code and forgetting the
[Bilibili login](../sources/bilibili-login.md) the server owns.

| Route                               | Body                     | Answer                      |
| ----------------------------------- | ------------------------ | --------------------------- |
| `PUT /sources/bilibili/login`       | `{cookie, refreshToken}` | the Bilibili source (below) |
| `DELETE /sources/bilibili/login`    | `?signOut=1` optional    | the Bilibili source         |
| `GET /sources/bilibili/login/qr`    |                          | `BilibiliQR`                |
| `POST /sources/bilibili/login/qr`   |                          | `BilibiliQR`                |
| `DELETE /sources/bilibili/login/qr` |                          | `BilibiliQR`                |

The Bilibili source is `bilibili` of `GET /sources` (see
[Videos](videos.md)). Its login fields are:

```json
{"signedIn", "login", "loginCheckedAt", "loginSource", "loginVia", "loginName",
 "loginStoredAt", "loginRefreshedAt", "loginRefreshCheckedAt", "loginRefreshError", "loginStorable", "loginUnreadable"}
```

`login` is `signed_in`, `expired`, `unchecked` or `not_configured`, and
`loginCheckedAt` the latest check of the login in use. `loginSource` is
`stored`, `environment` or null without a login; `loginVia` (`import` or
`qr`), `loginStoredAt`, `loginRefreshedAt`, `loginRefreshCheckedAt` and
`loginRefreshError` describe a stored login and are null otherwise.
`loginName` is the nickname the latest check of this login saw.
`loginStorable` is false when the process keeps no stored login.
`loginUnreadable` is true while a stored record no longer decrypts; reads
then use the environment's login, and `DELETE` without `signOut` drops the
record. No answer
carries a cookie, a token or an account ID.

`PUT` stores `{cookie, refreshToken}`: the `Cookie` header of a private
window signed in to the account and that window's localStorage value
`ac_time_value`. The cookie must carry `SESSDATA` and `bili_jct`; the other
known login cookies are kept and the rest dropped. A missing field, another
field or a malformed value is `400 invalid_input`. The import replaces any
stored login, sends no request, and leaves the login `unchecked` until the
next signed-in read.

`DELETE` drops the stored login; without one it answers the source as it
is. With `?signOut=1` it first ends the session at Bilibili, one request
that never waits out or overrides the captcha backoff: during the backoff it
answers `503 bilibili_blocked` with `blockedUntil`, a captcha answer starts
the backoff and answers the same, another failure answers `502` with its
code and keeps the login, and a record that no longer decrypts cannot sign
out (`409 bilibili_login_unreadable`).

`BilibiliQR` is the process's QR sign-in:

```json
{"state", "error", "expiresAt", "qr": {"size", "path"} | null}
```

`state` is `waiting`, `scanned`, `signed_in`, `expired`, `failed` or null
before the first sign-in, and `error` the failure's code. `qr` is the code
to scan, one SVG path over a square of `size` modules with its quiet zone,
and is present, with `expiresAt`, only while the sign-in waits. `POST`
starts a sign-in (answering the open one instead while one waits): it
reads the device identity when due and `qrcode/generate`, and answers
`503 bilibili_blocked` during the backoff or `502` with the code of a failed
request. The server then polls the sign-in itself; the console only reads
`GET`. A sign-in stores the login like an import and checks it through
`nav` before it reports `signed_in`, so the source status that follows names
the account; a failed check leaves the login unchecked. `DELETE` ends a waiting
sign-in as `expired`.

Writes answer `503 read_only` on a read-only database,
`503 secrets_key_unavailable` without `LLM_SECRETS_KEY`, and
`409 bilibili_login_changed` when another write replaced the login since
the request read it. An import, a forget and a QR start answer
`503 bilibili_unavailable` when the process has no Bilibili reader.
