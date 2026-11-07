# Unreleased

> [!NOTE]
> The new Works page shows its covers from `data/works/`. Run
> `npm run works:package -- --from /path/to/kano_official` on the instance
> to package them, then `npm run variants` (or a copies run from the
> console) to make their smaller copies; until packaged, the page shows each
> cover as unavailable. The Journey page's release milestones show the same
> covers, so they are unavailable there too until packaged. The Journey
> page's own images are packaged the same way into `data/journey/`: run
> `npm run history:package -- --from /path/to/kano_official` before
> `npm run variants`. The variants command no longer takes `-static`. The
> uploads' video thumbnails need no packaging: the media step saves them over
> its next runs.

<details>
<summary><strong>Public pages</strong></summary>

- A Works page lists Kano's discography, led by the acoustic cover trilogy
  one, two and three: her own releases as a cover shelf with each edition's
  covers, tracklist with photos of its discs, package contents and store
  bonuses and links from album tracks to the same recording's uploads, the works she is featured on with
  the tracks she sang or wrote, and her niconico, YouTube (members-only
  included) and bilibili uploads as a cover grid with filters and search.
  Each instance saves the uploads' video thumbnails itself; an upload the
  site knows shows its length and whether it can still be watched, a disc
  beside an upload's title lists the albums that later included it, and the
  list names the date it covers.
- A Works release's sheet and the Works and Journey image viewers keep one
  size whatever the release or image; on a wide screen the release's cover
  and figures stay on the left while its tracklist and details scroll beside
  them, in columns of their own on the widest screens. Each edition, such as
  初回限定盤 or 通常盤, is a card with its own cover and format.
- The Journey page's address is `/journey`; `/history` links redirect to it.
  Its Chinese name is 足迹.
- Journey's release milestones show their release's covers from the Works
  page, each linking to the release there; the doujin album one is dated
  December 29, 2017, and the ルル MV August 13, 2026.
- Journey shows both BOOTH artworks of the Hanayori anniversary acrylic
  stand, dated August 1, 2020, a frame of the SONA teaser video for the
  teaser milestone, the head and shoulders of the full-length 2021 singer
  illustration in its timeline thumbnail, the Pixio ambassador illustration
  in its gallery, the full-size SONA joining merchandise image, and a
  milestone for the March 29, 2020 VR concert 「ReBirth」 with its rehearsal
  photo.

</details>
