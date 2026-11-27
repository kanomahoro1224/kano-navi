# Scripts Agent Guide

The root [AGENTS.md](../AGENTS.md) invariants apply. Scripts are TypeScript
maintenance tools and CI helpers; the service itself is Go.

## Packaged Collections

Journey images (`data/journey/`), LINE BLOG archive media (`data/lineblog/`)
and Works covers (`data/works/`) are ignored runtime data, packaged from local
reference or recovery collections and listed in the tracked
`media-catalog.json` of `src/history/`, `src/lineblog/` and `src/works/`.
Preserve original bytes, catalog hashes, credits and provenance when changing
them.

- Journey: `package-history-media.ts`, `export-journey-api.ts`,
  `backend/internal/journey/`; see
  [History archive maintenance](../docs/development/history-archive.md)
- LINE BLOG: `package-lineblog.ts`, `export-lineblog.ts`,
  `backend/internal/lineblog/`; see
  [LINE BLOG archive maintenance](../docs/development/lineblog-archive.md)
- Works: `package-works.ts`, `export-works-api.ts`, `backend/internal/works/`;
  see [Works collection maintenance](../docs/development/works-archive.md)

Their API contracts are in [Public API](../docs/architecture/public-api.md).

## CI Helpers

- `ci-plan.ts` selects Actions jobs from changed paths; see
  [GitHub Actions](../docs/development/testing/github-actions.md).
- `privacy-allowlist.json` registers public hosts with a reason; see
  [Sensitive-information scan](../docs/development/security.md#sensitive-information-scan).
- `check-patterns.ts` holds request owners and file sizes to
  `pattern-baseline.json`, which only `make pattern-baseline` lowers; see
  [Pattern ratchet](../docs/development/testing/validation.md#pattern-ratchet).
