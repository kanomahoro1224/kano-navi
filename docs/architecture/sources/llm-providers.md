# LLM Provider Catalog

The model provider catalog, queues and capability requirements.

`/admin` manages providers centrally. A provider stores the API host, format
(Responses, Chat Completions, or Anthropic Messages), timeout, and encrypted key. On an operator
action the server requests `GET <baseUrl>/models` with the stored key, a
timeout, rejected redirects, and a 4 MiB response limit; it accepts OpenAI-style
`{ data: [{ id }] }`, `{ models: [...] }`, or bare arrays. Upstream bodies and
errors are reduced to fixed messages. Tags for newly added models are suggested
from the model ID and stay editable. Removing a model removes its queue
targets. Model discovery uses only a `baseUrl` that passed the
[provider address rules](../admin-api/models.md#extraction-and-models).

API hosts can omit `/v1`: bare origins use `/v1`, while OpenRouter and Gemini
use their preset API roots. Explicit versioned roots and custom paths are
preserved, and a pasted inference endpoint is reduced to its API root.
`backend/internal/llmadmin/catalog.go` applies this rule to server calls,
and `src/lib/llm-endpoints.ts` applies the same rule to UI previews.
Anthropic requests use `x-api-key` and `anthropic-version: 2023-06-01`.
Model discovery follows `has_more`/`last_id` with 100 models per page and a
20-page cap under one timeout. Extraction offers one tool with the existing
JSON schema under `tool_choice: auto` (newer Claude models reject a forced
tool choice) and a system instruction to call it, then validates its
`tool_use.input` locally; a reply without that tool call fails the attempt; cached images are sent
as base64 image blocks. Model and inference responses are bounded to 4 MiB of
streamed bytes, and redirects are rejected. Connection tests and inference share
the protocol-specific output limit: `max_output_tokens` for Responses,
`max_completion_tokens` for Chat Completions, and `max_tokens` for Anthropic.

The instruction keeps titles and cited evidence in the post's wording and asks
for English detail and reason and a short lowercase English status word; an
entry without a status, or with a cancellation-like one, stores `pending`.
A local validator rejects invalid calendar dates, time formats, enumerations,
or confidence values before any event write. Times run up to `29:59`, as
Japanese schedules count late-night hours on from the evening's date: a start
of `25:30` on one date is stored as `01:30` on the next, with that date in its
event ID, so it matches the same entry written as `01:30`. An end is read from
the same evening, and one before the start falls on the following day. An
empty time means no time. A date more than 180 days before the post's
publication (Asia/Tokyo) moves to the following year when that lands between
a week before and three months after the post, so a December notice's 1/5 is
the coming January; any other date stays as the model gave it. It also rejects a result that is
not an object, a non-array `events` field, a result with neither a
classification nor an events list, and a `schedule` result without entries
unless its action is `cancel`; a rejected result is a failed attempt and
leaves existing events untouched. So is a reply the provider reports as cut
off at the token limit (`max_tokens`, `max_output_tokens`, `length`), refused,
or incomplete, even when its tool call parses; image extraction and
schedule-image reviews follow
the same rule, and a refusal stores the fixed `LLM refused the request`. `schedule` results update automatic
events. A model never deletes or definitively cancels an event: `action=cancel`
matches the clearest existing event and writes an `llm_suspected` cancellation
overlay containing the reason, source evidence, confidence, and post ID. The
overlay also handles cancellation-like event status strings on add/update
results. Such entries never create a definitively cancelled event or retire
existing source evidence; mixed results still update their scheduled entries.
Cancellation-only entries without an unambiguous existing target remain in
extraction metadata. The overlay applies to manually locked events too, so a manually entered event can
remain visible with the model's cancellation evidence. `not_schedule` and
`uncertain` results only record the extraction outcome and leave existing events
in place; a `not_schedule` result sends the post's events to the Review queue
(see [Event precedence](../data-model/event-precedence.md)). Input fingerprints (including text, publication time, URL, and cached image
hashes), the detection path, input mode and the extractor version make these
outcomes idempotent. Each stored extraction also names the queue target that
produced it, but the cache ignores that part: when the newest attempt of the
current input is `success` or `uncertain`, any queue target's result stands,
so adding, removing, reordering or replacing queue targets and models sends no
post again, and an empty queue keeps the cached state. A post runs again when
its input changes, when its newest attempt failed (under the backoff below),
and on an operator's reprocess request. Date-only events store `starts_on`, a null `starts_at`, and
`time_precision = unknown`; the model must not invent a specific time.
The extractor never deduplicates against other posts or reservations beyond
shared IDs; the later `reconcile` step gives an event for a linked YouTube
video the video's start and merges date-only and X-notice duplicates into the
more precise event (see
[Duplicate merging](../data-model/duplicate-merging.md)). Re-extraction rewrites
the source row but never its merge columns.

Every stored post has an LLM processing state. Failed, skipped, and uncertain
attempts remain visible to operators, and `POST /api/admin/posts/:id/reprocess`
queues a forced pass that bypasses the successful-result cache. The automatic
scan consumes queued posts even when they no longer match the current keyword
heuristic; a missing provider or queue target or a pending image keeps the reprocess request
pending,
while a provider failure consumes it and starts the failure backoff over. Queued posts run before keyword
candidates within the shared scan limit. They use the detection route the
operator requested; without one, a post that a keyword path also selected
keeps that route, image posts use `schedule_board`, and text posts use
`schedule_message`. The route chooses the prompt path, not the models.

Providers and the model queues are configured explicitly in `/admin`; initialization
does not create or enable an OpenAI provider. Provider API keys are encrypted with
`LLM_SECRETS_KEY`; ciphertext is never returned by an API response or written
to extraction `raw_json`. A legacy `OPENAI_API_KEY` can be imported once with
`kano-go migrate-llm` (`npm run migrate:llm`) after creating an `openai-default` provider in `/admin`;
synchronization never reads that environment variable.
Candidates from either X account use the same idempotent fingerprint and
manual-lock rules. The stage flags are stored as `schedule_keyword_enabled`,
`schedule_vision_enabled`, and `schedule_message_enabled`. The Detection page
saves them, with `schedule_message_all_posts`, as one of two modes: keyword
match (the three stages, not all posts) or everything to the LLM (the message
stage on all posts); `PUT /api/admin/extraction` accepts any combination.
Once (marker `schedule_detection_mode_settled`), the first standalone start
stores running flags that match neither mode as everything to the LLM when
the message stage runs on all posts and as keyword match otherwise. Flags that
already match a mode, from storage or the environment, and flags with no stage
running are not written. The legacy `schedule_extraction_enabled` setting
(or `SCHEDULE_EXTRACTION_ENABLED`) is only the default for a stage flag that
is not set; it is not a runtime master switch, so an enabled stage runs even
when it contains `0`. `schedule_message_all_posts` is off unless set
explicitly and does not follow the legacy setting.

The `featured_video_id` setting is maintained through `/admin`. It points to an
existing local `videos` row (or an empty value for no Featured item), and source
synchronization never changes it. The dashboard includes the selected video
even when it falls outside the most recent 30 rows.

Image extraction requires every declared image to have a linked ready cache
file. Missing links, partially ready images, absent files or unsafe paths keep
`media_pending`, preserve a forced reprocess request, and cannot replace an
existing complete extraction or retire its events. Text-only overrides do not
bypass that readiness gate.
