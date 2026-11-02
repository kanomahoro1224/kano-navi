# Unreleased

> [!NOTE]
> The extraction prompt asks for a calendar label, which changes the extractor
> version. After upgrading, every known X post within the schedule lookback is
> extracted again on the following `schedule` runs, so model calls rise for a
> while until the backlog clears. Operator edits, locks and tombstones are kept.
> Startup adds `events.tag`, `events.tag_source` and `events.tag_attempted_at`
> to an existing database; no manual migration is needed. The admin event
> list (`GET /api/admin/events`) reports `tagSource` as `manual`, `llm` or
> `title`, and the `reconcile` result adds a `tagged` count.

<details>
<summary><strong>Public pages</strong></summary>

- A calendar entry can carry a label (`live`, `game`, `talk`, `song` or
  `video`) in its `tag` field, so a client can show what kind of activity an
  entry is instead of guessing from its title. The label belongs to the event,
  so a notice without a video link can carry one too.
- Past videos shows four columns of covers on wide screens, three on narrow
  ones and two on phones. A month long enough to scroll keeps the same columns
  as a short one.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- The schedule editor sets an entry's label, and an empty choice clears it. A
  saved label is the operator's decision, outranks the automatic one and keeps
  the entry from being relabelled later.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- The extraction prompt may return a label when the post or image states the
  activity. An unknown label is dropped without losing the entry, and a post
  that states no label keeps the label already on the entry.
- YouTube reservations never pass through that prompt, so the translation
  queue also labels them from their title (`tagSource = title`), with the
  queue's retries and provider status. A title the model cannot classify is
  recorded rather than asked again; a title whose request fails counts as
  failed, leaves the run going and is asked again after the titles not yet
  tried, so an outage marks nothing as decided.
- A rule merge lends an X notice's label to the event that survives, including
  merges made before the notice had a label, and replaces a label guessed from
  the title. Operator and locked entries keep their own label.

</details>
