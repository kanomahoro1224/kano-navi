# Packaging and Verification

Packaging the archive, exporting its API, the copies archive, and verifying the result.

## Packaging

To rebuild the article files, index, media catalog and `data/lineblog/` from
a checkout of the recovery archive, run:

```bash
npm run lineblog:package -- --from /path/to/Kano_LINE_Blog_Recovery
```

The archive holds `BLOG/` (Japanese originals), `BLOG_zh-cn/` and `BLOG_en/`
(the same articles with Simplified Chinese or English translation lines
inserted; both are machine translations of the Japanese),
and `media/`. A video over 100 MiB would sit in `media-parts/` as a
gzip-compressed tar split into numbered parts, which packaging restores.
Packaging:

1. Parses every article and keeps the highest retained version of each one.
2. Matches each translation file's lines against the Japanese ones; inserted
   lines become that language's translation paragraphs of the block before
   them. A translation file of another version than the article counts for
   nothing, and an archive without a translation folder leaves that language
   out.
3. Records each `[Image source]` line on the image it follows, keeps no local
   file paths, and reduces Amazon links to their product path, dropping
   affiliate and API query parts.
4. Validates every referenced media file before writing anything: name
   format, size limit (15 MiB per image, 256 MiB per video), magic bytes,
   name identity and, for a video, H.264 and AAC tracks. The design art in `src/lineblog/designs.ts` and the
   recovered pictures in `src/lineblog/recovered.ts` go through the same
   checks. A video missing from `media/` is restored from its parts.
5. Writes the media into `data/lineblog/` through staged renames, skipping
   files whose bytes already match, and then writes the article files
   (removing those of articles the archive no longer holds), the index and
   the media catalog.

Packaging performs no network requests and does not change the recovery
archive. It leaves `src/lineblog/comments/` and the server's copy alone; run
the [export](#public-api-export) afterwards. Review the written files
and the page together when updating them.

### Public API Export

After any change to the article files, index, comments files or media
catalog, rewrite the server's copy:

```bash
npm run lineblog:export
```

`scripts/export-lineblog.ts` reads only the tracked files under
`src/lineblog/` and derives what the page derives (months, article
neighbours, readable alternative text, pictographs, links between articles)
through the page's own modules, so the API and the page agree. It renames
`p` blocks to `paragraph` and `hr` to `rule`, turns media names into media
objects from the catalog, and sets `updatedAt` to the Japan date of the
newest article or comment. A media name missing from the catalog, an
unknown block or inline type, or an article file outside the index stops the
run. `tests/lineblog-api.test.ts` regenerates the copy in memory and fails
when the committed file differs.

### Copies Archive

`lineblog-variants.tar` packs the smaller copies of every archive image for
instances that install them instead of running the variants command
([Deployments](deployments.md)). Its paths start with `data/cache/variants/`
and it holds nothing else; the file-hash records the command writes under
`-data` name the workstation's paths and stay there. Rebuild it whenever the
archive images change, in a checkout whose `data/lineblog/` holds the
packaged media, into an empty `lineblog-variants/` so that copies of images
the archive no longer holds are left out:

```bash
npm run variants -- -out lineblog-variants/data
```

The command prints `0 failed, 0 remaining` for `lineblog` when every image is
done. Pack the copies with fixed ownership and modes, then write the checksum
beside the archive:

```bash
cd lineblog-variants && find data/cache/variants -type f | LC_ALL=C sort > ../lineblog-variants.files && tar --format=ustar --owner=0 --group=0 --mode='u=rw,go=r' -cf ../lineblog-variants.tar -T ../lineblog-variants.files && cd ..
```

```bash
sha256sum -b lineblog-variants.tar > lineblog-variants.tar.sha256
```

## Verification

Run `make ci-frontend`, `make go-test`, `make docs-check` and
`make sensitive-check`. `tests/lineblog.test.ts` packages a synthetic archive
(translation alignment, image sources, YouTube merging, split-video
restoration, rejection of mismatched media, the index and article files),
extracts comments from synthetic article pages (both name forms, ratings,
dropped links, the capture with the most comments), and checks the tracked
index, article files, comments files and media catalog against each other,
including every design's art, and the smartphone list's opening words;
`tests/lineblog-api.test.ts` checks the server's copy against the tracked
files, and `internal/httpapi/lineblog_test.go` and
`internal/httpapi/lineblog_api_test.go` cover the media route and the
public reads. Browser verification covers the article list, months, single
articles with the 前の記事/次の記事 pager and their comments, back and forward (including after a reload),
the image viewer, video
playback, the menu bar's and each article's translation switch in Chinese and English, the design picker and each design's header and profile,
three languages and themes, the PC layout at desktop and tablet widths, and
the smartphone template at 320, 375 and 414 pixels and on either side of
600 (profile popup, monthly pull-down, both pagers).
