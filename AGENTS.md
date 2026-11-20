# Agent Guide

Unofficial status board for Kano Mahoro (鹿乃まほろ). The Go service in
`backend/` serves the React frontend in `src/`; TypeScript is the frontend and
the maintenance scripts only.

## Where To Read Next

Before changing files under a path, read its guide, then open only the doc
pages it names. Architecture docs are short indexes of topic pages; open the
page a change needs, not every page.

- `backend/` (API, auth, MCP, schema, sources, workflows, media):
  [backend/AGENTS.md](backend/AGENTS.md)
- `src/` (public pages, operator console): [src/AGENTS.md](src/AGENTS.md)
- `scripts/` (packaged collections, exports, CI helpers):
  [scripts/AGENTS.md](scripts/AGENTS.md)
- `docs/`, `README.md`, any Markdown: [docs/AGENTS.md](docs/AGENTS.md)
- `docker/`, the image, deployment: [Docker](docs/operations/docker.md)
- Merging into `main` or preparing a release: the `merge-to-main` and
  `release` skills in `.claude/skills/`

## Product Invariants

- The browser reads a prepared SQLite snapshot through the local API. Only
  server-side workflows and their helpers read X, YouTube, Bilibili, the shop,
  or model providers; page loads, refreshes and revision requests never do. A
  failed source keeps its known snapshots and records its outcome.
- Manual event edits, confirmations and deletions create durable locks or
  tombstones. Automatic sync and extraction never replace or resurrect them.
- Public HTTP and MCP responses use sanitized projections. Raw errors, source
  item IDs, counters, job IDs, `raw_json`, secrets and absolute paths stay in
  their protected boundaries.
- Secrets and source credentials stay in the environment. A new outbound path
  follows the
  [external request rules](docs/development/security.md#external-request-rules).
- Production is the default: only explicit `APP_MODE=development` bypasses
  admin authentication, and `/admin` stays unlinked.
- Never silently drop columns or clear snapshots. Store parseable ISO
  timestamps and display Asia/Tokyo times.
- English is the base locale and the fallback. Sync and extraction store codes
  or English text, never localized display strings.

## Validation

The `Makefile` is canonical for local work and GitHub Actions; run the
smallest sufficient target from
[Validation targets](docs/development/testing/validation.md).

- Quick feedback: `make ci-fast`; docs: `make docs-check`; style, tooling and
  workflows: `make ci-style`
- Backend, data, API, sync: `make ci-backend`; UI: `make ci-frontend`
- Seeded snapshot and API: `make smoke`; image and runtime: `make docker-build`
  then `make production-smoke`
- Releases, major refactors, dependency or CI infrastructure changes: `make ci`

Begin with `git status --short` and keep existing user changes; never use a
destructive reset or checkout. Smoke uses disposable state; never run live
synchronization to validate. Tests protect concrete behaviors at the lowest
sufficient layer with synthetic records, reserved domains and synthetic URLs.

## Commits and Handoff

- Commit, amend, rebase, merge, push or rewrite history only when the user
  explicitly asks. Follow the
  [commit convention](docs/development/commit-and-release.md#commit-convention).
- Before a routine commit run `make ci-fast`, the affected-area target and
  `make sensitive-check`, then review the findings and `git diff --cached`.
  Stage only the requested scope. Before handoff, run proportional validation
  and `make sensitive-check`.
- Register a new public host with a reason in `scripts/privacy-allowlist.json`.
  Fix the source of a scanner finding; never hide it.
- Never commit `.env`, `.npmrc`, credentials, personal paths, logs, databases
  or WAL, runtime media, or unrelated changes. Ignore rules are not a secrecy
  boundary.
- Never invent identifiers: versions come from `package.json`; issue and PR
  numbers and release tags come from the repository or the user.
- When behavior changes, update its docs in the same change, following
  [docs/AGENTS.md](docs/AGENTS.md).
