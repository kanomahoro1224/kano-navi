# Documentation Agent Guide

The root [AGENTS.md](../AGENTS.md) invariants apply. These rules cover
`docs/`, `README.md`, `backend/README.md` and `.github/SECURITY.md`.

## Where Each Topic Belongs

- Features as visitors see them belong in the [User guide](user/index.md) and
  `README.md`: no file paths, endpoints, schema or implementation detail. When
  visitor-visible behavior changes, update the matching user page.
- How the system works belongs in [How it works](how-it-works.md); module, API
  and data contracts belong in [Architecture](architecture/index.md), one
  topic per document. Add a new document to its index rather than growing an
  unrelated one.
- Runtime setup and recovery belong in [Operations](operations/index.md).
- Local workflow, checks, content maintenance and contribution rules belong in
  [Development](development/index.md).
- Security reporting belongs in [SECURITY.md](../.github/SECURITY.md); technical
  boundaries belong in [Deployment security](operations/security.md) and
  [Secure development](development/security.md).
- Design reasons and current rules belong in the relevant architecture,
  operations or development document.
- Release notes, pending changes and per-release upgrade steps belong only in
  [History](history/index.md) (pending changes and upgrade steps in
  `history/unreleased.md`); follow
  [Release notes](development/commit-and-release.md#release-notes).
- [.env.example](../.env.example) is the complete runtime variable inventory;
  [Configuration](operations/configuration.md) documents each variable.

## Writing Rules

Docs describe the current system and its current decisions, not how it got
there.

- Write in present tense. Do not narrate change: no "previously", "no longer",
  "now", "as before", "was removed", "retired", "migration phase",
  version-dated asides, PR, issue or commit references, or before/after
  comparisons.
- When behavior changes, rewrite the affected section as if it had always been
  this way instead of appending a paragraph. Search for and remove every
  statement the change contradicts, including operations and development
  pages.
- Compatibility that still runs is current behavior. State it as a rule ("an
  `events` table without event sources is rebuilt on startup"), not as a
  story.
- Reference docs contain no TODOs, plans, roadmaps or dated measurement logs.
- State each rule or fact in one place and link to it instead of restating it.
- Documentation is English only, except the translated root READMEs
  (`README.zh-CN.md` and `README.ja.md`), which mirror `README.md`. Use
  repository-relative paths, synthetic examples and empty credential fields.

## Topic Pages

A large document is a short index (`<section>/<topic>.md`) of topic pages in
`<section>/<topic>/`, each under about 15K characters. Keep heading text
stable, since links use its anchors. Put new content on the page that owns its
topic; when a document or page grows past about 20K characters, split it the
same way and list each page in the index with one "open when" line. Run
`make docs-check` after changing links or headings.
