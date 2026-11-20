# Lifecycle

How media downloads run and how profile media and avatars are selected.

1. A source adapter discovers a remote image, and its snapshot write
   registers it inside the same transaction (`replaceMediaLink` in
   `internal/store/sqlite/source_write.go`).
2. The candidate is deduplicated by normalized source URL, recorded as
   `pending`, and linked to its owner. Registration does not download bytes.
3. The media step works through the queue in [batches](#step-bounds) of 20
   assets until it is empty. Never-checked assets come first, ordered by their
   newest owner (a video's scheduled or published time, or the publication
   time of the post or schedule post that links it), so thumbnails the board
   shows first are not left pending by a same-synchronization tie. Already
   checked assets and assets without a dated owner follow by creation or
   last-check time. It downloads within the
   [download bounds](#download-boundary), writes the bytes through a root
   confined to `data/` into
   `<source>/sha256/<first-two>/<content-sha256>.<extension>`, reuses an
   existing file there only when its hash agrees and otherwise atomically
   replaces it with the verified bytes, since the content address is
   authoritative, and records the row as
   `ready` with the width and height its header declares
   (`SaveMediaDownload` in `internal/store/sqlite/media_commands.go`). The same
   step measures up to 50 ready files cached before dimensions were probed,
   reading each contained, hash-checked file's header only, and reports them
   as `measured`; a run that measures any advances the dashboard revision. It
   then makes the [resized copies](resized-copies.md#making-copies) of up to 100 ready images
   that have none yet, newest first, and reports them as `resized`.
4. A failed or unavailable request updates the row to `failed` or `missing`.
   Existing ready files are retained; a failed synchronization never clears a
   known snapshot. Downloaded bytes that cannot be published, such as a
   content address the step cannot write, record `media_write_failed`.
   Transient failures (`media_timeout`, `media_unavailable`,
   `media_body_unavailable`, `media_rate_limited` and `media_write_failed`)
   back off from 30 minutes,
   doubling to a maximum of 24 hours. Missing files and other HTTP errors are
   retried after 24 hours. Rejected destinations, invalid images and oversized
   bodies are not retried automatically. Rediscovering the same URL preserves
   its retry deadline, including for ready files whose revalidation failed.
   A successful download or 304 revalidation clears the failure state.
5. A YouTube thumbnail that answers 404 for `maxresdefault` or `sddefault`
   (including their `_live` forms) is requested at the next smaller size,
   ending with `hqdefault.jpg`, and a niconico `.L` thumbnail that answers 404
   is requested at its plain size. The bytes are stored under the original
   URL, so the asset identity and links do not change.
6. A ready YouTube thumbnail linked to an upcoming video, or to one that
   started or was published within the last 24 hours, is revalidated at most
   every 30 minutes with its stored `ETag` and `Last-Modified`. So is the
   thumbnail of any video whose watch page was read after the file was last
   checked (a [follow-up check or refresh request](../sources/youtube-videos.md#follow-up-checks-and-refresh-requests)),
   whatever its age. A 304 only records the check; new bytes become the next
   content-addressed version, and only a changed hash advances the dashboard
   revision.
7. The file a revalidation replaces stays in the cache. `media_versions`
   keeps its hash, path, MIME type, size and fetch time under the asset ID,
   and every video linked to the asset records the replaced thumbnail in
   `video_revisions`. An operator can pin such a version, or the file of an
   earlier thumbnail URL, from the video's history.

## Step Bounds

One media step attempts at most 500 files and starts no new file after five
minutes. It then makes copies for at most 100 images and starts none after
two more minutes, which leaves the rest of the run's 20 minutes to the
steps after it. Images the bound leaves are made by the next step.
Each asset is attempted at most once per step, so an asset that stays due
after its check cannot repeat; a batch that holds no asset new to the step
ends it. A file that fails, including one whose bytes cannot be published,
does not end the step; a store that cannot record the failure, such as after
a lost lease, does. A `media_rate_limited` answer ends the step at once. The bounds are
constants (`StepBounds` in `internal/mediadownload`).

The step result records `attempted`, `ready`, `unchanged`, `failed`,
`measured`, `resized` and `remaining`: the never-checked assets still queued when the
step ends. When a bound ends the step, `stopped` names it
(`media_file_cap`, `media_time_budget`, `media_rate_limited` or
`media_no_progress`). The step is `partial` while `remaining` is above zero or
any file failed or was missing, and the next synchronization continues with
the queue, so a history window's images arrive over a few runs.

## Download Boundary

`internal/mediadownload` accepts only HTTPS URLs on port 443 on X's
`*.twimg.com` media CDN subdomains, YouTube's `*.ytimg.com` thumbnail CDN
subdomains, Bilibili's `*.hdslb.com` cover CDN subdomains (`i0`, `i1`, `i2`
and so on) and niconico's one thumbnail host, `nicovideo.cdn.nimg.jp`. The
suffix check requires a real subdomain boundary, so
lookalike hosts and the bare parent domains are rejected, as are userinfo,
fragments and query keys that look like credentials (`token`, `secret`,
`auth`, `api_key` and the like). Redirects are followed by hand: at most three,
each revalidated against the same policy. One ten-second deadline covers the
redirects, headers and body of a request (a smaller YouTube thumbnail size
gets its own). Bodies are capped at 10 MiB, and the sniffed magic bytes must
be a supported image whose type agrees with a declared image `Content-Type`.
Failures become fixed codes (`media_destination_rejected`,
`media_redirect_rejected`, `media_timeout`, `media_missing`,
`media_response_oversized`, `media_invalid_image` and so on); no upstream body
is stored.

The host policy is kept in code because this is also an SSRF boundary; the
broader privacy allowlist is not a download permission list. The cache
accepts image MIME types only, so X video URLs may be recognized as official
media while remaining unavailable to the image cache. A failed redownload
keeps the ready file, and versioned public reads never fetch media. Cached
files, including versions a revalidation replaced, are kept deliberately;
there is no garbage collection.

Content-addressed paths are deliberately preferred over layouts such as
`pics/x/<timestamp>` or `pics/youtube/<video-id>`. Ownership already lives in
`media_links`; storing bytes by hash deduplicates identical files, makes path
validation deterministic, and allows atomic replacement without trusting a
platform identifier as a path component.

## Selected Profile Media

Profile discovery reads at most 4 MiB of page bytes. Candidate downloads read
at most 15 MiB of image bytes, verify MIME and magic bytes, and follow at most
three independently allowlisted redirects under one deadline covering headers
and the complete body. Timeout/oversize failures cancel the response.

A candidate download and an explicit selection are separate operations. A failed
redownload records its outcome/error while keeping any ready candidate and
selected file available. A successful redownload updates the candidate hash and
MIME only; the selected file keeps its own active hash/MIME and versioned public
URL until the operator selects it. `/media/profile/:slot` validates and serves
that selected version, so its bytes, ETag, and URL hash agree. Selecting writes
`data/avatar/<slot>.<ext>` for the chosen slot. Deleting a candidate removes
only its row: its content-addressed file and any selected file stay on disk.

Startup repairs selected hash/MIME from the selected file's bytes rather than
the candidate, whatever the stored active metadata says. This is
idempotent and retains the manual selection and candidate version. Reads require
both the stored path and its symlink-resolved target to stay inside `data/avatar/`,
a regular file under the 15 MiB profile limit, and the existing supported-image
magic-byte validation. Unavailable/illegal files clear only active hash/MIME,
leaving other snapshots and configuration intact. HTTP uses the same bounded
reader, checks its hash/MIME against active metadata, and sends those exact bytes;
an unexpected file replacement returns 404 until reconciled or selected again.

reads and writes stay bounded under the same path, MIME, magic-byte, and
redirect limits as [Selected Profile Media](#selected-profile-media).

## Account Avatars

Per-account X avatar downloads queue into `media_assets` with `source = 'x'`
and no `media_links` rows, so the `media` sync step fetches remote `pbs.twimg.com`
images under `data/x/sha256/`. Manual uploads store content-addressed images
under `data/avatar/sha256/`. `GET /media/account-avatar/{id}?v={sha256}` (and
`HEAD`) serves a ready candidate by validated 64-hex candidate ID and matching
SHA-256 hash using the same root-contained streaming checks and immutable cache
headers as `/media/{id}`.

Workflow and operator downloads synchronize every candidate linked to the same
`media_id` in one transaction, including ready candidates whose bytes change.
An unchanged manual or hidden selection stays selected or hidden. Automatic
selection follows the [candidate observation order](../data-model.md)
rather than download completion. A changed media version or visible avatar
advances `dashboard_revision`; replaced shared media files retain their version
metadata. Reporting a cached URL reconciles its candidate with the stored asset.
