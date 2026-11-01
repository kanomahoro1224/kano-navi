# Unreleased

> [!IMPORTANT]
> **Upgrade notes.** Startup adds the `push_devices` table, which holds the
> HarmonyOS push registrations and their switches; nothing is migrated.

<details>
<summary><strong>Operator console</strong></summary>

- The Videos page gains a Calendar label, one of `live`, `game`, `talk` or
  `song`. The App's calendar shows it for the video's event instead of reading
  the title, so a label set in the console needs no App release; an empty
  choice reads the title again.
- The Videos page's live filter reads the broadcast state, so it lists the
  streams that are on air instead of answering with an empty list.
- The console says Edit where it used to say Correct, for the same fields.

</details>

<details>
<summary><strong>Push notifications</strong></summary>

- `POST /api/push/device` registers a HarmonyOS device with its Huawei Push
  Kit token and four switches (all notifications, live, video, post); the App
  calls it on launch, after a token rotation and whenever a switch changes.
  Registering again retires the same device's previous token.
- Huawei Push Kit delivery needs an AGC service account JSON file in
  `HUAWEI_PUSH_SERVICE_ACCOUNT`; while it is empty the process starts without
  push. `HUAWEI_PUSH_CATEGORY` names the category approved for the app, and
  `HUAWEI_PUSH_TEST=true` marks test messages.
- BambinoStatus posts normalized X updates to `/internal/ingest/bambino`,
  signed with `BAMBINO_INGEST_SECRET` and accepted inside
  `BAMBINO_INGEST_TOLERANCE_SECONDS`; accepted batches queue the
  post-processing chain, and `BAMBINO_INGEST_CATCHUP_SECONDS` re-submits it
  when the queue was busy.
- A post accepted from that route reads the same type and label as one the
  timeline parser stored, so a schedule notice looks the same either way.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- A chat-completions provider that takes only `max_tokens` and
  `response_format: json_object` (DeepSeek) no longer fails its request with
  HTTP 400: the token limit keeps its portable name, and the JSON schema
  travels in the system prompt when the provider cannot take it in
  `response_format`.

</details>
