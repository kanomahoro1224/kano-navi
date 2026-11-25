# Testing and CI

The backend is Go. `make go-test` runs its contracts and vet without Docker.
`make ci-backend` runs golangci-lint with `scripts/golangci-lint.yml`, vet,
and the contracts again under the race detector (`make go-race`, which needs
cgo and a C compiler). `make go-check` validates what those leave out:
formatting (`make go-format-check`), the contracts without cgo and the
Linux/Windows builds (`make go-build-check`), and disposable
read-only/development containers (`make go-container-check`). Source tests, including the historical-window API adapter, use
fake transports and synthetic credentials; no check synchronizes live
sources. See [Backend](../architecture/backend.md).

GitHub Actions runs the lint and vet checks once, then runs the race suite as
four parallel `backend-race-shard` jobs. Each shard lists the same Go tests and
uses recorded race-detector durations to assign each test to one shard. The
local `make ci-backend` target still runs the complete unsharded suite. Use
`make RACE_SHARD=1/4 backend-race-shard` to exercise one slice locally, and
`make RACE_RUN=<run id> backend-race-durations` to refresh
`scripts/go-test-durations.json` from the artifacts of a successful CI run;
the latter requires an authenticated GitHub CLI.

`make test-frontend` runs the frontend tests in `tests/`: the dashboard and
admin request hooks, the month archive reads, polling, the pure view and
filter helpers, `robots.txt`, the board's preload in `index.html` and its
own copy, the history catalog and packaging, and the works packaging and
views.

The Makefile is the canonical validation entry point. GitHub Actions invokes
its targets so local and hosted checks protect the same contracts. Use
`make ci-fast` for ordinary local feedback, then add the smallest affected-area
target when the change needs compilation or tests. Aggregate checks are for
broad changes, CI/tooling work, releases, and pre-commit validation at those
boundaries.

- [Validation Targets](testing/validation.md): open when you pick or change a Make target.
- [Test Scope](testing/test-scope.md): open when you add or change tests.
- [GitHub Actions](testing/github-actions.md): open when you change workflows, the CI plan, or the gate.

## Related Docs

- [Local development](local-dev.md)
- [Design](design.md)
- [Secure development](security.md)
- [Commit and release](commit-and-release.md)
