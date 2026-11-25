# LINE BLOG Archive

The LINE BLOG archive API.

`GET /api/lineblog`, `GET /api/lineblog/posts/{id}` and
`GET /api/lineblog/posts/{id}/comments` serve the
[LINE BLOG archive](../../development/lineblog-archive.md) page's articles and
comments to other clients. The server embeds them from the tracked page data
([Public API Export](../../development/lineblog-archive/packaging.md#public-api-export)),
so they change only with a release, never read SQLite, and carry no
`revision`. Each body carries `updatedAt`, the Japan date of the newest
archived article or comment. Text is locale-neutral: a localized value is
`{"ja", "zh-CN", "en"}` with `null` for a language the archive lacks, and no
read picks a language from the request. Fixed wording, such as the rights
notice and labels, stays in each client. Media are same-host
`/media/lineblog/<name>` URLs (on `CDN_PUBLIC_BASE` when it is set);
third-party addresses appear only as
provenance links that the page opens on request, never as something a client
must load.

The reads take only `GET` (other methods answer 405 with `Allow: GET`),
revalidate through their [ETag](snapshot.md) and negotiate
gzip. `{id}` is an article number; an unknown or malformed one answers 404
`not_found`, and any other path below `/api/lineblog/` (an empty, further or
trailing segment) answers 404 `api_not_found`.

A **media object** is `{url, width, height, mimeType}`: `url` is the
`/media/lineblog/<name>` address, `width` and `height` the displayed size of
an image (`null` for a video), and `mimeType` `image/jpeg`, `image/png` or
`video/mp4` (always H.264 with AAC audio). A missing file answers 404 on its
route; clients show an unavailable state with the provenance link.

`GET /api/lineblog` lists the archive:

| Field             | Value                                                                                                                                                                                                                                                            |
| ----------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `updatedAt`       | `YYYY-MM-DD`                                                                                                                                                                                                                                                     |
| `originalBlogUrl` | The blog's original address, as provenance                                                                                                                                                                                                                       |
| `posts`           | Every article, newest first: `id`, `published` (ISO instant), `title` (localized), `thumbnail` (a media object of its first archived picture or video cover, always an image with its size, or `null`) and `commentCount` (its archived comments, 0 without any) |
| `months`          | Each Japan month (`YYYY-MM`) with articles, newest first, with its `total`                                                                                                                                                                                       |

`GET /api/lineblog/posts/{id}` returns one article with its neighbours in
the page's pager order:

| Field        | Value                                                                                                                                                                                                                        |
| ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `updatedAt`  | `YYYY-MM-DD`                                                                                                                                                                                                                 |
| `post`       | `id`, `published`, `title` (localized), `url` (the original address), `archiveUrl` (the Wayback Machine capture it was recovered from), `commentCount` (its archived comments, 0 without any) and `blocks`, in reading order |
| `previousId` | The next older article (the page's 前の記事), or `null` for the oldest                                                                                                                                                       |
| `nextId`     | The next newer article (the page's 次の記事), or `null` for the newest                                                                                                                                                       |

Each block has a `type`:

| `type`      | Fields                                                                                                                                                                                                                                                                         |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `paragraph` | `content` (inlines) and `translations`: `{"zh-CN", "en"}`, each a list of translation paragraphs (each a list of inlines) shown after the paragraph, or `null`. Both are machine translations of the Japanese. A paragraph of only non-pictograph pictures is a row of figures |
| `quote`     | `content` (inlines)                                                                                                                                                                                                                                                            |
| `rule`      | none                                                                                                                                                                                                                                                                           |
| `video`     | `video` (media object)                                                                                                                                                                                                                                                         |
| `youtube`   | `videoId`, `watchUrl`, `video` (the downloaded copy, a media object or `null`), `cover` (media object or `null`), `coverSourceUrl` and `archiveUrl` (provenance or `null`)                                                                                                     |
| `card`      | A link card: `title`, `url`, `articleId`, `archiveUrl` and `description` (or `null`), and `image` (an image inline or `null`)                                                                                                                                                  |

Inlines also have a `type`:

| `type`  | Fields                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `text`  | `text`; line breaks are `\n`                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `link`  | `href`, `content` (inlines) and `articleId`, the archived article the link opens, or `null`                                                                                                                                                                                                                                                                                                                                                                                          |
| `image` | `image` (media object, or `null` for a picture the archive never captured), `alt` (readable alternative text or `null`), `emoji` (a pictograph drawn at text size, not opened in a viewer), `href` (the link the picture opened, or `null`), `sourceUrl` (where the archived bytes came from, or `null`), `remoteUrl` (the original address of a picture never captured, or `null`) and `recovery` (`stand-in` or `tweet` for a picture added after the recovery archive, or `null`) |

`GET /api/lineblog/posts/{id}/comments` returns `updatedAt` and the
article's archived `comments`, newest first, in Japanese as archived (`[]`
for an article without any): `number` (deleted comments leave gaps),
`author` (the display name), `posted` (ISO instant, to the minute in Japan
time), `body` (plain text with `\n` line breaks) and `rating`
(`{icon: "star" | "face", value: 1–5}` or `null`). Commenters' links are
never kept.
