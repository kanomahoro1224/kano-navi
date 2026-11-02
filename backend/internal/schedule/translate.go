package schedule

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// Translator is the machine translation step. It translates X post text and
// YouTube video titles into the target locales with the models of the
// separate translation queue, using the extractor's provider client and
// failover policy. It never replaces a manual or platform translation: those
// rank above machine rows when readers resolve them, and a locale they cover
// is not sent again.
type Translator struct {
	e *Extractor
}

const (
	// maxTranslationInput bounds the wording sent to a model.
	maxTranslationInput = 4000
	// eventTagPastDays and eventTagFutureDays bound which calendar entries the
	// model labels: recent ones and announcements up to three months ahead.
	eventTagPastDays   = 30
	eventTagFutureDays = 90
)

// NewTranslator shares the extractor's options. Its scan limit and lookback
// are the operator's sync settings, read when each run starts.
func NewTranslator(store *sqlite.Store, opts Options) *Translator {
	return &Translator{e: New(store, opts)}
}

var (
	translationLinks    = regexp.MustCompile(`https?://\S+`)
	translationHandles  = regexp.MustCompile(`[@#＃][\p{L}\p{N}_]+`)
	translationKindText = map[string]string{
		sqlite.TranslationOwnerPost:  "X post",
		sqlite.TranslationOwnerVideo: "YouTube video title",
	}
)

// hasTranslatableText reports whether anything but links, tags, mentions,
// emoji and punctuation remains.
func hasTranslatableText(text string) bool {
	text = translationHandles.ReplaceAllString(translationLinks.ReplaceAllString(text, " "), " ")
	for _, r := range text {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// Run translates up to the scan limit of candidates: operator requests
// first, then owners published within the lookback days that lack a
// usable translation. Without a configured translation queue it records
// nothing and keeps requests pending.
func (t *Translator) Run(ctx context.Context) (Result, error) {
	if t == nil || t.e == nil || t.e.db == nil {
		return Result{Status: "failed", Counts: map[string]any{}}, ErrStore
	}
	counts, err := t.translatePending(ctx)
	if counts == nil {
		counts = map[string]any{}
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{Status: "failed", Counts: counts}, ctxErr
		}
		return Result{Status: "failed", Counts: counts}, ErrStore
	}
	// The same queue labels the calendar entries whose title carries no other
	// evidence, so a YouTube-only announcement still reaches readers with a
	// kind, and then the videos no entry labels.
	if tags, videoTags, tagErr := t.inferTags(ctx); tags != nil {
		counts["eventTags"] = tags
		if videoTags != nil {
			counts["videoTags"] = videoTags
		}
		if tagErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Result{Status: "failed", Counts: counts}, ctxErr
			}
			return Result{Status: "failed", Counts: counts}, ErrStore
		}
	}
	return Result{Status: scanStatus(counts), Counts: counts}, nil
}

func (t *Translator) translatePending(ctx context.Context) (map[string]any, error) {
	e := t.e
	counts := map[string]any{"attempted": 0, "success": 0, "skipped": 0, "failed": 0, "dropped": 0}
	providers, reason, err := e.routeProviders(ctx, routeTranslate, "text")
	if err != nil {
		return counts, err
	}
	if len(providers) == 0 {
		counts["disabled"], counts["reason"] = true, reason
		return counts, nil
	}
	settings, err := e.store.SyncSettings(ctx)
	if err != nil {
		return counts, err
	}
	limit, lookback := settings.TranslationScanLimit, settings.TranslationLookbackDays
	since := e.now().Add(-time.Duration(lookback) * 24 * time.Hour)
	// Local skips cost no model call, so a few extra candidates keep them
	// from shrinking the budget.
	candidates, err := e.store.ListTranslationCandidates(ctx, since, limit*2)
	if err != nil {
		return counts, err
	}
	for _, item := range candidates {
		if countOf(counts, "attempted") >= limit {
			break
		}
		if err = ctx.Err(); err != nil {
			return counts, err
		}
		status, err := t.translate(ctx, item, providers, counts)
		if err != nil {
			return counts, err
		}
		if status != "skipped" {
			counts["attempted"] = countOf(counts, "attempted") + 1
		}
		counts[status] = countOf(counts, status) + 1
	}
	return counts, nil
}

// tagJob is one title the inference labels: the calendar entry or video it
// belongs to, and where its answer and a failed attempt are recorded.
type tagJob struct {
	title, platform string
	record          func(*string) error
	fail            func() error
}

