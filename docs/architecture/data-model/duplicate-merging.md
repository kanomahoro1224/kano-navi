# Duplicate Merging

How duplicate events are merged.

The additive `events` columns `merged_into` (target event ID), `merge_source`,
`merge_reason` and `merged_at` record that an event duplicates another. They
are a separate durable decision: the X upsert and the YouTube writer never
name them, so re-extraction, retirement and relisting of the source row, or a
reservation refresh, keep the merge. `merge_source` names the owner:

- `rule`: the deterministic `reconcile` workflow step
  (`backend/internal/store/sqlite/event_merge.go`). It recomputes its own
  decisions from the current rows on every run, so it is idempotent and
  releases a merge whose target was deleted, merged or moved to another
  date, or whose event became manual or locked or now holds another owner's
  merge. An event holding another owner's merge is never merged away, so no
  merge points at a merged event. Deleting an event, or retiring it
  automatically when its last source evidence goes, releases the rule merges
  into it in the same transaction, so a duplicate older than the
  reconciler's window is shown again.
- `manual`: an operator decision (`merge_reason` `manual`), which the rules
  never change. `merge_source = manual` with a null `merged_into` is an undone
  merge; the reconciler never merges that event again. Deleting the target
  undoes an operator merge into it (`merge_reason` `unmerged_from:<id>`), so
  no entry stays hidden behind a tombstone.
- Any other owner is left untouched by the rules in the same way, and its
  merges stay when their target is deleted.

The rules consider active, non-deleted events from 30 days before to 60 days
after the current Japan date:

- Rule B: an automatic X `stream`, `member` or `release` with an exact or
  approximate time, or an X `event` (an entry that names no activity, such as
  「このあと20時からはじまります！」) with an exact time, merges into the only
  compatible YouTube event on the same date that starts within 30 minutes. A
  generic stream may merge into a members-only stream, not the reverse; a
  release only into a release; an `event` into a stream or members-only
  stream. The stored time precision stays unchanged.
