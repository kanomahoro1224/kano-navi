# Frontend

## Presentation: `src/`

`src/main.tsx` routes between the public board, the static `/history`
page in `src/history/`, the static `/lineblog` archive in `src/lineblog/`, the
static `/about` page in `src/about/`, and `/admin`.
Links between the four public pages open in place: the next page's code
loads while the current page stays on screen (hovering or focusing a link
starts that load), then the pages cross-fade under a fixed header with the
View Transition API. Back and forward restore each entry's scroll position,
including after a reload and for the entries the browser adds for fragment
links within a page; a remembered position wins over the address's fragment.
`/admin`, other paths, modified clicks and links
with a target load normally; back and forward within `/admin` or another path
stay with that page, which keeps the browser's own scroll restoration and
handles its fragment moves (the console's views), and `index.html` applies the saved theme and
language before the first paint so no load flashes the other theme. History copy and image metadata are
static; verified fixed images ship in `public/assets/history/` and `dist/`.
`scripts/package-history-media.ts` updates these files from a local reference
archive without network requests.
See [History archive maintenance](../development/history-archive.md). The
`/lineblog` article index and per-article files are static JSON written by
`scripts/package-lineblog.ts`, and each article's archived comments are
static JSON written by `scripts/package-lineblog-comments.ts`; the page picks
an article, a month or a page from its query (`?article=`, `?month=`,
`?page=`) and moves between them in place, pushing an entry whose scroll the
shared navigation restores. Each article file and comments file is its own
build chunk: the page's code waits for the articles its address shows (and a
single article's comments), a move keeps the current view on screen until
the next view's files have loaded (pointing at or focusing a link starts
that load), and a failed article load offers a reload. Windows up to 600
pixels wide render the blog's smartphone template and wider ones its PC
layout, switching on a media query; see
[Smartphone Template](../development/lineblog-archive.md#smartphone-template). Its
media load from `/media/lineblog/<name>`, including the header and profile
art of the design of the newest article shown; see
[LINE BLOG archive maintenance](../development/lineblog-archive.md). The four
public pages share `src/site/`: `site-shell.tsx` (header with page navigation and
Japan clock, footer, skip link, section title, empty and loading states;
narrow screens show the site name and fold the page links into a menu behind
the `kano-seal.tsx` deer seal; the footer is the fan-made and unaffiliated
line, About and Back to top, with the page's disclaimer behind a switch),
`media-dialog.tsx` (the native modal image viewer: its close button stays in
the screen's top-right corner, and a click anywhere outside the image, its
arrows and its details closes it; a set takes the viewer's full height and
centres each image in it, so its arrows stay in place across image sizes,
and shows the details panel for every image once any image has details; an
image's small-print note closes the panel, or the viewer when there is none;
every control that opens it carries `media-opener`, which shows the zoom
cursor), `links.tsx` (external
links and platform marks), and `site.css` (shell, controls, tone classes and
the dialog). A `pill-button is-collapsible` shows only its icon at 560px and
below, so it keeps its label as `aria-label`. The board lives in
`src/dashboard/`: `use-dashboard.ts` owns the snapshot request and the
revision stream, `format.ts` owns Japan-time and event-status helpers, `content.ts` holds static public links, and
`components/` renders the next-up stage and profile card, the on-sale strip
(`sale-strip.tsx`: up to three sales from `currentSales`, on sale now by
their close and then those opening within seven days by their opening,
rendered only when one qualifies), the week guide,
the post wall, the video shelf, and the link directory, plus
`archive-dialog.tsx`, the native modal sheet that browses one month of the
schedule, posts or videos, and `sheet-dialog.tsx`, the smaller modal sheet
that shows one long post or one schedule entry in full and can open above
the month sheet. `video-groups.ts` sorts the shelf's videos from
the local snapshot: the `stream` kind is a stream and every other
[form](data-model.md#video-form-and-tags) a work, whatever the video's tag;
a video's phase follows the
stage's live rule (a fresh confirmed state first, then the stream's
three-hour grace or an explicit end), so a reservation past its start is a
started ticket rather than an archive and an operator-confirmed cancellation
leaves the shelf once its start passes. A past video whose `isShort` is
`true` is a Short before its kind is read, so a Short broadcast is neither
an archive nor a work; `null` (unknown) is an ordinary upload. The
lead is the featured video, or else the newest work tagged `song` or
premiere, or else the newest work, so a Short leads only when featured; each video appears
once. A video carries two chips. The form chip (`videoFormLabel`) names a
Short, then a stream's waiting room, then the form (stream while it may be
on air by the same rule as a live entry, then stream archive, premiere
while it waits or airs by `videoPhase`'s rule, then video); a premiere that
has aired is an ordinary video and has no form chip (`null`); a reported end that has passed makes it an archive even while a
`live` result is still fresh, as `videoPhase` files it, and every caller
passes the board's clock. The tag chip (`VideoTagChip` in
`src/dashboard/components/event-tag.tsx`) adds the tag with the schedule's
icon, in the schedule's words for a stream (gameplay, talk, singing) and as
the work's content otherwise (gameplay, talk, music, where a note replaces
the singing microphone, as on the archive's Songs button); it appears on every
card, row, ticket and the lead, including stream archives that hide the
form chip. The Next Up ticket names the video's tag, else the entry's
activity label, before the form; a `member` entry adds the members-only chip
(`MembersChip`) beside it unless the video's availability chip already
shows `members`, `private` or `removed`. Schedule entries follow the same split:
`eventKinds` marks a release or video entry as a work, and `eventTagKey`
names a work's `song` label music, any other singing. `scheduleEntries` turns
a [sale period](data-model.md#sale-periods) into the guide's entries at its
ends: the closing entry carries the close as its date and time, both carry
`saleMark` (`opens` or `closes`, shown as a chip) and the stored period in
`saleWindow`, and `eventStatus` gives both the period's status (`upcoming`,
`open`, `done`). The filter offers All plus Streams, Works and Shorts for each kind
present, and only when two are. Shorts render as 9:16 cards cropped from the
centre of the 1280×720 cover (the vertical frame between YouTube's blurred
pillarbox): one row of up to six in the All view, fewer as columns narrow and
a swipeable rail on phones, and up to twelve in the Shorts view. The shelf
shows the length from `durationSeconds` when known. `videoLinks` in
`src/dashboard/video-groups.ts` orders a video's platform links (the video,
then each of its `alternates`); the first is the link its card, ticket or row
follows. For `zh-CN` a past video with a bilibili alternate puts bilibili
first; reservations, started and live videos keep their own platform. With
two or more links, the shelf cards, rows, tickets and the stage's latest
video show `PlatformLinks` in that order. Every video cover carries an
`aria-hidden` `PlatformBadge` (`src/dashboard/components/platform-links.tsx`)
of `videoPlatforms`: the links' platforms, led by the video's own platform
when it is private or removed. Such a cover with no link left fades
(`is-closed`). `VideoAvailabilityChip` marks `members` in the solid `--leaf`
green, as do the schedule's `member` entries, and `private` and `removed`
muted. Its styles are in
`src/dashboard/board.css` on top of `src/site/site.css` and the shared tokens
in `src/index.css`. The hidden `/admin` console
lives in `src/admin/` and is organized around operator tasks: `index.tsx`
registers the views (Overview, Review queue, Schedule, Posts, Videos, Profile
images, Runs, Sources, Detection, LLM, Site) and renders the shell
with hash routing (`#view/tab`; a `#content/<tab>` link opens the matching
view and a `#models` link opens LLM); `use-admin-data.ts` owns every
[Admin API](admin-api.md) call, the session and the overview poll that tracks
the active run while the console is visible; `api.ts` is the transport and error mapping; `types.ts` holds
the wire shapes; `ui.tsx` the console primitives (buttons, panels, native
modal dialog, toasts); `views/` one file per section; and `admin.css` styles
them with `adm-` prefixed selectors. `src/components/ui/` contains
basic UI primitives, while the CSS files contain layout and design tokens. The
presentation layer must not open SQLite or call X, YouTube, OpenAI,
or a third-party proxy directly. The optional mainland China site comes from
the same-origin `/api/visitor-region` response, read at most once per page
load through `src/lib/visitor-region.ts`. When the operator has enabled it,
the footer links to the mirror while the interface is Simplified Chinese;
when the response also reports mainland China, the root app shows a
dismissible Toast offering the site, once per browser session. About's credit
for the mainland China server site is static copy in `src/about/` and does
not read this response.

