# History Archive Maintenance

The [Journey](../user/journey.md) page shows a multilingual timeline of 46
milestones from 2010 onward from the maintainer's `kano_official` reference
collection, with 39 images of its own and the covers of its releases from the
[Works page](works-archive.md). This page covers where that content lives,
how its provenance is recorded, and how the images are packaged.

## Content and Provenance

- `src/history/milestones.ts` owns milestone copy, dates, identity, and evidence
  links. Year, month, day, and range precision is preserved. A release
  milestone names its release on the Works page by ID (`release`).
- `src/history/media-catalog.json` lists the images: public titles in three
  languages, related milestones (none for an image shown only in the
  gallery), identities, image type, source page and file URLs, dimensions,
  MIME type, size, SHA-256, source identity, date basis, the file name in the
  reference collection (`filename`) and the packaged name (`name`, 12 hex
  digits of the SHA-256 and `.jpg` or `.png`).
  An optional `focus` (a CSS `object-position`) anchors an image's cropped
  timeline thumbnail, such as the head of a full-length figure; the gallery
  and the image dialog show the whole image. Original local paths are
  excluded.
- `src/history/media.ts` groups images by milestone. Each image's URL is
  `/media/journey/<name>`, which the server caches as immutable while the
  file's bytes match its name
  ([Journey images](../architecture/media-cache.md#journey-images)).
  A release milestone shows every cover its release lists in
  `src/works/personal.json`, in that order, from `/media/works/<name>`, with
  the Works page's caption, source page, third-party mark and credit.
- `data/journey/` holds the packaged original image files. It is ignored
  runtime data, like `data/works/`, and holds no release covers.
- `src/history/history-i18n.ts` supplies image and provenance translations.
- `backend/internal/journey/journey.json` is the body of `GET /api/journey`
  ([Public API](../architecture/public-api.md#journey)): the branches,
  milestones and images above with the release covers resolved, in every
  language. `npm run journey:export` writes it from these modules, and
  `tests/journey-api.test.ts` fails when it differs from them, so run the
  export with every change to the milestones, the catalog or a named
  release's covers.

Release dates, Japan-time post dates, stream dates, depicted outfit reveals,
and acquisition dates are distinct. Unknown dates remain unknown. Mahoro's
first stream is May 29, 2021, while the character-introduction image is dated
to its May 30 post. The current agency full-body image is a September 30, 2026
website capture, separate from the August 22 debut stream.

Four official-stream frames are explicitly labelled as sourced from a
third-party archive, with original stream URLs and frame timestamps. The
SONA teaser is a frame of the artist's own teaser video, with the video's
link and frame timestamp. Cover
art, product illustrations, promotional visuals, and model demonstrations
remain distinct. Illustrations alone do not establish a new streaming model.
Agency press-release images display their requested publisher credit.

A release cover carries the release's date from the Works page. The first
solo album, `ばんび～の` (January 16, 2011), appears in both the board summary
and the full timeline; its cover is marked as a third-party file, and the
product record supplies the release date. The 2022 `いちごいちえ Celebration`
entry records the October 7 MV publication. Its planned split-EP CD release
was cancelled in the November 7 official notice, so its two covers are marked
as artwork of a cancelled release, dated as the Works page records it rather
than by the planned November 30 release, and are also listed under MKLNtic.

## Image Packaging

The Journey images are packaged on each instance from the local reference
collection, like the [Works covers](works-archive.md#packaging); the
repository and the container image carry only their catalog. They need no
database media rows or runtime downloads. The collection holds about 62 MiB
of original bytes. Release covers are not part of it: they are the Works
page's covers in `data/works/`. A deployment without either shows the
unavailable state on those images.

To package the images from the local reference collection, run:

```bash
npm run history:package -- --from /path/to/kano_official
```

Then make their smaller copies with `npm run variants` or a copies run from
the console ([Resized copies](../architecture/media-cache.md#resized-copies)).

Packaging validates the entire selection before writing: manifest entries,
contained real paths, the 32 MiB size limit, SHA-256 identities, packaged
names, and image magic bytes must match the catalog. Each image is staged and
renamed into `data/journey/`; one already there with the same bytes is kept,
and nothing is removed. Original bytes, credits, and provenance are
preserved. Packaging performs no network requests and does not change the
reference collection or runtime SQLite database. Review the catalog and the
page together when updating the collection.

`/journey` reads no `media_assets` rows. A snapshot that holds rows for these
images keeps them, and `/media/:id` serves them like any other ready media.

When moving a deployment, preserve the ignored `data/` directory for source
snapshots, dynamic media, provider configuration, the Works covers and the
Journey images. Missing files show an unavailable state while retaining
original-source links.

## Requests and Verification

The page has no data-fetching hook or external embeds. Gallery and timeline
images are lazy loaded from the Journey and Works image routes; the
introductory portrait is loaded eagerly. A release cover's details
link to its release on the Works page (`/works?release=<id>`). Thumbnails, the portrait and the identity
pictures offer the server's smaller copies through `srcset`, made ahead from
the packaged files by the variants command or a copies run and kept in the
runtime variant cache ([Resized copies](../architecture/media-cache.md#resized-copies)); the image
dialog loads the original. Original websites open only when visitors follow
a link. Provenance hosts are documented in the privacy allowlist without
expanding the remote downloader's destination policy.

Run `make ci-frontend`, `make docs-check`, and `make sensitive-check`.
`tests/history-media.test.ts` checks the catalog and its packaged names, that
every named release exists and its covers are packaged Works covers, and
packaging with synthetic archives and without network requests. Browser verification covers identity/image-type filters, year
navigation, dialog focus and Escape, source links, three languages, themes,
and desktop/mobile layouts.
