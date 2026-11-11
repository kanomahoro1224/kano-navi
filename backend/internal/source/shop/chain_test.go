package shop

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/webbotauth"
)

// atomFeedBody is a synthetic catalog feed in Shopify's shape: a tagged
// product with a period and a price in its summary, an untagged product, and
// entries whose ID or link leaves the shop.
const atomFeedBody = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:s="http://jadedpixel.com/-/spec/shopify">
  <id>https://shop.milpr.com/collections/all.atom</id>
  <link rel="alternate" type="text/html" href="https://shop.milpr.com/collections/all"/>
  <title>サンプル</title>
  <entry>
    <id>https://shop.milpr.com/products/11</id>
    <published>2026-10-24T19:00:08+09:00</published>
    <updated>2026-10-25T10:00:00+09:00</updated>
    <link rel="alternate" type="text/html" href="https://shop.milpr.com/products/join-acrylic"/>
    <title>サンプル加入記念グッズ &amp; アクリルスタンド</title>
    <s:type>アクリルスタンド</s:type>
    <summary type="html"><![CDATA[<p>商品説明</p><p>＜受注期間＞<br>2026年10月24日 〜 2026年11月24日</p><table><tr><td>価格</td><td>¥1,650</td></tr></table>]]></summary>
    <s:variant><s:price currencyCode="JPY">1650</s:price></s:variant>
    <s:tag>鹿乃まほろ</s:tag>
    <s:tag> サンプル </s:tag>
  </entry>
  <entry>
    <id>https://shop.milpr.com/products/12</id>
    <published>2026-10-20T12:00:00+09:00</published>
    <link rel="alternate" type="text/html" href="https://shop.milpr.com/products/other-goods"/>
    <title>サンプル他社グッズ</title>
    <summary type="html"><![CDATA[<p>＜受注期間＞<br>2026年10月20日 〜 2026年11月20日</p>]]></summary>
  </entry>
  <entry>
    <id>https://elsewhere.example.test/products/13</id>
    <published>2026-10-20T12:00:00+09:00</published>
    <link rel="alternate" href="https://shop.milpr.com/products/stray"/>
    <title>Stray</title>
  </entry>
  <entry>
    <id>https://shop.milpr.com/products/14</id>
    <published>2026-10-20T12:00:00+09:00</published>
    <link rel="alternate" href="https://elsewhere.example.test/products/stray"/>
    <title>Stray</title>
  </entry>
