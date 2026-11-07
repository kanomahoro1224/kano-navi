# ADR-0007: The Bilibili Dynamics Listing Requires an Operator Login Cookie

- Status: Accepted
- Date: 2026-11-07

## Context

Kano posts Chinese dynamics (动态) on Bilibili that announce Bilibili-only
uploads and streams, which her X accounts do not always repeat. The board
should list them as posts and extract their schedules.

Bilibili's space listing, `/x/polymer/web-dynamic/v1/feed/space`, is the
only endpoint that lists an account's dynamics. Signed in, it needs the
`SESSDATA` cookie and a WBI signature. Anonymous, it also needs the
`dm_img` fingerprint parameters a browser computes from its canvas and
mouse, and even then it answers unreliably; Bilibili's captcha meets anonymous
callers after a few requests and blocks the server's address for hours, a
block the upload read already shares. A dynamic's own opus page, by contrast,
renders its state on the server and answers anonymously.

## Decision

The listing runs only when the operator configures `BILIBILI_SESSDATA`.
Without it the `bilibili` step records the listing as skipped with
`login_required` and makes no anonymous attempt; no fingerprint parameters
are generated. The cookie stays in memory, goes only to `api.bilibili.com`,
and an expired one is reported as `bilibili_login_expired`.

An operator can still add one dynamic by its link without the cookie: that
read uses the opus page on `www.bilibili.com`, one request, under the same
captcha backoff.

The listing reads the first page only, at most once per
`BILIBILI_MIN_INTERVAL_MINUTES` on its own clock, and shares the account's
captcha backoff with the upload read.

## Consequences

- A deployment without a Bilibili account lists no dynamics automatically;
  operators add the ones they want by link.
- Imitating a browser fingerprint is never part of the client, so a change
  to Bilibili's anonymous checks cannot turn into a captcha loop.
- The cookie is that account's credential. Operators are told to prefer a
  secondary account and to replace the value when it expires.
- Dynamics posted faster than one page per interval are missed by the
  listing; Kano posts a few a week, far below a page of about a dozen.

## Related Docs

- [Sources: Bilibili](../architecture/sources.md#dynamics)
- [Configuration](../operations/configuration.md)
