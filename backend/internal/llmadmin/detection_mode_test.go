package llmadmin

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

func TestSettleDetectionModeMapsMixedStageFlagsOnce(t *testing.T) {
	flags := func(keyword, vision, message, allPosts string) map[string]string {
		return map[string]string{
			"schedule_keyword_enabled":   keyword,
			"schedule_vision_enabled":    vision,
			"schedule_message_enabled":   message,
			"schedule_message_all_posts": allPosts,
		}
	}
	keyword := flags("1", "1", "1", "0")
	everything := flags("0", "0", "1", "1")
	cases := []struct {
		name   string
		stored map[string]string
		env    map[string]string
		want   map[string]string
	}{
		{"all posts beside the keyword path", flags("1", "1", "1", "1"), nil, everything},
		{"all posts with only the vision stage", flags("0", "1", "1", "1"), nil, everything},
		{"keyword path without messages", flags("1", "1", "0", "0"), nil, keyword},
		{"keyword path without vision", flags("1", "0", "1", "0"), nil, keyword},
		{"all posts without the message stage", flags("1", "1", "0", "1"), nil, keyword},
		{"environment mix", nil, map[string]string{"SCHEDULE_MESSAGE_ALL_POSTS": "1"}, everything},
		{"keyword mode", keyword, nil, keyword},
		{"everything mode", everything, nil, everything},
		{"disabled stages", flags("0", "1", "0", "1"), nil, flags("0", "1", "0", "1")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := sqlite.OpenStandalone(filepath.Join(t.TempDir(), "database", "kano.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			ctx := context.Background()
			for key, value := range tc.stored {
				if err = setAppSettingAt(ctx, store.DB(), key, value, "2026-03-01T00:00:00.000Z"); err != nil {
					t.Fatal(err)
				}
			}
			lookupEnv := func(name string) (string, bool) {
				value, ok := tc.env[name]
				return value, ok
			}
			if err = SettleDetectionMode(ctx, store, lookupEnv); err != nil {
				t.Fatal(err)
			}
			config, err := ScheduleExtractionConfig(ctx, store.DB(), lookupEnv)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]bool{
				"schedule_keyword_enabled":   config.KeywordEnabled,
				"schedule_vision_enabled":    config.VisionEnabled,
				"schedule_message_enabled":   config.MessageEnabled,
				"schedule_message_all_posts": config.MessageAllPosts,
			}
			for key, value := range tc.want {
				equal(t, key, got[key], value == "1")
			}
			// A later mix the operator stores through the API stays as it is.
			if err = setAppSettingAt(ctx, store.DB(), "schedule_message_all_posts", "1", "2026-03-02T00:00:00.000Z"); err != nil {
				t.Fatal(err)
			}
			if err = setAppSettingAt(ctx, store.DB(), "schedule_keyword_enabled", "1", "2026-03-02T00:00:00.000Z"); err != nil {
				t.Fatal(err)
			}
			if err = SettleDetectionMode(ctx, store, lookupEnv); err != nil {
				t.Fatal(err)
			}
			if config, err = ScheduleExtractionConfig(ctx, store.DB(), lookupEnv); err != nil {
				t.Fatal(err)
			}
			equal(t, "second start keyword", config.KeywordEnabled, true)
			equal(t, "second start all posts", config.MessageAllPosts, true)
		})
	}
}

func TestSettleDetectionModeKeepsEnvironmentDefaultsThatMatch(t *testing.T) {
	store, err := sqlite.OpenStandalone(filepath.Join(t.TempDir(), "database", "kano.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err = SettleDetectionMode(ctx, store, func(string) (string, bool) { return "", false }); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := LookupAppSetting(ctx, store.DB(), "schedule_keyword_enabled"); found {
		t.Fatal("a matching default was stored and would hide later environment changes")
	}
	if value, _, _ := LookupAppSetting(ctx, store.DB(), detectionModeSettled); value != "1" {
		t.Fatal("the run was not recorded")
	}
}
