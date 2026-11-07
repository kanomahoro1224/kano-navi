# ADR-0008: Extraction Normalizes Source Time Zones in Code

- Status: Accepted
- Date: 2026-11-07

## Context

The board stores and shows schedules in Asia/Tokyo. Kano's X posts are
written for Japanese readers in Japan time, so the extraction prompt asks
the model for Asia/Tokyo dates and times. Her Bilibili dynamics are written
for Chinese viewers in Beijing time (UTC+8): 「今天北京时间17点上传」 means
18:00 in Japan, and a late-night 23:30 falls on the next Japan date. Asking
the model to convert would make every Bilibili time depend on its arithmetic,
which models get wrong on day rollovers, and no stored field would show what
the post actually said.

## Decision

The model returns each entry's date and times as written, with the zone
they are written in: an optional `timeZone` of `Asia/Tokyo` or
`Asia/Shanghai`, defaulting to the source's zone (Japan for X, Beijing for
Bilibili). Code converts the date and times to Asia/Tokyo with fixed
offsets, including the rollover into the next day. Relative dates (今天,
明天) resolve from the publication time given in the post's zone.

Each source has its own prompt and schema: X keeps its prompt and schema
unchanged, so its extraction identities and event IDs stay the same, and
Bilibili adds `timeZone` and the Chinese wording. A zone outside the two is
an invalid entry.

## Consequences

- Converting is deterministic and tested; a new source in another zone adds
  a profile and a zone, not prompt arithmetic.
- Stored events stay in Asia/Tokyo, so readers, merges and feeds are
  unchanged.
- The translation step keeps times as written ("17:00 Beijing time"), since
  converting belongs to extraction.

## Related Docs

- [Sources: Source time zones](../architecture/sources.md#source-time-zones)
