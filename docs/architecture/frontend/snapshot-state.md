# Snapshot Requests and Recovery

How the board requests, caches, revalidates and recovers its snapshot.

## Dashboard Snapshot Requests

The dashboard hook coordinates one current request. Snapshot refreshes share an
in-flight promise; a manual refresh cancels an older revision read, and stale
completions cannot install state. A 15-second deadline covers headers and JSON
consumption.

Each successful snapshot is also kept in `localStorage`
(`kano-dashboard-snapshot-v1`, with the time it was stored). The next visit
renders a stored snapshot under a week old at once and reads the API as usual;
the board reports a refresh rather than loading, and the first read replaces
it or, on failure, leaves it shown with the error. Additive projection fields
are filled with compatible defaults while a stored response does not contain
them. Without a usable stored snapshot the stage shows its placeholder, which
has a full ticket's shape (the 16:9 art band and a stub with a two-line
title, three lines at 640px and below), so the usual ticket fills it in place.

While the page is visible the hook keeps an `EventSource` on the
[revision stream](../public-api/snapshot.md#revision-stream). A pushed revision that
differs from the snapshot's reads the snapshot after a random delay of up to
three seconds, so the boards that hear one change do not all read at once; an
unchanged one refreshes a stale calendar. A change pushed while a snapshot
read is in flight is compared once that read ends. Between changes a
completion-scheduled timer only revalidates the snapshot, since time-derived
fields such as `liveStatus` expire without a revision change: one second
after the projection's `nextTransitionAt` when that comes within the minute,
and otherwise a minute after the last read. A transition the browser's clock
had already passed when the snapshot read started waits for the minute, so a
browser clock ahead of the server's cannot repeat the read; one that passes
while the snapshot is in transit is read again at once.
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
wake listeners. The local clock updates freshness copy; snapshot revalidation
keeps the time-dependent projection current without source synchronization.

## Time-dependent Snapshot State

The profile card ends with one row: the counts, each an icon and a number
whose window label is its tooltip and screen-reader text, then one icon per
source, X, YouTube, bilibili and the shop, whose tooltip and screen-reader text carry its last successful read
from `meta.sourceFetchedAt`; a read older than 24 hours marks the icon out of
date with a dot, and a source never read fades. A read that stores nothing
moves that time without a revision, so the minute revalidation brings it. The card has no reload: the
board revalidates by itself, and the API failure alert carries the retry.
The header holds no snapshot state, so it keeps the same width on every
page. A run's outcome in `meta.lastSync` is operator diagnosis and does not
change a source's freshness; a source that keeps failing shows once its last
read ages out. The local clock updates these warnings without a request.
Missing timestamps, and ones more than five minutes ahead of the local clock
(a visitor clock slightly behind the server still shows a fresh read), remain
waiting. This is a display threshold, not a synchronization timer.

The API supplies event and video availability, counts, focus order, spotlight
identity, sale IDs and shelf order in the dashboard projection. `view` fields
travel with calendar and archive records so the same codes render in every
reader. The minute revalidation obtains a fresh projection when time-dependent
states can expire without a durable revision; a visibility or focus wake also
uses the same request path. `deriveDashboardAt` returns a versioned projected
snapshot as received. A response without the projection follows the compatible
local derivation path, including the live grace, sale expansion, focus yield,
latest-video eligibility and cancellation rules needed by that response.

`pickSpotlight` joins the projected event and video IDs to the returned
summary, translates the projected live and members-chip flags, and keeps the
existing full or compact layout. `pickCompanion` follows the projected
companion ID. URL identity matching remains a link presentation concern: it
orders platform links and labels the destination without deciding focus or
availability. The public API exposes only the stable IDs, codes and booleans;
source item IDs and raw provider data remain on the server. Selected-media startup
repairs follow the [durable revision contract](../data-model/event-precedence.md),
so the board's revision follow-up reloads a changed avatar URL without
increasing full-snapshot request frequency. This requires no source
synchronization or external request.

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
[ETag](../public-api/snapshot.md) and receives a bodiless 304 when
nothing changed, so the minute refresh, and an open dialog's reread after it,
cost little when the board is quiet.

The archive dialogs read through `useArchive` in `use-dashboard.ts`, only
while one is open: the schedule reads the Monday-first weeks covering the month
from `/api/calendar`, and past videos read
the [video timeline](../public-api/calendar-and-archives.md#bounded-public-archives) from `/api/videos`
in pages of 30, from the newest video or from the page holding a chosen
month. They
follow the same rules as the calendar: pages of one revision are appended, a
later page from another revision rereads the loaded pages atomically (up to
three attempts under the 12-second deadline), and a board revision change
rereads them while the dialog stays open. Schedule and video archives also reread
their loaded pages after each successful full board snapshot, even at the same
revision, so expired live states disappear there too. A snapshot arriving during
an archive read or pagination waits for that work to settle before refreshing;
a failed refresh keeps the complete known prefix and retries on the next board
snapshot. These refreshes
share the board's visibility pause and snapshot backoff rather than starting a
separate archive timer. Changing the month, the video
category or the timeline's start month, or closing the dialog, cancels the
read and discards late pages.
While the video timeline is open, `useArchiveMonths` reads
its month index from
[`/api/videos/months`](../public-api/calendar-and-archives.md#bounded-public-archives)
and rereads it on each board revision. The month label opens a jump grid
(`MonthPicker`): a year stepper bounded by the oldest indexed month and the
latest month (this Japan month, or a later month holding a video
reservation), over twelve months whose counts follow the account or category
filter. Months without records and months
after the latest are marked `aria-disabled`; arrow keys move between months,
Page Up and Page Down change the year, and Escape or a press outside closes the
grid before the dialog. A failed index read keeps a known index, and without
one every month up to the latest is offered. The schedule's month label opens
the same grid without an index: every month is offered and the year stepper
is unbounded.
Until a month's first page arrives, the schedule dialog shows the month grid
with its dates and a shimmering bar in each day, so a month change keeps the
body in place; after a failed first read the day panel shows no outlines until a
retry starts.
A click anywhere in a month-grid day picks it, listing its entries beside the
grid (below it on phones). The current month opens on today; in another month
the side panel asks for a day, and on phones the day list waits below the
grid until one is picked. An empty month's note and a failed read sit in the
side panel (below the grid on phones), never above the grid. A grid entry is a button: it picks its day, marks
the entry in that list and scrolls it into view, again on every pick, and the
list's entries open the details sheet. Only the day heading of the list is a
live region, so an opened sheet is not announced as an update. The panel can
show that week on the board. On phones entries are dots that ignore taps, so a tap always
picks the day. The video dialog is a timeline
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
month chosen in the jump grid scrolls into view when it is loaded. Otherwise
the timeline restarts at the page of the whole newest-first timeline that
holds the month's newest video (`timelineStartPage`: the month index's videos
of the later months, in the chosen category, over the page size), reads the
following page with it so the month can reach the top, and scrolls to the
month once it is read. Its first loaded page is the snapshot's `firstPage`;
while that is after page 1, a button above the list reads the page before it
and puts its videos above, and the video that was first keeps its place on
screen; when the snapshot changed in between, it rereads the whole run from
that newer page instead. The button stays focusable during background
rereads and ignores presses while a read runs. A refresh rereads the whole
loaded run, from `firstPage` on. While
the months around a jump or the newly read videos render at their real height
in place of the `content-visibility` estimate, a `ResizeObserver` keeps the
target in place for up to a second, letting go as soon as the reader scrolls.
Without a month index the timeline reads from the chosen month's last day
(`until`), and the button above returns to the newest. Latest scrolls to the
top once the loaded run reaches the newest video, and otherwise restarts the
timeline from it. The
category buttons (`VideoArchiveFilter`) carry the tag icons, pass the API's
`category` filter, show the index's per-category totals over every month
(`categoryCounts` of the loaded timeline while the index is unknown), and keep
the choice until the dialog closes.
Images match their applicable inclusive period, without an updated-at fallback.
The bounded focus snapshot refreshes at least once per minute while visible to
advance its shortlist as time passes, even when the durable revision is unchanged.
