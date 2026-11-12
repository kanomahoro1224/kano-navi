package workflow

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/source/history"
	shopsource "github.com/yexca/kano-navi/backend/internal/source/shop"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const shopListing = `{"products":[{"id":11,"handle":"join-acrylic","title":"サンプル加入記念グッズ アクリルスタンド","tags":["鹿乃まほろ"],"published_at":"2026-10-24T19:00:08+09:00","body_html":"＜受注期間＞<br>2026年10月24日 〜 2026年11月24日","variants":[{"price":"1650"}]}]}`

func TestShopStepWritesSalesAndWaitsOutA429(t *testing.T) {
	s, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "data", "database", "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	paths, limited := []string{}, false
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		status, body := 200, shopListing
		switch {
		case limited:
			status, body = 429, "local_rate_limited"
		case r.URL.Path == "/collections.json":
			body = `{"collections":[{"handle":"join_goods","title":"サンプル 加入記念グッズ"}]}`
		}
		header := http.Header{}
		header.Set("Retry-After", "600")
		return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	adapter := shopsource.NewAdapter(shopsource.NewClient(&http.Client{Transport: rt}))
	adapter.Options.RequestDelay = 0
	steps := &SourceSteps{Store: s, Shop: adapter}
	run := func(window *history.Window) AttemptResult {
		t.Helper()
		result, err := steps.Execute(context.Background(), StepInput{StepID: sqlite.ShopSource, WorkflowID: sqlite.SourceScheduleWorkflow(sqlite.ShopSource), Window: window})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := run(nil)
	if first.Status != "succeeded" || first.Result["sales"] != 1 || first.Result["changed"] != true || strings.Join(paths, ",") != "/collections/new-arrivals/products.json,/collections.json" {
		t.Fatalf("first=%#v paths=%v", first, paths)
	}
	var url string
	if err = s.DB().QueryRow(`SELECT url FROM events WHERE source='shop' AND event_type='sale'`).Scan(&url); err != nil || url != "https://shop.milpr.com/collections/join_goods" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	// The operator's read-all pages the catalog; the known product needs no
	// collection list.
	paths = nil
	all := run(&history.Window{Method: history.MethodCatalog, Mode: history.ModeAll})
	if all.Status != "succeeded" || all.Result["pages"] != 1 || all.Result["reachedEnd"] != true || strings.Join(paths, ",") != "/products.json" {
		t.Fatalf("all=%#v paths=%v", all, paths)
	}
	// A 429 fails the step, keeps the entry, and the next timed read waits
	// for its Retry-After without a request.
	limited, paths = true, nil
	failed := run(nil)
	if errs, _ := failed.Result["errors"].([]string); failed.Status != "failed" || len(errs) != 1 || errs[0] != "shop_rate_limited" {
		t.Fatalf("failed=%#v", failed)
	}
	state, err := s.ShopState(context.Background())
	if err != nil || state.BlockedUntil.Before(time.Now().Add(9*time.Minute)) {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	paths = nil
	if held := run(nil); held.Status != "succeeded" || held.Result["reason"] != shopsource.SkippedRateLimited || len(paths) != 0 {
		t.Fatalf("held=%#v paths=%v", held, paths)
	}
	var count int
	if err = s.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE source='shop' AND deleted_at IS NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("entries=%d err=%v", count, err)
	}
}

// The step reads through the run's frozen chain and records the method that
// served and every method's outcome; a later method that served makes the
// run partial.
func TestShopStepRecordsTheChainOutcome(t *testing.T) {
	s, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "data", "database", "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	paths := []string{}
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		status, body := 200, shopListing
		switch r.URL.Path {
		case "/collections/new-arrivals/products.json":
			status, body = 404, ""
		case "/collections/all.atom":
			body = `<feed xmlns="http://www.w3.org/2005/Atom" xmlns:s="http://jadedpixel.com/-/spec/shopify"><entry><id>https://shop.milpr.com/products/11</id><published>2026-10-24T19:00:08+09:00</published><link rel="alternate" href="https://shop.milpr.com/products/join-acrylic"/><title>サンプル加入記念グッズ アクリルスタンド</title><summary type="html"><![CDATA[＜受注期間＞<br>2026年10月24日 〜 2026年11月24日]]></summary><s:tag>鹿乃まほろ</s:tag></entry></feed>`
		case "/collections.json":
			body = `{"collections":[]}`
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	adapter := shopsource.NewAdapter(shopsource.NewClient(&http.Client{Transport: rt}))
	adapter.Options.RequestDelay = 0
	steps := &SourceSteps{Store: s, Shop: adapter}
	chain := sqlite.DefaultSourceChain(sqlite.ShopSource)
	result, err := steps.Execute(context.Background(), StepInput{StepID: sqlite.ShopSource, WorkflowID: sqlite.SourceScheduleWorkflow(sqlite.ShopSource), Chain: &chain})
	if err != nil {
		t.Fatal(err)
	}
	tried, _ := result.Result["tried"].([]string)
	if result.Status != "partial" || result.Result["listing"] != "atom" || result.Result["fallback"] != "atom" || result.Result["signed"] != false ||
		strings.Join(tried, ",") != "signed: shop_no_signing_key,new_arrivals: shop_http_404,atom" || result.Result["sales"] != 1 {
		t.Fatalf("result=%#v", result)
	}
	if strings.Join(paths, ",") != "/collections/new-arrivals/products.json,/collections/all.atom,/collections.json" {
		t.Fatalf("paths=%v", paths)
	}
	// A chain of the signed read alone fails without a request.
	paths = nil
	signedOnly := sqlite.SourceChain{Methods: []sqlite.ChainMethod{{ID: "signed", Enabled: true}, {ID: "new_arrivals", Enabled: false}, {ID: "atom", Enabled: false}}}
	failed, err := steps.Execute(context.Background(), StepInput{StepID: sqlite.ShopSource, WorkflowID: sqlite.SourceScheduleWorkflow(sqlite.ShopSource), Chain: &signedOnly})
	if errs, _ := failed.Result["errors"].([]string); err != nil || failed.Status != "failed" || len(errs) != 1 || errs[0] != shopsource.ErrNoMethod || len(paths) != 0 {
		t.Fatalf("failed=%#v paths=%v err=%v", failed, paths, err)
	}
}
