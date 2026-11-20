# Bilibili Dynamics and Forwards

How Bilibili dynamics and forwards are read and stored as posts.

## Dynamics

The scheduled `bilibili_dynamics` source requires a signed-in caller
(`BILIBILI_COOKIE` or `BILIBILI_SESSDATA`). Without the cookie it records
`{ skipped: true, reason: "login_required" }` and makes no anonymous attempt.
Its configurable [method chain](../workflows/source-schedules.md#method-chains) tries the desktop
feed, `/x/polymer/web-dynamic/desktop/v1/feed/space?host_mid=<mid>`, before
the WBI listing, `/x/polymer/web-dynamic/v1/feed/space?host_mid=<mid>`.
The desktop method does not use WBI signing; the `dynamics` method signs with
Bilibili's WBI scheme (`wts` and `w_rid` from the rotating keys that the ticket
or `/x/web-interface/nav` names). WBI keys are cached for 12 hours and dropped
after a `v_voucher` or `-352` answer, which may mean they rotated.

The desktop feed's dynamic IDs sort numerically descending, matching
publication order. Ordinary endpoint failures can use the next enabled method.
Login and risk-control failures, including 429, stop the chain and start or
preserve the shared captcha backoff; no fallback request is made while blocked.
A signed-out `nav` answer, or `-101` from either listing, is
`bilibili_login_expired`. Both methods share the upload read's captcha
backoff and device identity, so a block either one meets holds both back. The
dynamics source runs on its own clock at least
`BILIBILI_MIN_INTERVAL_MINUTES` apart. One timed read requests the first page
only, plus the device identity and ticket when due; `nav` runs for the daily
login check, or when the WBI method is reached with stale keys. A desktop
success needs no WBI refresh. Manual fetches use their explicit WBI `space`
method and follow each offset to older pages.

