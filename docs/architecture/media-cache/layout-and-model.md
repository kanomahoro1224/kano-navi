# Layout, Model and HTTP Contract

Where media lives, how it is registered, and how it is served.

## Runtime Layout

```text
data/
  database/
    kano.sqlite
  x/
    sha256/<first-two>/<content-sha256>.<extension>
  youtube/
    sha256/<first-two>/<content-sha256>.<extension>
  bilibili/
    sha256/<first-two>/<content-sha256>.<extension>
  niconico/
    sha256/<first-two>/<content-sha256>.<extension>
  avatar/
    avatar.<extension>
    banner.<extension>
    sha256/<first-two>/<content-sha256>.<extension>
  lineblog/
    <12-hex-name>.<jpg|png|mp4>
  works/
    <12-hex-name>.<jpg|png>
  cache/media/
    sha256/<first-two>/<content-sha256>.<extension>
  cache/variants/
    <first-two>/<content-sha256>-<width>.<jpg|png|orig>
  cache/file-hashes/
    <first-two>/<path-sha256>
```

The whole `data/` directory is ignored by Git. It is created on demand by the
server or synchronization command. Each write goes to a randomly named
temporary file in its target directory (`.download-*`, `.part-*`,
`.variant-*` or `.hash-*`) that is atomically renamed into place. The tracked
`public/assets/` directory is reserved for fixed fallback branding.

Discovered X and YouTube profile candidates stay in their source namespaces;
local uploads (`data/avatar/sha256/`) and the selected avatar and banner files
remain runtime data in `data/avatar/`. A `cache_path` that starts with
`sha256/` resolves under `data/cache/media/`; readers serve such rows, while
the downloaders write only the source namespaces.

## Database Model

`media_assets` stores one row per normalized HTTP(S) source URL:

- `id`: the lowercase hex SHA-256 of the normalized source URL (HTTP or HTTPS,
  no userinfo, lowercase host, no fragment, an empty path becomes `/`), so a
  URL always maps to the same row. A row registered under a short
  `x-<32 hex>` identity is re-keyed to this identity, together with its links
  and cache metadata, when its URL is registered again;
- `source`, `source_url`: the platform adapter and original public URL;
- `status`: `pending`, `ready`, `missing`, or `failed`;
- `cache_path`: a path relative to the runtime `data/` directory, under `x/`,
  `youtube/`, `bilibili/`, `niconico/` or `avatar/`, or a `sha256/` path under
  `cache/media/`; never an absolute path or one with `..`;
- `mime_type`, `extension`, `byte_size`, `sha256`, `width`, and `height`
  (read from the image header; `0×0` marks a header that could not be read,
  such as AVIF, and reads report it as unknown);
- fetch/check/seen timestamps, validators (`etag`, `last_modified`), and the
  last error for maintainer diagnosis.
- `retry_attempts` and `next_retry_at`: persisted failure backoff.