- Rule A (after Rule B): a date-only automatic `stream`/`member` event merges
  into the timed stream on the same date whose normalized title overlaps its
  own (one contains the other or they share a token of at least two
  characters after punctuation, brackets and medium words such as 配信 are
  removed). Without a title overlap it merges only when the date has exactly
  one date-only event and one timed stream, and not into a stream of platform
  `bilibili` unless the date-only entry is `bilibili` as well. An entry
  stated Bilibili-only (see [Event platforms](events.md#event-platforms)) never
  merges into a stream whose own platforms lack `bilibili`. A YouTube event
  never merges into an X or Bilibili event.

- Rule B also moves an automatic entry of a Bilibili dynamic whose platform
  is `youtube` (such as 「今晚8点YouTube直播」), under the same constraints. A
  `stream` or `member` entry whose platforms include both `youtube` and
  `bilibili` is never a Rule B notice; Rule E moves it.
- Rule D (after Rule B): an automatic X `stream`, `member` or `release`
  whose platforms include `bilibili`, with an exact or approximate time,
  merges into the only
  compatible entry of a Bilibili dynamic (`source` `bilibili`) on the same
  date that starts within 30 minutes; a release only into a release. The
  dynamic is the platform's first-party post, as the YouTube event is for
  Rule B, and an X entry of platform `bilibili` is never a Rule B notice.
  An X notice of a simulcast whose model named Bilibili keeps `bilibili`
  first and merges here, then follows the dynamic's entry (below).
  Without a dynamics listing the X entry stands alone and still gets the
  room as `liveUrl`.

- Rule E (after Rule D): an automatic `stream` or `member` entry, of any
  source but YouTube, whose announced platforms are `bilibili` by a
  source's default (inferred) or include both `youtube` and `bilibili`
  merges into the only compatible YouTube `stream` or `member` event on the
  same date that starts within 30 minutes, as in Rule B
  (`merge_reason` `rule_e:simulcast`): it is the Bilibili side of a
  simulcast, and the YouTube event's shown platforms include `bilibili`
  (see [Event platforms](events.md#event-platforms)). An entry stated Bilibili-only,
  such as 「B站限定」, and a release never merge by this rule.

After every rule, an entry whose target is itself merged away goes on to
the final target, whichever rules made the two merges (an X notice Rule D
merged into a dynamic's entry that Rule E moves ends on the YouTube event),
so no merge points at a merged event.

- Rule C: an automatic `sale` merges into another sale of the same shop
  whose [period](events.md#sale-periods) covers its own, such as a deadline reminder
  into the announced period. Two sales are of the same shop when their links
  name the same page (host without `www`, path without a trailing slash, and
  query; a link to an X post names no shop), or, when either has no shop
  link, when their titles overlap as in Rule A once the words and counts
  every sale title uses (such as 受注, 受付, 締切, グッズ, 本日まで or 残り2日)
  and Kano's own name (鹿乃まほろ, its parts 鹿乃 and まほろ, and its
  romanization) are removed as well, so a title naming no goods or event
  matches no other sale. A period covers another that
  opens no earlier and closes no later, and a sale whose close is not known
  when its opening date falls inside; a deadline is covered only by a period
  that closes on its date, or by the same deadline. Of two sales covering
  each other the one that states more (a known opening, a close date, a start
  time, a close time) stays, then the smaller ID. Among several covering
  sales the one no other covers is the target; several leave the sale
  unmerged. A sale counts in the window by its close date as well as its
  opening date, and its `merge_reason` is `rule_c:sale_period`. Because the
  target covers the merged sale, the surviving entry shows the whole period.
- Several plausible targets leave the event unmerged and are counted as
  `ambiguous`. A cancellation overlay (`llm_suspected` or `manual_confirmed`)
  on either side freezes the current decision.

Before the rules run, the same step derives YouTube events from the stored
videos whose start falls in that window, so the YouTube side of a match
exists and carries its time whichever source was read first:

- An automatic, unlocked YouTube event (`source = youtube`, the video's
  reservation identity) takes its start from its video with `exact`
  precision: the reservation's start, or the publication time of an upload or
  Short. An X post that links a video writes that event with only the post's
  date or announced time. An end before the new start is cleared.
- An ended broadcast with a start but without any event or evidence row for
  its video gets its reservation event, as the YouTube writer would write it
  (see [YouTube](../sources/youtube.md)). A broadcast without a premiere verdict
  counts as a stream only when it lasted at least 30 minutes; a shorter one
  gets no event.

Manual locks and tombstones keep their rows, and no video gets a second
event identity.

Every public reader excludes merged rows, and the events
[hidden by their posts](posts.md#events-of-hidden-posts): the dashboard events,
totals, upcoming counts and summary/spotlight candidates, calendar items,
totals, pagination, day counts and adjacent dates, and MCP `schedule_list`. The
operator console lists them with their target and can undo a merge. Startup
adds the columns and the partial `events_merged_into_idx` index to existing
snapshots without rewriting events.

An entry of a Bilibili dynamic never merges into an X entry, by any rule; it
is otherwise an X entry for Rules A and C. A shop entry is an automatic sale
like any other for Rule C, so a post's entry of the same sale merges into it
or receives it, by link or title as the rule says.

## Bilibili releases

Before merging, `reconcile` recomputes the pairing of the automatic,
unlocked `release` entries of Bilibili dynamics in its window with the
account's Bilibili uploads. A release and an upload pair when each is the
other's only match within 30 minutes: the entry's link becomes the upload's
page and an active `bilibili-video` event source (`source_key` `upload`)
names it. A pairing that no longer holds (the release moved, or a second
release or upload made it ambiguous) is released first: its source row is
retired and the link goes back to the dynamic's. An upload another entry
still holds is not paired: a locked or manual entry keeps its upload row
whatever its state, and so does any other entry that is not deleted and
whose row is active, so a locked entry's row is never rewritten. An
unchanged run writes nothing; an
extraction that rewrites the link restores the dynamic's link until the next
run pairs it again. The upload source is evidence of the work, not an
announcement, so it keeps no entry alive that its dynamic no longer lists.
