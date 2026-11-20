# Board Projection and Views

The board projection and the event and video views every reader shares.

## Board Projection

The dashboard carries a locale-neutral `projection` for shared board
decisions. Its values are codes, booleans and record IDs, never localized
copy. Every ID names a record in the same response; the browser joins the
returned records by these IDs and does not infer their order from localized
text.

| Projection field   | Value                                                                                        |
| ------------------ | -------------------------------------------------------------------------------------------- |
| `version`          | Contract version; `1` is the current one                                                     |
| `nextTransitionAt` | Next ISO instant that can change a time-dependent decision, or `null` when none is known     |
| `focus.eventIds`   | Ordered event candidates of the focus                                                        |
| `focus.videoIds`   | Ordered video candidates of the focus, waiting rooms without a start last                    |
| `sales`            | Ordered IDs of every sale event the board's on-sale list shows, in place or in its sheet     |
| `spotlight`        | The [spotlight](#spotlight), or `null` when there is neither a next entry nor a latest video |
| `shelf`            | The [video shelf](#video-shelf)                                                              |

### Spotlight

The spotlight is the stage's one ticket: the next or current entry, or the
latest video when nothing is next.

| Spotlight field    | Value                                                                                                                                                                                               |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `mode`             | `next` for a current or upcoming entry; `latest` when the latest video is the fallback                                                                                                              |
| `eventId`          | The shown event, `summary.nextEvent`, or `null`                                                                                                                                                     |
| `videoId`          | The shown video: `summary.nextStream` in `next` mode, `summary.latestVideo` in `latest` mode, or `null`                                                                                             |
| `liveNow`          | `true` when the shown entry is the dashboard's `liveNow` broadcast, which has neither an event nor a video                                                                                          |
| `isLive`           | `true` when the shown entry is confirmed on air or its start time has passed                                                                                                                        |
| `membersChip`      | `true` when the shown event is a `member` entry paired with its video, the video is not members-only, private or removed, and a `game`, `talk` or `song` label takes the type's place on the ticket |
| `companionVideoId` | `summary.latestVideo`, shown beside the ticket when the shown entry has no video thumbnail and is not that video; otherwise `null`                                                                  |

`liveNow` says which entry is shown; `isLive` says whether it is on air, for
an event or a video as well. An event and a video are both set when the next
event and the next stream share a stream link. In `latest` mode only
`videoId` is set, and the booleans are `false`.

### Video Shelf

The shelf lists IDs from the returned `videos`. The lead is in none of the
four lists.

| Shelf field      | Value                                                                                                                                                    |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `leadId`         | The Featured video; otherwise the newest work that is a premiere or a song, then the newest work, neither private nor removed; `null` when there is none |
| `leadIsFeatured` | `true` when `leadId` is the operator's Featured video                                                                                                    |
| `reservations`   | Videos that are not past, and cancelled ones whose start is still ahead, earliest first                                                                  |
| `works`          | Past uploads and premieres that are not Shorts, newest first                                                                                             |
| `archives`       | Past streams that are not Shorts, newest first                                                                                                           |
| `shorts`         | Past Shorts, newest first                                                                                                                                |

## Event and Video Views

Every event and video carries a `view` of codes and booleans, never
localized copy. Calendar, archive and MCP `schedule_list` reads carry the
same event and video `view` objects as the dashboard, computed from the same
inputs: an event's source and the broadcast state of its linked video or
room. Clients keep an
additive fallback for responses without `projection` or `view` so a retained
snapshot remains readable while a deployment serves mixed versions.

| Event view field  | Value                                                                                                     |
| ----------------- | --------------------------------------------------------------------------------------------------------- |
| `state`           | One of the seven states below                                                                             |
| `typeCode`        | `stream`, `member`, `event`, `release`, `video` or `sale`, or `null` when the stored type is outside them |
| `activityCode`    | `live`, `game`, `talk`, `song` or `video`, or `null`                                                      |
| `activityContext` | `stream` or `work` when `activityCode` is set, otherwise the empty string                                 |
| `isWork`          | `true` only for the `release` and `video` types                                                           |
| `calendarEntries` | The entry's calendar occurrences, sale-expanded                                                           |

`state` is `upcoming` when the entry has not started, `live` when a linked
broadcast is on air, `open` when a sale period is open, `done` when a
non-pending entry or sale has ended, `pending` when the time or status still
needs confirmation, `cancelled` when the entry is cancelled, and
`cancellation_review` when cancellation evidence is awaiting confirmation.

An ordinary event has one calendar entry. A sale period has its opening and
its closing entry; a sale deadline has only its closing entry.

| Calendar entry field | Value                                                                |
| -------------------- | -------------------------------------------------------------------- |
| `saleMark`           | `null` for an ordinary event; `opens` or `closes` for a sale's end   |
| `startsOn`           | Japan date (`YYYY-MM-DD`) the occurrence is shown on                 |
| `startsAt`           | ISO instant of the occurrence, or `null` when only the date is known |
| `timePrecision`      | The stored `exact`, `approximate` or `unknown`                       |

| Video view field | Value                                                                                                      |
| ---------------- | ---------------------------------------------------------------------------------------------------------- |
| `phase`          | `upcoming`, `live` (confirmed on air), `started` (start passed, not confirmed live), `past` or `cancelled` |
| `formCode`       | `short`, `upcoming`, `stream`, `archive`, `premiere` or `video`, or `null` when no public form applies     |
| `shelfGroup`     | `reservation`, `work`, `archive` or `short`, or the empty string for a video omitted from the shelf        |
| `latestEligible` | Whether the video may be the stage's `latestVideo` fallback                                                |
| `focusEligible`  | Whether the video may be a focus candidate                                                                 |
| `tagContext`     | `stream` or `work`: whether a content tag is worded for a stream or a work                                 |
