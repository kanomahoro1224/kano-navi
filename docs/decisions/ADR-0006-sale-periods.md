# ADR-0006: Sale Periods as One Entry Shown at Its Ends

- Status: Accepted
- Date: 2026-11-07

## Context

Kano's posts announce merchandise pre-orders, made-to-order windows, ticket
sales and lotteries as often as streams. They name a concrete date, so the
extraction rightly treats them as schedule notices, but the event types had
no place for them. The model put the opening of a window under `release` and
its deadline under `event`, with no consistent rule, and the stored entry had
one date and an end time on that date, so a period such as 8/22–9/22 became
two unrelated entries or lost one of its ends. Several posts about the same
window (the opening, a reminder, the last day) each added their own entries.

A window that runs for weeks also does not fit the board's other readers. A
schedule entry on every day of the window would fill the month grid, which
shows three entries a day, and an open window would hold the stage's next
event for its whole length.

## Decision

A sale is its own event type, `sale`: a period to buy, pre-order, order or
apply for goods, media or tickets, a ticket lottery included. `release` stays
the date a work becomes available. Every other type keeps its meaning.

One period is one entry. `starts_on` and `starts_at` are its opening and the
additive `ends_on` and `ends_at` its close. A deadline whose opening is not
known stores the deadline as both dates and no start time. Only a sale has
`ends_on`; the extraction ignores a close date on any other type, and the
admin API rejects one.

Readers show a sale at its ends, not across its days: the opening on its
date and the close on its date, or the close alone for a deadline. The
calendar reads, day counts and adjacent dates include an entry on its close
date, and the iCalendar feed writes an opening and a closing entry. A sale
has its own status, from its period: upcoming before it opens, open until it
closes, ended after. It is never the next event; instead the board lists
the sales on sale now or opening within a week in their own row below the
stage, so a window lasting weeks stays visible without holding the stage.

The extraction prompt defines every event type in one sentence and asks for
one entry per sale period. The changed prompt and schema change the
extractor version, so stored posts are extracted again.

## Consequences

- Old sale entries are replaced as their posts are extracted again: an
  entry the new result no longer lists is retired with its evidence, as for
  any re-extraction. Manual and locked entries stay as the operator left
  them.
- Separate posts about the same window give separate entries, for example a
  reminder's deadline beside the announced period. The merge reconciler
  hides an entry behind the sale of the same shop link (or, without one, a
  title overlapping in what names the goods or event) whose period covers
  it. A merge only into a covering
  period keeps the surviving entry complete, so readers need no combined
  period; an opening and a deadline that no single entry covers both stay,
  and still read as the two ends of the sale.

## Related Docs

- [Data model: sale periods](../architecture/data-model.md#sale-periods)
- [Sources: schedule extraction](../architecture/sources.md)
- [User guide: schedule](../user/schedule.md)
