# Bilibili Login

Which login the Bilibili reads send, how the server keeps and refreshes the
one it owns, and where the login cookies go.

## Logins

A read sends one of two logins, resolved again for every read:

- **The stored login**, which the server owns. The operator gives it once in
  the console, by importing a private browser window's cookies and refresh
  token or by scanning a QR code (see
  [Bilibili login endpoints](../admin-api/bilibili-login.md)), and no
  browser uses it afterwards, so only the server refreshes it. It is one
  JSON record sealed with `LLM_SECRETS_KEY` in the `app_settings` row
  `bilibili_login`: `SESSDATA`, `bili_jct`, `DedeUserID`,
  `DedeUserID__ckMd5`, `sid`, the device cookies it signed in with, the
  refresh token, a refresh token whose confirm is still due, how it was
  given (`import` or `qr`) and when it was stored and last refreshed. Every
  write replaces the row only when it still holds the sealed value the
  writer read, so a concurrent process (`kano-go sync`, an import during a
  refresh) never puts an older login back.
- **The environment's login**, `BILIBILI_COOKIE` (a browser's cookies) or
  `BILIBILI_SESSDATA` (its `SESSDATA` alone), see
  [Configuration](../../operations/configuration.md). It stays in memory and
  is never refreshed.

The stored login wins while one is stored and opens. A record that no longer
decrypts (a changed `LLM_SECRETS_KEY`) leaves the environment's login in
use and opens `secrets_unreadable` for `bilibili`
([alerts](../alerts.md#rule-catalog)) until a forget, an import or a QR
sign-in replaces it. An import, a QR sign-in or a forget applies
to the next read in every process, without a restart.

The record and the login cookies never enter `sync_state`, `raw_json`, logs,
[response captures](../../development/security.md#external-request-rules) or
an API answer. The console sees where the login comes from, the nickname the
latest check saw, and the times and codes of its checks.

## Destinations

The login cookies go to `api.bilibili.com`, `passport.bilibili.com` and the
single `www.bilibili.com/correspond/1/<path>` page, never to the
device-identity or ticket request, the opus page, the live host or
`app.bilibili.com`. A signed-in request sends `SESSDATA` with the login's
`bili_jct`, `DedeUserID`, `DedeUserID__ckMd5` and `sid`, and the login's
device cookies (`buvid3`, `buvid4`, `b_nut`, `_uuid`, `buvid_fp`) in place of
the anonymous identity where it carries them, so the account and its device
match. Signed-in reads meet the captcha far less often, and both
[dynamics](bilibili-dynamics.md#dynamics) methods require a login.

The `passport.bilibili.com` requests and the correspond page are read with
the browser user agent, `https://www.bilibili.com/` as Referer, fixed paths,
the Bilibili [proxy](../outbound-proxy.md) area, the client's timeout and
size limit, and rejected redirects. Their answers carry the refresh token
and the cookies they set, so they are never captured. Captcha and 412
answers from them start the shared captcha backoff.

## Daily Check

The login is verified at most once a day through `nav`'s `isLogin`, when a
signed-in read runs; the check keeps the account's nickname (`uname`, at
most 64 characters) and the login it checked, so a new login starts
unchecked whatever the previous one's check found. A QR sign-in checks its
new login once at once, outside the backoff, before it reports success; an
imported login waits for the next signed-in read. While the latest check
found the login expired, no request sends it and the dynamics listing waits
for the next check (`{ skipped: true, reason: "login_expired" }` on the
`bilibili_dynamics` step, error `bilibili_login_expired`). The console
reports `signed_in`, `expired`, `unchecked` or `not_configured`.

## Refresh

The stored login is checked for a refresh once a day, at the start of the
`bilibili` or `bilibili_dynamics` step, whichever runs first, and never
while the shared backoff holds:

1. A confirm that an earlier check left pending goes first. Refused again,
   it is dropped and the check goes on: a confirm Bilibili already took
   (its answer lost, or the write after it failed) is refused every time,
   and the new login works without it. A confirm that got no answer stays
   pending and stops the check.
2. `GET passport.bilibili.com/x/passport-login/web/cookie/info` says whether
   Bilibili wants a refresh and gives a millisecond timestamp. Without a
   refresh due the check ends.
3. `refresh_<timestamp>`, encrypted with RSA-OAEP (SHA-256) under the fixed
   public key of Bilibili's web script and hex encoded, is the path of the
   correspond page, which renders `refresh_csrf` as the text of
   `<div id="1-name">`.
4. `POST .../web/cookie/refresh` with `csrf` (`bili_jct`), `refresh_csrf`,
   `source=main_web` and the refresh token sets new `SESSDATA`, `bili_jct`,
   `DedeUserID`, `DedeUserID__ckMd5` and `sid` cookies and returns the next
   refresh token.
5. The new login is stored, with the replaced token as the pending confirm.
   A failed write stops the check here: the old cookie keeps working until
   a confirm, so the next check tries again.
6. `POST .../web/confirm/refresh` with the new `bili_jct` and the replaced
   token invalidates the old cookie. A failure keeps the pending confirm for
   the next check (step 1); the new login works in the meantime.

The check adds `login: {outcome, requested, error}` to the step's result and
its requests to the step's `requested`. `outcome` is `current` (no refresh
due), `refreshed`, `confirmed` (only a pending confirm went through),
`superseded` (another write replaced the login during the check, which then
leaves it alone) or `failed`. The failure codes are:

| Code                              | Meaning                                                                                   |
| --------------------------------- | ----------------------------------------------------------------------------------------- |
| `bilibili_login_refresh_rejected` | `86095`: the token does not belong to the cookie, usually because a browser refreshed it  |
| `bilibili_login_refresh_failed`   | `cookie/info`, the correspond page or `cookie/refresh` failed otherwise, `-111` included  |
| `bilibili_login_save_failed`      | the refreshed login could not be stored, so no confirm was sent                           |
| `bilibili_login_confirm_failed`   | Bilibili refused the confirm, which the next check sends once more                        |
| `bilibili_login_expired`          | `-101`: Bilibili no longer accepts the login, which the console then reports as `expired` |

Every code but `bilibili_login_expired` opens `login_refresh_failed`, which
the next check that succeeds resolves. A refused login keeps working until
its cookie expires; an imported login must therefore never be used in a
browser again. When Bilibili replaces its public key, the correspond page
answers 404 and refreshes fail until the constant in
`backend/internal/source/bilibili/login.go` is updated.

## QR Sign-in and Sign-out

An operator's QR sign-in uses the server's own anonymous device identity
(made or renewed as a read would), so the stored login's device cookies are
the server's. `GET .../web/qrcode/generate` returns the address the code
encodes, which must be `https` on `passport.bilibili.com` or
`account.bilibili.com`, and a 32-character key. The API process then polls
`GET .../web/qrcode/poll?qrcode_key=` itself every 3 seconds until the
answer is a sign-in (inner code `0`, the cookies set and `refresh_token`),
an expiry (`86038`), a failure, or 180 seconds pass; `86101` waits for a
scan and `86090` for the confirmation in the app. A sign-in is stored like
an import. The address and key are secrets for that window, since whoever
polls with the key after the scan receives the login: they stay in that
process's memory, go only to the authenticated console as the QR image, are
never logged, and are dropped when the sign-in ends. One sign-in runs at a
time: a start while another starts or waits answers that one.

Forgetting the stored login can first end its session at Bilibili:
`POST passport.bilibili.com/login/exit/v2` with `biliCSRF` (`bili_jct`). A
login Bilibili already refuses (its sign-in page instead of JSON, or
`-101`) counts as signed out; any other failure keeps the login. A session
that ended is forgotten even when the device bookkeeping cannot be saved.

## Related Docs

- [Bilibili](bilibili.md)
- [Bilibili login endpoints](../admin-api/bilibili-login.md)
- [Configuration](../../operations/configuration.md)
- [Deployment security](../../operations/security.md)
