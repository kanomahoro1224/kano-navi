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
| Dependency advisories and npm registry signatures      | make audit            | Locked installation, Go and network      |

`make ci-fast` runs only formatting, documentation-link and sensitive-information
checks; it does not reinstall dependencies, run tests, or start Docker. Use it
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
Local tool directories (.claude and .codegraph) are excluded from formatting;
this does not widen the privacy scanner's exclusions.
