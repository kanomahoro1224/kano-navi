# Event Labels, Sales and Platforms

How events are labelled, how sale periods work, which platforms an event lists, and how event text is stored.

## Event Labels

`events.tag` is the calendar label of one entry: `live`, `game`, `talk`, `song`
or `video`, or NULL. `game`, `talk` and `song` name the content and take
priority; `song` covers singing streams, covers, originals, music videos and
song premieres. `live` is a stream and `video` an upload or premiere that none
of them fits or whose content is not shown, and a mixed entry takes the
activity it leads with. The extraction prompt and the title inference share
this definition. `tag_source` records who decided it: `manual` for an
operator, `llm` for the extraction prompt or an X notice merged into the
entry, and `title` for the title inference. Precedence is manual > `llm` >
`title` > none, except that among automatic labels a content label is never
replaced by a form label:

- An operator's value belongs to the locked row, so nothing automatic replaces
  it. Clearing a stored label is the operator's decision too: it stores
  `tag_source = manual` without a tag, so the entry shows no label and its
  videos are not sent to the title inference. Saving the label as automatic
  (`auto`) clears `tag`, `tag_source` and `tag_attempted_at`: the row stays
  locked, so the extraction and the entry's own title inference leave it
  alone, but it reads on to its videos' tags and no longer holds them back
  from the title inference. A save that sends the
  stored label back unchanged keeps its `tag_source`, so an unrelated edit or
  a cancellation decision does not make an automatic label the operator's.
- The extraction prompt returns a label only when the post or image states the
  activity. An unknown value is dropped and the entry is kept without a label.
  A post that states no label leaves the row's label and source as they are;
  a stated label replaces a `title` label on the same row unless a form would
  replace content.
- YouTube reservations are created by the sync without the extraction prompt,
  so the `translate` queue also labels them from their title. The request
  names the entry's form (a `release` entry is a premiere, any other a
  stream), so a title that shows no content still gets the right form label.
  A judgement without a label, including an answer outside the five codes,
  is stored as `tag_source = title` with no tag and is not asked again. A
  title every model fails on stays unjudged and only receives
  `tag_attempted_at`; it is offered again only once that attempt is a day
  old, so it costs at most one attempt a day rather than one a run.
  Candidates never attempted come first, then the least recently attempted,
  so a failing title cannot hold the head of the queue and a provider outage
  judges nothing. An entry that a labelled notice is
  merged into is not sent.
- A creator's rename, written by the YouTube sync, clears a `title`
  judgement that found no content (no label or a form label), so the new
  title is asked about again; a content label the title gave stays.
- Every rule merge left in place, including one kept from an earlier run, lends
  the label of its notice to the surviving event when that event has no label
  or a `title` one and is neither locked nor manual, unless a form would
  replace content. When several labelled notices share a target, a content
  label lends before a form label, then the smallest notice ID wins. An
  inherited `llm` label is replaced by later runs only when it is a form and
  the notice gives content. The copied label stays when the merge is released.

Storing a label or replacing one with a different value advances the
dashboard revision; a judgement without a label, a failed attempt, or a change
of `tag_source` alone does not.

### Shown Label

Public readers do not show `events.tag` as stored. Each read derives the
label of an entry from the entry and its YouTube videos: the videos linked to
the entry or to an entry merged into it, by the entry ID, its source item or
its event sources. The shown label is the first of:

1. the entry's label when `tag_source = manual` (an operator's row without a
   label reads on);
2. an operator's tag on one of its videos (`videos.manual_tag`);
3. the entry's content label;
4. a video's inferred tag (`videos.tag`);
5. the entry's form label.

