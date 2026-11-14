# Unreleased

<details>
<summary><strong>Upgrade notes</strong></summary>

- The public API only adds a read: `GET /api/posts` without `from` and `to`
  pages through every shown post, up to an optional `until` date and
  optionally for one `account`, instead of answering `400 invalid_range`.
  Range reads are unchanged.

</details>
