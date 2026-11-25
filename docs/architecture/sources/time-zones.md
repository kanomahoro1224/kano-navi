# Time Zones

Which time zone each source's times are read in.

## Time-zone rationale

The extraction model returns the date and time as written by the source, and code performs the fixed conversion to Asia/Tokyo. This keeps day rollovers deterministic and preserves the wording used for translation, while all stored schedule instants retain one display zone for readers, merging, and feeds.

## Source time zones

A Bilibili dynamic gets its own prompt: a Chinese dynamic written for
Chinese viewers, with times in Beijing time unless it says Japan time. The
model gives each entry's date and times as written and names their zone in
`timeZone` (`Asia/Tokyo` or `Asia/Shanghai`, or an alias such as `CST`,
`UTC+8`, `北京时间`, `JST` or `UTC+9`); code converts them to
Asia/Tokyo, including the day rollover (Beijing 23:30 is 00:30 the next day
in Japan), so the model does no time arithmetic
([source time zones](#source-time-zones)). An entry
without a known `timeZone` is in the source's zone, never an invalid
entry. X posts keep the Japan prompt and schema, and their entries are always
Asia/Tokyo whatever `timeZone` a model adds.

Events of a Bilibili dynamic have IDs `bilibili-schedule-<date>-<hash>`,
`events.source` and `event_sources.source` `bilibili`, and the dynamic's opus
page as the `event_sources` link. Extraction results and post states of a
dynamic are stored with source `bilibili` under its post ID.

Every entry, of either source, has its [announced
platforms](../data-model/events.md#event-platforms) from `youtube`, `bilibili` and
`other` (another platform, or someone else's channel or room such as a
collaboration), in announced order. An entry's **wording** is its title,
detail and cited evidence, and the post's text when the post has this one
entry. **Bilibili-only wording** calls it Bilibili-only (bilibili限定,
B站限定, B限 as its own word rather than the end of WEB限定, ビリビリ限定)
without also naming YouTube.

The Bilibili schema asks for a `platforms` array of the platforms the post
names for the entry, both for a simulcast, and none for a plain 直播; a
value outside the enum is dropped, never an invalid entry. Code decides
whether Bilibili alone is stated: `["bilibili"]` from the model or a room
link is stated only for Bilibili-only wording, and so is a stream or
release that names none with such wording; otherwise a dynamic's
`["bilibili"]`, and a stream or members-only stream naming none, is
inferred, the source's default.

The X schema keeps its single `platform` (`unknown` when the post does not
say, and for an invalid value), and code derives the set, always stated: a
stream or release of a post with Bilibili-only wording is `bilibili` when
the model named none, and else the model's platform holds. A stream or
release whose wording announces a simulcast (同時配信, 同时直播, 同步直播,
simulcast, or YouTube and Bilibili both named together with 同時) adds the
other of `youtube` and `bilibili` after the model's platform (`youtube`
first when the model named none), so a notice whose model named Bilibili
still meets the dynamic's entry by Rule D; naming both sites without 同時
is no simulcast, and an `other` entry, a collaboration elsewhere, stays
`other`. On either source a YouTube link adds `youtube`. The model gives a link only when the post
does: an entry keeps the post's link, or the model's explicit one. A
live-room link (`live.bilibili.com`) never stays as an entry's link, since
reads add the room (see [Live room](bilibili.md#live-room)): it makes the entry
`bilibili` (on X in place of its single platform) and the entry keeps the
post's link, unless the entry is `other`, a collaboration in someone else's
room, which keeps that link and gets no live state.

Extraction identities name neither prompt nor schema, so a schema change
of one source extracts no stored post again: an entry keeps its platforms
until its post's content changes or an operator asks for its extraction.

Reconciliation links a dynamic's release to the upload it announced and
merges an X notice of a Bilibili stream into the dynamic's entry (see
[Duplicate merging](../data-model/duplicate-merging.md)).