## Request and UI Ownership

Keep app composition in src/main.tsx, public snapshot requests in
src/dashboard/use-dashboard.ts, and admin requests and polling in
src/admin/use-admin-data.ts. Reuse the existing state and request patterns.
Shared primitives must remain independent of dashboard, history, and admin
features. Extract cohesive helpers when needed; avoid a repository-wide move
for a small change.

Every page, the board included, loads through a dynamic import with its own
CSS and translation chunks, so a direct visit downloads the shared runtime
(React, the settings provider and the base dictionary) and that page's code
only. The first page's import starts as soon as `src/main.tsx` runs. Because
the server answers every page with the same `index.html`, the board's chunks
cannot be listed there statically: the build
(`scripts/vite-page-preload.ts`) writes a small script ahead of the entry
that adds `modulepreload` and style `preload` links for them only when the
address is the board, so they download alongside the entry. The shared
settings provider owns persistent theme, interface language (`kano-locale`)
and content language (`kano-content-locale`, a locale or `interface` to
follow the interface); page message providers
extend its translator without duplicating preferences. The base dictionary
holds the copy shared by pages (header, footer, navigation, page status and
the translation toggle); the board's own copy is in
`src/dashboard/dashboard-i18n.ts`. While a page's code loads, the current
page stays on screen, or on a direct visit a localized loading status
appears after a short delay; a page error boundary offers a full-page
reload when a chunk fails (including after a deployment). Direct visits and
trailing-slash routes work through the existing SPA fallback, and
`/index.html`, which the server answers as a file, is the board
(`src/lib/page-path.ts`). Any other path
shows a localized not-found view with a link back to the board instead of the
board itself; the server answers it with status 404, so `src/main.tsx` and
`spaPages` in `backend/internal/httpapi/static.go` list the same pages.
History, the LINE BLOG archive and About
remain static and never mount the dashboard hook or request its API.

