# Packaged Media

Packaged LINE BLOG, discography, Journey and Works upload media.

## LINE BLOG Archive Media

`data/lineblog/` holds the recovered LINE BLOG images and videos, written only
by the maintainer's packaging command
([LINE BLOG archive maintenance](../../development/lineblog-archive.md)). They
have no database rows: the tracked `src/lineblog/media-catalog.json` lists
them. `GET /media/lineblog/<name>` (and `HEAD`) opens the directory as a root
and serves only a regular file directly inside it whose name is 12 lowercase
hex digits and `.jpg`, `.png` or `.mp4`; the type comes from the extension.
Responses carry `nosniff`, `default-src 'none'; sandbox`, an ETag of the name,
size and modification time, byte ranges, conditional requests and the
[media body time limits](layout-and-model.md#http-contract); images whose bytes match
their name are immutable and everything else revalidates
([Immutable Fixed Files](layout-and-model.md#immutable-fixed-files)). Any
other name, a directory, a symlink or a missing file answers `404`. The
server never writes this directory. Its images have smaller copies (see
[Resized Copies](resized-copies.md)); videos ignore `w`.

## Discography Covers

`data/works/` holds the cover images of the discography page, written only by
the maintainer's packaging command
([Works archive maintenance](../../development/works-archive.md)). They have no
database rows: the tracked `src/works/media-catalog.json` lists them.
`GET /media/works/<name>` (and `HEAD`) is served like
[LINE BLOG archive media](#line-blog-archive-media), with the same headers,
containment and `404` answers, except that a name ends in `.jpg` or `.png`
only: every cover is an image named after its bytes' SHA-256, so it has
smaller copies and is immutable once its bytes match its name. The server
never writes this directory. The uploads' covers are not among them; they are
[cached per instance](#works-upload-covers).

## Journey Images

`data/journey/` holds the Journey page's own images, written only by the
maintainer's packaging command
([History archive maintenance](../../development/history-archive.md)). They have
no database rows: the tracked `src/history/media-catalog.json` lists them.
`GET /media/journey/<name>` (and `HEAD`) is served like
[discography covers](#discography-covers): a name is 12 lowercase hex digits
of the image's SHA-256 and `.jpg` or `.png`, every image has smaller copies,
and it is immutable once its bytes match its name. The server never writes
this directory.

## Works Upload Covers

The Works page's uploads take their covers from this cache, so a fresh
deployment fills them itself. The server embeds the packaged upload list
(`internal/works`, see
[Upload covers](../../development/works-archive.md#upload-covers)), and every
media step, before its download, walks each upload's videos in cover order
(`PlanWorkCovers` in `internal/store/sqlite/works_covers.go`):

- a video whose stored row has a ready thumbnail, or whose works image is
  ready, is the upload's cover;
- a stored video whose own thumbnail is still pending or failing waits for
  it, so the shared image is never fetched twice;
- a YouTube video with neither gets its fixed thumbnail address,
  `https://i.ytimg.com/vi/<id>/maxresdefault.jpg`, registered at once (it
  steps down to `hqdefault` like any YouTube thumbnail);
- a niconico or Bilibili video is looked up first: niconico's public
  `getthumbinfo` answer names its thumbnail on `nicovideo.cdn.nimg.jp`
  (the large `.L` size is asked for, stepping down to the plain one, which is
  all older videos have), and Bilibili's view answer names its cover on
  `*.hdslb.com` (see [Sources](../sources/shop.md#works-cover-lookups));
- a video whose image is missing, at its source or as a ready row whose
  cached file is gone, or whose lookup found it deleted, is passed over for
  the upload's next video (the media row stays), and an upload with none left
  shows a printed sleeve.

A found image is an ordinary asset linked as `work-video /
<platform>:<id> / thumbnail / 0` and downloaded by the same step under the
platform's namespace; one URL is one asset, so a video synchronization
stores later shares the file. Lookups are recorded in `work_cover_lookups`
(see [Data model](../data-model/sqlite-snapshot.md)): a deleted video is never asked
again and a failed lookup waits from 30 minutes, doubling to a day. A works
image that answers `media_missing` and no stored record links is not
retried. Nothing here writes a `videos` row.

The console's Site page shows how the covers stand without a request to a
platform (`GET /api/admin/media/works-covers`, see
[Admin API](../admin-api/overview.md#works-upload-covers)): the covered, due, waiting and
uncovered uploads, and each upload without a cover with its videos' states,
last error codes and retry times. Its **Fill missing covers** starts a
one-step media run on the shared queue, so a deployment whose source timers
are off fills its covers too; each run keeps the lookup bounds above, and
running it again continues.
