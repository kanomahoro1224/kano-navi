# Comments and Smartphone Template

Archived comments and the smartphone page template.

## Comments

The recovery archive holds no comments; they come from Wayback Machine
captures of the article pages, which listed every comment in the page's
`コメント一覧`. To rebuild `src/lineblog/comments/` from a local directory of
raw captures (`.html` files saved with the `id_` form of the capture
address), run:

```bash
npm run lineblog:comments -- --from /path/to/captures
```

The script:

1. Reads every `.html` file of the directory, in file-name order, and
   recognises an article page by its canonical `lineblog.me/kano/archives/`
   address, not by its file name. Other pages are skipped.
2. Takes from each comment its number, the display name (the text of the
   `a.comment-url` link, or the plain name), the `YYYY/MM/DD HH:mm` date read
   as Japan time, the text with `<br>` as line breaks, and the rating picture
   as an icon and value. The commenter's link is dropped and never written.
   The two picture emoji the old comment form put in comment text become
   Unicode emoji; any other markup in a comment stops the run.
3. Keeps, for each article, the capture listing the most comments. A request
   for one capture can return a later one, so file names do not decide.
4. Writes a file for each article of `post-index.json` that has comments and
   removes the files of articles that have none. Run the
   [export](packaging.md#public-api-export) afterwards.

The script performs no network requests. Review the comments of changed
articles on the page before keeping the files.

The page shows the comments under the article in the blog's comment-list
style, in Japanese with `lang="ja"`; only the heading follows the interface
language. Comment text is never machine translated.

## Smartphone Template

LINE BLOG served phones a separate smartphone template, `lite2`, at the same
addresses, chosen by the browser's user agent. The page shows that template
in windows up to 600 pixels wide (the upright width of the blog's phones was
320 to 480 pixels, and the blog sent tablets of 600 pixels and more its PC
page) and the PC layout above that; both use the same query, views and data.

No smartphone page of Kano's blog was archived, and no archived smartphone
article or article list exists for any LINE BLOG or livedoor Blog. The
template follows what was archived:

- The platform's `css/lite2/common.css` (2016-10 and 2017-09) and LINE
  BLOG's `css/lite2/usr/lineblog.css` (2015-12 to 2018-04) give the colours,
  sizes and selectors: grey page with white boxes, the menu bar, the article
  list and article, the article pager, the comment list, the round bottom
  pager, the monthly pull-down and the footer.
- A LINE BLOG smartphone page of another blog (2015-12) gives the header
  frame: the header image, the sideways-scrolling menu bar with its current
  item in the blog's accent colour, the one-line profile and its popup.
- livedoor Blog smartphone comment pages on the same template give the exact
  comment list, bottom pager and footer markup.

What the page infers: the order of an article-list entry (title, picture,
opening words, footer with the date) from the skin's list selectors; the
article pager's middle ホーム label, which the skin draws without text; the
100-pixel profile photo in the popup, from the template's later profile
header; and the footer line's text, the blog's name. Kano's phone header
(`sp_design/header.jpg`) was never archived, so the header shows the middle
1040 of the PC art's 2600 pixels, which holds the art's drawn title and its
figure in every design. The list shows no comment counts, because an
article's comments load only with the article; the article shows its count.

Two parts depart from the skin on purpose. The footer line keeps its 9px
text but sits on the page's own surface instead of the skin's fixed dark
grey, so it follows the theme. The monthly pull-down keeps the skin's strip
(月別アーカイブ, or the month shown, in 12px grey with the pull-down arrow),
but opens the site's menu instead of the browser's list: the months grouped
under their years, each with its article count, opening at the month shown.

The template's icons are the original files, kept byte for byte under
`public/assets/lineblog/lite2/` at their original paths below
`parts.lineblog.me/img/lite2/`. Each came from a Wayback Machine capture:

| File                                | Capture          | Used for                                   |
| ----------------------------------- | ---------------- | ------------------------------------------ |
| `icon_comment02.png`                | `20180304080525` | comment counts                             |
| `usr/lineblog/icon_pager_first.png` | `20180304081032` | bottom pager                               |
| `usr/lineblog/icon_pager_prev.png`  | `20180304081032` | bottom pager                               |
| `usr/lineblog/icon_pager_next.png`  | `20180304081032` | bottom pager                               |
| `usr/lineblog/icon_pager_last.png`  | `20180304081033` | bottom pager                               |
| `usr/lineblog/arrow_pull.png`       | `20171220035523` | article pager, monthly pull-down, page top |
| `usr/lineblog/btn_close.png`        | `20180219103323` | profile popup                              |

The icons are grey on white or on transparency; the page multiplies them
onto the light theme and inverts and screens them onto the dark one. The
template's upward page-top arrow was never archived, so the pull-down arrow
turns up in its place.
