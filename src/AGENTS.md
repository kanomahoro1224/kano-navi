# Frontend Agent Guide

The root [AGENTS.md](../AGENTS.md) invariants apply. The
[Frontend](../docs/architecture/frontend.md) index lists its topic pages; open
only the page a change needs.

## Rules

- `src/main.tsx` composes `/`, `/posts`, `/journey`, `/works`, `/lineblog`,
  `/about` and `/admin`. Only the request modules call `fetch`: the page
  data hooks (`src/dashboard/use-dashboard.ts` for the board and posts,
  `src/admin/use-admin-data.ts` for the console's endpoints), the console's
  `src/admin/api.ts` transport, and the once-per-load readers
  (`src/lib/visitor-region.ts`, `src/works/use-works-uploads.ts`). New board
  request logic may live in a sibling module in `src/dashboard/` that
  `use-dashboard.ts` composes; components never request.
- Format displayed times through `src/lib/tokyo-time.ts`, which owns the
  Japan time zone and the shared `Intl.DateTimeFormat` builders.
- Style markup in its area's stylesheet, with that area's class names over
  the CSS variables in `src/index.css`, not with Tailwind utilities. Reuse
  `.round-button` and `.pill-button` (`src/site/site.css`) on public pages,
  the `src/admin/ui.tsx` components in the console, and the dropdown menu,
  tabs and tooltip wrappers in `src/components/ui/`.
- Follow [Design and interaction](../docs/development/design.md) and check
  affected UI in a browser at desktop and mobile widths.
- Avoid redundant helper labels and explanatory captions; let the layout,
  grouping and visual hierarchy explain themselves.
- English is the base locale; the frontend translates stored codes (see
  [Stored event text](../docs/architecture/data-model/events.md#stored-event-text)).
- The `media-catalog.json` files in `src/history/`, `src/lineblog/` and
  `src/works/` are packaged collections; follow
  [scripts/AGENTS.md](../scripts/AGENTS.md) before changing them.
- When visitor-visible behavior changes, update the matching
  [User guide](../docs/user/index.md) page.

## Which Page To Open

- Routing or a public page's presentation:
  [Presentation](../docs/architecture/frontend/presentation.md)
- App composition, page loading, locales, translations, media paths:
  [Request and UI ownership](../docs/architecture/frontend/request-ownership.md)
- The posts page and post cards:
  [Posts and post cards](../docs/architecture/frontend/posts.md)
- Console schedule, videos, review:
  [Console schedule](../docs/architecture/frontend/console-schedule.md)
- Console overview, queue, sources, posts, data views:
  [Console sources](../docs/architecture/frontend/console-sources.md)
- Snapshot requests, polling, recovery:
  [Snapshot state](../docs/architecture/frontend/snapshot-state.md)
- Response shapes: [Public API](../docs/architecture/public-api.md),
  [Admin API](../docs/architecture/admin-api.md)
- Tests: [Test scope](../docs/development/testing/test-scope.md)
