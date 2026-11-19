# Unreleased

<details>
<summary><strong>Upgrade notes</strong></summary>

- `GET /api/posts/months` adds `accounts` (`x`, `bilibili`) and
  `accountAvatars`.
- New command `kano-go x-archive` (`import`, `check`, `apply`) imports X
  account exports and reviews the posts an earlier import added; see
  Database operations, X Account Exports. A first import of an account
  outside `X_HANDLES` adds the app setting `x_archive_accounts`.

</details>

<details>
<summary><strong>Public pages</strong></summary>

- Posts get a page of their own, `/posts`, in the site menu: every stored
  post in one timeline by month and day, with a card per account and a map
  of every year's months to jump through, in an address that names the
  month and account. The board's **Past posts** opens it in place of the
  month dialog.
- The post archive offers MKLNtic, the unit account Kano shares, as an
  account of its own.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- X account exports fill in Kano's own posts with their wording, links,
  quotes and photos, and hide the replies and other people's posts an
  earlier import filed as hers; a slow local check sorts the hidden posts
  and shows her own thread replies again.

</details>
