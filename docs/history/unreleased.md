# Unreleased

<details>
<summary><strong>Public pages</strong></summary>

- A calendar entry can carry a label (`live`, `game`, `talk`, `song` or
  `video`) in its `tag` field, so a client can show what kind of activity an
  entry is instead of guessing from its title. The label belongs to the event,
  so a notice without a video link can carry one too.

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
  activity. YouTube reservations never pass through that prompt, so the
  translation queue also labels them from their title, and a title the model
  cannot classify is recorded rather than asked again. A rule merge carries an
  X notice's label onto the event that survives.

</details>
