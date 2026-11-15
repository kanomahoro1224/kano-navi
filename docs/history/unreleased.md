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
- The footer drops its About link and shows Notes and Back to top as icons.
- On a wide screen the common tags sit under the links, beside the journey
  card.
- Clicking the month or year arrows quickly in the month, Past posts and
  Past videos sheets no longer selects the text beside them.
- A schedule entry's details open its link from the arrow in the bottom
  corner, as the entry's card does, instead of an Open link button on a row
  of its own.
- A release's details on Works keep their width as you switch editions: a
  pane whose contents start or stop scrolling keeps its width.

</details>

<details>
<summary><strong>About</strong></summary>

- The apps sit side by side under the title: Kano Fans’ Treasure (Android,
  HarmonyOS 5+, Windows and iOS 13+) and Kano Navi for Apple (iOS, iPadOS
  and macOS 26+, linked to its repository). Unsigned platforms are marked. The
  mainland China server site is no longer listed as an app; its credit stays.
- The apps, credits, technologies and notice each sit under a small label
  instead of a numbered chapter.
- Credits, technologies and the notice are cards. The credits' wording
  links the people and the LINE BLOG recovery project. Contributors show their GitHub name with their
  login. The contributor card has no link to the full
  contributor list.
- The AI models show three and open the rest from "+3".

</details>
