package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAccountAvatarDownloadOrderPreservesLatestObservation(t *testing.T) {
	ctx := context.Background()
	filename := filepath.Join(t.TempDir(), "database", "snapshot.sqlite")
	store, err := OpenStandalone(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	_, old, err := store.RecordAccountAvatarURL(ctx, "x", "ordering", AccountAvatarOriginSync, "https://pbs.twimg.com/profile_images/101/old.png", false)
	if err != nil {
		t.Fatal(err)
	}
	_, recent, err := store.RecordAccountAvatarURL(ctx, "x", "ordering", AccountAvatarOriginSync, "https://pbs.twimg.com/profile_images/102/recent.png", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStandalone(filename)
	if err != nil {
		t.Fatal(err)
	}
	for index, candidate := range []AccountAvatarCandidate{recent, old} {
		content, _ := testPNGBytes(byte(index))
		if err = store.SaveMediaDownload(ctx, PendingMedia{ID: candidate.MediaID, Source: "x", SourceURL: candidate.SourceURL}, content, "image/png", "", "", "ready", ""); err != nil {
			t.Fatal(err)
		}
		state, readErr := store.GetAccountAvatar(ctx, "x", "ordering")
		if readErr != nil {
			t.Fatal(readErr)
		}
		if state.ActiveCandidateID != recent.ID {
			t.Fatalf("download %s replaced latest observed avatar: %+v", candidate.SourceURL, state)
		}
	}
	cachedOld, err := store.GetAccountAvatarCandidate(ctx, "x", "ordering", old.ID)
	if err != nil {
		t.Fatal(err)
	}
	operatorResult, err := store.SaveDownloadedAccountAvatarCandidate(ctx, old.ID, cachedOld.CachePath, cachedOld.MIMEType, cachedOld.ByteSize, cachedOld.SHA256, cachedOld.Width, cachedOld.Height, "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if operatorResult.ActiveCandidateID != recent.ID {
		t.Fatalf("operator retry selected stale candidate: %+v", operatorResult)
	}
	for _, mode := range []string{AccountAvatarModeHidden, AccountAvatarModeAuto} {
		state, modeErr := store.SetAccountAvatarMode(ctx, "x", "ordering", mode)
		if modeErr != nil {
			t.Fatal(modeErr)
		}
		if state.ActiveCandidateID != recent.ID {
			t.Fatalf("mode %s selected stale avatar: %+v", mode, state)
		}
	}
	if _, _, err = store.RecordAccountAvatarURL(ctx, "x", "ordering", AccountAvatarOriginSync, old.SourceURL, false); err != nil {
		t.Fatal(err)
	}
	state, err := store.GetAccountAvatar(ctx, "x", "ordering")
	if err != nil {
		t.Fatal(err)
	}
	if state.ActiveCandidateID != old.ID {
		t.Fatalf("actual source return to old URL ignored: %+v", state)
	}
	stateValue, err := store.DeleteAccountAvatarCandidate(ctx, "x", "ordering", old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stateValue.ActiveCandidateID != recent.ID {
		t.Fatalf("deletion did not restore latest remaining observation: %+v", stateValue)
	}
}

func TestAccountAvatarManualDownloadUpdatesSharedCandidates(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStandalone(filepath.Join(t.TempDir(), "database", "snapshot.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	url := "https://pbs.twimg.com/profile_images/201/shared.png"
	candidates := make([]AccountAvatarCandidate, 0, 3)
	for _, handle := range []string{"first", "second", "hidden"} {
		_, candidate, recordErr := store.RecordAccountAvatarURL(ctx, "x", handle, AccountAvatarOriginSync, url, false)
		if recordErr != nil {
			t.Fatal(recordErr)
		}
		candidates = append(candidates, candidate)
	}
	if _, err = store.SetAccountAvatarMode(ctx, "x", "hidden", AccountAvatarModeHidden); err != nil {
		t.Fatal(err)
	}
	previousHash := ""
	for version := byte(1); version <= 2; version++ {
		content, hash := testPNGBytes(version)
		path := "avatar/sha256/" + hash[:2] + "/" + hash + ".png"
		fullPath := filepath.Join(store.DataRoot(), filepath.FromSlash(path))
		if err = os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(fullPath, content, 0600); err != nil {
			t.Fatal(err)
		}
		before, revErr := store.GetDashboardRevision(ctx)
		if revErr != nil {
			t.Fatal(revErr)
		}
		if _, err = store.SaveDownloadedAccountAvatarCandidate(ctx, candidates[0].ID, path, "image/png", int64(len(content)), hash, 2, 2, "", "", true); err != nil {
			t.Fatal(err)
		}
		for _, candidate := range candidates {
			state, readErr := store.GetAccountAvatar(ctx, "x", candidate.AccountKey)
			if readErr != nil {
				t.Fatal(readErr)
			}
			got, readErr := store.GetAccountAvatarCandidate(ctx, "x", candidate.AccountKey, candidate.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if got.Status != "ready" || got.SHA256 != hash || got.CachePath != path {
				t.Fatalf("shared candidate not synchronized: %+v", got)
			}
			stream, streamErr := store.OpenAccountAvatarMedia(ctx, candidate.ID, hash)
			if streamErr != nil {
				t.Fatal(streamErr)
			}
			actual, readErr := io.ReadAll(stream.File)
			_ = stream.File.Close()
			if readErr != nil || !bytes.Equal(actual, content) {
				t.Fatalf("shared public bytes mismatch: %v", readErr)
			}
			if candidate.AccountKey != "hidden" && state.ActiveCandidateID != candidate.ID {
				t.Fatalf("ready shared avatar not activated: %+v", state)
			}
			if candidate.AccountKey == "second" && version == 2 && state.Mode != AccountAvatarModeManual {
				t.Fatalf("manual selection lost: %+v", state)
			}
			if candidate.AccountKey == "hidden" && (state.Mode != AccountAvatarModeHidden || state.ActiveCandidateID != "") {
				t.Fatalf("hidden selection changed: %+v", state)
			}
		}
		public, publicErr := store.AccountAvatars(ctx)
		if publicErr != nil {
			t.Fatal(publicErr)
		}
		if public["x:first"] == nil || public["x:second"] == nil || public["x:hidden"] != nil {
			t.Fatalf("unexpected public avatars: %+v", public)
		}
		queue, queueErr := store.PendingMedia(ctx, 100)
		if queueErr != nil {
			t.Fatal(queueErr)
		}
		if len(queue) != 0 {
			t.Fatalf("ready shared media still queued: %+v", queue)
		}
		after, revErr := store.GetDashboardRevision(ctx)
		if revErr != nil {
			t.Fatal(revErr)
		}
		if after <= before {
			t.Fatal("shared avatar change did not advance public revision")
		}
		if previousHash != "" {
			stream, openErr := store.OpenMedia(ctx, candidates[0].MediaID, previousHash)
			if openErr != nil {
				t.Fatalf("replaced shared media version lost: %v", openErr)
			}
			_ = stream.File.Close()
		}
		previousHash = hash
		if _, err = store.SelectAccountAvatarCandidate(ctx, "x", "second", candidates[1].ID); err != nil {
			t.Fatal(err)
		}
		stableRevision, revErr := store.GetDashboardRevision(ctx)
		if revErr != nil {
			t.Fatal(revErr)
		}
		if _, err = store.SaveDownloadedAccountAvatarCandidate(ctx, candidates[0].ID, path, "image/png", int64(len(content)), hash, 2, 2, "", "", true); err != nil {
			t.Fatal(err)
		}
		unchangedRevision, revErr := store.GetDashboardRevision(ctx)
		if revErr != nil {
			t.Fatal(revErr)
		}
		if stableRevision != unchangedRevision {
			t.Fatal("unchanged shared image advanced public revision")
		}
	}
}

func TestAccountAvatarReobservationRepairsStaleCachedCandidate(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStandalone(filepath.Join(t.TempDir(), "database", "snapshot.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	url := "https://pbs.twimg.com/profile_images/401/cached.png"
	_, candidate, err := store.RecordAccountAvatarURL(ctx, "x", "cached", AccountAvatarOriginSync, url, false)
	if err != nil {
		t.Fatal(err)
	}
	content, hash := testPNGBytes(4)
	if err = store.SaveMediaDownload(ctx, PendingMedia{ID: candidate.MediaID, Source: "x", SourceURL: url}, content, "image/png", "", "", "ready", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SelectAccountAvatarCandidate(ctx, "x", "cached", candidate.ID); err != nil {
		t.Fatal(err)
	}
	_, staleHash := testPNGBytes(5)
	if _, err = store.DB().ExecContext(ctx, `UPDATE account_avatar_candidates SET sha256=? WHERE id=?`, staleHash, candidate.ID); err != nil {
		t.Fatal(err)
	}
	before, err := store.GetDashboardRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	state, repaired, err := store.RecordAccountAvatarURL(ctx, "x", "cached", AccountAvatarOriginSync, url, false)
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.GetDashboardRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.SHA256 != hash || state.Mode != AccountAvatarModeManual || state.ActiveCandidateID != candidate.ID || after <= before {
		t.Fatalf("cached candidate not repaired with preserved manual selection: %+v %+v", state, repaired)
	}
}

func TestAccountAvatarObservationOrderUpgrade(t *testing.T) {
	ctx := context.Background()
	filename := filepath.Join(t.TempDir(), "database", "snapshot.sqlite")
	store, err := OpenStandalone(filename)
	if err != nil {
		t.Fatal(err)
	}
	_, first, err := store.RecordAccountAvatarURL(ctx, "x", "upgrade", AccountAvatarOriginSync, "https://pbs.twimg.com/profile_images/301/first.png", false)
	if err != nil {
		t.Fatal(err)
	}
	_, latest, err := store.RecordAccountAvatarURL(ctx, "x", "upgrade", AccountAvatarOriginSync, "https://pbs.twimg.com/profile_images/302/latest.png", false)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := testPNGBytes(3)
	for _, candidate := range []AccountAvatarCandidate{first, latest} {
		if err = store.SaveMediaDownload(ctx, PendingMedia{ID: candidate.MediaID, Source: "x", SourceURL: candidate.SourceURL}, content, "image/png", "", "", "ready", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = store.SelectAccountAvatarCandidate(ctx, "x", "upgrade", first.ID); err != nil {
		t.Fatal(err)
	}
	revision, err := store.GetDashboardRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE account_avatar_candidates SET created_at='2026-01-01T00:00:00.000Z', updated_at='2026-01-01T00:00:00.000Z'`,
		`UPDATE account_avatar_candidates SET updated_at='2026-01-02T00:00:00.000Z' WHERE source_url LIKE '%latest.png'`,
		`ALTER TABLE account_avatar_candidates DROP COLUMN observation_order`,
	} {
		if _, err = db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		store, err = OpenStandalone(filename)
		if err != nil {
			t.Fatal(err)
		}
		state, readErr := store.GetAccountAvatar(ctx, "x", "upgrade")
		if readErr != nil {
			_ = store.Close()
			t.Fatal(readErr)
		}
		after, revErr := store.GetDashboardRevision(ctx)
		if revErr != nil {
			_ = store.Close()
			t.Fatal(revErr)
		}
		if state.Mode != AccountAvatarModeManual || state.ActiveCandidateID != first.ID || len(state.Candidates) != 2 || after != revision {
			_ = store.Close()
			t.Fatalf("upgrade changed selection/history/revision: %+v revision=%d", state, after)
		}
		if attempt == 1 {
			auto, modeErr := store.SetAccountAvatarMode(ctx, "x", "upgrade", AccountAvatarModeAuto)
			if modeErr != nil {
				_ = store.Close()
				t.Fatal(modeErr)
			}
			if auto.ActiveCandidateID != latest.ID {
				_ = store.Close()
				t.Fatalf("upgrade lost stored candidate order: %+v", auto)
			}
		}
		if err = store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
