# Unreleased

<details>
<summary><strong>Public pages</strong></summary>

- The Posts page opens on the posts the browser showed last while it checks
  the archive for changes, reads the newest posts alongside its month index,
  and going back to it from another page returns to where you were reading.

</details>

<details>
<summary><strong>Runtime and development</strong></summary>

- The server keeps the post archive's month index and pages it has prepared
  until the dashboard revision changes, so repeated reads skip SQLite.

</details>
