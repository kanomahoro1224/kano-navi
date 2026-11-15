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
- Weekly schedule, Recent Posts, Recent streams and works, Links, and the
  month, Past posts and Past videos sheets show their titles without a small
  label above them; the weekly schedule shows the week's dates beside its
  title. The board's post and video
  filters show no counts. The public API still returns the counts.

</details>

<details>
<summary><strong>About</strong></summary>

- The apps sit side by side under the title: Kano Navi for Apple (iOS,
  iPadOS and macOS, linked to its repository) and Kano Fans’ Treasure
  (Android, HarmonyOS, Windows and iOS). Unsigned platforms are marked. The
  mainland China server site is no longer listed as an app; its credit stays.
- The apps, credits, technologies and notice each sit under a small label
  instead of a numbered chapter.
- Credits, technologies and the notice are cards. The credits' wording
  links the people and the LINE BLOG recovery project. Contributors show their GitHub name with their
  login.
- The AI models show three and open the rest from "+3".

</details>
