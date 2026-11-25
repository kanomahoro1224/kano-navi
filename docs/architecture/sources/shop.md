# Official Shop and Works Cover Lookups

How the official shop is read and how Works covers are looked up.

## Official shop

The `shop` step reads the sale periods of goods Kano takes part in from the
official shop, `https://shop.milpr.com`, a Shopify storefront, so a sale
reaches the board even when no post announces it. It needs no model: the
storefront's public product JSON and catalog feed state everything a period
needs. The step has its own [timer](../workflows/source-schedules.md#source-schedules) and runs
only in server-side workflows; page loads never read the shop.

`backend/internal/source/shop` builds four fixed URLs on that one host and
nothing else; no address comes from data:

- `/collections/new-arrivals/products.json?limit=50`, the newest 50
  products, read by the `signed` and `new_arrivals` methods;
- `/collections/all.atom`, the catalog's Atom feed, read by the `atom`
  method: the newest 25 products, with no further pages;
- `/products.json?limit=250&page=<n>`, the whole catalog, read only by the
  operator's [read-all fetch](../workflows/manual-fetches.md):
  at most six pages, three seconds apart, until a page lists fewer than 250
  products, counting those left out as unusable;
- `/collections.json?limit=250`, the collection list, read at most once per
  run, three seconds after the listing, and only when a listed product is in
  no stored sale of its period.

Every request names the client honestly as `kano-navi-go/shop`. The shop
answers clients that pose as a browser with `429 local_rate_limited`, so the
client never sends a browser's user agent. With an active
[Web Bot Auth](../web-bot-auth.md) key, the client also identifies itself with a
signature: Shopify limits unsigned automated traffic most strictly and gives
signed requests a higher limit, and the shop asks server-to-server clients to
sign. A request is signed after the `shop` [proxy area](../outbound-proxy.md)'s
transport is chosen.

Timed runs, Run now and full syncs list the newest products through the
shop's [method chain](../workflows/source-schedules.md#method-chains), in the operator's order:

| Method         | Request                                   | Signed            |
| -------------- | ----------------------------------------- | ----------------- |
| `signed`       | the newest 50 of the new-arrivals listing | always            |
| `new_arrivals` | the newest 50 of the new-arrivals listing | never             |
| `atom`         | the catalog's Atom feed, newest 25        | when a key is set |

Without a key, `signed` is passed over without a request
(`signed: shop_no_signing_key` in `tried`). Signing is a method of its own
for the JSON listing so an operator can keep an unsigned read beside it,
while the feed, which has no unsigned twin in the chain, is signed whenever
a key exists. The collection list follows the listing that served: signed
after `signed`, or after `atom` with a key, unsigned after `new_arrivals`.
The read-all fetch signs every catalog page when a key is set.

The chain stays polite to an address the shop may already be holding back.
A 429 is the shop's verdict on the whole address, so it ends the run without
another method: a signed 429 is never retried unsigned, and an unsigned one
never moves on to the feed. A timeout (`shop_timeout`) or an unreachable host
(`shop_unavailable`) ends it too. A failure of the method alone
(`shop_http_<status>`, `shop_redirect_rejected`, `shop_response_oversized`,
`shop_invalid_json`, `shop_invalid_atom`) lets the next enabled method try,
three seconds later, but a run makes at most three listing requests, enough
to reach Atom after both JSON methods fail; with the collection list that
is at most four requests. A run whose enabled methods can make no request
fails with `shop_no_method` and makes none.

A 429's `Retry-After` (seconds or a date; a minute when missing, at most a
day) is stored for the source, not the method, and no read, timed or manual,
signed or not, requests the shop before it passes
(`{ skipped: true, reason: "rate_limited" }`). A read whose listing or
collection list fails records the last code and changes no entry. The step
result names the method that served in `listing`, each method reached in
`tried` (`<method>` for the one that served, `<method>: <code>` for the
others), the failed requests' codes in `errors`, and whether the listing was
`signed`; a later method that served makes the run `partial`, with
`fallback` naming it.

The Atom feed is read with `encoding/xml`: an entry's ID
`https://shop.milpr.com/products/<numeric id>` gives the product ID, its
alternate link `https://shop.milpr.com/products/<handle>` the handle, and its
`title`, `published` and Shopify `s:tag` elements the rest. The HTML of its
`summary` is the description the period is read from, as `body_html` is for
the JSON. An entry whose ID or link names another host, or that lacks a
title or publication time, is left out. The feed states no stock, and its
prices, in the summary and the variant elements, are never decoded or kept.

A product counts only when it carries the tag `鹿乃まほろ`, a constant of the
code, since the board is Kano's, and when its description states a dated
period: a label (受注期間, 販売期間, 受付期間 or 抽選受付期間), a dated
opening, and on the same line `〜`, `～` or `~` and a dated close, such as
`＜受注期間＞ 2026年10月24日 〜 2026年11月24日`. Full-width digits read as
digits, every Unicode space (such as the no-break space of `&nbsp;`) reads
as a plain space, a weekday or time may follow the opening, and a close
without a year falls on or after the opening: in the opening's year, or the
next one when its month and day come earlier, and only then is the date
checked, so `2027年12月1日 〜 2月29日` closes on 2028-02-29. A permanent item (`販売期間 2026年10月1日～`
with no close), a product without a period and a period with impossible
dates are skipped and counted by reason (`open_ended`, `no_period`,
`invalid_period`, `untagged`). Stock (`variants[].available`) is not the
period: a sold-out product still has its sale. Prices, stock and images are
never decoded. Products from either listing reach the same grouping and
writing below.

