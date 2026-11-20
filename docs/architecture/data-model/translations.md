# Translations

How translations are stored and resolved.

Post text and video titles keep their source wording in `posts.text` and
`videos.title`. `translations` stores their translations for the target
locales `en`, `ja` and `zh-CN`, keyed by
`(owner_type, owner_id, field, locale, origin, platform)`: `owner_type` is
`post` (field `text`), `video` (field `title`), `x_status` (field `text`,
a pooled X status keyed by its status ID) or `bilibili_status` (field
`text`, a pooled forwarded dynamic keyed by its post ID), `origin` is `manual`,
`platform` or `machine`, and `platform` names the platform of a platform row
(`youtube`, `bilibili`) and is empty otherwise. One row per origin lets the
three coexist; readers resolve each locale by precedence: manual, then
platform (YouTube before Bilibili, then any other), then machine.

Every row stores `source_hash`, the SHA-256 of the trimmed source wording it
translates; SQL compares it through the deterministic `translation_hash()`
function. A machine row counts only while its hash matches the current
wording, so a changed post or title retires it until the translate step
replaces it. A manual row always counts; the console flags it when the
wording changed after it was written. Platform rows are owned by the source
writers that store them (`UpsertPlatformTranslationTx`,
`DeletePlatformTranslationTx` in `backend/internal/store/sqlite/translations.go`),
which keep them current and remove them when the platform no longer provides
one; readers do not compare their hash. YouTube synchronization writes the
`en` and `zh-CN` titles a creator localized (platform `youtube`, see
[Localized titles](../sources/youtube-videos.md#localized-titles)), and the
[video merge](videos.md#video-merging) writes a merged Bilibili upload's title
(platform `bilibili`). Automatic writers never change a
manual row, and every write that changes what readers see increments the
dashboard revision.

`translation_states` holds one row per post or video the machine step has
seen: the hash and detected `source_language` (`ja`, `en`, `zh`, `ko`,
`other`, or `none` when there is nothing to translate) of the wording it last
attempted, its `status` (`success`, `failed`, `skipped`), the last attempt and
sanitized error, and `requested`, an operator's request for a forced pass. A
locale whose language matches the detected source language needs no
translation, and `none` needs none at all: a Japanese X post or YouTube title
needs `en` and `zh-CN`, a Chinese Bilibili dynamic `en` and `ja`. Wording the
step has not seen in its current form needs `en` and `zh-CN`, and `ja` only
when it is a Bilibili dynamic, stored or pooled, or a Bilibili title, since X
posts and statuses and YouTube titles are presumed Japanese until detected;
the console shows such a `ja` as the source. A post
or video is complete when every locale it needs has a usable row. The
machine step translates posts of the sources `x` and `bilibili` and the
unmerged videos public readers list (YouTube videos and Bilibili uploads
whose form is settled, see [Video form and tags](video-form.md#video-form-and-tags)). The
public dashboard, archives and
video lists add `translations` (`{locale: {text, origin, platform}}`) and
`sourceLanguage` to posts and videos; providers, models and hashes stay in
the console. A Bilibili video whose language the machine step has not
detected reports `sourceLanguage` `zh`. A video's translations follow its effective
title, `COALESCE(manual_title, title)`, so an operator's title correction
retires its machine translations.

Events have no translation rows of their own. A board or calendar event
whose URL names a YouTube video in the translate step's scope (by
`stream_identity`) carries that video's `translations` and `sourceLanguage`
while its trimmed title equals the video's effective title; an event with
an operator's or an extraction's own wording carries none. The projection is
computed at read time, so a video translation's revision already covers it.

A post's available `quote` carries `translations` and `sourceLanguage`,
computed at read time. A quote of a stored post (a self-quote, or a forward
of an own stored dynamic) carries that post's. Any other quote carries the
pool rows of its ID, `x_status` on an X post and `bilibili_status` on a
Bilibili forward, resolved against the quote's own `text` in `quote_json`: a
machine row and the detected language count only while their source hash
matches that text, and manual and platform rows always count. The read uses
the translation tables alone and never reads a pool table. The machine step
translates a pooled X status while an own post quotes it or, for a related
account's status, an own account reposts it, and a pooled dynamic while an
own forward forwards it, when it is not a stored post (see
[Translation](../sources/translation.md)).

Startup creates both translation tables in existing snapshots; no post,
video or event row is rewritten.

Translation model attempts also record bounded diagnostics in
`sync_runs.counts_json` under `translate.diagnostics`: configured model and
provider, owner reference, attempt and output budget, known stop reason,
numeric token usage and a fixed error category. These are protected run
metadata, not public translation fields. The payload contains no provider
response text or reasoning; see [Translation](../sources/translation.md).