</feed>`

func TestParseAtomMapsEntriesToProducts(t *testing.T) {
	products, err := ParseAtom([]byte(atomFeedBody))
	if err != nil || len(products) != 2 {
		t.Fatalf("products=%+v err=%v", products, err)
	}
	first := products[0]
	if first.ID != 11 || first.Handle != "join-acrylic" || first.Title != "サンプル加入記念グッズ & アクリルスタンド" ||
		!first.PublishedAt.Equal(time.Date(2026, 10, 24, 10, 0, 8, 0, time.UTC)) || fmt.Sprint(first.Tags) != "[鹿乃まほろ サンプル]" || !first.Tagged() {
		t.Fatalf("first=%+v", first)
	}
	if products[1].Tagged() {
		t.Fatalf("untagged=%+v", products[1])
	}
	candidates, outcomes := Candidates(products)
	if len(candidates) != 1 || candidates[0].Period.Key() != "2026-10-24/2026-11-24" || outcomes[SkipUntagged] != 1 {
		t.Fatalf("candidates=%+v outcomes=%v", candidates, outcomes)
	}
	for _, body := range []string{"<html>not a feed</html>", "{}", "<feed"} {
		if _, err := ParseAtom([]byte(body)); ErrorCode(err) != "shop_invalid_atom" {
			t.Fatalf("%q: %v", body, err)
		}
	}
}

func testSigner(t *testing.T) *webbotauth.Signer {
	t.Helper()
	key, err := webbotauth.ParseKey(base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("s", 32))))
	if err != nil {
		t.Fatal(err)
	}
	signer, err := webbotauth.NewSigner(key, "https://board.example.test")
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

type seenRequest struct {
	path   string
	signed bool
}

// chainShop answers each path with the given status (200 when absent) and
// records every request and whether it was signed.
func chainShop(t *testing.T, statuses map[string]int, signer *webbotauth.Signer) (*Adapter, *[]seenRequest) {
	t.Helper()
	seen := []seenRequest{}
	client := NewClient(&http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("User-Agent") != UserAgent {
			t.Fatalf("user agent=%q", r.Header.Get("User-Agent"))
		}
		signed := r.Header.Get("Signature") != ""
		if signed && (r.Header.Get("Signature-Agent") != `"https://board.example.test"` || !strings.Contains(r.Header.Get("Signature-Input"), `tag="web-bot-auth"`)) {
			t.Fatalf("signature headers=%v", r.Header)
		}
		seen = append(seen, seenRequest{r.URL.Path, signed})
		if status, ok := statuses[r.URL.Path]; ok {
			if status == -1 {
				return nil, fmt.Errorf("unreachable")
			}
			return answer(status, "local_rate_limited", "Retry-After", "60"), nil
		}
		switch r.URL.Path {
		case "/collections.json":
			return answer(200, `{"collections":[{"handle":"join_goods","title":"サンプル加入記念グッズ"}]}`), nil
		case "/collections/all.atom":
			return answer(200, atomFeedBody), nil
		}
		return answer(200, products(product(11, "join-acrylic", "サンプル加入記念グッズ アクリルスタンド", "2026-10-24T19:00:08+09:00", period, Tag))), nil
	})})
	client.Signer = signer
	adapter := NewAdapter(client)
	adapter.Options = Options{Now: func() time.Time { return time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC) }}
	return adapter, &seen
}

const (
	arrivalsPath = "/collections/new-arrivals/products.json"
	atomPath     = "/collections/all.atom"
)

func TestChainSkipsTheSignedReadWithoutAKey(t *testing.T) {
	adapter, seen := chainShop(t, nil, nil)
	result, err := adapter.Read(context.Background(), State{}, nil, false, nil)
	if err != nil || result.Listing != MethodArrivals || result.Fallback || result.Signed || result.Requested != 2 ||
		fmt.Sprint(result.Tried) != "[signed: shop_no_signing_key new_arrivals]" || len(result.Errors) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if fmt.Sprint(*seen) != fmt.Sprint([]seenRequest{{arrivalsPath, false}, {"/collections.json", false}}) {
		t.Fatalf("seen=%v", *seen)
	}
	// A chain of the signed read alone can make no request.
	adapter, seen = chainShop(t, nil, nil)
	result, err = adapter.Read(context.Background(), State{}, nil, false, []string{MethodSigned})
	if ErrorCode(err) != ErrNoMethod || result.Requested != 0 || len(*seen) != 0 || result.State.LastError != ErrNoMethod || !result.State.BlockedUntil.IsZero() {
		t.Fatalf("result=%+v err=%v seen=%v", result, err, *seen)
	}
}

func TestChainSignsWithAKey(t *testing.T) {
	adapter, seen := chainShop(t, nil, testSigner(t))
	result, err := adapter.Read(context.Background(), State{}, nil, false, nil)
	if err != nil || result.Listing != MethodSigned || !result.Signed || fmt.Sprint(result.Tried) != "[signed]" || len(result.Sales) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	// The collection list follows the listing's signing.
	if fmt.Sprint(*seen) != fmt.Sprint([]seenRequest{{arrivalsPath, true}, {"/collections.json", true}}) {
		t.Fatalf("seen=%v", *seen)
	}
	// The unsigned read stays unsigned with a key, and so does its
	// collection list; the Atom read is signed.
	adapter, seen = chainShop(t, nil, testSigner(t))
	if _, err = adapter.Read(context.Background(), State{}, nil, false, []string{MethodArrivals}); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(*seen) != fmt.Sprint([]seenRequest{{arrivalsPath, false}, {"/collections.json", false}}) {
		t.Fatalf("seen=%v", *seen)
	}
	adapter, seen = chainShop(t, nil, testSigner(t))
	if result, err = adapter.Read(context.Background(), State{}, nil, false, []string{MethodAtom}); err != nil || result.Listing != MethodAtom || !result.Signed || result.Listed != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if fmt.Sprint(*seen) != fmt.Sprint([]seenRequest{{atomPath, true}, {"/collections.json", true}}) {
		t.Fatalf("seen=%v", *seen)
	}
	// The operator's whole-catalog read signs every page.
	adapter, seen = chainShop(t, nil, testSigner(t))
	if result, err = adapter.Read(context.Background(), State{}, nil, true, nil); err != nil || !result.Signed || result.Pages != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, request := range *seen {
		if !request.signed {
			t.Fatalf("seen=%v", *seen)
		}
	}
}

// A 429 holds for the whole address, so it ends the read: a signed 429 is
// never retried unsigned, and an unsigned one never moves on to the feed.
func TestChainEndsAtA429OrAHostFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses map[string]int
		signer   bool
		tried    string
		code     string
		blocked  bool
	}{
		{"signed 429", map[string]int{arrivalsPath: 429}, true, "[signed: shop_rate_limited]", "shop_rate_limited", true},
		{"unsigned 429", map[string]int{arrivalsPath: 429}, false, "[signed: shop_no_signing_key new_arrivals: shop_rate_limited]", "shop_rate_limited", true},
		{"unreachable", map[string]int{arrivalsPath: -1}, true, "[signed: shop_unavailable]", "shop_unavailable", false},
	} {
		var signer *webbotauth.Signer
		if tc.signer {
			signer = testSigner(t)
		}
		adapter, seen := chainShop(t, tc.statuses, signer)
		result, err := adapter.Read(context.Background(), State{}, nil, false, nil)
		if ErrorCode(err) != tc.code || result.Requested != 1 || len(*seen) != 1 || fmt.Sprint(result.Tried) != tc.tried || result.Sales != nil ||
			result.State.LastError != tc.code || result.State.BlockedUntil.IsZero() == tc.blocked {
			t.Fatalf("%s: result=%+v err=%v seen=%v", tc.name, result, err, *seen)
		}
	}
}

// A failure of the method alone lets the next one try, within two listing
// requests and the collection list.
func TestChainFallsBackWithinTheBudget(t *testing.T) {
	adapter, seen := chainShop(t, map[string]int{arrivalsPath: 403}, testSigner(t))
	result, err := adapter.Read(context.Background(), State{}, nil, false, []string{MethodSigned, MethodAtom, MethodArrivals})
	if err != nil || result.Listing != MethodAtom || !result.Fallback || result.Requested != 3 ||
		fmt.Sprint(result.Tried) != "[signed: shop_http_403 atom]" || fmt.Sprint(result.Errors) != "[shop_http_403]" || len(result.Sales) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if fmt.Sprint(*seen) != fmt.Sprint([]seenRequest{{arrivalsPath, true}, {atomPath, true}, {"/collections.json", true}}) {
		t.Fatalf("seen=%v", *seen)
	}
	// Two failed listings end the read before a third method.
	adapter, seen = chainShop(t, map[string]int{arrivalsPath: 503}, testSigner(t))
	result, err = adapter.Read(context.Background(), State{}, nil, false, nil)
	if ErrorCode(err) != "shop_http_503" || result.Requested != MaxListingRequests || len(*seen) != MaxListingRequests ||
		fmt.Sprint(result.Tried) != "[signed: shop_http_503 new_arrivals: shop_http_503]" || result.Listing != "" {
		t.Fatalf("result=%+v err=%v seen=%v", result, err, *seen)
	}
	// The skipped signed read costs no request, so without a key both
	// unsigned methods fit.
	adapter, seen = chainShop(t, map[string]int{arrivalsPath: 404}, nil)
	result, err = adapter.Read(context.Background(), State{}, nil, false, nil)
	if err != nil || result.Listing != MethodAtom || result.Signed || !result.Fallback ||
		fmt.Sprint(result.Tried) != "[signed: shop_no_signing_key new_arrivals: shop_http_404 atom]" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if fmt.Sprint(*seen) != fmt.Sprint([]seenRequest{{arrivalsPath, false}, {atomPath, false}, {"/collections.json", false}}) {
		t.Fatalf("seen=%v", *seen)
	}
}

func TestAtomSalesKeepNoPrice(t *testing.T) {
	adapter, _ := chainShop(t, nil, nil)
	result, err := adapter.Read(context.Background(), State{}, nil, false, []string{MethodAtom})
	if err != nil || len(result.Sales) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	raw, _ := json.Marshal(result.Sales)
	if strings.Contains(string(raw), "1,650") || strings.Contains(string(raw), "1650") || strings.Contains(string(raw), "価格") {
		t.Fatalf("sale keeps a price: %s", raw)
	}
	if result.Sales[0].Collection != "join_goods" || result.Sales[0].Products[0].Handle != "join-acrylic" {
		t.Fatalf("sale=%+v", result.Sales[0])
	}
}

// The client reads the holder for every request, so a key the operator
// generates or removes applies to the next read without a new client.
func TestClientFollowsTheHolder(t *testing.T) {
	holder := webbotauth.NewHolder()
	adapter, seen := chainShop(t, nil, nil)
	adapter.Client.Signer = holder
	if result, err := adapter.Read(context.Background(), State{}, nil, false, nil); err != nil || result.Listing != MethodArrivals || result.Signed {
		t.Fatalf("no key: %+v %v", result, err)
	}
	holder.Set(testSigner(t), nil)
	*seen = nil
	if result, err := adapter.Read(context.Background(), State{}, nil, false, nil); err != nil || result.Listing != MethodSigned || !(*seen)[0].signed {
		t.Fatalf("with key: %+v %v %v", result, err, *seen)
	}
	holder.Set(nil, nil)
	*seen = nil
	if result, err := adapter.Read(context.Background(), State{}, nil, false, nil); err != nil || result.Listing != MethodArrivals || (*seen)[0].signed {
		t.Fatalf("removed: %+v %v %v", result, err, *seen)
	}
}
