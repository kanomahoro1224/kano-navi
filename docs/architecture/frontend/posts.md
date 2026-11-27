# Posts and Post Cards

The post archive page and how post cards and entry links render.

## Post Archive Page

`/posts` (`src/posts/posts-app.tsx`) is one newest-first timeline of every
stored post, with an activity map beside it to move through the years. The
board's Past posts link opens it with the wall's chosen account. The address
names the view, `?month=YYYY-MM&account=<handle or mid>`
(`src/posts/navigation.ts`); a malformed value falls back to the default, and
without a month the timeline starts from the newest post. Choosing a month or
an account pushes a history entry, so back and forward return to it; the
month in view replaces the address's month as the page scrolls, without an
entry, and the newest month leaves the address without one.

`usePostArchiveIndex` in `src/dashboard/post-archive.ts` reads
[`/api/posts/months`](../public-api/calendar-and-archives.md#bounded-public-archives) once per visit: the month
index, the accounts the page offers (the board's X accounts, then the
archive-only ones, then Bilibili mids) and their avatars. Until it arrives
the page shows the last index it knew; a failed read keeps that one, and
without one offers a retry. The posts read through `useArchive` from the post
timeline of
[`/api/posts`](../public-api/calendar-and-archives.md#bounded-public-archives), following the index's
revision, as the video dialog reads its own: the newest posts read alongside
the index, a chosen month waits for this visit's index and starts the
timeline at the page holding the month's newest post (`timelineStartPage`,
from the index's counts for the chosen account), the next page loads as the
end scrolls near, and **Newer posts** above the first month reads the page
before it while the first post keeps its place on screen. A chosen month that
is already loaded scrolls into view instead.

The page keeps its last reads, so a return opens on them. The document keeps
the month index and the last timeline as loaded; `localStorage`
(`kano-post-archive-v1`) keeps the index and the first page of the last three
accounts' timelines from their newest post, each for a week. A timeline opens
on a kept run of the same account and start without a read. Post pages carry
no time-derived fields, so the run is read again only when this visit's index
names another revision, or once when the index read fails. Back or forward to
an address whose month the kept timeline holds resumes that timeline from
its own start without a jump, so the site's navigation restores the scroll.
A change to the shape of either answer takes a new storage key. A month without the account's
posts opens the nearest earlier month that has some (`nearestMonth` in
`src/posts/timeline.ts`), and changing the account keeps the month in view
that way unless the timeline is at its newest posts.

The account cards (`account-band.tsx`) show each account's portrait, the
years its posts span and their total from the month index (in the rail they
have no outline, and the chosen one is washed in berry); All accounts counts
every post, including promoted posts filed under their authors, and has no
portrait, its name, years and total on one line. In windows 960px and wider
they sit in the rail above the activity map, All accounts across the rail
with its total at the right edge over the map's totals and the accounts one
or two a row as its width allows; the rail takes the width the 680px column leaves
and scrolls on its own when it is taller than the window. Narrower windows
show
them above the timeline. The activity map
(`activity-map.tsx`) has a row per year, newest first, from the archive's
first year to the newest month's, a square cell per month shaded by the
quartile of the chosen account's monthly counts it falls in, and the year's
total at the end of the row once the rail is 260px wide; a month with posts
is a button that opens the timeline there, and the month in view is
outlined. The map keeps the rail cards' inset, so its years line up under
the portraits, its first month under the account names and its totals with
the cards' right edge. The map sticks beside the timeline; in windows
narrower than 960px it gives way to the archive dialog's month controls
(`ArchiveMonthBar` in its timeline form, with the jump grid and **Latest**). They stick under the site
header as one band with it, carrying its scrolled stripe, and the month
button also names the day in view, which the timeline reports as the page
scrolls. Since the bar names the month, a timeline that starts from the
newest posts keeps its first month's heading for screen readers only, and a
jump lands on the month's first day with the heading under the bar.

`post-timeline.tsx` groups the loaded posts by Japan month and day
(`groupPostsByDay`): a heading per month with its count from the index, and
per day a date that sticks beside its posts; on phones the date heads its
posts and scrolls with them, since the month controls name the day. The posts
are the board's post cards in full, with the time alone, while a quoted post
keeps its date; the page takes away their boxes, so they lie between rules
(one across each day, a fainter one between a day's posts) and a quote hangs
from a rule at its left. A month that starts a year below a later one opens
with that year in large figures under a gingham strip (decorative, since the
month heading names the year). A lone image stands at most 440px tall, narrowing to keep
its frame's shape, and three or four images share a 3:2 frame. Until the
first page arrives, skeleton posts hold the column's place. Images open in
the site's media viewer.

### Post Cards and Entry Links

A post card resolves its account avatar from `dashboard.accountAvatars` (and the
archive response's `accountAvatars`) keyed by `<source>:<lowercase_handle>`. A
cached non-hidden account avatar renders that account's image; an explicit
`null` entry (hidden mode) renders the handle initial badge. When no entry is
present, an own post falls back to `profile.avatarUrl` while a promoted post
(`post.promoted`), a related account's or a reposted one, shows its author's
initial on the sunken surface beside its handle. The initial is the handle's
first letter or digit, and an image that fails to load gives way to it. A
promoted post appears in the
All tab and the archive, never under an own account's tab, even an account that
reposted it. A post with `repostedBy` opens with a muted repost mark,
"Reposted by @account", naming those accounts as one locale list (`Intl.ListFormat`).

A post card's wording, images and quote run from its top; only the engagement
counts sit at its foot. Two to four images share one 16:10 frame (one tall
image beside two for three), and a fifth or later is counted as "+N" on the
fourth tile, which opens the image viewer. A lone image with a known size
keeps its own shape between 2:1 and a square on the board (3:4 in the archive
and the full post dialog); one past those bounds sits whole on a blurred copy
of itself instead of being cropped, and one of unknown size takes the 16:10
frame.

A post that quotes another status (`post.quote`, see
[Quoted posts](../data-model/posts.md#quoted-posts)) shows it nested in the post note,
below the post's own wording and images and above the engagement counts: a
card on the quieter sunken surface with "display name @handle · time" (an X
author's cached avatar first, from `accountAvatars`; no initial when there is
none) and the quoted wording clamped to four lines. On the board a quote with
wording shows its first image as a square thumbnail beside the wording, with
"+N" in its corner for the rest; a quote without wording, the full post
dialog and the archive show the quote's images like a post's, bounded like
the board's. The card follows the quoted post's link (its x.com page, or for
a Bilibili forward the forwarded dynamic's or upload's Bilibili page) from
anywhere except its images, which open the image viewer with the quote's
images (a lone video's or GIF's still plays on X). A forward's card names the
forwarded dynamic's author by display name after the bilibili mark, never by
`@` or mid. Read more also appears when the clamp hides quoted wording; the
full post dialog and the archive show the whole quote. An unavailable quote
shows a localized "Quoted post unavailable" placeholder linking to the
quote's `url`. Quoted wording follows the content language like the
post's own: the quote's `translations` (its stored post's for a self-quote,
otherwise the quoted status's own) show with their own translation switch,
and a quote without one shows its wording as written.
A quote an operator hid is absent from the snapshot, so the board shows the
post without it.

A Bilibili dynamic (`source: "bilibili"`) shows the bilibili mark and name,
linked to the account's space, instead of an `@handle`, and its open link
goes to the opus page. It has no engagement counts. A forward shows the
dynamic it forwards as its quote card, and an empty text (a forward without
wording of its own, or an emoji-only dynamic without stickers) leaves only
its images or its quote. The
wall's top line adds bilibili while it holds a dynamic. While a dynamic or
its quote shows its original wording, `PostWording` renders its `rich`
segments when it has them: each sticker with a local `/media/` address is an
inline picture at text height whose `alt` and `title` are its caption, and
any other sticker is its caption as text. A translation shows as text,
without stickers. The board matches no captions and loads no other address.

An entry follows its first platform link (`eventLink` in
`src/dashboard/format.ts`), or its `url`, or, in a snapshot without
`links`, a stream's `liveUrl ?? url`. `eventLinks` orders an entry's
`links` as `videoLinks` orders a video's: a platform whose `liveStatus` is
`live` first, marked live, then for zh-CN once the entry is done its
bilibili link, else its own platform first; a platform without a link is
left out. An entry shows them with `PlatformLinks` (`platform-links.tsx`,
the live one marked) when it has more than one, or one beside an
announcement of its own (`eventAnnouncement`: its `url` when no platform
link is that page): on its stage ticket, on its week and calendar card,
and in its sheet, where the open arrow in the corner then opens the
announcement; the announcement is never labelled as a platform. While an
entry is on air, its week and calendar card's On air chip (`LiveLinks`)
ends in the marks of its live platforms, each a link named for screen
readers and the tooltip, in place of the status chip and those platforms'
links; the sheet keeps the named links. A
full stage ticket of an entry marks its `platforms` on the art with
`PlatformBadge`. When the stage shows the reserved video of an entry, the
ticket adds the entry's links on platforms the video has no copy on, such
as the bilibili room of a simulcast, marks the video's own link live while
the probe confirms it, and reads live, without a countdown, while any of
its links is. `PlatformLinks` names a link on another platform by its
host. The dashboard's `liveNow` becomes a live stage
ticket, "Live now on bilibili", unless an entry shown on that platform, of
type stream or member and confirmed `live`, starts at S with S−2h ≤
`startedAt` ≤ S+3h, as the server matches broadcasts; the match never
compares links. The
post wall's account filter adds a bilibili button for each of
`meta.bilibiliAccounts`, with the bilibili mark instead of the mid. A snapshot restored from the
browser drops `liveNow` until the first fresh read.