English is the base locale. Every message exists in English, a key missing
from Japanese or Simplified Chinese falls back to it, and a browser language
that is none of the three selects it. Stored records are not interface copy:
event status codes and the generated YouTube details become labels through
`src/lib/event-text.ts`, which also reads the stored legacy Chinese values,
while other details and operator notes show as stored (see
[Stored event text](data-model.md#stored-event-text)).

A related account's promoted post (`post.promoted`) shows the account's
initial on the sunken surface in place of Kano's portrait, beside its
handle; it appears in the All tab and the archive, never under an own
account's tab.

A post that quotes another status (`post.quote`, see
[Quoted posts](data-model.md#quoted-posts)) shows it nested in the post note,
below the post's own wording and images and above the engagement counts: an
indented card on the quieter sunken surface with "display name @handle ·
time", the quoted wording clamped to four lines and up to three thumbnails
("+N" counts the rest). The card follows the quoted post's x.com link from
anywhere except its thumbnails, which open the image viewer with the quote's
images. Read more also appears when the clamp hides quoted wording; the full
post dialog and the archive show the whole quote with larger thumbnails. An
unavailable quote shows a localized "Quoted post unavailable" placeholder
linking to x.com. Quoted wording follows the content language like the
post's own: the quote's `translations` (its stored post's for a self-quote,
otherwise the quoted status's own) show with their own translation switch,
and a quote without one shows its wording as written.
A quote an operator hid is absent from the snapshot, so the board shows the
post without it.

A Bilibili dynamic (`source: "bilibili"`) shows the bilibili mark and name,
linked to the account's space, instead of an `@handle`, and its open link
goes to the opus page. It has no quote and no engagement counts, so neither
shows, and an empty text (an emoji-only dynamic) leaves only its images. The
wall's top line adds bilibili while it holds a dynamic. A stream or
members-only entry links to `liveUrl ?? url` (`eventLink` in
`src/dashboard/format.ts`). The dashboard's `liveNow` becomes a live stage
ticket, "Live now on bilibili", unless an entry of the same platform, type
stream or member and confirmed `live` starts at S with S−2h ≤ `startedAt` ≤
S+3h, as the server matches broadcasts; the match never compares links. The
post wall's account filter adds a bilibili button for each of
`meta.bilibiliAccounts`, with the bilibili mark instead of the mid. A snapshot restored from the
browser drops `liveNow` until the first fresh read.

The console's schedule (`src/admin/views/schedule.tsx`) has a list, a
calendar and an images view. The browser remembers list or calendar in local
storage; the images view has its own address (`#schedule/images`), which the
Review queue links to. It lists the newest stored schedule images as the
Review queue's decision cards (`schedule-images.tsx`) under Active,
Approved, Needs review and Not a schedule tabs. Active leaves out images an
operator labelled Not a schedule, so a rejected card leaves the list; the
Not a schedule tab keeps them, where the label can be cleared. A card
shows its source post's time and wording, folded to four lines. Correct
dates opens a wide dialog with the image and the whole post above the form,
side by side or stacked on a phone. A card's covered dates note when they come from the post's events, and a dated card
offers Show in calendar, which opens `#schedule/YYYY-MM`: the calendar on
that month. Below the month grid the calendar lists the schedule images
whose period overlaps the month, as the same cards, leaving out those
labelled Not a schedule. The list and the calendar share a search field and status tabs
for the review tasks: All, Not confirmed, Suspected (cancellation) and
Deleted. The list groups entries
under one heading per Japan date, with the day's count and a button that
opens a new entry on that date; Upcoming, Past and All dates tabs set the
date range beside it. Each row (`schedule-parts.tsx`) shows the time in
Japan (or All day), the title, which opens the entry dialog, and its badges;
an unconfirmed entry has a labelled Confirm button before the icon actions.
An automatic entry whose every source post is hidden carries a Hidden by its
posts badge and is dimmed, since the public pages leave it out.
The calendar (`schedule-calendar.tsx`) shows a Monday-first month grid
without merged duplicates. Each entry is a chip with its time and title,
styled by review state as the legend below names; a chip opens the entry,
and a day opens its rows below the grid with the same actions and its new
entry button. On a narrow grid the chips become dots. The entry dialog is
either timed or all day. A timed entry needs a start time. Its length
starts at the type default, the [calendar feed's](public-api.md#public-feeds)
length for the type, and saves that end as `endsAt` like the presets and a
custom end. **No end time** saves none, so the board treats a stream as
open-ended; a saved entry without an end opens on it, so an unrelated edit
never adds one. A sale replaces the length with a close date and an optional
close time (`endsOn` and `endsAt`).
A preview line renders the entry as calendar subscribers will see it.

The console's video list (`src/admin/views/videos.tsx`) puts the featured
video above a list with a search field and task tabs: All, Upcoming,
Bilibili, Missing translation and Queued (an open refresh or translation
request); a `#videos/<tab>` link opens that tab. Uploads
merged into another video appear on that video's row, so only the Bilibili
tab lists them on their own. Each row shows the
thumbnail, the title, which opens the correction dialog, the platform, time,
kind, tag, length and format, its badges and merge line, the translation chips,
and the feature, correct and open actions. The correction dialog
(`video-editor.tsx`) opens with a preview of the board's row as the draft
leaves it (the chosen thumbnail, title, source, kind, tag, format and the
number of corrections) and the page-read facts. Below, the corrections
(title, kind, tag, format, who can watch and thumbnail files) sit beside the platform's record (the merge
section and what the platform replaced); one column on narrow screens. A
corrected field is marked and offers a reset to the platform value beside its
label. Refresh, cancel and save stay in the footer.

The Review queue (`src/admin/views/review.tsx`) lists only what waits for a
decision, in tabs for schedule images that need review, suspected
cancellations, video merges and posts; a decision removes the item
and the sidebar badge counts again. The images tab links to the Schedule
page's images view. An image or a suspected cancellation is a decision
card: its left edge takes the state colour (honey
while it needs a decision, leaf once the image reaches the board, neutral
when rejected), the image or the entry's date sits on the left, and the body
shows the badges and source link, the model's verdict with a confidence bar
and its reason or evidence, the facts, and a bottom bar with the note field
and the decision buttons. A cancellation needs a reason before it can be
confirmed. The video merges tab (`merge-parts.tsx`) lists each Bilibili
upload the merge rule merges at its next run or leaves to the operator,
beside its candidates: Merge attaches it to one, and Keep separate keeps it
apart so the rule never merges it. The posts tab has two labelled filters:
Step, Schedule extraction or Translation, each with the overview's failure
count, and State within the step: Failed or Uncertain for extraction;
translation has only failures and shows no State filter. A new step keeps
the state when the step has it and falls back to Failed otherwise; below
560px each filter takes its own full-width line. Posts queued for a run wait
on the run, not the operator, so the tab lists none of them: a note under
the filters counts the step's queued posts from the overview's `backlog` and
links to them in the post list. Each row names the step that failed
(schedule extraction or translation) with its error. A failed extraction
offers Dismiss beside the LLM action; a translation row offers Translate
again, which queues a machine pass. The list pages 50 posts at a time.
Each row has a selection box on its left edge, and shift-click selects the range from the last toggled row. A selection
bar above the list, which sticks under the top bar, selects the whole page
and applies the state's actions to every selected post in one request:
Dismiss and Process again for failed extractions, Process again for
uncertain posts, Translate again for failed translations. A new filter or
page clears the selection.

Console dates are `YYYY-MM-DD` and times `YYYY-MM-DD HH:mm`, in Japan time
and the same in every console language (`src/admin/format.ts`); a stored
day adds its short weekday.

The console's Overview (`src/admin/views/overview.tsx`) opens with the
queue: the active run with its steps and Cancel, or Idle, then the timers in
the order they start, from the overview's `queue`. A due timer shows Due and
whether it waits for the current run; a source timer that is not due has
Start now, or Run next while a run holds the queue. The live-status probe is
listed without a button. While timers are off in the process the panel says
so instead of listing them. Under the timers, Waiting for a run lists the
overview's `backlog`: each kind of operator request that has any (posts to
extract again, X posts and videos to read again, posts and video titles to
translate), with its count and the run that takes it. A row opens the post
list or the video list on those requests; nothing shows while none waits.

The console's Sources page (`src/admin/views/sources.tsx`) has a row of
tabs for X, YouTube, Bilibili uploads and Bilibili dynamics, each with the
platform's logo, a dot (or
a lock) for whether its timer runs here and the state as its tooltip.
The selected source opens with a summary: what it reads, its timer state,
next run, last run and last successful read, the timer switch (which saves
at once) and Run now. While the timer runs, Run now starts it as the
overview queue's Start now does, and reads Run next while a run holds the
queue; while it is off, Run now runs the source's steps once and waits for a
free queue. A source that `SKIP_*` turns off is locked. Below the
summary the main column holds the timed reads, one form with one Save and
Discard: the interval and, for X and YouTube, the sync setting; and the
[method chain](workflows.md#method-chains), with every method's switch and
up/down moves, a method the process cannot run greyed with the variable it
needs, and each Bilibili timer's one fixed method read-only. Save sends only the
changed fields, and the timer switch keeps unsaved edits.

Settings below the timed reads are folded: a header row with the title,
its state and any quick switch opens onto the settings and figures, and
each fold saves on its own. X adds its X API limits (gate and daily cap,
with the API's place in the chain and today's usage in the header, and
today's usage and each account's timeline failures shown as meters against
them), then the related accounts panel
(`src/admin/views/related-accounts.tsx`): each account's handle and keyword
rules, one per line, side by side (stacked on a phone) with its last read,
stored and promoted counts and last error below, an Add account button that
starts with the default rules, and one Save; a link opens External posts.
YouTube adds its progressive confirmation, where push and the
live-status probe are folds with their switch in the header (the probe's fold
also lists its active videos, each with Stop checking, and its stopped
videos, each with Check again), followed by
the read-only
[follow-up checks](sources.md#follow-up-checks-and-refresh-requests), and
yt-dlp, with its status in the header. Bilibili uploads adds the Bilibili
live room, the live-status probe's Bilibili part: the room's state and the
check's switch in the header, its interval, next check and the stream
entries it follows, then the room's last check. Each Bilibili tab adds,
below the timed reads, its read's state: the account, what is stored, the
last read and its error, the login cookie and its last check, whether the
source runs here and the shared captcha backoff, with the sign-in hints.
X and YouTube fold their timed-read
limits in the main column and the manual-fetch limits, which they share,
under the manual fetch, one field per row with its label beside the input.
Both limit folds edit the page's one read of the settings. Longer
explanations sit in a dialog behind a question mark beside a title or
label: the chain, the X API limits, YouTube's per-run limit, its
progressive confirmation and each of its steps, and yt-dlp.

The manual fetch sits beside the main column: method, mode, count or
dates, options, then the newest fetch's summary. The Bilibili tabs hold only
their manual action there: one upload added by BV ID or link, or one dynamic
by its link, disabled while the source does not run here. On a narrower screen the side column follows
the main one, and on a phone the tabs share the width and show only the
source names, and a fold's header controls move under its title.

External posts (`src/admin/views/external-posts.tsx`, in the Data group
after Posts) lists the X status pool in the post list's feed layout:
author, name and Japan time; the wording with its image count, links to the
own posts that quote it (opening Posts searched for the post), the accounts
that reposted it, a link to a promoted status's post in Posts and a locked
decision's reason; the translation summary of a status that has no post;
badges for its state (On the board, Demoted, Stored only, Own post), a lock
and its reasons; and icon actions to promote a related account's status,
demote a promoted one, return a locked one to the keyword rules and open it
on X. Promote and demote open a dialog that shows the wording and requires
a reason. Filters cover the text or ID, the author (the related accounts),
the reason and the state; a header link opens the related accounts on
Sources.

The console's post list (`src/admin/views/posts.tsx`) is a feed of posts
separated by rules, one row of columns per post: the account over the Japan
time, the wording (three lines) and its quote, the post's own cached images
(up to three, "+N" counts the rest), the translation status, the extraction
state with a Hidden badge for a post hidden from the public pages, a
Promoted badge, which opens External posts, for a related account's
promoted status, and a
Schedule image badge for a post with a schedule image record, and the
edit, "process again with the LLM" and open actions. The columns after the
wording have fixed widths, so they line up from post to post. A text-only
post shows no image cell; a post whose images are not cached yet shows one
placeholder with the reason. A feed under 1000px puts the account, time,
state and actions on one line above the wording, images and translations,
and under 560px the state gets a line of its own. A hidden post's wording is
dimmed. Beside the account, extraction, translation and date filters,
Queued for keeps the posts with an open operator request: any, Extract
again, Read again from X or AI translation. A `#posts/queued-<kind>` link
opens the list on one kind.

Edit opens the post in a dialog: the whole wording, every cached image, the
translations, the quote, the schedule image form and a switch that hides the
post from the public pages or shows it again, with the LLM and open actions
in its footer. An X post with images can be marked as a schedule image there:
the image (a choice when the post has several), a required reason and an
optional first and last day; a post with a record shows its badge and a link
to the Schedule page's images view.

The quote is a read-only card on the sunken surface under the wording, so
its thumbnails never read as the post's own images. Its header holds the
author, handle and time, badges for a quote hidden from the board and a
queued refresh, and icon actions that hide the quote from the board or show
it again, queue a refresh from the source (disabled while one is queued) and
open the quoted post. Below come the wording, folded to three lines with a
"Show full quote" toggle that appears only when the clamp cuts it, the ready
quote thumbnails that open the image viewer, or the unavailable state. The
edit dialog shows the quote's whole wording. Nothing edits the quote itself.

Post text and video titles come with server-resolved `translations` (see
[Translations](data-model.md#translations)). `src/components/translated-text.tsx`
chooses what a reader sees from the content language, not the interface
locale: each content language gets that locale's translation by default,
except that Japanese content keeps wording detected as Japanese
(`pickTranslation`). The header's language menu
(`src/components/preference-controls.tsx`) sets both; the content language
defaults to "Same as interface", and the operator console shows only the
interface choice. The LINE BLOG paragraph translations and the schedule
month grid follow the same content locale, while feed links, history copy
and the bilibili-first rule follow the interface locale. Translated wording
is followed by `TranslationToggle`, a translate icon and "AI" whatever the
translation's origin, lit (`is-active`, `aria-pressed`) while the
translation shows; selecting it shows the source and turns it off. Its
tooltip and accessible name spell out the origin (AI, platform or editor),
and on a title the tooltip holds the source. A post's switch follows its
last word; while the board's six-line clamp hides the end of the post, it
keeps its hidden place in the text and a copy shows beside Read more. The
LINE BLOG archive's marks use the same class and states. Video and schedule titles
go through `src/dashboard/components/translated-title.tsx`: `TranslatedTitle`
for a card title (spotlight tickets, shelf tickets, cards and rows, archive
rows, week-guide entries) and `TranslatedHeading` for the shelf's lead card.
A button cannot go inside a link, so these cards are
not links themselves: the title is the link (`.card-link`), and
`followCardLink` in `src/site/links.tsx` opens it from anywhere else on the
card. A week-guide entry's title is instead the button that opens the
entry's sheet, which holds its link, and `followCardLink` presses it the
same way. A clamped title clamps only its wording, so the switch stays visible.
Month-grid chips are too small for a switch and show the reader's wording
unmarked. An operator's
Featured title from the focus record shows as written. The wording shown
carries its `lang` attribute. The console's
completeness chips and translation editor are in
`src/admin/views/translation-parts.tsx`, shared by posts and videos.

Board images from the media cache, LINE BLOG archive images and the fixed
history images load through `srcset`: `src/lib/media.ts` lists the server's
[resized copies](media-cache.md#resized-copies) below an image's known width,
and each card or thumbnail passes a `sizes` hint for its layout. The
full-image viewers, profile media and other bundled assets keep their single
URL.
The schedule header's Subscribe menu (`calendar-subscribe.tsx`) offers the
[calendar feed](public-api.md#public-feeds) as a `webcal:` link and a copyable
`https` address built by `src/lib/feeds.ts` from the page's origin. Both carry
the reader's language as `?lang=ja` or `?lang=zh-CN`; English keeps the bare
path. The settings provider points `index.html`'s Atom and calendar discovery
links at the same language whenever the locale changes.

SQLite stores one text per seeded timeline and link row. The board translates
that text through src/dashboard/seed-copy.ts, keyed by the exact seed wording
in static-seed.json, so a row whose text was changed shows as stored. Update
both files together; the frontend tests require English and Japanese copy for
every seed row.

The dashboard hook coordinates one current request. Snapshot refreshes share an
in-flight promise; a manual refresh cancels an older revision read, and stale
completions cannot install state. A 15-second deadline covers headers and JSON
consumption.

Each successful snapshot is also kept in `localStorage`
(`kano-dashboard-snapshot-v1`, with the time it was stored). The next visit
renders a stored snapshot under a week old at once, through the same local
time derivation, and reads the API as usual; the board reports a refresh
rather than loading, and the first read replaces it or, on failure, leaves it
shown with the error. The key's suffix changes when the snapshot's shape
does. Without a usable stored snapshot the stage shows its placeholder, which
has a full ticket's shape (the 16:9 art band and a stub with a two-line
title, three lines at 640px and below), so the usual ticket fills it in place.

While the page is visible the hook keeps an `EventSource` on the
[revision stream](public-api.md#revision-stream). A pushed revision that
differs from the snapshot's reads the snapshot after a random delay of up to
three seconds, so the boards that hear one change do not all read at once; an
unchanged one refreshes a stale calendar. A change pushed while a snapshot
read is in flight is compared once that read ends. Between changes a
completion-scheduled timer only revalidates the snapshot once a minute, since
time-derived fields such as `liveStatus` expire without a revision change.
EventSource reconnects by itself after a drop, and the reconnected stream
resends the current revision. A refused stream, or three errors without a
message between them, hands over to revision polling every 30 seconds; the
stream is tried again after five minutes of polling. Failed requests back off
through 8/16/32/60 seconds. Failed first loads and failed reloads retry
snapshots, preserving a previously loaded snapshot. A hidden page closes the
stream and pauses its timer; becoming visible reopens the stream, whose first
message is the current revision, and visibility/focus events coalesce into a
prompt check while respecting failure backoff and a one-second minimum gap.
Unmount closes the stream, aborts requests and removes deadlines, timers and
wake listeners. The 30-second local time derivation below runs separately.

## Time-dependent Snapshot State

The overall state, shown by the header dot and the profile card, is the
latest completed run in `meta.lastSync`: its outcome and its finish time (its
start time when it has none). An API failure takes priority, then a run that
is neither `success` nor `partial` (needs review); a `partial` run is partial
and a run older than 24 hours is out of date. Below it (on phones, before
its dot on one row, with the state's wording left to screen readers and the
dot's tooltip), the profile card lists
the last successful X, YouTube and bilibili reads from `meta.sourceFetchedAt`,
each marked out of date after 24 hours; these source times do not change the
overall state. The local clock updates both warnings without a request.
Missing timestamps, and ones more than five minutes ahead of the local clock
(a visitor clock slightly behind the server still shows a fresh read), remain
waiting. This is a display threshold, not a synchronization timer.

The board rederives event/video availability, counts, and focus candidates from
its local snapshot every 30 seconds and when the page regains visibility/focus,
even when `dashboard_revision` is unchanged. Expired activities yield to later
events; date-only entries use the current Asia/Tokyo date and never acquire a
synthetic start time. A fresh probe-confirmed `liveStatus` decides first:
`live` stays live until a known end, while `ended` and `upcoming` are never
on air, and the On air badge and the schedule's On air status use the same
rule, so the week guide never folds a stream as past while the stage shows
it on air; a confirmed `upcoming` waiting room past its reserved time stays
upcoming there. Without one, streams and
memberships retain a three-hour live grace period, or their explicit end time; other activities, including YouTube
premieres, count as live only before an explicit end. A sale period is
never a focus candidate. A started candidate
without an explicit end or a confirmed live state is only presumed live: in `nextEvent`, `nextStream` and
`pickSpotlight` it yields to a later timed candidate that has started or starts
within 30 minutes, while a candidate with an explicit end keeps its place until
that end. Events and videos for the same YouTube video, matched by the identity
below, form one spotlight candidate. A video linked to an event inherits its explicit end time for
both `nextStream` and `pickSpotlight`. YouTube watch, youtu.be, live, and shorts
links match by the video identity parsed locally from their public URL, ignoring
share/query parameters and supported YouTube subdomain differences. Other links
retain exact URL matching; different videos and lookalike hosts do not match.
`pickSpotlight` gives a candidate with artwork the full ticket and one without
it the compact layout; `pickCompanion` then adds the latest video beneath a
compact ticket unless it is the same link. Both read `summary.latestVideo`,
which `deriveDashboardAt` recomputes as the newest non-upcoming video that is
not a Short (`isShort === true`), unless it is the featured video
(`featuredVideoIdOf`: `meta.featuredVideoId`, or the focus video of a
snapshot without that field), and that is neither private nor removed
(`videoIsGone`). An unknown format or availability stays eligible, and when every recent upload is a
Short the stage shows no latest video. The play mark appears only on
YouTube or Twitch links; X links read as an announcement.
Matching needs no protected source item IDs or additional public API fields.
At the end instant the video yields to later focus candidates. This derived video
`endsAt` belongs to local snapshot state. Without an explicit end, a stream's
three-hour grace ends exactly three hours after the start. Confirmed cancellation flags prevent local
rederivation from restoring a cancelled reservation. Selected-media startup
repairs follow the [durable revision contract](data-model.md#event-precedence),
so the board's revision follow-up reloads a changed avatar URL without
increasing full-snapshot request frequency. This requires no source
synchronization or external request.

## Installable App

`index.html` links `public/manifest.webmanifest` (name "What’s Kano Up To?",
short name "Kano Navi", `start_url` and `scope` `/`, `display: standalone`,
background and theme colour `#fdf9f6`, the light `--background` token) and
the icons in `public/assets/icons/`: a 32 px favicon, 192 and 512 px icons, a
512 px maskable icon with the avatar inside the 80% safe zone on the
avatar's own pink, and a 180 px `apple-touch-icon`. They are resized copies of
the bundled `public/assets/kano-avatar.jpg` and follow its provenance; the
theme-color metas switch between the light and dark backgrounds.
There is no service worker or offline mode: an installed app loads the same
pages, which read the live local API. The Go static server serves
`.webmanifest` as `application/manifest+json` and, like other unhashed public
files, revalidates the manifest and icons on each use, so a replaced icon
takes effect without renaming it.

## Provider Company Icons

The provider presets are OpenAI, Anthropic, DeepSeek, OpenRouter, Gemini, and
Custom. Company marks are bundled local SVGs from Lobe Icons 1.90.0 (MIT);
they never require a browser CDN request. The license ships in
`public/assets/providers/LICENSE.txt`. Configured providers use the API host,
name, or stable ID to identify a mark; unknown custom providers retain their
initial. Removing a preset does not delete saved providers.

The LLM view shows the extraction, schedule-image check and translation queues
side by side, then the recent failures, then the
providers as a list beside the selected provider's detail. Recent failures
lists each stored failure's stage, relative time (the Japan time on hover),
message, model and a link to its post or video; it loads with the view and
on its reload button. A queue change
(add from the model picker, drag or move, remove) shows at once and is saved
in order: each queue has at most one save in flight, and edits made meanwhile
collapse into the latest order, sent next. A refused save returns the queue to
its last stored order and drops the edits that followed it. Each queue marks
the model each stage calls first. The provider dialog holds only the connection, with the presets above
it; there is no default model. Saving a new provider with a key selects it and
lists its models, an explicit operator action. The detail adds ticked models
to the catalog, edits their tags, turns them on or off, puts them in any queue
whose tag they carry and tests them one at a time. Removing a model takes a
second click on its danger-toned icon within a few seconds.

List filters in the console are one row of pills, one per filter, with a
date range pill and a clear button that appears while a filter narrows the
list. A choice filter opens its own list under the pill, which follows the
listbox keyboard pattern. Paged lists show the total, the first and last
page numbers with two either side of the current page, and a jump field
that goes to the nearest existing page. The Runs page puts the run history
beside the open run on a wide
screen (the newest run is open until another is chosen) and opens a run in
place on a narrower one. The open run is in the address (`#runs/<id>`): the
overview's recent runs link there, and a linked run scrolls into view, read
on its own and listed first when newer runs pushed it off the first page. A
run lists its steps one line each: the step's
outcome, its window and up to three non-zero counts, problems first. A step
opens to everything it counted, as label and value lines with zeros
receding; failed and partial steps open by themselves. Run details explain
stored error codes in words and keep the code beside them. A failed or
partial run copies its diagnostics for an agent or an issue: each step's
outcome and error code, then the stored run as JSON, with codes and English
labels so the text matches the source. A browser that refuses the clipboard
shows the text in a dialog to copy by hand.

## Related Docs

- [Design and interaction](../development/design.md)
- [Testing](../development/testing.md)
- [User guide](../user/index.md)
- [History archive maintenance](../development/history-archive.md)
- [LINE BLOG archive maintenance](../development/lineblog-archive.md)

## Polling and Calendar Recovery

Calendar and admin requests use the shared lifecycle/deadline helper with a
12-second deadline including body consumption. The public snapshot coordinator
uses the 15-second deadline and refresh deduplication described above. All requests
are cancelled on unmount and reject superseded responses. An admin view's
`useResource` read supersedes only that view's earlier read, so two views
reading the same path load independently, and only the view's newest load
writes its state. Automatic polls wait for an active request rather
than starting overlapping work. Their timer is scheduled after settlement,
including failure, with exponential backoff capped at 60 seconds. Public polls
pause while hidden, and so does the admin overview poll, which reads at once
when the console becomes visible again. A failed initial snapshot retries even
before any revision has been recorded; manual retry is available. Admin 401
stops polling and requires login. Failed initial admin loads also retry.

The board's calendar shows one week; its selected week lives in
`use-dashboard.ts`. It reads
one bounded page, shows the complete range/day counts, and offers more-event
pagination. Changing the range cancels old work and begins at page one; errors keep a retry control.
Every same-range refresh rereads the loaded page range and replaces it atomically,
even at the same revision: time-based public fields such as a probe's trusted
`liveStatus` can expire without a stored write. A refresh stops
at the new last page if the range shrank. A change while reading that range restarts
from page one, with at most three attempts under the shared request deadline.
Failed or repeatedly changing reads retain the complete known generation for retry;
they never append pages from different revisions. Every successful full board
snapshot, including its visible minute refresh and manual reload, refreshes the
calendar. Active calendar work finishes first, and later revision checks,
pushed or polled, consume any pending time refresh or calendar revision.
The calendar snapshot and its errors are tagged with their range. Only the
first read shows the loading placeholder. A week change keeps the last loaded
week rendered with its own dates, dimmed and inert after a short delay, and
swaps the whole body (days, count and schedule image) once the selected week
arrives; a failed read reports its error only while its week is selected. A
refresh of the shown week keeps its days or empty-week note in place and swaps
the data when it arrives, so a quiet week does not flicker on each refresh. The
schedule-image strip below the days appears only for a week with a schedule
image; a week without one has no strip or placeholder.
Adjacent-date hints support jumping to stored activity in either direction.

Snapshot, calendar and archive reads use `cache: "no-cache"`: the browser
revalidates its stored copy with the response's
[ETag](public-api.md#public-snapshot-contract) and receives a bodiless 304 when
nothing changed, so the minute refresh, and an open dialog's reread after it,
cost little when the board is quiet.

The archive dialogs read through `useArchive` in `use-dashboard.ts`, only
while one is open: the schedule reads the Monday-first weeks covering the month
from `/api/calendar`, past posts read the month's own days from
[`/api/posts`](public-api.md#bounded-public-archives), and past videos read
the [video timeline](public-api.md#bounded-public-archives) from `/api/videos`,
from the newest video or from the last day of a chosen start month. They
follow the same rules as the calendar: pages of one revision are appended, a
later page from another revision rereads the loaded pages atomically (up to
three attempts under the 12-second deadline), and a board revision change
rereads them while the dialog stays open. Schedule and video archives also reread
their loaded pages after each successful full board snapshot, even at the same
revision, so expired live states disappear there too. A snapshot arriving during
an archive read or pagination waits for that work to settle before refreshing;
a failed refresh keeps the complete known prefix and retries on the next board
snapshot. Post archives follow stored revisions. These refreshes
share the board's visibility pause and snapshot backoff rather than starting a
separate archive timer. Changing the month, the post account, the video
category or the timeline's start month, or closing the dialog, cancels the
read and discards late pages.
While the post archive or the video timeline is open, `useArchiveMonths` reads
its month index from
[`/api/posts/months` or `/api/videos/months`](public-api.md#bounded-public-archives)
and rereads it on each board revision. The month label opens a jump grid
(`MonthPicker`): a year stepper bounded by the oldest indexed month and the
latest month (this Japan month, or a later month holding a video
reservation), over twelve months whose counts follow the account or category
filter. Months without posts and months
after the latest are marked `aria-disabled`; arrow keys move between months,
Page Up and Page Down change the year, and Escape or a press outside closes the
grid before the dialog. A failed index read keeps a known index, and without
one every month up to the latest is offered.
A click anywhere in a month-grid day picks it, listing its entries beside the
grid (below it on phones) until the hide button clears the selection and the
grid takes the full width. A grid entry is a button: it picks its day, marks
the entry in that list and scrolls it into view, again on every pick, and the
list's entries open the details sheet. Only the day heading of the list is a
live region, so an opened sheet is not announced as an update. The panel can
show that week on the board. On phones entries are dots that ignore taps, so a tap always
picks the day. The post archive lays out full
posts either as a masonry wall or as a single column, a choice kept in
`localStorage` (`kano-post-archive-layout`). The masonry measures each post
and places it, in order, under the column that ends highest
(`masonryLayout` in `src/dashboard/post-rows.ts`), so it reads row by row
while the cells stay in post order for keyboard and screen readers; it
re-measures when the wall or a post resizes. A container query on the dialog
head hides the layout switch while the wall is narrower than two masonry
columns, where both layouts are one column. The video dialog is a timeline
(`VideoTimeline`): one section per Japan month of the loaded videos, in the
API's newest-first order, under a header that sticks to the top of the dialog
body and names the month with its indexed count. Each month is a grid of
cards, each the cover above the title, then the Japan date and time, the
phase or form chip, the tag chip and platform links: four columns, three or
two as the dialog narrows. The next page loads once the load button comes
within 600 px of the visible area (an `IntersectionObserver` on the dialog
body), and the button stays for keyboards; after a failed read only the button
loads again. A polite status line counts the loaded and total videos. Loaded
pages stay in memory until the dialog closes; months out of view skip layout
and paint (`content-visibility: auto`) and the cards are memoized, so a
change of the scrolled-to month renders no card again. In the
dialog's timeline mode (`ArchiveDialog`'s `timeline`), the month label follows
the scroll (the last month whose header has reached the top of the body) and
is not a live region, and the month arrows give way to a Latest button. A
month chosen in the jump grid scrolls into view when it is loaded; otherwise
the timeline restarts from that month, offering a button for the newer videos,
and Latest returns to the newest. The
category buttons (`VideoArchiveFilter`) carry the tag icons, pass the API's
`category` filter, show the index's per-category totals over every month
(`categoryCounts` of the loaded timeline while the index is unknown), and keep
the choice until the dialog closes. An image viewer opened from the
post archive stacks above it; Escape closes the topmost dialog and focus
returns to its opener.
Images match their applicable inclusive period, without an updated-at fallback.
The bounded focus snapshot refreshes at least once per minute while visible to
advance its shortlist as time passes, even when the durable revision is unchanged.
