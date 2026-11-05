package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/llmadmin"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// The migrated key opens under the key the service reads: both trim the
// environment value.
func TestMigrateLLMSealsUnderTheServiceKey(t *testing.T) {
	database := filepath.Join(t.TempDir(), "database", "kano.sqlite")
	store, err := sqlite.OpenStandalone(database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB().Exec(`INSERT INTO llm_providers (id, name, base_url, model, created_at, updated_at)
		VALUES (?, 'Synthetic', 'https://provider.example.test/v1', '', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z')`, llmadmin.LegacyProviderID); err != nil {
		t.Fatal(err)
	}
	store.Close()
	t.Setenv("OPENAI_API_KEY", " synthetic-provider-key\n")
	t.Setenv("LLM_SECRETS_KEY", "  synthetic-secrets-key-0123456789\n")
	if code := runMigrateLLM(database, false); code != 0 {
		t.Fatalf("exit %d", code)
	}
	store, err = sqlite.OpenStandalone(database)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key, err := llmadmin.ProviderAPIKey(context.Background(), store.DB(), llmadmin.LegacyProviderID, "synthetic-secrets-key-0123456789")
	if err != nil || key != "synthetic-provider-key" {
		t.Fatalf("the service key does not open the migrated key: %q %v", key, err)
	}

	t.Setenv("LLM_SECRETS_KEY", "   ")
	if code := runMigrateLLM(database, true); code != 2 {
		t.Fatalf("a whitespace-only secrets key was accepted: exit %d", code)
	}
}