// inferTags labels the calendar entries that carry no other evidence of
// what they are, then the videos no entry labels. YouTube reservations are
// created by the sync and never pass through the extraction prompt, and an
// upload often has no entry at all, so the title is the only thing to read;
// the judgement is short-text work like translation, and it uses that queue
// and its retry policy. Entries go first and videos use what is left of the
// same per-run limit, so the video backlog drains over successive runs. A
// title that fails counts as failed and the run moves on; only a store write
// or cancellation stops it. videos is nil when the run never reached them.
func (t *Translator) inferTags(ctx context.Context) (events, videos map[string]any, err error) {
	e := t.e
	events = map[string]any{"attempted": 0, "success": 0, "skipped": 0, "failed": 0}
	providers, reason, err := e.routeProviders(ctx, routeTranslate, "text")
	if err != nil {
		return events, nil, err
	}
	if len(providers) == 0 {
		events["disabled"], events["reason"] = true, reason
		return events, nil, nil
	}
	settings, err := e.store.SyncSettings(ctx)
	if err != nil {
		return events, nil, err
	}
	limit := settings.TranslationScanLimit
	if limit < 1 {
		limit = 1
	}
	now := e.now().In(jst)
	from := now.AddDate(0, 0, -eventTagPastDays).Format("2006-01-02")
	to := now.AddDate(0, 0, eventTagFutureDays).Format("2006-01-02")
	entries, err := e.store.ListEventTagCandidates(ctx, from, to, limit)
	if err != nil {
		return events, nil, err
	}
	jobs := make([]tagJob, 0, len(entries))
	for _, item := range entries {
		id := item.ID
		jobs = append(jobs, tagJob{
			title: item.Title, platform: "YouTube",
			record: func(tag *string) error { _, err := e.store.RecordEventTag(ctx, id, tag, e.now()); return err },
			fail:   func() error { return e.store.RecordEventTagFailure(ctx, id, e.now()) },
		})
	}
	if err = t.runTagJobs(ctx, jobs, providers, events); err != nil {
		return events, nil, err
	}
	videos = map[string]any{"attempted": 0, "success": 0, "skipped": 0, "failed": 0}
	if limit -= len(entries); limit < 1 {
		return events, videos, nil
	}
	uploads, err := e.store.ListVideoTagCandidates(ctx, from, limit)
	if err != nil {
		return events, videos, err
	}
	jobs = make([]tagJob, 0, len(uploads))
	for _, item := range uploads {
		id, platform := item.ID, "YouTube"
		if item.Source == "bilibili" {
			platform = "Bilibili"
		}
		jobs = append(jobs, tagJob{
			title: item.Title, platform: platform,
			record: func(tag *string) error { _, err := e.store.RecordVideoTag(ctx, id, tag, e.now()); return err },
			fail:   func() error { return e.store.RecordVideoTagFailure(ctx, id, e.now()) },
		})
	}
	return events, videos, t.runTagJobs(ctx, jobs, providers, videos)
}

// runTagJobs labels each job in turn and counts the outcomes.
func (t *Translator) runTagJobs(ctx context.Context, jobs []tagJob, providers []provider, counts map[string]any) error {
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		status, err := t.inferTag(ctx, job, providers)
		if err != nil {
			return err
		}
		if status != "skipped" {
			counts["attempted"] = countOf(counts, "attempted") + 1
		}
		counts[status] = countOf(counts, status) + 1
	}
	return nil
}

// inferTag asks each configured target in turn for the label of one title
// and returns success, skipped or failed. A null answer is a real result: the
// entry or video keeps no label, and the judgement is recorded so the title
// is not sent again. When every target fails, the attempt is only stamped, so
// the title is asked again after the others.
func (t *Translator) inferTag(ctx context.Context, job tagJob, providers []provider) (string, error) {
	e := t.e
	// Over-long wording costs no call, as in translation; an operator can
	// label it by hand.
	if utf8.RuneCountInString(job.title) > maxTranslationInput {
		return "skipped", job.record(nil)
	}
	call := modelCall{
		instructions: eventTagInstructions,
		name:         eventTagToolName,
		schema:       eventTagSchema,
		maxTokens:    1024,
		text:         job.platform + " title: " + job.title,
		includeText:  true,
		jsonOutput:   true,
	}
	message := "provider request failed"
	attempts := e.modelAttempts(ctx)
	for _, target := range providers {
		for attempt := 0; attempt < attempts; attempt++ {
			tag, err := t.attemptEventTag(ctx, target, call)
			if err == nil {
				if err = job.record(tag); err != nil {
					return "", err
				}
				if err = e.updateProviderStatus(ctx, target.id, "success", ""); err != nil {
					return "", err
				}
				return "success", nil
			}
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			message = storedMessage(err)
			if attempt == attempts-1 || !transientModelFailure(message) {
				break
			}
			if err = e.sleep(ctx, retryDelay(attempt)); err != nil {
				return "", err
			}
		}
		if err := e.updateProviderStatus(ctx, target.id, "failed", message); err != nil {
			return "", err
		}
	}
	return "failed", job.fail()
}

