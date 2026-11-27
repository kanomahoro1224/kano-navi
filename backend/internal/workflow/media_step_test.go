package workflow

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/digest"
	"github.com/yexca/kano-navi/backend/internal/mediadownload"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

func TestMediaStepIsPartialWhileImagesRemainQueued(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := range 3 {
		u := fmt.Sprintf("https://pbs.twimg.com/media/step-%d.png", i)
		testutil.Exec(t, p, `INSERT INTO media_assets(id,source,source_url,status,created_at,updated_at) VALUES(?,'x',?,'pending','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`, digest.SHA256Hex(u), u)
	}
	client := mediadownload.NewClient(&http.Client{Transport: xTestRT(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("\x89PNG\r\n\x1a\n" + r.URL.Path))}, nil
	})})
	steps := &SourceSteps{Store: s, Media: client, MediaBounds: &mediadownload.Bounds{Batch: 1, Files: 2, Budget: time.Minute}}
	out, err := steps.Execute(context.Background(), StepInput{StepID: "media"})
	if err != nil || out.Status != "partial" || out.Result["ready"] != 2 || out.Result["remaining"] != 1 || out.Result["stopped"] != mediadownload.StoppedFileCap {
		t.Fatalf("first out=%#v err=%v", out, err)
	}
	// The next run continues with what is left and empties the queue.
	out, err = steps.Execute(context.Background(), StepInput{StepID: "media"})
	if err != nil || out.Status != "succeeded" || out.Result["ready"] != 1 || out.Result["remaining"] != 0 || out.Result["stopped"] != nil {
		t.Fatalf("second out=%#v err=%v", out, err)
	}
}

func TestMediaStepCountsFilesMissingAtTheSource(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, name := range []string{"kept", "gone"} {
		u := "https://pbs.twimg.com/media/" + name + ".png"
		testutil.Exec(t, p, `INSERT INTO media_assets(id,source,source_url,status,created_at,updated_at) VALUES(?,'x',?,'pending','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`, digest.SHA256Hex(u), u)
	}
	client := mediadownload.NewClient(&http.Client{Transport: xTestRT(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "gone") {
			return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("\x89PNG\r\n\x1a\n" + r.URL.Path))}, nil
	})})
	steps := &SourceSteps{Store: s, Media: client, MediaBounds: &mediadownload.Bounds{Batch: 5, Files: 5, Budget: time.Minute}}
	out, err := steps.Execute(context.Background(), StepInput{StepID: "media"})
	if err != nil || out.Status != "partial" || out.Result["ready"] != 1 || out.Result["missing"] != 1 || out.Result["failed"] != 0 {
		t.Fatalf("out=%#v err=%v", out, err)
	}
}