Video tags are content tags only, and with several videos the alphabetically
first tag is taken. A video's tag reads the entry in the other direction (see
[video form and tags](video-form.md#video-form-and-tags)),
so the calendar and the video archive agree on content. Nothing is copied
between the rows. The derivation looks every entry and video up by ID, and a
store test checks the query plan for it. The admin event list shows the
stored label and its source.

## Sale-period rationale

A sale is stored as one period and shown at its opening and closing dates. This keeps long order windows out of every intervening calendar day while preserving both dates for calendar feeds and status calculation.

## Sale Periods

`events.event_type` is `stream`, `member` (a members-only stream),
`appearance`, `release` (the date a work becomes available), `sale` or
`event` (any other entry). A `sale` is a period to buy, pre-order, order or
apply for goods, media or tickets, and is the only type that stores a close:
the nullable `ends_on` holds its Japan close date and `ends_at` its close
instant when the time is known. A sale's `ends_at` always has its `ends_on`,
the Japan date it falls on; the store rejects a manual entry whose close
time falls on another date. Every other row has a null `ends_on`; the admin
API rejects one and the extraction ignores it. Sales come from post
extraction and from the [official shop](../sources/shop.md#official-shop), whose
entries are periods with the opening time when known and the close date
without a time. A sale row takes one of three
shapes:

| Shape        | `starts_on` / `starts_at`  | `ends_on` / `ends_at`                   |
| ------------ | -------------------------- | --------------------------------------- |
| Period       | opening date, opening time | a later close date, close time          |
| Deadline     | the close date, no time    | the same date, close time or null       |
| Opening only | opening date, opening time | null, or the same date and a close time |

Every reader shows a sale at its ends rather than across its days: a period
on its opening date and on its close date, a deadline on its date alone, and
any other sale on its opening date. Calendar pages therefore include a sale
whose `ends_on` falls in the range, count it in `dayCounts` on its close date
when that is later than its opening date, and take close dates into the
adjacent dates; the dashboard's current-week selection includes it the same
way. The dashboard also carries up to 10 sales that are on sale on the
current Japan date or open within seven days (a deadline counts as on sale
until its date), closing soonest first, so a period that opened weeks ago
reaches the board's on-sale list. A sale is never `nextEvent`. Reminders of one sale merge into its
period ([Rule C](duplicate-merging.md)). The partial `events_ends_on_idx` index
supports the close-date reads, and startup adds the column empty and the
index to existing snapshots without rewriting events.

## Stored Event Text

English is the base language of stored text. An event's `status` holds either a
lowercase English code the interface translates or an operator's note, shown as
written. YouTube synchronization writes `scheduled`, `live` or `ended`; an
extraction entry without a status, or with a cancellation-like one, stores
`pending`, and the model is asked for short English words such as `scheduled`,
`tentative` or `released`. Automatic `detail` text is English: `YouTube Live`
or `YouTube Premiere`, which the interface reads as codes and translates, or
the model's summary, shown as written. Titles keep their source wording.
The labels and the codes they cover live in `src/lib/event-text.ts`.

Startup replaces Chinese display text in stored events (`已预约`, `会员限定`,
`直播中`, `已完成`, `待确认` and `YouTube Live / 预约`-style details) with the
codes and English wording on rows synchronization owns (automatic, unlocked,
not deleted) and increments the dashboard revision when a row changes. Manual
locks and tombstones keep their text; the frontend reads those legacy values
as the same codes and wording.

## Event Platforms

An entry happens on one or more platforms. `events.platforms` is the JSON
array of its **announced platforms**, each of `youtube`, `bilibili` and
`other` at most once, in announced order (the entry's own platform first),
and `events.platforms_inferred` is 1 when the array came from a source's
default rather than from the wording or an operator; a Bilibili-only entry
is one whose array is `["bilibili"]`, stated. `events.platform` is
the first announced platform, or `unknown`, for readers of the single
value. Startup adds the columns without rewriting the array: `platform` is
null on rows written before it existed, and the startup that adds it sets
`youtube` on the YouTube writers' events. A row whose array is null reads as
`["youtube"]`, stated, when its `platform` is `youtube`, and as no platform
otherwise.

Extraction writes the announced platforms with the other fields of an
automatic entry, under the same manual locks and tombstones (see
[Source time zones](../sources/time-zones.md#source-time-zones)); the YouTube writers
write `["youtube"]` and the shop step `["other"]`, both stated. Entries of
one post that name the same activity (the same date, time and title, as a
simulcast listed once per platform) are one event: it keeps the first
entry's fields and every entry's platforms in listed order, inferred only
when all of them were. An operator's entry carries the operator's platforms,
which are part of its manual lock.

`events.url` is the announcement's link (a post, an operator's link, a
paired upload, a watch link), never the live room, because readers use the
link as the identity of one broadcast. A stored row whose `url` is a
live-room link (`live.bilibili.com`) and whose platforms do not include
`other` reads as announced on `bilibili` with no `url`; the row is not
rewritten.

Readers compute an entry's **shown platforms**: its announced platforms,
those the automatic entries merged into it lend, and `bilibili` when the
live room's broadcast is the simulcast of its YouTube broadcast (observed;
see [Live room](../sources/bilibili.md#live-room)). Only merges that join platforms
lend: Rule D, Rule E and an operator's merge; Rules A, B and C join
duplicates of one entry. Nothing observed is written. Each shown platform
has a link only to a page on that platform: YouTube's is the watch link (the
entry's `url` when it names a YouTube video, or that of a YouTube event an
operator merged into it), Bilibili's the account's live room for a `stream`
or `member` entry once a live check learned the room (or the stored room
link of an older row) and a release's Bilibili upload page, and `other`'s
the entry's `url`. A platform without such a page has a null link: the
announcement (a post) stays the entry's own `url`, never labelled as a
platform. The entry's `liveStatus` is `live` while any of its platforms is:
its own video's state, then a merged YouTube event's, then the room's.
Public events report the single value as `platform` and the shown platforms
as `platforms`; full reads add `links`, and `liveUrl`, the room, to every
`stream` or `member` entry shown on Bilibili (see
[Public API](../public-api/snapshot.md)). The dashboard's
`liveNow` reports a live broadcast that matches no entry; no entry is
created for it.
