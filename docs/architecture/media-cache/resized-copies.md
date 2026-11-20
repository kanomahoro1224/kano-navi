# Resized Copies

How resized image copies are made and served.

A `w` query parameter asks for a smaller copy of an image:

| Request                                | Original                                    |
| -------------------------------------- | ------------------------------------------- |
| `GET /media/<id>?v=<hash>&w=<width>`   | a ready asset                               |
| `GET /media/lineblog/<name>?w=<width>` | a `.jpg` or `.png` file in `data/lineblog/` |
| `GET /media/works/<name>?w=<width>`    | a `.jpg` or `.png` file in `data/works/`    |
| `GET /media/journey/<name>?w=<width>`  | a `.jpg` or `.png` file in `data/journey/`  |

`w` must be one of 320, 640, 960, 1280 or 1920; any other value, or a
repeated `w`, answers 404. A LINE BLOG video ignores a valid `w`, and static
files ignore `w` entirely.

All of them share one cache keyed by the SHA-256 of the original's bytes, so an
image found in several places is resized once. A copy is stored only when the
bytes it was made from match the hash it is stored under.

Requests never decode. A request is answered with the stored copy of that
width, or with the original when there is none: not made yet, or marked as
kept for that width. Copies are made ahead of requests:

- the [media step](lifecycle.md) makes them for ready assets, newest first,
  within its [bounds](lifecycle.md#step-bounds);
- the [variants command](#variants-command) makes them for the LINE BLOG
  archive, the discography covers and the Journey images;
- the [copies run](#copies-run), started from the console's Site page, makes
  them for every group within one step's budget.

All three use one core (`internal/variants`), described in
[Copy Groups](#copy-groups).

## File Hashes

The archive files have no database rows, so the server finds
their copies by hashing them itself (contained in their directory like the
original's own response). Cached media files are hashed the same way when
they are checked against their rows. The file is streamed through the hash,
at most two files at a time, and the result is remembered by absolute path,
size and modification time: in memory, and as a one-line record in
`data/cache/file-hashes/<first-two>/<path-sha256>`. A file is therefore read
for its hash once, not on every request or restart. A file whose size or
modification time differs from its record is hashed anew; one replaced with
the same size and modification time keeps the remembered hash until its
record is removed. An archive file larger than 32 MiB has no copies and is
not hashed for one.

## Making Copies

Making an image's copies takes one of two source slots, kept from reading the
original (at most 32 MiB) through checking its hash and decoding it once
(JPEG, PNG or WebP no larger than 40 megapixels, about 160 MB as RGBA). At most
two originals and their decodes are therefore in memory at once in a process.
A maker waiting for a slot holds no original, and concurrent makers of one
image wait for the first and then find its copies.

The original is scaled once to the widest missing width it is wider than;
every narrower width is scaled from that copy, so a large original is scanned
once rather than once per width, and the decoded original can be released
before the narrower widths are made. Scaling averages each target pixel over
the source area it covers, on premultiplied colour so transparency blends
correctly. It reads the source one row at a time and holds only the target
and one row of sums, so scaling adds about the target's size (a 1920-pixel
wide copy of a tall image is a few tens of MB) to the decoded original.
Each copy is encoded as JPEG at quality 82 (or PNG at the default
compression level when the image has transparency) and stored atomically in
`data/cache/variants/<first-two>/<hash>-<width>.<jpg|png>` as soon as it is
encoded, so at most one encoded copy waits in memory besides the widest.
When the original is no wider than a width, is a GIF (possibly animated) or
AVIF, cannot be decoded, or the copy would not be smaller, an empty `.orig`
marker records that the original is served for that width, so it is not
decoded again. A failed write stops the image. Every width must have a valid
copy or marker before the image is skipped; malformed or truncated copies are
treated as missing and replaced on the next call.

## Copy Groups

Copies are made per group of originals:

| Group      | Originals                                                                                                           |
| ---------- | ------------------------------------------------------------------------------------------------------------------- |
| `lineblog` | `.jpg` and `.png` files with archive names directly in `<data>/lineblog/`                                           |
| `works`    | `.jpg` and `.png` files with archive names directly in `<data>/works/`                                              |
| `journey`  | `.jpg` and `.png` files with archive names directly in `<data>/journey/`                                            |
| `media`    | the current files of ready X, YouTube, Bilibili and niconico assets, newest fetch first; earlier versions have none |

These are the files the server answers `w` for; symlinks, dotfiles and
archive files over 32 MiB are skipped. A call makes one group's
missing copies two images at a time. An image that is done is skipped without
being read, and an archive file is streamed through its hash first
(remembered under the data directory, see [File Hashes](#file-hashes)), so a
stopped call resumes where it left off. A call may be bounded by a number of
originals to load and a time budget after which no image starts. Its result
has the shape of a step result:

| Field       | Meaning                                                                  |
| ----------- | ------------------------------------------------------------------------ |
| `attempted` | originals loaded to make copies                                          |
| `made`      | of those, originals that got at least one copy                           |
| `kept`      | of those, originals that got only keep-original markers                  |
| `skipped`   | originals that were already done                                         |
| `failed`    | originals that could not be read, did not match their hash or be stored  |
| `remaining` | originals not reached because a bound stopped the call                   |
| `stopped`   | the bound that stopped it: `variants_file_cap` or `variants_time_budget` |

A failed original is tried again by the next call. Malformed or truncated
derived copies are removed when their replacement is written.

A status report per group counts the originals, those with every width
stored as a copy or a keep marker (`ready`), those whose hash has not been
read yet (`unhashed`, whose widths count as missing), and the copies, kept
widths and missing widths of each width. It reads no original and hashes
nothing: it checks each cached copy's image header and format end marker, then
matches media by their rows' hashes and archive files by their remembered
hashes. Its cost is one directory listing of the cache plus the bounded
metadata reads for cached copies and one `stat` per archive file.

## Variants Command

`kano-go variants` (`npm run variants`) makes the `lineblog`, `works` and
`journey` groups' copies without bounds and prints each group's counts. It
exits non-zero when it is interrupted; running it again resumes.

| Flag    | Default | Meaning                                                                      |
| ------- | ------- | ---------------------------------------------------------------------------- |
| `-data` | `data`  | data directory holding `lineblog/`, `works/`, `journey/` and the file hashes |
| `-out`  | `-data` | data directory whose `cache/variants/` is written                            |

An empty or missing directory gives its group no originals.

With `-out` set, the copies can be made on a workstation and copied into the
server's `data/cache/variants/`: their names depend only on the originals'
bytes. The hashes it reads are remembered under `-data`, by that machine's
paths, and are not needed on the server, which hashes each file once itself.

## Copies Run

The console's Site page shows the [status](#copy-groups) of the `lineblog`,
`works`, `journey` and `media` groups and starts a copies run
(`POST /api/admin/runs {"kind":"variants"}`, see
[Admin API](../admin-api/runs.md#runs)). The run is one `variants` step on the shared
single-flight queue. It makes the groups' missing copies in that order
within 5 minutes; a group reached after the budget counts its originals as
`remaining` without loading any. The step's result is the summed call result
of [Copy Groups](#copy-groups), and it is partial while originals remain or
fail. Running it again continues, since done images are skipped. The step
uses the service's data directory, so only the API process offers it.

The page reads the status when it opens and after each run, never on a
timer. Its panel and one meter per group render at once and fill in when the
status arrives, so a slow listing never holds the page. The service lists the cache once and answers that listing again until
another run finishes; copies change only through runs, so a newer listing
could differ only after the variants command or files changed by hand, which
the page's Check again (`refresh=1`) covers. One listing runs at a time.

## Responses

The response is always an image: a copy, or the original whenever there is no
copy. Copies and originals are streamed
from their files, never held in memory. A response carries `nosniff`,
`default-src 'none'; sandbox`, and range and conditional handling. A copy's
ETag is `<original-sha256>-<width>`, read from its name, so serving it hashes
nothing; a copy is checked to be a regular file within 10 MiB whose first
bytes match its extension. A `/media/<id>` copy is immutable when `v` is
given; archive copies are cached like their originals
([Immutable Fixed Files](layout-and-model.md#immutable-fixed-files)). Originals are never
changed; the variant and file-hash directories are derived caches that may be
deleted at any time and are rebuilt by the media step, the variants command
and the copies run; the file hashes are read again when needed.

Pages request copies through `srcset` and `sizes` with the widths below an
image's known width (every width when it is unknown) plus the original; the
full-image viewers keep loading the original. Profile media and other bundled
`/assets/` files have no copies.