`media_links` associates an asset with a domain record without coupling the
media table to every content table. Its owner tuple is `(owner_type, owner_id,
role, position)`, for example `post / 209... / post-image / 0` or `video / abc /
thumbnail / 0`. A post's own images use `post-image`; the images of a status
it quotes, or of the dynamic a Bilibili forward forwards, use `quote-image`
under the same post, so the board shows them in the quote card and
extraction never reads them (see [Quoted posts](../sources/quoted-posts.md) and
[Forwards](../sources/bilibili-dynamics.md#forwards)). An X post's `post-image` and `quote-image`
links record in `kind` what the image shows: `photo`, or `video` or `gif`
for the thumbnail of one; `NULL` reads as `photo`. The asset is always the
image: the cache never downloads a video or GIF file. A Bilibili dynamic's stickers use
`post-emoji`, and those of the dynamic a forward forwards `quote-emoji`,
under the same post: one link per distinct picture, at most 32 each, and one
asset per picture however many posts use it. They are downloaded like any
asset, shown only inside the post's original wording (see
[Bilibili stickers](../data-model/bilibili-stickers.md)) and never schedule
candidates. A row of the X or Bilibili status pool has
no media link: a related account's status registers its images as
`post-image` links only when it is promoted into posts (see
[Related accounts](../sources/x-accounts.md#related-accounts)), so nothing is downloaded
for statuses that stay in the pool.

The `media_url`, `thumbnail_url`, and image URL columns of content tables are
source-snapshot fields and are not treated as safe browser URLs by the
dashboard query.

The cache accepts only AVIF, GIF, JPEG, PNG, and WebP. SVG and
arbitrary document MIME types are rejected so cached platform input cannot be
served as active same-origin content.

## HTTP Contract

The server exposes `GET /media/<id>` for ready assets only. The ID must match
the opaque 64-character format, the database row must be `ready`, and the
resolved file must remain under the cache root. The response never accepts a
filesystem path or a source URL from the request. Missing, unsafe, or unready
assets return `404`. A `v` that names a `media_versions` hash of a ready asset
serves that kept earlier file under the same containment and hash checks; any
other version answers `404`.

The file is streamed from disk, never read whole into memory, with its row's
MIME type, `nosniff`, `default-src 'none'; sandbox`, the content hash as its
ETag and the fetch time as `Last-Modified`; ranges, `HEAD` and both
conditional validators are answered from the open file. Its bytes are checked
against the row's hash once per file, not per request: the download records
the hash it wrote, and a file without a record is hashed once when it is
first served ([File Hashes](resized-copies.md#file-hashes)). A file whose size or modification
time changes is checked again, and one whose bytes do not match answers `404`.

A media body is not bound by the [listener's write
timeout](../backend.md#listener-timeouts): each write must finish within 30
seconds and the whole body within 30 minutes. A large video therefore reaches
a slow client that keeps reading, and a client that stops reading is let go.

Dashboard media fields include the safe `...Url` (a
`/media/<id>?v=<content-sha256>` route or a tracked `/assets/...` fallback), the
`...Status`, the opaque ID, and the public source URL for traceability. The
version parameter changes whenever bytes change, so versioned responses can be
cached as immutable even though the source identity is stable. With
`CDN_PUBLIC_BASE` set, the routes the public reads return carry that base in
front of them; the origin keeps serving the same paths for the CDN to pull
(see [CDN](../../operations/cdn.md)). A bare media ID
is revalidated, and an unready remote URL is represented by `null`, so the
browser does not silently fall back to a CDN.

## Immutable Fixed Files

LINE BLOG archive images, discography covers and Journey images are
addressed by their bytes' SHA-256, so the server lets browsers keep them for a
year without revalidating (`public, max-age=31536000, immutable`) once it has
checked that the bytes match the address:

| Request                                             | Immutable when                                          |
| --------------------------------------------------- | ------------------------------------------------------- |
| `GET /media/lineblog/<name>.(jpg\|png)[?w=<width>]` | the file's SHA-256 starts with the name's 12 hex digits |
| `GET /media/works/<name>.(jpg\|png)[?w=<width>]`    | the file's SHA-256 starts with the name's 12 hex digits |
| `GET /media/journey/<name>.(jpg\|png)[?w=<width>]`  | the file's SHA-256 starts with the name's 12 hex digits |

Everything else answers `public, max-age=0, must-revalidate`: LINE BLOG
videos (named after their watch URL, not their bytes) and an archive image,
cover or Journey image whose bytes do not match its name. A copy (`w`) takes its original's verdict, since
the variant cache only stores a copy made from bytes with the hash it is
filed under. The hash is the file's remembered one
([File Hashes](resized-copies.md#file-hashes)); a file whose size or modification time
changes is hashed anew, so its old addresses revalidate.

## Verification

Run `make go-test` for path, identity, registration, downloader-host,
MIME, and dashboard contract tests. `make check` runs those tests together with
the sensitive scan, documentation checks, and the frontend build. Tests use
synthetic response bodies; CI does not download live media.

## Complete Post Media Declarations

A successful complete post update replaces only its `post-image` and
`quote-image` links inside the post transaction, preserving declared order and
removing vanished positions.
An explicit empty media list clears the links and primary source URL; an omitted
media declaration retains the previous snapshot. Failed reads make no post write.
Unreferenced cached bytes may be retained, but neither post display nor extraction
uses those old links. Extraction waits for every declared image and its contained
regular cache file; one ready image cannot stand in for a multi-image post.
