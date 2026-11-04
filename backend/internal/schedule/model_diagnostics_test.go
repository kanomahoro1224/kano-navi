package schedule

import (
	"net/http"
	"strings"
	"testing"
)

func TestTranslationRefusalDoesNotRetryOrStoreProviderWording(t *testing.T) {
	for _, protocol := range []string{protocolAnthropic, protocolChat, protocolResponses} {
		t.Run(protocol, func(t *testing.T) {
			f, translator := newTranslatorFixture(t)
			f.addProvider("main", protocol, "https://translate.example.test", "synthetic-model", `["text"]`)
			f.queueTranslation("main")
			f.addPost("1", "", "おはよう", recent(1), "")
			f.provider.respond = func(recordedRequest) (int, string) {
				switch protocol {
				case protocolAnthropic:
					return http.StatusOK, `{"stop_reason":"refusal","content":[{"type":"text","text":"synthetic private refusal"}]}`
				case protocolChat:
					return http.StatusOK, `{"choices":[{"finish_reason":"stop","message":{"refusal":"synthetic private refusal"}}]}`
				default:
					return http.StatusOK, `{"status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"synthetic private refusal"}]}]}`
				}
			}
			result := runTranslator(t, translator)
			if result.Status != "failed" || f.provider.count() != 1 || f.text(`SELECT last_error FROM translation_states`) != modelRefused {
				t.Fatalf("result=%v requests=%d", result, f.provider.count())
			}
			if strings.Contains(stringify(result.Counts), "private refusal") {
				t.Fatal("provider refusal wording leaked")
			}
		})
	}
}

func TestModelDiagnosticsIgnoreUntrustedAndInvalidMetadata(t *testing.T) {
	d := modelDiagnostics{}
	call := modelCall{jsonOutput: true, diagnostics: &d}
	payload, err := parseJSON([]byte(`{"stop_reason":"synthetic private response","usage":{"input_tokens":"synthetic credential","output_tokens":-1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = call.checkResponse(payload, protocolAnthropic); err != nil || d.stopReason != "unknown" || d.inputTokens != nil || d.outputTokens != nil {
		t.Fatalf("diagnostics=%+v err=%v", d, err)
	}
	for _, value := range []float64{0.5, 1_000_000_001} {
		if usageCount(value) != nil {
			t.Fatalf("invalid usage admitted: %v", value)
		}
	}
}

// A schedule extraction the model cut off or refused fails the attempt and
// leaves the post's events, whatever partial answer came with it.
func TestTruncatedOrRefusedExtractionKeepsEvents(t *testing.T) {
	for name, reply := range map[string]string{
		// A cut-off tool call can still parse as a whole, plausible answer.
		"anthropic truncated": `{"stop_reason":"max_tokens","content":[{"type":"tool_use","name":"` + extractionToolName + `","input":{"classification":"not_schedule","events":[],"confidence":0.4}}]}`,
		"anthropic refused":   `{"stop_reason":"refusal","content":[{"type":"text","text":"synthetic private refusal"}]}`,
		"chat truncated":      `{"choices":[{"finish_reason":"length","message":{"content":"{\"classification\":\"not_schedule\",\"events\":[]"}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			// The first extraction stores the post's event.
			f := messageFixture(t)
			f.scan()
			if got := f.text(`SELECT COUNT(*) FROM events WHERE deleted_at IS NULL`); got != "1" {
				t.Fatalf("setup events=%s", got)
			}
			if strings.HasPrefix(name, "anthropic") {
				f.addProvider("p-anth", protocolAnthropic, "https://anthropic.example.test", "model-c", `["text"]`)
				f.exec(`DELETE FROM llm_route_targets`)
				f.queue("p-anth")
			}
			f.provider.respond = func(recordedRequest) (int, string) { return http.StatusOK, reply }
			f.requestReprocess("post-1")
			f.scan()
			if got := f.text(`SELECT COUNT(*) FROM events WHERE deleted_at IS NULL`); got != "1" {
				t.Fatalf("a cut-off or refused answer changed the events: %s", got)
			}
			if got := f.text(`SELECT status FROM post_llm_states WHERE post_id='post-1'`); got != "failed" {
				t.Fatalf("post status=%s", got)
			}
			if strings.Contains(f.text(`SELECT COALESCE(group_concat(COALESCE(last_error,''),' '),'') FROM post_llm_states`), "private refusal") {
				t.Fatal("provider refusal wording leaked")
			}
		})
	}
}
