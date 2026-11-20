# Fetch Methods

The read methods each source offers to fetches.

## X timeline method

Timeline fetches page the public FxTwitter timeline
(`api.fxtwitter.com/2/profile/<handle>/statuses`) newest first. FxTwitter
lists 20 statuses a page whatever `count` a request names, so a `count` of
200 takes ten pages, more than the default
[page budget](manual-fetches.md#budgets-and-checkpoints). A `count` fetch always begins at the
newest page and keeps no cursor, so it reads the newest posts every time; one
its budget stops short of its count reports
`history_page_budget_reached; raise the fetch pages to read the whole count`.
An `all` fetch the budget stops reports `history_page_budget_reached` and
continues when repeated. Each status is stored from the timeline page itself, without
vxtwitter detail requests or the incremental pending queue. A `count` fetch
stops after the account's newest `count` own statuses, and an `all` fetch at
the end of the timeline. Reposts are skipped and replies are not listed. A
status the timeline lists without usable content is counted as failed and
reported as `<id>: x_invalid_response`; a timeline that ends before a `count`
fetch's count reports `x_timeline_end`. Timeline pages use the X source
client's protections: a 10-second timeout, a 4 MiB response limit, rejected
redirects and controlled `x_*` error codes.

## X API method

`backend/internal/source/history` calls only
`https://api.x.com/2/tweets/search/all`
([X full-archive search](https://docs.x.com/x-api/posts/search/quickstart/full-archive-search),
which needs full-archive access). The environment-only credential travels in
the `Authorization: Bearer` header, never in URLs, SQLite or results. Each
request has the request timeout and pages wait the request spacing apart (see
[sync settings](../../operations/configuration/sync.md#sync-settings)), and redirects are rejected so the
credential header never follows one. Each response has a fixed 4 MiB byte
limit: the adapter checks `Content-Length` and counts streamed bytes,
cancelling an oversized body before parsing or writing its page. Posts are
mapped as the X chain's API method maps them, quoted statuses included (see
[X API in the chain](../sources/x.md#x-api-in-the-chain)), and the posts a search
returns count toward the day's X API usage; a manual fetch ignores the
chain's gate and daily cap. Searches end at least ten seconds before the
present. Without a start time the endpoint
covers only 30 days, so a `count` search starts at the Snowflake epoch
(November 2010), where status IDs begin; it asks for the posts it still needs,
10 to 100 a page, and stops at the count. X bills every post a search reads.

Failures reach run history only as controlled codes:
`source_api_http_<status>`, `source_api_request_failed`,
`source_api_response_too_large`, `source_api_invalid_response`,
`source_api_invalid_records` and `source_api_repeated_page`, never upstream
bodies or credentials. A fetch that stops at its page budget reports
`history_page_budget_reached; repeat this fetch to continue`. Source access,
quota and search coverage depend on the platform; this feature does not
promise a complete export of its archive.

## YouTube methods

The API and RSS methods read through the YouTube source client, with its 10-second
timeout, 4 MiB response limit, rejected redirects and controlled codes. The
`api` method pages the channel's uploads playlist (`playlistItems.list` on the
`UU` playlist, the key only in the `X-Goog-Api-Key` header), newest first; an
entry that a new upload shifts onto the next page is listed once. The `rss`
method reads the public feed once, with the feed's retries after an upstream
5xx or connection failure (see [YouTube](../sources/youtube.md)). Failures record
`youtube_api_*` or `youtube_feed_*` codes.

The `ytdlp` method requires `YTDLP_BINARY` and runs the configured executable
with `--flat-playlist --dump-single-json --skip-download`. It reads the newest
channel entries, maps the metadata yt-dlp provides onto the stored video fields,
and never downloads media. A `count` fetch is capped at 200 entries; an `all`
fetch uses the same bounded 200-entry request and reports more work when the
executable returns exactly 200 entries. It records `youtube_ytdlp_*` codes.
The executable follows the YouTube
[proxy route](../outbound-proxy.md#yt-dlp).

Videos are written as the listing describes them: title, publication time,
largest thumbnail and an `unknown` kind, since a listing cannot tell a
broadcast from an upload. The `api` method then checks
the details of each page's videos that are not stored yet, and of the stored
ones too with `refresh`, through one `videos.list` call per 50 (see
[Data API](../sources/youtube.md#data-api)). It follows the recent step's rules: the
classification, the per-format playlists for an ended broadcast without a
stored premiere verdict, localized titles, and
the recent step's upsert of videos and reservation events, so manual locks
and deletion tombstones hold. It reads no watch page, so an ended broadcast
the answer cannot classify is written as the API describes it (an
[unclassified answer](../sources/youtube.md#data-api), which creates no event), and a
video the API leaves undecided keeps its listed row. The answers commit with
their page and its checkpoint. A failed details call records its
`youtube_api_*` code and leaves the step `partial`; the page's videos stay as
listed and the fetch goes on.

A `count` fetch reads the newest page of each format playlist, as the recent
step does, at most once per details call. An `all` fetch reaches broadcasts
far below those pages, so it pages both playlists to their end instead, at
most 20 pages (1,000 entries) each: once per run, the first time a page's
details name an ended broadcast that needs a verdict, keeping the two
membership sets in memory for the rest of the run. A video on the live list
is a stream and one on the long-form list a premiere; one seen on both, or on
neither, gets no verdict. A list that the cap or a failed page cuts short
records its code (`youtube_format_list_incomplete`, or the page's
`youtube_api_*` code) and names only what it read, so a broadcast it did not
reach gets no verdict. An `all` fetch therefore spends about two quota units
per 50 new uploads (a playlist page and a details call), plus one unit per 50
entries of each format playlist once per run when it holds an ended broadcast.

The `rss` method only lists: its 15 uploads are within the recent step's own
listing, which checks their details. Neither method reads the streams page,
watch pages or the detail queue, and neither writes incremental state. The
`ytdlp` method also writes only the listed video rows and does not advance the
timed source cursor.

## Bilibili methods

The `space` method of the uploads pages
`/x/series/recArchivesByKeywords` by page number, and that of the dynamics
pages `/x/polymer/web-dynamic/v1/feed/space` by the offset each page
answers, both newest first, through the Bilibili client the timers read with
(see [Bilibili](../sources/bilibili.md)). Pages are 15 seconds apart, far wider
than the other sources' request spacing, because Bilibili answers quick runs
of requests with a captcha. The dynamics fetch needs the login cookie and
checks it and the WBI keys as the timed listing does when they are due; it
fails with `bilibili_login_required` without a cookie and
`bilibili_login_expired` while the latest check found it expired. Each page
keeps only the account's own records, as the timed reads keep them.

A fetch shares the timers' captcha backoff and is never forced: while the
backoff lasts it fails at once with `bilibili_blocked` and makes no request,
and the Sources page disables its form. A captcha met on the first page
fails the fetch with its `bilibili_risk_control_*` code and starts the
backoff; one met later keeps the committed pages, records the code and
stops. A successful page lifts the backoff and records the read's success.
A fetch ignores `BILIBILI_MIN_INTERVAL_MINUTES` and leaves the timers'
attempt clocks alone, so it never delays or advances a timed read. The
upload read's newest-upload cursor only moves forward.
