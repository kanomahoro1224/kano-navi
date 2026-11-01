package llmadmin

import (
	"context"
	"errors"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// detectionModeSettled records that SettleDetectionMode has run.
const detectionModeSettled = "schedule_detection_mode_settled"

// SettleDetectionMode runs once per database, on the first start that calls
// it. The Detection page offers two modes, each a fixed set of stage flags:
// keyword match (keyword, vision and message stages, no all-posts) and
// everything to the LLM (message stage with all-posts only). Running stage
// flags that match neither are stored as the nearest mode: everything to the
// LLM when the message stage runs on all posts, keyword match otherwise.
// Flags that already match a mode, from storage or the environment, are not
// written, and flags with no detection stage running stay as they are, so a
// disabled pipeline is not turned on. lookupEnv supplies the SCHEDULE_*
// defaults and may be nil.
func SettleDetectionMode(ctx context.Context, store *sqlite.Store, lookupEnv func(string) (string, bool)) error {
	if !store.Writable() {
		return nil
	}
	tx, err := store.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, settled, err := LookupAppSetting(ctx, tx, detectionModeSettled); err != nil || settled {
		return err
	}
	config, err := ScheduleExtractionConfig(ctx, tx, lookupEnv)
	if err != nil {
		return err
	}
	keyword := config.KeywordEnabled && config.VisionEnabled && config.MessageEnabled && !config.MessageAllPosts
	everything := !config.KeywordEnabled && !config.VisionEnabled && config.MessageEnabled && config.MessageAllPosts
	now := sqlite.ISOTime(time.Now())
	if config.Enabled && !keyword && !everything {
		llm := config.MessageEnabled && config.MessageAllPosts
		for _, stage := range []struct {
			key string
			on  bool
		}{
			{"schedule_keyword_enabled", !llm},
			{"schedule_vision_enabled", !llm},
			{"schedule_message_enabled", true},
			{"schedule_message_all_posts", llm},
		} {
			value := "0"
			if stage.on {
				value = "1"
			}
			if err = setAppSettingAt(ctx, tx, stage.key, value, now); err != nil {
				return errors.New("detection mode could not be stored")
			}
		}
	}
	if err = setAppSettingAt(ctx, tx, detectionModeSettled, "1", now); err != nil {
		return errors.New("detection mode could not be stored")
	}
	return tx.Commit()
}
