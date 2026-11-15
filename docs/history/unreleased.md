# Unreleased

<details>
<summary><strong>Upgrade notes</strong></summary>

- The public API only adds a read: `GET /api/posts` without `from` and `to`
  pages through every shown post, up to an optional `until` date and
  optionally for one `account`, instead of answering `400 invalid_range`.
  Range reads are unchanged.

</details>

<details>
<summary><strong>Public pages</strong></summary>

- Recent Posts show a lone image whole in its own shape, a very tall or very
  wide one on a blurred copy of itself, and count images past four on the
  fourth tile. A card's images and quote follow its wording instead of
  sinking to its foot.
- A quoted post shows its author's portrait when the board has one, and on
  the board its first image beside its wording with a count of the rest.
  Past posts and the full post show a quote's images like a post's.
- A post whose portrait fails to load shows its account's initial.

</details>
