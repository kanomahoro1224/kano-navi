# Video Modeling

How a video's form and tags are modelled.

## Modeling rationale

Events, video form, video content, and sale periods use separate fields because they answer different reader questions. A stream or premiere is a publication form; a game, talk, or song is content; a sale is a period with two endpoints. Keeping those dimensions independent lets calendar labels and archive filters agree without guessing from titles or overwriting operator corrections.

## Video Form and Tags

A video has three independent layers, each with one owner
([video form and tags](#video-form-and-tags)).

**Form.** `kind` is `stream`, `premiere`, `upload` or `unknown`, written by
synchronization from platform facts only:

- A YouTube detail check (watch page or Data API) that found a broadcast
  start writes `premiere` when the premiere verdict says so and `stream`
  otherwise; one that found no broadcast start writes `upload`. An ended
  broadcast whose answer cannot tell (an unclassified Data API answer) keeps
  a stored `stream` or `premiere` and is `unknown` without one.
- A listing (feed, uploads playlist, uploads tab, yt-dlp metadata, history
  window) inserts `unknown`, and a listing never replaces a stored row's
  kind; the detail check settles it.
- A Bilibili upload is `upload` or `stream` (a live replay), settled as
  [Bilibili form](#bilibili-form) describes. The list read inserts `upload`
  for a new row and never replaces a stored row's kind.

`manual_kind` corrects the form with `stream`, `premiere` or `upload`, and a
Bilibili upload's with `stream` or `upload`.
Readers show `COALESCE(manual_kind, kind)`.

**State.** Upcoming, live, ended and cancelled are not part of the kind:
`is_upcoming`, `live_status`, `scheduled_at`, the linked entry's `ends_at` and
its operator-confirmed cancellation say where a stream or premiere stands. A
stream archive is a `stream` whose broadcast ended.

**Content.** A video's tag is `game`, `talk` or `song`. Readers derive the
shown tag on each read as the first of:

1. `manual_tag`, the operator's video tag;
2. the [label](events.md#event-labels) of the video's schedule entry (its
   `youtube-<id>` event, an event whose `source_item_id` it is, or one with a
   YouTube `event_sources` row for it; deleted entries are skipped and the
   label of the entry it is merged into comes first), when that label is
   `game`, `talk` or `song`;
3. `tag`, the title inference's tag of the video itself.

The entry label is never copied into the video row, so an edit, merge or
release on the entry shows at once. `live` and `video` labels name a form and
never reach a video. A video without a tag is `other` in the archive's
category filter.

`tag`, `tag_source` and `tag_attempted_at` follow the entry inference's rules
(see [Event labels](events.md#event-labels)): the `translate` queue asks about the
videos public readers list (see [Video merging](videos.md#video-merging)), a Bilibili
upload only while nobody marked it private or removed, each not merged into
another and without a manual tag or judgement,
whose entries, if any, carry neither a content label, a title judgement nor
an operator's label, are not merged into such an entry, and are not
automatic reservations still within the entry inference window. An
operator's row that never had a label (`tag_source` empty) reads on to the
video's tag, so it does not hold the video back; a cleared label
(`tag_source = manual`) does. Only `game`, `talk` or `song` is stored; another
answer or none is stored as `tag_source = title` without a tag and is not
asked again until a creator's rename clears that judgement, whether the
recent synchronization or a history page sees the new title first (a stored
tag, or an operator's `manual_title`, keeps it); a failure only stamps
`tag_attempted_at` and waits a day, as an entry does. Videos
never attempted come first, then the least recently attempted, each newest
first. Storing a tag advances the dashboard revision; a judgement without
one or a failed attempt does not. A Bilibili upload whose form is unsettled
is asked for form and tag together instead (see [Bilibili form](#bilibili-form)),
after the entries and before the other videos, from the same per-run limit.

### Bilibili Form

Bilibili's unsigned endpoints carry no live-replay flag, so a Bilibili
upload's form is settled by the first of:

1. the operator's `manual_kind`;
2. a rule, applied by the `translate` step before any model call to every
   upload whose form neither the operator nor a rule settled. A rule that
   applies only later, once the broadcast it reads is known, replaces a form
   the model settled meanwhile:
   - a title naming a replay (`直播回放`, `录播`, `直播录像`, `回放`) is a
     `stream`;
   - an upload of at least thirty minutes published within 48 hours after a
     Bilibili broadcast ended, at least half that broadcast's length, is a
     `stream`. A broadcast is a `stream` entry shown on Bilibili (its
     platforms, or an entry merged into it) that is neither deleted, merged
     nor cancelled by an operator, ending at its `ends_at` or else its
     YouTube archive's start plus length; or one of the live room's last two
     broadcasts, the last one ending at the last check that saw it live;
   - an upload shorter than ten minutes is an `upload`;
3. the title inference: the `translate` queue asks about each unmerged
   upload nobody settled or marked private or removed, with its title,
   length and the first 200 characters of its description, for the form
   (`upload`, `stream` or `uncertain`) and the content tag together.

`kind_source` records who settled the stored `kind`: `rule` or `llm`, `NULL`
while unsettled; an operator's `manual_kind` shows as `manual`. An
`uncertain` answer sets `kind_review` to `uncertain`; a failed call counts
`kind_attempts` and stamps `kind_attempted_at`, waits a day and on the third
failure sets `kind_review` to `failed`. An upload with a `kind_review` and no
settled form waits in the Review queue for the operator and is not asked
again; a rule that later settles it clears the review. Settling the form of
an unmerged upload advances the dashboard revision. Startup adds the columns
to existing snapshots; existing Bilibili rows start unsettled and the next
`translate` run settles them.

A Bilibili upload has no premieres, Shorts or waiting room: `is_short`,
`is_upcoming`, `live_status` and `availability` stay empty, and readers treat
a `stream` as a stream archive.

Startup rewrites a row that still carries an older title kind, once:
`manual_kind` `GAMEPLAY`, `TALK` and `MUSIC` become `manual_tag` `game`,
`talk` and `song` and clear the form correction, `LIVE ARCHIVE` and
`UPCOMING LIVE` become `stream` and `VIDEO` becomes `upload`. `kind` is
derived from the stored facts: a start with a premiere verdict decides; a
start without one is a `premiere` for an old `MUSIC` or `VIDEO` kind and a
`stream` otherwise; a row without a start is an `upload` once
`live_checked_at` records a detail check and `unknown` otherwise; a Bilibili
row is an `upload`. A row already in the new shape is untouched.
