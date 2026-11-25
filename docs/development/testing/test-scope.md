# Test Scope

What tests protect and at which layer.

Tests should guard an observable contract, state transition, security boundary,
or previous regression. Choose the lowest sufficient layer. Go tests live
beside their source as `*_test.go`, frontend tests live in `tests/`, and
scanner tests live under `scripts/`. Do not add tests for prose or a purely
visual change without a documented interaction or responsive contract. Do not
mirror internal implementation.

Backend coverage, by package under `backend/internal/`:

- `store/sqlite`: dashboard and calendar projections and their bounds over a
  10,000-record synthetic history, stream identity, manual locks and
  tombstones, read-only opening, startup upgrades of older snapshots (including
  the legacy events rebuild and selected profile repair), source writes that
  keep cursors on rollback, and history pages that commit with their checkpoint.
  Retry tests cover restart/rediscovery backoff, missing-file recovery, retained
  ready files and 304 resets; freshness tests exclude historical and cursor-only
  writes and retain timeline head-page times across resumed pages. Admin post
  tests cover ID search before pagination and inclusive Japan-date boundaries.
- `httpapi`: public reads, media containment and validators, compression and
  precompressed siblings and static and fixed-media caching, the admin session
  and origin guard, event mutations, and the operator console routes for
  workflows, jobs and manual source fetches.
- `workflow` and `jobs`: single-flight reservation, leases, retry, recovery,
  cancellation, timers, and the source, media and history steps; each job
  kind's steps and history label, and run listings that match each run's
  detail.
- `publicview`: the public projections over records whose operator-only
  columns all carry a marker, which no projection may contain.
- `source/x`, `source/youtube`, `source/history`, `mediadownload`: adapters
  against fake transports, covering budgets, pending work, account isolation,
  oversized and redirected responses, controlled error codes and resumed pages.
- `schedule` and `llmadmin`: extraction and normalization against recorded
  reference vectors (`schedule/testdata/*.json`), protocol-specific provider
  requests, routing and failover, provider validation, and secret handling.
- `mcpapi`, `adminauth`, `contentadmin`, `secretstore`: the public and
  control tool boundaries, login and rate limits, profile media and schedule
  image review, and the stored secret format.

Use manually authored synthetic records, example.invalid URLs, fixed neutral
timestamps, and temporary files. `testutil.NewSnapshot` creates an empty
database from `internal/store/sqlite/bootstrap.sql`, so records written by a
test must satisfy the real columns, defaults and constraints; a test that needs
an older layout alters that database explicitly. Fakes of a store interface
reject what the real store rejects. Never derive fixtures from operator databases,
provider responses, credentials, local paths, or logs. Do not call live X,
YouTube, image CDN, or model APIs. Dependency installation and image builds may
access package registries; that is separate from platform-data synchronization.

For UI changes, run the build and review the affected pages at desktop and
mobile widths, including loading, empty, error, stale, and dialog states.
For API/schema changes, run the backend tests and smoke. The smoke helper
builds the Go backend into a temporary directory, runs `kano-go seed` twice
against a temporary SQLite file, starts `kano-go serve` on a loopback port,
checks fresh snapshot reads, revision polling, unready media and
unauthenticated admin access, then removes the fixture. The backend runs from
that temporary directory with an explicit synthetic environment, so it never
opens the operator database or reads .env. Initialization may create
the standard empty cache directories; it does not download media.

`tests/dashboard-requests.test.ts` loads the actual hook through Vite without
environment files and uses fake time and controlled responses to exercise
recovery, stalled JSON deadlines, deduplication, races, visibility and
unmount. Component and hook tests render with React DOM into a happy-dom
window from `tests/dom.ts`, which loads no scripts, styles or pages, and drive
views with DOM events; browser review covers the production bundle in real
browsers. Together with `tests/archive-requests.test.ts`,
it checks that same-revision live-state expiry refreshes the loaded calendar
and archive pages atomically, queues a refresh behind pagination, and retains
known pages on failure or cancellation. `tests/polling.test.ts` uses controlled
timers to check retry cadence, its cap, deadlines, overlapping-work
prevention, stale responses and disposal. `tests/latest-wins.test.ts` checks
that LLM queue saves keep one request in flight per queue, send only the
latest waiting order and drop edits made on top of a refused save.
`tests/history-media.test.ts` checks
the history catalog, the shipped image bytes and packaging validation.
`tests/lineblog.test.ts` packages a synthetic LINE BLOG archive and checks the
tracked article and media catalogs and the page's query views.
`tests/works.test.ts` packages a synthetic works collection and checks its
tables, code mappings, wording rules, cover validation and the page's sort,
search and link helpers. `tests/journey-api.test.ts` and
`tests/works-api.test.ts` rebuild the Journey and Works API bodies from the
tracked page data and fail when the committed backend copies differ.
No timing thresholds enter CI.

To review the real hooks and responsive UI against a backend, start an
isolated instance: run `kano-go serve` from a temporary directory with
`APP_MODE=development`, a synthetic `LLM_SECRETS_KEY`,
`WORKFLOW_SCHEDULER_ENABLED=0` and a temporary `-database`, and point Vite at
it with `KANO_API_PORT`. Do not exercise a development build against the
operator's `data/` directory. The synthetic 10,000-record benchmark is
`go test ./internal/store/sqlite -run '^$' -bench BenchmarkDashboardSynthetic10k`
in `backend/`; see [Performance measurements](../performance.md).

Production smoke creates a disposable container from the built image, uses
synthetic production authentication, disables the source timers, and publishes a
random loopback-only port. Its anonymous data volume is removed in cleanup.
It validates the image's `kano-go serve` entry point, fresh SQLite, public API,
SPA page routes, the protected admin API, and the CA bundle that outbound
HTTPS needs. It does not mount host data or pass host secrets.
This HTTP check does not claim to verify browser interaction.
