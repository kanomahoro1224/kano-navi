# Validation Targets

The Make targets for each kind of change and how formatting stays portable.

| Change or task                                         | Target                | Requirements                             |
| ------------------------------------------------------ | --------------------- | ---------------------------------------- |
| Fast local format, docs and privacy feedback           | make ci-fast          | Installed Node.js and Go                 |
| Documentation links                                    | make docs-check       | Node.js                                  |
| Formatting, docs, workflows, CI plan, scanner, privacy | make ci-style         | Locked dependencies and Go               |
| SQLite, API, sync, media or extraction                 | make ci-backend       | Go and a C compiler                      |
| TypeScript frontend, scripts, tests, and config        | make typecheck        | Locked dependencies                      |
| Public or admin UI                                     | make ci-frontend      | Locked dependencies                      |
| Seed and HTTP snapshot integration                     | make smoke            | Node.js and Go                           |
| Production image                                       | make docker-build     | Docker with Linux containers             |
| Production entry point, pages and guards               | make production-smoke | The locally built DOCKER_IMAGE           |
| All checks without Docker or installation              | make check            | Locked dependencies, Go and a C compiler |
| Full locally portable Actions validation               | make ci-local         | Locked dependencies and Docker           |
| Install then full validation                           | make ci               | Node/npm, Go and Docker                  |
| Manual privacy review before handoff or commit         | make sensitive-check  | Node and Git                             |
| Request owners and file sizes against the baseline     | make pattern-check    | Node and Git                             |
| Dependency advisories and npm registry signatures      | make audit            | Locked installation, Go and network      |

`make ci-fast` runs only formatting, documentation-link, sensitive-information
and [pattern ratchet](#pattern-ratchet) checks; it does not reinstall
dependencies, run tests, or start Docker. Use it
for small edits, then add `make typecheck`, `make go-test` or `make test-frontend`
when the affected code needs that check.

make ci-style runs make go-format-check, the gofmt step of make go-check on LF
copies of the Go sources, so make check catches Go formatting without Docker.
ci-local runs make go-check after make check, matching the hosted Go builds
and Go container jobs.
make install uses npm ci. Narrow targets and ci-local do not reinstall.
make ci installs first, verifies the registry signatures of the fresh
installation with make npm-audit-signatures, then runs ci-local. Both full targets enforce image
build before production smoke even under make -j. Override NPM, NODE, DOCKER,
DOCKER_BUILD, DOCKER_BUILD_ARGS, or DOCKER_IMAGE for the corresponding toolchain.
GOLANGCI_LINT_VERSION, ACTIONLINT_VERSION and GOVULNCHECK_VERSION pin the Go
tools that `go run` builds; Dependabot does not update them.
DOCKER is an executable path/name for the smoke helper, not a shell command.
check-docs, check-sensitive, and privacy-check are aliases of docs-check and
sensitive-check.
npm run check is the Node-only formatting/test/privacy/docs/build sequence; use
make check to include the Go tests and API smoke as well.

## Cross-platform Formatting

Prettier accepts the file's existing LF/CRLF style through endOfLine=auto,
so a Windows checkout does not require rewriting otherwise unchanged source.
Local tool state (.claude apart from its skills and the shared settings.json,
and .codegraph) is excluded from formatting; this does not widen the privacy
scanner's exclusions.

## Pattern Ratchet

`scripts/check-patterns.ts` (make pattern-check, in ci-fast and ci-style)
turns two code-shape rules into a check that fails only when a change adds an
exception. It reads the files Git can see, like the
[sensitive-information scan](../security.md#sensitive-information-scan), and
its data is `scripts/pattern-baseline.json`.

- Request owners: only the files under `requests.allowed` call the global
  `fetch` in `src/`, tests aside (see
  [Request and UI Ownership](../../architecture/frontend/request-ownership.md)).
  A new owner needs a reviewed entry with a reason.
- File size: a `.ts`, `.tsx`, `.go` or `.mjs` file over `fileSize.limit`
  (800) lines fails unless the baseline records it. A recorded file may grow
  by `fileSize.slack` (20) lines past its count and fails beyond that. Test
  files are code and are baselined like any other file. Message dictionaries
  with no logic, such as `src/admin-i18n.ts`, are listed under
  `fileSize.exempt` with a reason; other i18n tables stay under the limit.

A finding names its fix: route the request through an owner, or split the
file into cohesive modules. Never raise a recorded count or add an exception
to pass. When files shrink, the check prints notes and still passes;
`make pattern-baseline` (`--update-baseline`) then lowers the recorded counts
and drops files under the limit, owners without requests and deleted paths.
It never adds an entry or raises a count, so a renamed oversized file moves
its entry by hand in a reviewed change. `make test-patterns` covers the rules
with synthetic fixtures.
