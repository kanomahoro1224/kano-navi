# Extraction, Models and Translations

Extraction settings, model providers and translations.

## Translations

| Method and path                              | Body                                                         | Result                    |
| -------------------------------------------- | ------------------------------------------------------------ | ------------------------- |
| `GET /translations`                          |                                                              | `{counts, locales}`       |
| `POST /translations/request`                 | `{missing: true, ownerType?, source?}` or `{ownerType, ids}` | `202 {queued, run, busy}` |
| `PUT /translations/{owner}/{id}/{locale}`    | `{text}`                                                     | `{translation}`           |
| `DELETE /translations/{owner}/{id}/{locale}` |                                                              | `{translation}`           |

`owner` and `ownerType` are `post`, `video`, `x_status`, a pooled X status
by its status ID, or `bilibili_status`, a pooled forwarded dynamic by its
post ID (see [Translations](../data-model/translations.md)); `locale`
is `en`, `ja` or `zh-CN`. `counts` maps `post` and `video` to `{total, incomplete,
requested, sources}` over the posts and videos the translate step covers;
`sources` holds the same totals per source (`x` and `bilibili` for posts,
`youtube` and `bilibili` for videos), which the console's platform tabs show. A translation summary is
`{status, sourceLanguage, machineStatus, lastAttemptAt, lastError, requested,
locales}` with `status` `complete` or `incomplete`, `machineStatus` `never`,
`success`, `failed`, `skipped` or `outdated` (the wording changed after the
last attempt), and one entry per locale: `{locale, status, origin, platform,
text, manual, manualStale, machine, machineAt, model}`. A locale's `status` is
`translated`, `missing`, `stale` (only an outdated machine row), `source`
(already in that language) or `not_needed`; `text` and `origin` are what
readers see.

Translation `lastError` distinguishes `output_truncated`,
`no_structured_output`, `missing_translation_en`,
`missing_translation_zh_cn`, `invalid_translation` and `model_refused`,
alongside the generic request categories (for example `timeout` or
`authentication`). It never contains upstream response wording. A run's
`run.counts.translate.diagnostics` records bounded, sanitized per-attempt
metadata; see [Translation](../sources/translation.md).

`PUT` stores a manual translation (trimmed, at most 8000 characters) that
outranks platform and machine rows; `DELETE` removes it. Both answer the
owner's new summary and `404 not_found` for an unknown owner. `POST
/translations/request` queues forced machine passes for the listed owners
(at most 200), or with `missing: true` for every owner of `ownerType`, or
every post and video without one, lacking a usable translation, whatever
its age; `source` keeps such a request to one of the owner type's sources
and needs `ownerType` (any other pairing answers `400`). Owners outside the translate step's scope, such as a pooled status
that is a stored post, are not queued. When it
queued any and the run queue is free it starts a `translate` run; `busy: true`
means the requests wait for the next translate step.

## Extraction and models

| Method and path                          | Body                                                                                       | Result                                                                       |
| ---------------------------------------- | ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| `GET /extraction`                        |                                                                                            | extraction settings                                                          |
| `PUT /extraction`                        | any subset of `{keywordEnabled, visionEnabled, messageEnabled, messageAllPosts, keywords}` | extraction settings                                                          |
| `GET /llm`                               |                                                                                            | `{providers, models, queue, verificationQueue, translationQueue, modelTags}` |
| `GET /llm/failures`                      |                                                                                            | `{items}` newest 30 failures                                                 |
| `POST /llm/providers`                    | provider fields                                                                            | `201 {provider}`                                                             |
| `PUT /llm/providers/{id}`                | provider fields                                                                            | `{provider}`                                                                 |
| `DELETE /llm/providers/{id}`             |                                                                                            | `204`                                                                        |
| `POST /llm/providers/{id}/test`          | `{model?}`                                                                                 | `{ok, method, model, remoteModelCount, provider}`                            |
| `POST /llm/providers/{id}/discover`      |                                                                                            | `{provider, models}` remote catalog                                          |
| `POST /llm/providers/{id}/models`        | `{models, origin?}`                                                                        | `201 {provider, models}`                                                     |
| `PUT /llm/providers/{id}/models`         | one model `{id, ...}`                                                                      | `{provider, model, models}`                                                  |
| `POST /llm/providers/{id}/models/remove` | `{ids}`                                                                                    | `{provider, models, removed}`                                                |
| `PUT /llm/queue`                         | `{route?, targets: [{providerId, modelId}]}`                                               | `{route, queue}`                                                             |

Extraction settings are `{enabled, keywordEnabled, visionEnabled,
messageEnabled, messageAllPosts, keywords}`; `enabled` is derived: the message
stage alone, or keyword candidates plus boards. `messageAllPosts` makes the
message stage select every post instead of only heuristic matches. `queue` is
the ordered `[{providerId, modelId}]` text schedule extraction list; every
target names a catalog model (`modelId` is required), and a save replaces the
whole list. `route` selects the queue a save replaces: `schedule` (the
default) for text extraction, `verify` for image schedule extraction and
schedule-image review (targets must carry the `image` tag), or `translate` for
translation and other short text tasks (targets must carry the `text` tag).
Removing a model removes its targets from every queue. Providers have no default model: a provider response
carries no `model` field, a `model` field in a provider write is ignored, and a
provider test without `{model}` only lists the provider's models. Creating a
provider or adding catalog models never queues anything. Model IDs may
contain `/`, so model writes name the model in the body. Provider responses
report `apiKeyConfigured` only; keys are write-only. A provider write that
changes `baseUrl` while a key is stored must carry `apiKey`, otherwise it is
`400 invalid_input`, so a stored key is only sent to the address it was
entered for.

A provider `baseUrl` may name a model server on the local network: a private
or shared (carrier-grade NAT) IPv4 address, a unique-local IPv6 address, or a
name ending in `.local`, `.lan`, `.internal` or `.home.arpa`, such as
`host.docker.internal`. Production accepts plain HTTP only for these
local-network addresses; every other address must use HTTPS. This machine
(`localhost`, `.localhost` and loopback addresses), link-local addresses
(including cloud metadata endpoints), multicast and the other
special-purpose ranges are refused. Names are not resolved, so a public name
is accepted wherever it points; only the operator sets provider addresses.

`GET /llm/failures` lists the 30 newest stored model failures, newest first:
failed schedule extractions (one per input and queue target), failed image
extractions and schedule-image checks, and owners whose last machine translation failed. A
stopped run (`LLM request cancelled`) is not listed. Each item is `{route,
subject, subjectId, url, providerId, model, message, at}`: `route` is
`schedule`, `verify` or `translate`; `subject` is `post`, `image` (a schedule
image, with its post's `url`) or `video`; `providerId` is known for
extractions and `model` for extractions and image checks; `message` is the
stored fixed message, never upstream wording; `at` is when the call failed.
