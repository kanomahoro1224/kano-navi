# Design and Interaction

The public board should be easy to scan and keep source provenance visible.
Use the shared semantic tokens in src/index.css, the public shell and controls
in src/site/, and the existing shadcn/ui-style primitives in
src/components/ui/. Public pages use cream paper with hairline rules rather
than boxing everything, a rounded display face for names and titles, tabular
numerals for times, and strawberry gingham as the signature edge. Admin keeps
its own adm- styles on the same tokens. Preserve light/dark themes and the
English (base), Japanese, and Simplified Chinese interface copy. Do not
introduce a separate token system for a single page.

## Page Boundaries

- / is the public snapshot board. Refresh reads the local API and preserves known
  state when a request fails.
- /history is static, sourced content with local fixed images and original links.
- /works is the static discography in an editorial treatment: on top of the
  shared shell it gives the shared tokens a warm ivory and clay reading in
  both themes, sets names and figures in a serif, separates with hairlines
  and outlined chips, and lets each cover's record slide out of its sleeve
  (a photo of the real disc where there is one; where no disc was pressed,
  the cover printed on a disc with a clear hub). The header sets the
  figures cut from the acoustic cover trilogy's covers in a small landscape,
  which in one column sits between the title and the lead:
  each album's grooved disc sinks behind two far ridges like a setting sun,
  and the figures stand and sit on curved grassy ground with flowers in the
  three albums' colours, the land fading out at its edges. The scene has two
  sides, album order on side A and two leading on side B; a small two-sided
  record at its top right turns over to flip them while one and two sink and rise in
  their new places and the ground and flowers morph between the sides'
  curves, and this device remembers the side. It falls back to
  the trilogy's sleeves when a cutout is missing. A cover shelf of Kano's own releases by year opens
  a sheet for each release: a header with the sleeve over a blurred wash of
  the cover, then the tracklist, with a round photo beside each disc that has
  one, beside the edition's details. The section tabs carry their counts and
  no separate figures or notes; a tab chosen by pointer keeps no focus
  outline, while keyboard focus draws one the tab bar leaves room for. A
  closed sheet returns focus only to the control that opened it, never to the
  section panel a tap in Safari focuses. On phones the English title fits one
  line, the filter chips scroll sideways in one row, and a cover's record
  rises only while a finger is on it. The works she
  is featured on are a list; her uploads are a grid of 16:9 covers (on phones
  a list of rows, thumbnail beside words), with the site marks as small icons
  at the end of the date line and a
  printed sleeve in one of the palette's tones where no thumbnail is saved.
  The order is a single icon that ends the toolbar's row; search and
  narrowing toggles join that row on a wide screen and take a second row
  otherwise, and search shows focus by thickening its clay underline rather
  than boxing the field. Rows rise
  in as they scroll into view where the browser ties animation to scrolling,
  and motion stops under reduced-motion settings. Codes get interface copy;
  titles and credits keep their wording and language.
- /lineblog is the static LINE BLOG archive. It reproduces the archived blog's
  own layout (its header art, white menu bar with its Home icon, article column beside a
  profile sidebar, its font stack and accent colour) inside the shared
  header and footer, and on phones the blog's smartphone template (white
  boxes on a grey page); its surfaces and text still come from the shared
  tokens, so both themes apply.
- /about is a static project, attribution, and code-license page.
- /admin is the hidden operator console. Its styles use adm- prefixed selectors.
  Do not add it to public navigation. Lazy page loads show a localized status
  and offer a full reload on failure, using the shared theme tokens.

Keep loading, empty, stale, error, and unavailable-media states explicit. Never
replace an unready local media URL with a remote CDN URL. Preserve event date
precision and show Asia/Tokyo times; do not invent a start time for date-only
records. Distinguish automatic, manually confirmed, suspected cancellation, and
operator-confirmed cancellation states.

## Interaction Review

Check desktop and mobile layouts on the affected pages. Preserve semantic
buttons, labels, keyboard navigation, visible focus, and dialog Escape/focus
behavior. Open images through the shared image viewer and give each opener the
`media-opener` class, so every page shows the same cursor and closes the same
way. Use status color to communicate state, with text or an icon available
as another cue. Keep implementation details out of visitor-facing copy.

## Related Docs

- [User guide](../user/index.md)
- [Frontend ownership](../architecture/frontend.md)
- [History archive maintenance](history-archive.md)
- [Testing](testing.md)
