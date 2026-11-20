# Content and Media

What the archive holds and how its media is recovered.

- `src/lineblog/posts/<article number>.json` holds one article: its article
  number, Japan-time publication instant, title, original address, the
  Wayback Machine capture it was recovered from, and its body as typed blocks
  (paragraphs of text, links and images; YouTube embeds; link cards; rules),
  plus the media catalog entries of the files it references. Paragraphs carry
  the archive's Simplified Chinese and English translation paragraphs. The shapes are in
  `src/lineblog/types.ts`.
- `src/lineblog/post-index.json` lists every article, newest first, with its
  number, publication instant, titles and the media name of its first
  archived picture or video cover. The page's lists, month archive, article
  neighbours and links between articles read only this index.
- `src/lineblog/media-catalog.json` lists each referenced media file with its
  MIME type, size, SHA-256 and, for images, the displayed width and height.
  It is the complete catalog, including the design art; the page itself
  reads the entries carried by each article's file.
- The media files themselves (about 1.08 GiB: 305 images and 25 videos, each
  under 100 MiB) are ignored runtime data in `data/lineblog/`. They are
  too large for the repository and the image. The server reads them through
  `GET /media/lineblog/<name>`, which serves only a regular file directly
  inside that directory whose name is 12 lowercase hex digits plus `.jpg`,
  `.png` or `.mp4`, with a fixed type for the extension, `nosniff`, a
  sandboxing CSP, byte ranges and conditional requests. Images whose bytes
  match their name are immutable and videos revalidate
  ([Immutable fixed files](../../architecture/media-cache/layout-and-model.md#immutable-fixed-files)).
  Images also have smaller copies in the runtime variant cache, made ahead
  by the variants command ([Resized copies](../../architecture/media-cache/resized-copies.md)):
  the recent-post thumbnails, profile photo, article pictures, smartphone
  list pictures, link-card pictures and video covers request them through
  `srcset` (a video poster
  takes one 960-pixel copy), while the image viewer loads the original.
- `src/lineblog/comments/<article number>.json` holds the comments of one
  article as an archived copy of its page listed them, newest first: each
  comment's number, the writer's display name, the Japan-time minute as an
  ISO instant, the text with `\n` line breaks, and the old comment form's
  star or face rating when it had one. Articles without archived comments
  have no file. See [Comments](comments-and-template.md#comments).
- `backend/internal/lineblog/archive.json` is the server's copy of the
  articles and comments in the shape of the
  [public LINE BLOG reads](../../architecture/public-api/lineblog.md),
  which the server embeds. It is exported from the files above
  ([Public API Export](packaging.md#public-api-export)).
- `src/lineblog/content.ts` derives the page views (pages of three articles,
  months in Japan time, single articles) from the index, and the media URLs.
  `src/lineblog/post-files.ts` loads article files on demand, and a single
  article's comments file with it: each one builds into its own
  content-hashed chunk, so a view loads only the articles it shows, and the
  server caches them as immutable build files. A comments file that fails to
  load shows the article without comments.
- `src/lineblog/lineblog-app.tsx` renders the page and remembers the
  reader's translation and design. `navigation.ts` handles in-page addresses
  and the phone layout switch, `translation.tsx` the translated lines,
  `article.tsx` an article's blocks, `comments.tsx` the comments, `menus.tsx`
  the menu bar's pickers, and `desktop.tsx` and `smartphone.tsx` the two
  layouts.
- The blog's own header art and profile photos, and the menu bar's Home icon,
  are archive media like the articles' pictures: nine JPEGs and one PNG in the
  recovery archive's `media/`, packaged into `data/lineblog/` and the media
  catalog. The menu shows the icon in place of the word Home, as the blog did;
  the word stays its alternative text. `src/lineblog/designs.ts`
  lists each design with its media names, the original address of each file
  and the Wayback Machine capture its bytes come from. The table follows every
  index, article and listing capture of the blog: four headers, five profile
  photos, three accent colours and two profile texts, which changed at
  different times. A design applies from the Japan date of the first capture
  that shows it (the 2016 art from its own `?v=0318` stamp), and a page uses
  the design of the newest article it shows. Two profile photos were never
  archived (one from 2018-01 to 2018-05, one shown on 2018-12-04); those
  periods keep the photo before or after them. Each design also carries the
  official-site address of the menu bar, which changed with the 2018-05-18
  design; the menu bar's 鹿乃オフィシャルサイト lists every address with the
  months it was linked. A reader can pick one design in place
  of the automatic one: the picker offers each run of designs with the same
  header, profile photo and accent once, applying the run's latest design,
  and the browser remembers the choice.

Archive media are named by the first 12 hex digits of a SHA-256: an image by
its bytes, a downloaded YouTube video by its watch address
(`https://www.youtube.com/watch?v=<id>`). Images the recovery archive kept
only as remote addresses stay as those addresses and show as not archived; the
page never loads them. Embedded tweets keep only their text and links.

Every video is H.264 with AAC audio, the one pairing every browser and the
iOS player decode, and packaging refuses any other codec. A YouTube download
in another codec (VP9 or AV1) is transcoded in the recovery archive's
`media/` under the same name, which comes from its watch address, before
packaging; the original download moves to the archive's `media-original/`,
which packaging never reads. The rate cap keeps a copy near YouTube's own H.264 rates: 3 Mbit/s
for 1080p, as below, and `-maxrate 2M -bufsize 4M` below 1080p.

```bash
ffmpeg -i in.mp4 -map 0:v:0 -map 0:a:0 -c:v libx264 -preset slow -crf 22 -maxrate 3M -bufsize 6M -profile:v high -pix_fmt yuv420p -c:a copy -map_metadata 0 -movflags +faststart out.mp4
```

`src/lineblog/recovered.ts` lists the pictures recovered after the archive was
made, each with the address its bytes came from. Their files sit in the
recovery archive's `media/`, and packaging stores them as archive media with
that address as the image's source:

- **Store thumbnails** replace their remote addresses, from Wayback Machine
  captures of the same addresses. Amazon renders a thumbnail anew for each
  request, so each is the same picture at the same address, not necessarily
  the same bytes the blog showed.
- **Stand-ins** replace three pictures of the 2017-11-18 article whose
  originals were never archived: the same official artist photo and bonus
  pictures from a news report of the same week. One original can take several
  stand-ins. The page labels them as stand-ins under the pictures.
- **Tweet pictures** follow the paragraph of an embedded tweet in the
  2017-11-01 article, as the tweet showed them; the page labels them as the
  tweet's pictures.

Two pictures of the 2017-11-18 article stay not archived: one from a deleted
tweet and one photo with no public copy.
