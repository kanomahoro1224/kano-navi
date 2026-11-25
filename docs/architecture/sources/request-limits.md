# Source Request Limits

Request limits and budget fairness shared by every source.

Every source client requests fixed destinations with a 10-second timeout
(the X API, manual or in the chain, uses the request timeout setting; see
[X API method](../workflows/fetch-methods.md#x-api-method))
that stays active through body consumption, rejects redirects, and records
only controlled error codes. The Bilibili client builds its URLs itself on
its three fixed hosts and checks the mid and the dynamic ID before a
request; the shop client builds its four fixed paths on `shop.milpr.com`
(see [Official shop](shop.md#official-shop)). Response bodies are limited to 4 MiB
for X, YouTube and the shop and 2 MiB for Bilibili; a timed-out or oversized stream is cancelled
and releases its reader so the single-flight queue can run a subsequent job.

The YouTube client builds every public URL itself on `https://www.youtube.com`:
`/feeds/videos.xml?channel_id=<id>`, `/channel/<id>/streams`,
`/channel/<id>/videos` and `/watch?v=<id>`, after checking that the channel
ID matches `UC` plus 22 characters and the video ID 11 characters. The Data
API is reached only at `https://www.googleapis.com/youtube/v3/` (see
[Data API](youtube.md#data-api)). A redirect answer is reported as
`youtube_redirect_rejected` and an oversized body as
`youtube_response_oversized`, with `youtube_feed_` in place of `youtube_` for
the feed.

## Budget Fairness

Incremental X rotates the first account across runs using independent
`x-budget` state, so a one-request budget also serves later accounts. Failed
requests consume their turn and budget. Each account retains its own durable
pending-ID queue and incremental cursor. YouTube detail work uses an independent
`youtube-details` durable queue: new discoveries join the tail, attempted work
rotates before its request, failed work remains retryable, and completed ordinary
videos leave the queue. Reservations continue to rotate for refresh. Reopening
SQLite preserves those turns. Manual fetches use separate rotation/queue
identities and never rewrites incremental cursors or their check metadata.