// attemptEventTag makes one call and validates its label; nil means the
// model found none.
func (t *Translator) attemptEventTag(ctx context.Context, target provider, call modelCall) (*string, error) {
	value, err := t.e.callModel(ctx, target, call)
	if err != nil {
		return nil, err
	}
	root, ok := value.(*object)
	if !ok {
		return nil, fail("event tag result was not an object")
	}
	tagValue, _ := root.get("tag")
	if tagValue == nil {
		return nil, nil
	}
	text, isString := tagValue.(string)
	if !isString || !sqlite.ValidEventTag(text) {
		return nil, fail("event tag result has an unknown label")
	}
	return &text, nil
}

// translate handles one candidate and returns success, skipped, failed or
// dropped (the wording changed while the model was working).
func (t *Translator) translate(ctx context.Context, item sqlite.TranslationCandidate, providers []provider, counts map[string]any) (string, error) {
	e := t.e
	outcome := sqlite.TranslationOutcome{OwnerType: item.OwnerType, OwnerID: item.OwnerID, SourceHash: item.SourceHash}
	if !hasTranslatableText(item.Text) {
		outcome.Status, outcome.SourceLanguage = "skipped", "none"
		return "skipped", e.store.RecordTranslationOutcome(ctx, outcome, e.now())
	}
	// Over-long wording costs no call; an operator can translate it by hand.
	if utf8.RuneCountInString(item.Text) > maxTranslationInput {
		outcome.Status, outcome.Error = "skipped", "text is too long to translate"
		return "skipped", e.store.RecordTranslationOutcome(ctx, outcome, e.now())
	}
	baseCall := modelCall{
		instructions: translationInstructions,
		name:         translationToolName,
		schema:       translationSchema,
		maxTokens:    min(4000, max(1024, 400+3*utf8.RuneCountInString(item.Text))),
		text:         "Kind: " + translationKindText[item.OwnerType] + "\n\n" + item.Text,
		includeText:  true,
		jsonOutput:   true,
	}
	message := "provider request failed"
	attempts := e.modelAttempts(ctx)
	for _, target := range providers {
		call, repaired := baseCall, false
		for attempt := 0; attempt < attempts; attempt++ {
			diagnostics := modelDiagnostics{}
			call.diagnostics = &diagnostics
			result, err := t.attempt(ctx, item, target, call)
			recordTranslationDiagnostic(counts, item, target, attempt, call.maxTokens, diagnostics, err)
			if err == nil {
				saved, err := e.store.SaveMachineTranslation(ctx, result, e.now())
				if err != nil {
					return "", err
				}
				if err = e.updateProviderStatus(ctx, target.id, "success", ""); err != nil {
					return "", err
				}
				if !saved {
					return "dropped", nil
				}
				return "success", nil
			}
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			message = storedMessage(err)
			if attempt == attempts-1 {
				break
			}
			retry, wait := retryTranslation(&call, message, &repaired)
			if !retry {
				break
			}
			if wait {
				if err = e.sleep(ctx, retryDelay(attempt)); err != nil {
					return "", err
				}
			}
		}
		if err := e.updateProviderStatus(ctx, target.id, "failed", message); err != nil {
			return "", err
		}
	}
	outcome.Status, outcome.Error = "failed", message
	return "failed", e.store.RecordTranslationOutcome(ctx, outcome, e.now())
}

