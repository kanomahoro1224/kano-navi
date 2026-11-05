# Unreleased

<details>
<summary><strong>Public pages</strong></summary>

- The weekly schedule folds past entries to their time and title, so a busy
  day that has passed no longer stretches the week, and entries no longer
  carry an Ended status; a folded entry still opens its full details.
- The footer is a single compact line: it drops the decorative mark and the
  page links the header already holds, keeps About and Back to top, and folds
  each page's notes behind a Notes switch.
- Past posts jumps to any month: the month name opens a year-by-year grid that
  shows each month's post count for the chosen account and dims months
  without posts. A new public read, `/api/posts/months`, serves the month
  index.
- Past videos is one scrolling list of every month instead of a month at a
  time: sticky month headers carry each month's count, older videos load as
  the list nears its end, the month name follows the scroll and jumps to any
  month, and Latest returns to the newest. The category counts cover every
  month. `/api/videos` without a date range pages through all videos, up to
  an optional `until` date, and `/api/videos/months` serves the video month
  index.

</details>
