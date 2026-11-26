package workflow

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

func TestBilibiliStepsCheckTheStoredLoginOnceADayAndReadWithIt(t *testing.T) {
	s, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "data", "database", "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const stored = "synthetic0stored%2C1767225600%2Cabcde%2A33"
	paths, listingCookie := []string{}, ""
	read := biliResponder(t, &paths, "")
	adapter := bilisource.NewAdapter(bilisource.NewClient(&http.Client{Transport: xTestRT(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/x/passport-login/web/cookie/info":
			paths = append(paths, r.URL.Host+r.URL.Path)
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"refresh":false,"timestamp":1767225600000}}`))}, nil
		case "/x/polymer/web-dynamic/desktop/v1/feed/space":
			listingCookie = r.Header.Get("Cookie")
		}
		return read(r)
	})}), bilisource.Options{Account: bilisource.DefaultAccount, MinInterval: 3 * time.Hour, PageSize: 20, SESSDATA: syntheticLogin})
	adapter.Logins = &bilisource.Logins{Records: s, Key: "synthetic-secrets-key-for-tests-only"}
	login, err := bilisource.ImportLogin("SESSDATA="+stored+"; bili_jct=0123456789abcdef0123456789abcdef", "fedcba9876543210fedcba9876543210", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = adapter.Logins.Put(context.Background(), login); err != nil {
		t.Fatal(err)
	}
	steps := &SourceSteps{Store: s, Bilibili: adapter}
	result, err := steps.Execute(context.Background(), StepInput{StepID: sqlite.BilibiliDynamicsSource, WorkflowID: sqlite.AdhocSyncWorkflow})
	if err != nil {
		t.Fatal(err)
	}
	checked, _ := result.Result["login"].(map[string]any)
	// The check made the device identity (spi and ticket), which the read
	// then reused, and asked cookie/info.
	if result.Status != "succeeded" || checked["outcome"] != bilisource.RefreshCurrent || checked["requested"] != 3 || result.Result["requested"] != 5 {
		t.Fatalf("result=%#v", result)
	}
	if !strings.Contains(listingCookie, "SESSDATA="+stored) || strings.Contains(listingCookie, syntheticLogin) {
		t.Fatalf("the listing did not read with the stored login: %q", listingCookie)
	}
	state, err := s.BilibiliState(context.Background(), bilisource.DefaultAccount)
	if err != nil || state.Refresh.CheckedAt.IsZero() || state.Refresh.LastError != "" || adapter.LoginStatus(context.Background(), state) != "signed_in" {
		t.Fatalf("refresh=%#v status=%s err=%v", state.Refresh, adapter.LoginStatus(context.Background(), state), err)
	}
	result, err = steps.Execute(context.Background(), StepInput{StepID: "bilibili", WorkflowID: sqlite.AdhocSyncWorkflow})
	if err != nil || result.Result["login"] != nil {
		t.Fatalf("a second check the same day=%#v err=%v", result, err)
	}
}
