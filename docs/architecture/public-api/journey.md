# Journey and Works Collections

The packaged collections and the Journey collection with its media.

`GET /api/journey`, `GET /api/works` and `GET /api/works/upload-list` serve
the data of the Journey and Works pages, so a client other than the web page
can draw them. Their bodies ship with the build: the export scripts
(`npm run journey:export`, `npm run works:export`) write them from the
pages' tracked data with the pages' own modules into
`backend/internal/journey/` and `backend/internal/works/`, which embed them,
and the frontend tests fail when a committed body differs from the data
([Journey](../../development/history-archive.md#content-and-provenance),
[Works](../../development/works-archive.md#content-and-media)). They read no
snapshot rows, so they carry no `revision` and their ETags change only with
a build that changes the data. Each answers `GET` alone (405 with
`Allow: GET` otherwise), revalidates through its
[ETag](snapshot.md) and negotiates gzip; a deeper path, such
as `/api/journey/<id>`, is 404 `api_not_found`.

The bodies are locale-neutral. A localized text is `{ja, zh-CN, en}`, with
`null` for a language the data lacks; the server never picks a language.
Kinds, roles, formats and labels are codes the client words itself, and so is
the fixed wording around them (section titles, rights notices). Images are
same-host relative URLs (`/media/journey/<name>`, `/media/works/<name>`), or
those routes on `CDN_PUBLIC_BASE` when it is set, served under the
[media HTTP contract](../media-cache/layout-and-model.md#http-contract);
third-party addresses appear only as provenance links, never as files a
client must load. `updatedAt` is a Japan date (`YYYY-MM-DD`).

## Journey

`GET /api/journey` returns `updatedAt`, the later of the newest check date of
the Journey images and the Works collection's date, `branches`, `milestones`
in page order (by stored date, oldest first) and `galleryOnly`, the images
that belong to no milestone and appear only in the gallery.

| Branch field | Value                                                                |
| ------------ | -------------------------------------------------------------------- |
| `id`         | Identity code, such as `singer` or `mahoro`                          |
| `tone`       | Colour code of the identity, such as `honey`                         |
| `since`      | Year the identity starts                                             |
| `label`      | Localized name                                                       |
| `portrait`   | The identity's portrait as a [media item](#journey-media), or `null` |

| Milestone field | Value                                                                                              |
| --------------- | -------------------------------------------------------------------------------------------------- |
| `id`            | Stable milestone code                                                                              |
| `date`          | As stored: `YYYY`, `YYYY-MM` or `YYYY-MM-DD`; never narrowed to a guessed day                      |
| `datePrecision` | `year`, `month` or `day`, the part of `date` that is known                                         |
| `dateEnd`       | Last day (`YYYY-MM-DD`) of a period, or `null`                                                     |
| `dateNote`      | A name written after the date, such as the event `C93`, or `null`                                  |
| `branch`        | Branch `id`                                                                                        |
| `kind`          | Code, such as `release`, `debut`, `stream` or `artwork`                                            |
| `highlight`     | Whether the timeline emphasizes it as a key milestone                                              |
| `title`         | Localized text                                                                                     |
| `detail`        | Localized text, or `null`                                                                          |
| `sourceUrl`     | The strongest public page for the entry, or `null` when the only evidence is a third-party archive |
| `links`         | Further pages as `{url, label}`; `label` is a code (`agency`, `x`, `cancelledRelease`)             |
| `releaseId`     | ID of the Works release whose covers it shows, or `null`                                           |
| `media`         | Its [media items](#journey-media), catalogued images first, then the release's covers              |

### Journey Media

A media item is one image of the timeline or gallery. `origin` is `fixed` for
an image of the Journey catalog and `release` for a cover of the milestone's
Works release, so a client draws the Journey without `/api/works`. Every
item has every field below; a field that does not apply to its origin is
`null`.

| Field                         | Value                                                                                                                                               |
| ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `origin`                      | `fixed` or `release`                                                                                                                                |
| `id`                          | Stable image code; a release cover's is `<releaseId>/<name>`                                                                                        |
| `url`                         | `/media/journey/<name>` or `/media/works/<name>`                                                                                                    |
| `width`, `height`, `mimeType` | Size in pixels and `image/jpeg` or `image/png`                                                                                                      |
| `branches`                    | The branch `id`s whose gallery lists it; a release cover adds the identities the release also belongs to                                            |
| `category`                    | `artwork`, `goods`, `frame`, `model` or, for a release, `cover`                                                                                     |
| `title`                       | Localized text; a release cover's is the release title in every language                                                                            |
| `date`                        | Japan date the image is dated by, or `null` when unknown                                                                                            |
| `dateKind`                    | What that date is: `post`, `announcement`, `publication`, `stream`, `outfit`, `capture` or `release`                                                |
| `sourceKind`                  | Who published the file: `artist`, `organizer`, `creator`, `video`, `streamArchive`, `shop`, `brand`, `agency`, `officialCover` or `thirdPartyCover` |
| `note`                        | Code of the provenance note the page shows, or `null`                                                                                               |
| `focus`                       | CSS `object-position` that anchors a cropped thumbnail, or `null`                                                                                   |
| `credit`                      | Credit line the publisher requests, shown as written, or `null`                                                                                     |
| `creator`                     | Who made the picture, as written, or `null`                                                                                                         |
| `frameTime`                   | `HH:MM:SS` of a video frame, or `null`                                                                                                              |
| `sourcePage`                  | The page the image comes from (a release cover's file page), or `null`                                                                              |
| `sourceUrl`                   | Original third-party file address (provenance only), or `null`                                                                                      |
| `originalStream`              | Original stream address at the frame of a third-party archive frame, or `null`                                                                      |
| `releaseId`                   | The Works release of a cover                                                                                                                        |
| `cancelled`                   | Whether every edition of that release was withdrawn                                                                                                 |
| `cover`                       | The cover as a [Works image](works.md#works-images), with its caption, credit and third-party mark                                                  |