// Each target gets at most three calls. A truncated reply needs more output
// room, an invalid result gets one explicit correction, and only transient
// request failures wait before retrying. Other failures advance the queue.
func retryTranslation(call *modelCall, message string, repaired *bool) (retry, wait bool) {
	switch sqlite.TranslationErrorCode(message) {
	case "output_truncated":
		if call.maxTokens >= 4000 {
			return false, false
		}
		call.maxTokens = min(4000, call.maxTokens*2)
		return true, false
	case "missing_translation_en", "missing_translation_zh_cn", "invalid_translation", "invalid_response", "no_structured_output":
		if *repaired {
			return false, false
		}
		*repaired = true
		call.instructions += "\n\nThe previous response failed validation (" + sqlite.TranslationErrorCode(message) + "). Return the complete JSON object matching the schema, with sourceLanguage, english and simplifiedChinese. For every target language different from sourceLanguage, provide a non-empty translation. If a song title or name needs to stay unchanged, copy it into the complete displayed text rather than returning an empty string. Do not omit fields or add commentary."
		return true, false
	}
	if transientModelFailure(message) {
		return true, true
	}
	return false, false
}

// transientModelFailure reports a request failure worth waiting for and
// retrying on the same target: a network failure, a timeout, or HTTP 408,
// 429 or 5xx.
func transientModelFailure(message string) bool {
	if message == "LLM request failed" || message == "LLM request timed out" {
		return true
	}
	const prefix = "LLM request failed (HTTP "
	if strings.HasPrefix(message, prefix) {
		status, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(message, prefix), ")"))
		if err == nil && (status == 408 || status == 429 || status >= 500 && status <= 599) {
			return true
		}
	}
	return false
}

func recordTranslationDiagnostic(counts map[string]any, item sqlite.TranslationCandidate, target provider, attempt, budget int, d modelDiagnostics, err error) {
	entries, _ := counts["diagnostics"].([]any)
	// Bound the durable diagnostic payload even with a long fallback queue.
	if len(entries) >= 600 {
		counts["diagnosticsOmitted"] = countOf(counts, "diagnosticsOmitted") + 1
		return
	}
	entry := map[string]any{
		"ownerType": item.OwnerType, "ownerId": item.OwnerID,
		"provider": target.id, "model": target.model,
		"attempt": attempt + 1, "maxTokens": budget,
	}
	if d.stopReason != "" {
		entry["stopReason"] = d.stopReason
	}
	if d.inputTokens != nil {
		entry["inputTokens"] = *d.inputTokens
	}
	if d.outputTokens != nil {
		entry["outputTokens"] = *d.outputTokens
	}
	if err != nil {
		entry["error"] = sqlite.TranslationErrorCode(storedMessage(err))
	}
	counts["diagnostics"] = append(entries, entry)
}

func (t *Translator) attempt(ctx context.Context, item sqlite.TranslationCandidate, target provider, call modelCall) (sqlite.MachineTranslation, error) {
	value, err := t.e.callModel(ctx, target, call)
	if err != nil {
		return sqlite.MachineTranslation{}, err
	}
	result, err := normalizeTranslation(value, item.Text)
	if err != nil {
		return sqlite.MachineTranslation{}, err
	}
	result.OwnerType, result.OwnerID, result.SourceHash = item.OwnerType, item.OwnerID, item.SourceHash
	result.ProviderID, result.ModelID = target.id, target.model
	return result, nil
}

var translationLocaleFields = map[string]string{"en": "english", "zh-CN": "simplifiedChinese"}

// translationLanguage is the source language that makes a locale unnecessary.
var translationLanguage = map[string]string{"en": "en", "zh-CN": "zh"}

// normalizeTranslation validates a model result: a known source language and
// a bounded translation for every locale that differs from it.
func normalizeTranslation(value any, source string) (sqlite.MachineTranslation, error) {
	root, ok := value.(*object)
	if !ok {
		return sqlite.MachineTranslation{}, fail("translation result was not an object")
	}
	language := jsToLower(jsTrim(stringOr(property(root, "sourceLanguage"), "")))
	if !sqlite.SourceLanguages[language] {
		return sqlite.MachineTranslation{}, fail("translation result has an unknown source language")
	}
	result := sqlite.MachineTranslation{SourceLanguage: language, Texts: map[string]string{}}
	if language == "none" {
		return result, nil
	}
	limit := min(sqlite.MaxTranslationRunes, max(200, 6*utf8.RuneCountInString(source)))
	for _, locale := range sqlite.TranslationLocales {
		if translationLanguage[locale] == language {
			continue
		}
		text := jsTrim(stringOr(property(root, translationLocaleFields[locale]), ""))
		if text == "" {
			return sqlite.MachineTranslation{}, fail("translation result is missing " + locale)
		}
		if utf8.RuneCountInString(text) > limit {
			return sqlite.MachineTranslation{}, fail("translation result is too long")
		}
		result.Texts[locale] = text
	}
	return result, nil
}
