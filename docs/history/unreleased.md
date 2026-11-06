# Unreleased

> [!NOTE]
> Model calls rise for a while after upgrading. The extraction prompt
> defines every event type and asks for sale periods, which changes the
> extractor version, so every stored X post that the detection settings
> select is extracted again, the ones processed longest ago first. Startup
> adds the empty `events.ends_on` column without rewriting events. Public
> events and MCP schedule reads add `endsOn`, and the event type `sale`
> joins the admin API's types.

<details>
<summary><strong>Public pages</strong></summary>

- Pages keep the same width when a scrollbar appears or disappears: a page
  that grows or shrinks, an image viewer or sheet, or the language and
  calendar menus no longer shift the layout sideways on desktops with
  space-taking scrollbars. The operator console keeps its width the same way.
- Merchandise orders, pre-orders, ticket sales and lotteries are their own
  Sale entries on the schedule, shown on the day they open and the day they
  close (or on the deadline alone) with an On sale status. They never take
  the next-up ticket, and the calendar subscription lists their opening and
  closing ([ADR-0006](../decisions/ADR-0006-sale-periods.md)). Reminders of
  a sale, such as a last-day post, fold into the announced period instead of
  adding another entry. An On sale row below the next-up ticket lists up to
  three sales on sale now or opening within a week, with their close or
  opening time.
- On phones the Now section is shorter, so the On sale row and the schedule
  come up sooner. The profile card leads the section as a compact identity
  row with a short banner strip and a small avatar beside the name; it
  leaves out the counts, and its last row holds the X, YouTube and
  bilibili read times, the run's state dot and the reload button. The
  countdown sits with the ticket's time, the latest video card puts its
  labels above the thumbnail, and the On sale cards scroll sideways in one
  row. On wider screens the profile counts open the schedule, post wall and
  video shelf.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- The extraction prompt defines each event type and returns one entry per
  sale period with its opening and its close, instead of separate entries
  of mixed types for the opening and the deadline.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- The schedule editor offers the Sale type, with a close date and time in
  place of the length.

</details>
