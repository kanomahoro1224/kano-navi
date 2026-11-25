# Works

The packaged Works collection, its tracks and images, and Works uploads.

`GET /api/works` returns `updatedAt`, the Japan date the collection was
checked through, `summary` (`personal`, `participation` and `uploads` counts
and `firstYear` and `lastYear`), `personal` and `participation`, oldest
first as the collection lists them. Titles and names keep the collection's
wording. The collection's phrase-coded wording (see
[Wording](../../development/works-archive.md#wording)) is rendered into
localized text with the page's own phrases, so a client needs no phrase
dictionary; such a field is `null` when the collection gives no text.

| Personal release field | Value                                                                                                                                                                                    |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `id`, `date`, `title`  | Release ID (Japan date and a title hash), first Japan release date, title as written                                                                                                     |
| `format`               | `album`, `single`, `ep` or `doujin`                                                                                                                                                      |
| `digital`, `cancelled` | Released for download or streaming; every edition withdrawn                                                                                                                              |
| `unit`                 | Unit name of a unit work, or `null`                                                                                                                                                      |
| `label`, `catalog`     | Localized text                                                                                                                                                                           |
| `covers`               | [Works images](#works-images)                                                                                                                                                            |
| `disc`                 | Photo of the disc for the shelf, or `null`                                                                                                                                               |
| `printedDisc`          | The record shows the cover printed on a disc instead of a drawn one                                                                                                                      |
| `cutout`               | Figure cut out of the cover on a transparent ground, or `null`                                                                                                                           |
| `tracks`               | The [tracks](#works-tracks) every edition shares                                                                                                                                         |
| `discs`                | Photos of the shared discs as `{disc, image}`                                                                                                                                            |
| `editions`             | Editions on sale; empty for a release sold in one plain version                                                                                                                          |
| `storeBonuses`         | Bonuses for any edition as `{store, item, images}`; `store` and `item` are localized text                                                                                                |
| `stores`               | Streaming and download listings for any edition as `{item, links}`; `item` is localized text or `null` for the main item, and each link is `{platform, url, hiRes}` with a platform code |

An edition has `name`, `catalog`, `price`, `format` and `packaging` as
localized text, `date` (its own Japan date when it came out after the
release, or `null`), `cancelled`, `jan` (a string, empty when unknown),
`covers`, `bonuses` (localized texts), `bonusImages`, `storeBonuses`,
`stores`, its full `tracks`, `discs` and `printedDisc`, shaped as on the
release.

## Works Tracks

A personal track has `disc`, `discLabel` (localized text or `null`), `no`,
`title`, `credit` and `note` (localized text or `null`) and `listen`, the
uploads of the same recording, or `null`. `listen` keeps platform video IDs
in the shape of the [upload list](#works-upload-list): `niconico` and
`bilibili` (an ID or `null`), `youtube` and `youtubeMembers` (lists of IDs).

A participation work has `id`, `date`, `title`, `roles` (codes `vocal`,
`lyrics`, `composition`, `voice`, `guest`), `forms` (codes such as `digital`,
`bonus`, `drama`, `cdr`, `boxSet`, `notForSale`, `promo`), `unit` (the unit
name, or `null`), `label`, `event` and `catalog` as written (empty when
unknown), `sourceUrl` (the page
the record follows), `cover` (a Works image or `null`) and `tracks`, each
`{disc, no, title, credit, kano, kanoCredits}`: `credit` is the database's
artist line as written, `kano` whether Kano took part in the track, and
`kanoCredits` her further credit codes (`lyrics`, `composition`,
`arrangement`, `animation`, `other`).

## Works Images

Every cover, disc photo, bonus image and cutout is `{url, width, height,
mimeType, caption, sourceUrl, sourceLabel, credit, thirdParty}`: `url` is
`/media/works/<name>`, `caption` localized text or `null`, `sourceUrl` the
page the file was taken from or `null`, `sourceLabel` the site named instead
of a page or `null`, `credit` `{role, name, url}` (role `scan`, `photo` or
`fan`; `name` and `url` may be `null`) or `null` for a file from a person's
own copy, and `thirdParty` whether the file is a store's, database's or
collector's copy of the release's artwork rather than the publisher's file
(`null` for a cutout).

## Works Upload List

`GET /api/works/upload-list` returns `updatedAt` and `uploads`, the
collection's uploads oldest first, each `{id, date, song, originalArtist,
kind, with, niconico, youtube, youtubeMembers, bilibili, bilibiliTitle,
albums, tracks}`. `kind` is `cover`, `original`, `mv`, `project`,
`compilation`, `game` or `composition`. Videos are platform IDs, never URLs:
`niconico` and `bilibili` are an ID or `null`, `youtube` and
`youtubeMembers` lists of IDs. `originalArtist`, `with` and `bilibiliTitle`
are free text, empty when there is none; `albums` (the albums that include
the recording) and `tracks` (a compilation's songs) are lists of free-text
names as the collection writes them, not release IDs. The list never joins
stored data; [`/api/works/uploads`](#works-uploads) adds each upload's cover
and stored videos by the same upload ID.

## Works Uploads

`GET /api/works/uploads` joins the Works page's packaged upload list
([Upload covers](../../development/works-archive.md#upload-covers)) with stored
data by video ID, on each read and without writing. It returns `updatedAt`,
the list's date, `revision`, and `uploads`, keyed by upload ID, for each
upload with a ready cover or a stored video: `cover` (`{url, width, height}`:
a `/media/<id>?v=<sha256>` URL of this site (on `CDN_PUBLIC_BASE` when set)
with smaller copies and the size, with
`width` and `height` `null` while unknown; or `null` without a cover) and
`videos`, the stored YouTube and Bilibili videos of the upload in list order
as `{source, id, title, url, publishedAt, scheduledAt, durationSeconds,
availability, translations, sourceLanguage}`, where `publishedAt`,
`scheduledAt`, `durationSeconds` and `availability` are `null` while
unknown. Uploads with neither are left
out. A stored video's title is its shown one (an operator's correction
first); raw JSON and thumbnails' source URLs never appear.

`recent` lists, newest first, at most 48 of the stored videos dated after
`updatedAt` that the page shows above the list, derived on each read under
the rules in
[Uploads after the collection date](../../development/works-archive.md#uploads-after-the-collection-date);
`recentTotal` counts them all.
Each is `{id, source, title, url, publishedAt, cover, origin, translations,
sourceLanguage, alternates}`: `source` is `youtube` or `bilibili`,
`publishedAt` is a premiere's scheduled time or the publish time, `cover`
is its ready cached thumbnail in the shape above or `null`, `origin` is
`auto` (listed by its tag, form and format) or `operator` (an operator's
decision includes it), and `alternates` names, for a YouTube video, its
merged Bilibili uploads as `{source, url}`. No kind, artist, credits or
albums are inferred, and the
operator's decision itself, availability and counters stay out. Other videos
outside the list never appear. The read revalidates through its
[ETag](snapshot.md) and never fetches platforms.
