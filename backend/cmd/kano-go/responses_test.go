package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/sourcecapture"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

func TestResponsesListAndExport(t *testing.T) {
	database := filepath.Join(t.TempDir(), "kano.sqlite")
	store, err := sqlite.OpenStandalone(database)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"ok":true}`)
	if err = store.RecordSourceResponse(context.Background(), sourcecapture.Response{
		Source: "x", Endpoint: "x://api.example.test/2/tweets", Method: "GET",
		StatusCode: 200, ContentType: "application/json", CapturedAt: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), Body: want,
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	listed, code := captureStdout(t, func() int {
		return runResponses([]string{"list", "-database", database, "-source", "x"})
	})
	if code != 0 || !strings.Contains(string(listed), "application/json") || !strings.Contains(string(listed), "x://api.example.test/2/tweets") {
		t.Fatalf("list exit=%d output=%q", code, listed)
	}
	exported, code := captureStdout(t, func() int {
		return runResponses([]string{"export", "-database", database, "1"})
	})
	if code != 0 || !bytes.Equal(exported, want) {
		t.Fatalf("export exit=%d body=%q", code, exported)
	}
}

func captureStdout(t *testing.T, command func() int) ([]byte, int) {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := command()
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previous
	output, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return output, code
}
