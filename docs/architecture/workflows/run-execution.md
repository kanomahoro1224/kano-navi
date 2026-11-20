# Run Execution

How runs execute in the single-flight queue and which packages own them.

Every run is a durable `workflow_runs` row with its ordered
`workflow_run_steps` and one `workflow_step_attempts` row per try; the
console's job list and the `sync_runs` history are projections of them (see
[Backend](../backend.md#jobs-and-workflows) for the reserved run IDs). A run
freezes what its steps read: `fetch_windows_json` for operator windows,
`source_sync_json` for schedule sync settings, `source_chain_json` for their
method chains and `source_lookback_json` for per-step days. Retry and recovery therefore repeat the same work without a
saved definition.

Statuses are fixed enums. Runs use `queued`, `running`, `succeeded`,
`partial`, `failed`, `cancelled` and `blocked`; steps use the same set with
`pending` in place of `queued`; attempts use `running` and the terminal
values. `trigger_source` and `execution_source` are `operator`, `scheduler`
or `recovery`. Idempotency keys are stored as SHA-256 hashes. Admin
projections omit raw payloads, stack traces, paths, credentials and provider
text.

Each step records the adapter that serves it in `adapter_capability`
(`x_recent`, `x_related`, `youtube_recent`, `bilibili_recent`, `bilibili_dynamics`,
`youtube_live`,
`media_download`, `schedule_extract`, `text_translate`, `media_variants` or
`event_reconcile`), which projections read before the constrained
`capability` column (`synthetic`, `x_recent` or `capability_not_migrated`).
A step without an adapter finishes `blocked` with `capability_not_migrated`
and never counts as successful work.

**Lease.** Claiming a queued run takes the SQLite write lock, sets it
`running` and writes the single `workflow_leases` row (scope `global`) with
an owner, a generation and an expiry two minutes ahead; a heartbeat renews
it every 40 seconds. Every step, attempt and run write, and every source
transaction, checks owner, generation and expiry in the same transaction,
so a worker that lost its lease cannot commit. A renewal that finds the
lease lost cancels the run's outbound requests; a renewal that fails to write,
such as on a busy database, is left to the next one, and the renewal after an
expiry finds the lease lost. A partial unique index allows at most one
`queued` or `running` run, so a second start answers `409` with `busy` and the
active run. A dispatched run has at most 20 minutes. A run stopped by that
limit, or by a lost lease, finishes `failed` with `timeout` or
`lease_expired` and its stopped step is retryable; only an operator's
cancellation or a shutdown finishes it `cancelled`.

**Retry.** A step error carries a category. `timeout`, `rate_limited` and
`dependency_unavailable` are retryable and get one more attempt in the same
run, five seconds later; `permanent`, `schema_failure`, `validation_failure`, `cancelled` and
`blocked` are not. The final failure of `x`, `youtube`, `bilibili` or
`translate` is recorded with its code and the run continues, finishing
`partial`, or `failed` when no step completed; a source step that carries a
[manual fetch](manual-fetches.md) is the exception. Any
other final failure ends the run and marks its pending steps `blocked`.
`POST /api/admin/runs/{id}/retry` requeues a `failed`, `partial` or
`blocked` run while no other run is active, when it has a retryable failed
step and no permanent failure: retryable failed and blocked steps return to
`pending`, and completed steps and earlier attempts stay.

**Recovery.** At startup and every 30 seconds the server recovers `running`
runs whose lease is missing or expired: their running attempts become
`failed` with `lease_expired` (retryable), their running steps return to
`pending`, and the run returns to `queued` with `execution_source =
recovery` and `recovery_count` incremented. A queued run is then dispatched.
`POST /api/admin/runs/{id}/cancel` records the request; a queued run is
cancelled at once, and a running one stops its network requests and finishes
`cancelled` with its pending steps. Finishing a run updates its `sync_runs`
row and the dashboard revision and releases the lease.

**Retention.** The server process deletes finished runs past their
retention window when it starts and then once a day. Succeeded and
cancelled runs are kept 30 days by default; failed, partial and blocked runs
90 days. Both windows are operator settings in `app_settings`
(`run_retention_succeeded_days`, `run_retention_failed_days`, 0–3650; 0 keeps
those runs forever), edited on `/admin` → Runs → History cleanup, and the
next pass applies a saved value. A run's age counts from `finished_at`
(`created_at` when it has none). A pass deletes a run with its
`workflow_run_steps`, `workflow_step_attempts` and `sync_runs` rows in one
transaction, at most 200 runs per transaction, then applies the same windows
to `sync_runs` rows that belong to no stored run (`success`; `failed` and
`partial`). It never deletes a queued, running or leased run, the newest
run of each `workflow_id` (the live-status probe paces itself and the
Sources page reads each schedule's last run from it) or the newest
successful `sync_runs` row (the board's freshness reads it), whatever their
age. A deleted run can no longer be retried. The pass is local database
work, not a run: it does not use the queue, and
`WORKFLOW_SCHEDULER_ENABLED=0` does not stop it.

The same daily maintenance pass prunes the source response archive on its own
30-day success and 90-day HTTP-error windows and enforces its 512 MiB compressed
body limit. Capture retention does not depend on workflow-run retention.

Schedule-step outcomes combine post extraction and image verification. An
image-only failure cannot be reported as success: a scan with failures and no
successful/cached/uncertain work is `failed`; mixed outcomes are `partial`.
Workflow progress, persisted `sync_runs`, and standalone scans use the same
aggregation. Completed sync/workflow jobs expose their persisted run, including
its failure status, and always release the single-flight queue after settlement.

## Implementation Ownership

backend/internal/jobs owns the ordered step catalog and presents durable
workflow runs as operator jobs. internal/workflow maps steps to their adapters
(`SourceSteps`), executes runs with leases, retries and recovery, and owns the
single-flight queue, and the timer that starts due source schedules
(`SourceChains`) through it. `jobs.QueueManager` submits Run now and step
runs and freezes each source step's schedule sync and method chain on them.
A new module needs one catalog entry and one step adapter; the console reads
the catalog rather than maintaining another step list. See
[Backend](../backend.md#jobs-and-workflows).

Adapters, candidate selection, model routes, modality, retries, and per-post
reprocess behavior are defined in [Sources and extraction](../sources.md).
Register media candidates before bounded downloads; only successfully fetched
records are upserted. A failed step records its outcome and retains known state.
