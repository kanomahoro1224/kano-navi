# ADR-0005: Video Form From Platform Facts, Content From Tags

- Status: Accepted
- Date: 2026-11-02

## Context

A video carried one `kind` that mixed two questions: how the video was
published (a broadcast, a premiere, an upload) and what it is about (a game,
a talk, a song). Every listing derived it from the title with a fixed regular
expression list, and the detail check then replaced it: a broadcast became
`LIVE ARCHIVE` or `UPCOMING LIVE` whatever it was about, and only uploads and
premieres kept the title's guess. Bilibili had its own list in another order.

The result was wrong in both directions. A gameplay stream lost its content
once checked, so the archive's category filter had to read the calendar label
of the video's schedule entry instead. An upload whose title said `配信` or
`live` was shown as a stream, and a cover titled `歌ってみた` matched no
YouTube rule. The board kept two vocabularies (`MUSIC` and `song`, `GAMEPLAY`
and `game`) and mapped between them in several places.

The schedule entries of YouTube reservations already carry a content label
(`events.tag`), decided by the operator, the extraction prompt or a model
reading the title. Most broadcasts and premieres have such an entry.

## Decision

A video keeps three independent layers, each with one owner.

### Form: `kind`

`videos.kind` is one of `stream`, `premiere`, `upload` or `unknown`, decided
only from what YouTube reports:

| Facts                                                         | Kind       |
| ------------------------------------------------------------- | ---------- |
| A detail check found a broadcast start, premiere verdict true | `premiere` |
| A detail check found a broadcast start, otherwise             | `stream`   |
| A detail check found no broadcast start                       | `upload`   |
| Only a listing named the video                                | `unknown`  |

An ended broadcast whose answer cannot tell a premiere from a stream keeps
the stored `stream` or `premiere` and is `unknown` without one. A Bilibili
upload is `upload`; one merged
into a YouTube video is shown through that video, so it follows the YouTube
kind. No title rule decides the form any more; both `Classify` functions are
removed.

The operator's `manual_kind` corrects the form with the same three codes
(`unknown` is not a correction).

### State

Upcoming, live, ended and cancelled stay where they are: `is_upcoming`,
`live_status`, `scheduled_at`, `ends_at` and the operator-confirmed
cancellation of the matching entry. A stream archive is a `stream` whose
broadcast ended; a premiere uses the same state. The kind never encodes a
state, so the `UPCOMING LIVE` to `LIVE ARCHIVE` conversion and the board's
special case for a past `UPCOMING LIVE` disappear.

### Shorts

`is_short` and `manual_is_short` stay a separate three-state format flag. A
Short is not a kind.

### Content: tag

A video's tag is `game`, `talk` or `song`; a video without one is `other` in
the archive filter. The schedule labels `live` and `video` name a form and
never reach a video. The shown tag is the first of:

1. the operator's video tag (`videos.manual_tag`);
2. the content label of the video's schedule entry, read at query time with
   the existing match (the entry's own row, its source item or its event
   sources; a merge target's content label first, while a form label there
   does not hide the entry's own), so the entry's own order (manual, then
   prompt or merged notice, then title inference) holds;
3. the video's inferred tag (`videos.tag`, with `tag_source` and
   `tag_attempted_at`).

The schedule label is never copied into the video row, so an operator's edit
or a released merge on the entry shows at once without a second write. The
operator's video tag ranks above the operator's entry label because it is
the more specific correction.

The calendar reads the same layers in the other direction, so an entry and
its video agree on content. The label an entry shows is the operator's entry
label when there is one; otherwise the first of an operator's tag on one of
its videos, the entry's content label, a video's inferred tag and the
entry's form label (see
[Shown label](../architecture/data-model.md#shown-label)). An operator who
tags a video in the archive therefore relabels its calendar entry too, and a
notice that only says "stream" does not hide a game the title shows. The
derivation runs on every public event read; it looks each entry and video
up by ID, which costs microseconds per entry, so nothing is materialized.

### Inference for videos

The translate step's title inference also labels videos that the schedule
cannot: public videos with no manual tag and no judgement yet, none of whose
matching entries (or the entries those are merged into) carries a content
label, a title judgement or an operator's label, or is an unjudged
automatic reservation still inside the entry inference window. An entry
whose only label is a form from a notice, or an operator's row without a
label, does not hold the video back, since the calendar reads on to the
video's content tag there. Bilibili uploads are not public, so they are not
asked about. It uses the same prompt, schema,
model queue and retry policy. The prompt names covers, originals and music
videos as `song`, so uploads are labelled as well as premieres. A `game`,
`talk` or `song` answer is stored; `live`, `video`, an unknown label or none
is stored as a judgement without a tag, so the title is not sent again; a
failed call only stamps `tag_attempted_at`. The entries are labelled first and the videos use
what is left of the same per-run limit, newest unattempted first, so the
existing backlog drains over successive runs.

### Upgrade

The schema upgrade adds `manual_tag`, `tag`, `tag_source` and
`tag_attempted_at` to `videos` and rewrites stored values once, touching only
rows still in the old shape:

- `manual_kind` `GAMEPLAY`, `TALK` and `MUSIC` become the manual tag `game`,
  `talk` and `song` and clear the form correction; `LIVE ARCHIVE` and
  `UPCOMING LIVE` become `stream` and `VIDEO` becomes `upload`.
- `kind` is derived again from the stored facts: a start with a premiere
  verdict decides directly; a start without a verdict is a `premiere` when
  the old kind was `MUSIC` or `VIDEO` and a `stream` otherwise. The old check
  gave title kinds to scheduled videos only when they were premieres, but a
  row without a verdict may come from an older check, and `GAMEPLAY` or
  `TALK` on a scheduled row is far more often a stream than a premiere, so
  only the song and plain-video kinds count as premiere evidence. No start
  is an `upload` once checked and `unknown` otherwise; every Bilibili row is
  an `upload`.

The old title kinds of uploads are not turned into inferred tags: they are
the regular expression guesses this decision replaces, and the inference
backlog labels them instead.

## Consequences

- The board's Streams/Works split reads the form alone, and the archive's
  category filter reads the tag alone. Neither guesses from the other.
- An upload that only a listing has named is `unknown` until its detail
  check; the board shows it as a video and an operator's refresh request
  settles it early.
- Content labels of uploads depend on a configured translation queue. Until
  the backlog is judged, those uploads count as `other`.
- Public video projections gain `tag`. `kind` values change, so the frontend,
  the Atom feed category and the admin editor move to the new codes in the
  same release.
