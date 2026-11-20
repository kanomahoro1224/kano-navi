# Database and Maintenance

SQLite and dynamic media live under the ignored data/ directory, together with
the packaged LINE BLOG archive media in data/lineblog/, the packaged
discography covers in data/works/, which the Journey page's release
milestones also show, and the Journey page's own images in data/journey/. A fresh
installation starts with static profile, timeline, and resources, plus empty
source snapshots and LLM provider/model routes.

## Seed and Synchronization

```bash
make seed                    # Idempotently add missing static records
npm run seed -- -overwrite   # Restore the shipped profile, timeline and resources
make sync                    # Update the snapshot from public sources once
```

These run `kano-go seed` and `kano-go sync`. In a container deployment use
`docker compose run --rm kano seed` (or `sync`, `migrate-llm`), which runs the
command against the mounted `./data`. Normal operation does not need them:
the service seeds at startup, and synchronization is started from `/admin`,
MCP or a source timer. A one-shot `sync` shares the service's single-flight
reservation and reports a busy queue while the service is synchronizing.

Synchronization accesses the network and is therefore not part of the default
CI build. To debug one source, set `SKIP_X=1` or `SKIP_YOUTUBE=1`; never put a
temporary credential in shell history, source code, or the database.

Provider keys are managed in `/admin` and encrypted in SQLite with the
environment-only `LLM_SECRETS_KEY`. With no configured provider key, the
schedule-extraction stage reports a safe skip. To store an `OPENAI_API_KEY`
as the encrypted key of the `openai-default` provider, set both environment
variables, run `kano-go migrate-llm` (`npm run migrate:llm`) once, and remove
`OPENAI_API_KEY` afterwards. `SCHEDULE_MESSAGE_ENABLED` controls the
single-message detector; `SCHEDULE_KEYWORD_ENABLED` and
`SCHEDULE_VISION_ENABLED` control the board path. `SCHEDULE_EXTRACTION_ENABLED`
only supplies the default of a stage flag that has no stored or environment
value; the stage flags determine what runs.
Provider API keys entered in `/admin` require `LLM_SECRETS_KEY` and are
encrypted before they reach SQLite; they are never returned by the API. Once
configured, keys are read only by the server-side synchronization process.
Provider eligibility is based on the original post modality: Text, Image, or
both for mixed input. A failed provider gets up to the
[attempts per model](configuration/sync.md#sync-settings) in total calls before the
next compatible provider is used.
X synchronization reads both handles in `X_HANDLES` and keeps a separate cursor
for each account.

Source HTTP responses are captured during workflow runs and stored compressed
in the SQLite snapshot. Text responses are retained after HTTP content
decoding, before source parsing; each stored body is capped at 4 MiB, and the
compressed capture store is capped at 512 MiB. Successful responses expire
after 30 days and errors after 90 days. List recent metadata with
`kano-go responses list` (`-source` filters by source); export one verified
response body with `kano-go responses export <id>`. Export writes only the body
to standard output so it can be redirected to a local file or piped into an
analysis tool. The command opens the database read-only, and capture bodies
are never returned by the HTTP API. Keep database backups and exports
access-controlled.

## Backup and Recovery

Stop the server and any CLI synchronization before copying the complete data/
directory. Preserve SQLite and any WAL/SHM sidecars, media, and profile files
together. Keep the environment-only LLM_SECRETS_KEY in an access-controlled
secret backup so encrypted provider keys remain usable. Never commit backups.
Restore into a stopped deployment and verify health and the public snapshot
before enabling source timers. Avoid simultaneous Windows/Linux access to
the same SQLite file.

Use additive schema upgrades in
backend/internal/store/sqlite/schema_upgrade.go. Startup upgrades an older
snapshot in one transaction and never clears it; back up data/ before the
first start of a new release all the same. Seeding fills missing
static IDs; it is not a database restore. The -overwrite option intentionally
replaces the shipped static records and should only be used for reviewed
maintenance.

## Manual Maintenance

Use `/admin` instead of direct SQL for event maintenance. Creating, editing,
confirming, or deleting through that API records manual precedence and preserves
source links. Direct database edits can omit the lock/tombstone rules and should
be reserved for reviewed recovery work.

The `/mcp` endpoint is intentionally narrower than `/admin`. Its public tools
read the prepared snapshot and sanitized schedule/status views. Bearer-protected
tools may request a revision, start a full synchronization, or run the automatic
keyword/board/message scan. Revision requests return the new revision directly;
synchronization and scans return asynchronous job results. MCP has no
manual confirmation, edit, delete, profile-media selection/upload, SQL, file, or
arbitrary URL-proxy operation. A page refresh or revision request never performs
an external fetch.

## X Account Exports

[Account exports](../architecture/sources/x-accounts.md#account-exports) add an X
account's posts from a browser exporter's files, `<handle>.json` and
`<handle>-media.zip`, and hide the posts an earlier third-party import filed
under the account that the export does not list. Back up data/ first, then:

1. Copy the exports into an ignored directory of the deployment, such as
   `data/imports/x/`.
2. Count the work without writing:
   `docker compose run --rm kano x-archive import -dir /app/data/imports/x -dry-run`
   (locally `kano-go x-archive import -dir data/imports/x -dry-run`).
   `-account <handle>` limits the run to named exports.
3. Run it again without `-dry-run`. It may run while the service runs; the
   board shows the change at its next revision. An account outside
   `X_HANDLES` becomes an archive-only account.
4. Copy the database to a workstation and read the hidden posts there:
   `kano-go x-archive check -database <copy> -out decisions.jsonl`, with
   `-capture <directory>` naming the earlier import's capture when it is at
   hand, so its marked reposts need no request. The run is
   long (6 seconds or more per post) and can stop and resume at any time; a
   rerun appends to the same file. `-limit` reads only that many posts, and
   `-interval`, `-min-interval` and `-max-interval` change the pace.
5. Copy `decisions.jsonl` to the deployment and apply it with
   `x-archive apply -in <file>`, first with `-dry-run`. Own posts and thread
   replies that only the import hid are shown again; the rest stay hidden,
   and the console's post list shows them for an operator's decision.

The exports and the decisions file stay out of Git like the rest of data/.

## Related Docs

- [Schema and precedence](../architecture/data-model.md)
- [Media layout](../architecture/media-cache.md)
- [Configuration](configuration.md)