A listing keeps the account's own `DYNAMIC_TYPE_WORD` and `DYNAMIC_TYPE_DRAW`
items (the opus and picture cards), `DYNAMIC_TYPE_ARTICLE` items only in
their `MAJOR_TYPE_OPUS` form, whose text comes from the opus summary, and
forwards (`DYNAMIC_TYPE_FORWARD`, see [Forwards](#forwards)); a live-start
notice and a collection update are not posts. A supporters-only dynamic
(`basic.is_only_fans`, 充电专属) is never a post, even when a supporter's
login carries it in full. An upload's dynamic
(`DYNAMIC_TYPE_AV` with a `MAJOR_TYPE_ARCHIVE` card) of the account is not a
post either: the read stores it as the upload it publishes (BV ID, title,
cover, description, length from `duration_text`, published at the dynamic's
time), written as the upload list writes one, so a new upload reaches the
store even while the list meets the captcha. The upload read's newest
cursor stays its own. The stored text joins the nodes' text (words, topics, mentions,
links) and leaves out emoji nodes (`RICH_TEXT_NODE_TYPE_EMOJI`); an opus
title leads it, and a picture post's `desc` nodes give it beside its
pictures. A dynamic without rich-text nodes, as older listings give one,
takes its plain text field instead (the opus summary's, else `desc.text`),
HTML-unescaped; it names no sticker pictures, so any caption in it stays as
written. An older album without a major type lists its pictures under `draw`
or `opus`, and both are read. The nodes, emoji included, stay in `raw_json`. An
emoji-only dynamic therefore has empty text and is translated as `none`
without a model call. Up to nine pictures are kept as the post's images.

An emoji node is a sticker (custom emoji): its text is the caption, such as
`[笑]`, and its picture is one of the node's `webp_url`, `gif_url` or
`icon_url`, preferred in that order and kept only as an HTTPS address on
`*.hdslb.com`. `raw_json` keeps the node with its caption and that picture
as `emoji.url`. The distinct pictures of a dynamic, at most 32, are
`post-emoji` links of its post, so they are [cached](../media-cache.md) like
its images; they are never images of the post, schedule candidates or
extraction input, and the text, its translations and extraction never
include a caption. The board places each sticker into the stored text by
its node's order (see
[Bilibili stickers](../data-model/bilibili-stickers.md)).

Dynamics are `posts` rows with `source = 'bilibili'`, the ID
`bilibili-<id_str>` (dynamic and X status IDs are both numbers of about 19
digits and share `posts.id`), `account_handle` the mid, `url`
`https://www.bilibili.com/opus/<id_str>` and `published_at` the publication
time in UTC. They have no engagement counts; a forward has a quote. A read
replaces a dynamic's text, time, pictures, stickers and quote, keeps an
operator's hide, and advances the dashboard revision only when the text,
time, link, pictures, quote, quoted pictures, either set of stickers, the
nodes that place the stickers of a dynamic that has any, or hide changed.
Its pictures are
`post-image` links, so they feed
[schedule extraction](extraction.md) once cached; a
schedule board on a Bilibili picture is not registered as a
[schedule image](schedule-images.md).

An operator adds or re-reads one dynamic by its `bilibili.com/opus/<id>` or
`t.bilibili.com/<id>` link from `/admin` (see
[Admin API](../admin-api/videos.md#bilibili-uploads)). That read takes the dynamic from
the server-rendered state (`window.__INITIAL_STATE__`) of its
`www.bilibili.com/opus/<id>` page, which needs no login; it respects the
captcha backoff and accepts only dynamics of the configured account whose
comment area is a post's (`basic.comment_type` 11 pictures, 12 article, 17
text) and that the page shows without a fallback; any other is
`bilibili_unsupported_dynamic`. The read takes the dynamic as its page
shows it, with no quote: a forward and its quote come from the listing, and
a single read never rewrites a stored forward (`raw_json.type`
`DYNAMIC_TYPE_FORWARD`), whose row stays as the listing stored it.

## Forwards

A forward is the account's own post that quotes the dynamic it forwards, the
way an X post [quotes a status](quoted-posts.md): it has its own dynamic ID,
publication time and wording, and the listing carries the forwarded
dynamic in full in its `orig` object, so a forward needs no request of its
own. Its text is its `desc` nodes without emoji (or its plain `desc.text`
when it has no nodes), its stickers are its own `post-emoji` links, and it
has no pictures of its own. The listing keeps every forward of the account.
The store leaves out a new forward only when its quote is unavailable and
its own wording has neither text nor sticker captions, which would be an
empty card; a stored forward whose dynamic becomes
unavailable is rewritten with the unavailable quote, loses its
`quote-image` links and advances the dashboard revision, with or without
wording of its own.

`backend/internal/source/bilibili/dynamic.go` reads `orig` into the same
sanitized projection an X quote has, stored in `posts.quote_json`:

- `id` is `bilibili-<orig id_str>`, the post ID that dynamic has (or would
  have) as a stored post, so a forward of an own dynamic resolves to that
  post like an X self-quote;
- `authorHandle` is the author's mid and `authorName` the display name,
  HTML-unescaped, trimmed and at most 50 characters;
- `text` is built like a listed dynamic's (opus title and summary, or `desc`
  nodes, emoji left out); a forwarded forward shows its own wording only and
  is not followed further;
- `publishedAt` is its publication time in UTC, and `url` its
  `https://www.bilibili.com/opus/<id>` page, or for a forwarded upload
  (`DYNAMIC_TYPE_AV`) the `https://www.bilibili.com/video/<BV ID>` page, with
  the video title as text and its cover as the one image.

Its pictures (HTTPS on `*.hdslb.com`, at most nine) are `quote-image` links
of the forward and its stickers `quote-emoji` links, neither ever a schedule
candidate; `quote_json` holds no image address. The forward's
`raw_json.orig` keeps the available dynamic's title and nodes beside its
type and ID, the source of its stickers. A deleted dynamic (`DYNAMIC_TYPE_NONE`, `MAJOR_TYPE_NONE`), a
missing `orig`, a dynamic of any other kind (a live room, a collection,
goods, a vote, an article in its old form), a supporters-only dynamic or one
with neither wording (including sticker captions) nor pictures is unavailable,
with only its ID and its opus page as URL (an upload too); when even its ID
is unknown, `id` is empty and `url` is the
forward's own opus page. An `orig` of an unexpected shape is unavailable
as well, keeping its ID when `id_str` still reads, and never fails the page.
An unavailable quote leaves the dynamic's pool row as it was.

Every available forwarded dynamic also enters the Bilibili status pool,
`bilibili_statuses`, in the forward's transaction, whatever its author. The
pool row and `quote_json` come from the same projection. The row keeps the
post ID, the author's mid and name, the wording, publication time, URL, a
sanitized `raw_json` with the pictures' addresses, and `forwarded_at`, the
publication time of the newest own forward of it. No public reader reads or
joins the pool. The [translate step](translation.md) translates a forwarded
dynamic that is not a stored post by its post ID (`owner_type`
`bilibili_status`), so it is translated once however often it is forwarded,
and [schedule extraction](extraction.md) reads its wording
as context of the forward. A forward's events are Bilibili-sourced events
like any dynamic's.