The products of one period form one group, with one entry, not one entry
per product. The shop writes some titles with decomposed kana (`ホ` followed
by the combining voiced sound mark U+3099 for `ボ`) and others with the
precomposed kana, so every product and collection title is stored with each
kana and a following U+3099 or U+309A composed, as Unicode NFC composes
them; nothing a reader sees changes. A group's key is its title without
whitespace:

1. A product that a stored group of the same period holds stays in it, so a
   group keeps its identity however the newest-50 window cuts it.
2. A new product joins the collection whose title, ignoring whitespace, its
   own title starts with, the longest such of at least four characters
   (`鹿乃まほろ 加入記念グッズ` for `鹿乃まほろ加入記念グッズ アクリルスタンド`).
   The group takes the collection's title and links
   `https://shop.milpr.com/collections/<handle>`.
3. Any other product joins the group of its period whose key its title
   starts with, the longest one.
4. Any other starts a group titled by its title without the last word,
   linking its first product's `https://shop.milpr.com/products/<handle>`.
   Shorter titles are grouped first, so `… 2026 ランダム缶バッジ 全8種`
   joins `… 2026`.

Each group is an automatic `sale` event (see
[Sale periods](../data-model/events.md#sale-periods)) with `source` `shop`,
`source_item_id` the group key and `source_key` the period
(`<opens>/<closes>`), and an `event_sources` row of the same identity:

| Field                     | Value                                                                                                                                      |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `title`                   | the group title as the shop writes it                                                                                                      |
| `starts_on` / `starts_at` | the opening date; `starts_at` is the earliest `published_at` of the group's products that falls on that Japan date, to the minute, or null |
| `ends_on` / `ends_at`     | the close date; `ends_at` stays null, since the JSON gives no close time                                                                   |
| `time_precision`          | `exact` with `starts_at`, otherwise `unknown`                                                                                              |
| `url`                     | the collection page, or the first product's page                                                                                           |
| `detail`                  | null                                                                                                                                       |
| `platform` / `platforms`  | `other` / `["other"]`, stated (`platforms_inferred` 0)                                                                                     |

`raw_json`, on the event and its source row, holds only the group's title,
link and collection handle, the matched period line, and each product's ID,
handle, title and publication time (at most 100 products). The source row
follows every read, so a locked or deleted entry still keeps its products and
its identity. A read rewrites an entry only when a shown field changed, and
then advances the dashboard revision. Nothing is retired: a product that
leaves the listing, or the shop, keeps its entry, and a changed period is a
new group whose old entry stays. Manual edits, locks and tombstones win as
for every automatic entry; an operator who adds the close time saves a
locked entry the shop never rewrites. [Rule C](../data-model/duplicate-merging.md)
merges a post's entry of the same sale into the shop's, which states an
opening time and a close date. The read's bookkeeping is the `sync_state`
row of source `shop` and account `shop.milpr.com`; its last success is the
shop's [freshness](../public-api/snapshot.md).

## Works Cover Lookups

The media step finds covers for the Works page's uploads that no stored
video covers (see
[Works upload covers](../media-cache/packaged-media.md#works-upload-covers)). YouTube needs no
lookup. The others are bounded per step: at most 30 niconico and 5 Bilibili
lookups, none started after two minutes, newest upload first.

- niconico: `https://ext.nicovideo.jp/api/getthumbinfo/<sm|nm|so ID>`, one
  request per video through the media
  [outbound proxy](../outbound-proxy.md) area, with a 10-second timeout, a
  256 KiB body limit and no redirects. Only the answer's video ID and a
  thumbnail URL on `https://nicovideo.cdn.nimg.jp/thumbnails/<n>/<n>[.<hash>]`,
  where both `<n>` are the requested video's number, are read; `DELETED`,
  `NOT_FOUND` and `COMMUNITY` mean the video is gone. A deadline before or
  during the body is `niconico_timeout`. The first 429
  (`niconico_rate_limited`) is recorded as a failed lookup and ends the
  step's niconico lookups.
- Bilibili: the view endpoint above, for any uploader, with the account's
  device identity and backoff. A blocked source makes no request, and the
  first captcha or rate-limit answer starts the shared backoff and ends the
  step's lookups. A lookup never changes the list read's interval, last
  success or last error, and the step stops looking up while
  `SKIP_BILIBILI` is set.

The step's result adds `works`: `queued` YouTube images, `lookups`,
`resolved`, `missing` and `failed` lookups, the `covered`, `waiting` and
`uncovered` uploads, and `bilibili: "blocked"` or `"skipped"` when that source
was not asked. A failed lookup makes the step `partial`.
