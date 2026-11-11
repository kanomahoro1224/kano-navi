package webbotauth

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// RFC 8037 appendix A.1's example key and A.3's thumbprint of it.
const (
	rfc8037D          = "nWGxne_9WmC6hEr0kuwsxERJxWl7MmkZcDusAxyuf2A"
	rfc8037X          = "11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"
	rfc8037Thumbprint = "kPrK_qmxVWaYVA9wwBF6Iuo3vVzz7TxHCTwXBygrS4k"
)

func TestThumbprintMatchesRFC8037(t *testing.T) {
	key, err := ParseKey(rfc8037D)
	if err != nil {
		t.Fatal(err)
	}
	if jwk := key.JWK(); jwk != (JWK{Kty: "OKP", Crv: "Ed25519", X: rfc8037X}) {
		t.Fatalf("jwk=%+v", jwk)
	}
	if key.Thumbprint() != rfc8037Thumbprint {
		t.Fatalf("thumbprint=%s", key.Thumbprint())
	}
	if key.Seed() != rfc8037D {
		t.Fatalf("seed round trip=%s", key.Seed())
	}
}

// RFC 9421 appendix B.2.6 signs a request with the B.1.4 Ed25519 test key.
// Ed25519 is deterministic, so the same base gives the same signature.
func TestSignatureBaseMatchesRFC9421Ed25519Example(t *testing.T) {
	der, err := base64.StdEncoding.DecodeString("MC4CAQAwBQYDK2VwBCIEIJ+DYvh6SEqVTm50DFtMDoQikTmiCqirVv9mWG9qfSnF")
	if err != nil {
		t.Fatal(err)
	}
	// The PKCS#8 wrapping ends with the 32-byte seed.
	key, err := ParseKey(base64.RawURLEncoding.EncodeToString(der[len(der)-32:]))
	if err != nil {
		t.Fatal(err)
	}
	components := []component{
		{`"date"`, "Tue, 20 Apr 2021 02:07:55 GMT"},
		{`"@method"`, "POST"},
		{`"@path"`, "/foo"},
		{`"@authority"`, "example.com"},
		{`"content-type"`, "application/json"},
		{`"content-length"`, "18"},
	}
	list := innerList(components, `;created=1618884473;keyid="test-key-ed25519"`)
	base := signatureBase(components, list)
	want := `"date": Tue, 20 Apr 2021 02:07:55 GMT
"@method": POST
"@path": /foo
"@authority": example.com
"content-type": application/json
"content-length": 18
"@signature-params": ("date" "@method" "@path" "@authority" "content-type" "content-length");created=1618884473;keyid="test-key-ed25519"`
	if base != want {
		t.Fatalf("base=\n%s", base)
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(key.private, []byte(base)))
	if signature != "wqcAqbmYJ2ji2glfAMaRy4gruYYnx2nEFN2HN6jrnDnQCK1u02Gb04v9EDgwUPiu4A0w6vuQv5lIp5WPpBKRCw==" {
		t.Fatalf("signature=%s", signature)
	}
}

func testSigner(t *testing.T) *Signer {
	t.Helper()
	key, err := ParseKey(rfc8037D)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner(key, "https://board.example.test")
	if err != nil {
		t.Fatal(err)
	}
	signer.Now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	signer.Random = bytes.NewReader(append(bytes.Repeat([]byte{7}, 32), bytes.Repeat([]byte{9}, 32)...))
	return signer
}

// dictionaryEntry reads one member of a structured-field dictionary.
func dictionaryEntry(t *testing.T, value, label string) string {
	t.Helper()
	for _, member := range strings.Split(value, ", ") {
		if name, rest, ok := strings.Cut(member, "="); ok && name == label {
			return rest
		}
	}
	t.Fatalf("no %s in %q", label, value)
	return ""
}

// verify rebuilds the signature base of the labelled signature from a
// message's Signature-Input and the given component values, and checks its
// Signature against public.
func verifyLabel(t *testing.T, header http.Header, label string, values map[string]string, public ed25519.PublicKey) string {
	t.Helper()
	list := dictionaryEntry(t, header.Get("Signature-Input"), label)
	sig := dictionaryEntry(t, header.Get("Signature"), label)
	if !strings.HasPrefix(sig, ":") || !strings.HasSuffix(sig, ":") {
		t.Fatalf("headers=%v", header)
	}
	ids := strings.Fields(list[1:strings.Index(list, ")")])
	components := []component{}
	for _, id := range ids {
		value, ok := values[id]
		if !ok {
			t.Fatalf("no value for %s", id)
		}
		components = append(components, component{id, value})
	}
	base := signatureBase(components, list)
	raw, err := base64.StdEncoding.DecodeString(strings.Trim(sig, ":"))
	if err != nil || !ed25519.Verify(public, []byte(base), raw) {
		t.Fatalf("signature %s does not verify over\n%s", label, base)
	}
	return base
}

func verify(t *testing.T, header http.Header, values map[string]string, public ed25519.PublicKey) string {
	t.Helper()
	return verifyLabel(t, header, "sig1", values, public)
}

func TestSignRequestCoversAuthorityAndAgent(t *testing.T) {
	signer := testSigner(t)
	req := httptest.NewRequest(http.MethodGet, "https://Shop.Example.Test:443/collections/new-arrivals/products.json?limit=50", nil)
	if err := signer.SignRequest(req); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Signature-Agent"); got != `"https://board.example.test"` {
		t.Fatalf("agent=%s", got)
	}
	nonce := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	wantInput := `sig1=("@authority" "signature-agent");created=1800000000;expires=1800000060;keyid="` + rfc8037Thumbprint + `";alg="ed25519";tag="web-bot-auth";nonce="` + nonce + `"`
	if got := req.Header.Get("Signature-Input"); got != wantInput {
		t.Fatalf("input=%s", got)
	}
	public, _ := base64.RawURLEncoding.DecodeString(rfc8037X)
	base := verify(t, req.Header, map[string]string{`"@authority"`: "shop.example.test", `"signature-agent"`: req.Header.Get("Signature-Agent")}, public)
	want := `"@authority": shop.example.test
"signature-agent": "https://board.example.test"
"@signature-params": ("@authority" "signature-agent");created=1800000000;expires=1800000060;keyid="` + rfc8037Thumbprint + `";alg="ed25519";tag="web-bot-auth";nonce="` + nonce + `"`
	if base != want {
		t.Fatalf("base=\n%s", base)
	}
	// Each request draws a new nonce.
	again := httptest.NewRequest(http.MethodGet, "https://shop.example.test/", nil)
	if err := signer.SignRequest(again); err != nil {
		t.Fatal(err)
	}
	if again.Header.Get("Signature-Input") == req.Header.Get("Signature-Input") {
		t.Fatal("nonce repeated")
	}
}

func testHolder(t *testing.T, retired ...Retired) *Holder {
	t.Helper()
	holder := NewHolder()
	holder.Now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	holder.Set(testSigner(t), retired)
	return holder
}

func TestDirectoryIsSignedAndHoldsThePublicKeyOnly(t *testing.T) {
	holder := testHolder(t)
	recorder := httptest.NewRecorder()
	holder.ServeDirectory(recorder, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7657"+DirectoryPath, nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != DirectoryContentType {
		t.Fatalf("code=%d header=%v", recorder.Code, recorder.Header())
	}
	body := recorder.Body.String()
	if body != `{"keys":[{"kty":"OKP","crv":"Ed25519","x":"`+rfc8037X+`"}]}` || strings.Contains(body, `"d"`) || strings.Contains(body, rfc8037D) {
		t.Fatalf("body=%s", body)
	}
	var directory struct{ Keys []map[string]string }
	if json.Unmarshal(recorder.Body.Bytes(), &directory) != nil || len(directory.Keys) != 1 {
		t.Fatalf("body=%s", body)
	}
	public, _ := base64.RawURLEncoding.DecodeString(directory.Keys[0]["x"])
	// The verifier fetched the directory from the agent's origin, whatever
	// Host the proxy passed on.
	base := verify(t, recorder.Header(), map[string]string{`"@authority";req`: "board.example.test"}, public)
	want := `"@authority";req: board.example.test
"@signature-params": ("@authority";req);alg="ed25519";keyid="` + rfc8037Thumbprint + `";tag="http-message-signatures-directory";created=1800000000;expires=1800003600`
	if base != want {
		t.Fatalf("base=\n%s", base)
	}
	head := httptest.NewRecorder()
	holder.ServeDirectory(head, httptest.NewRequest(http.MethodHead, DirectoryPath, nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Signature") == "" {
		t.Fatalf("head=%d %q", head.Code, head.Body.String())
	}
	post := httptest.NewRecorder()
	holder.ServeDirectory(post, httptest.NewRequest(http.MethodPost, DirectoryPath, nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post=%d", post.Code)
	}
}

// After a rotation the directory lists the retired key until its grace
// passes and signs once with every key it lists; requests sign with the
// current key alone.
func TestDirectoryKeepsARetiredKeyThroughItsGrace(t *testing.T) {
	old, err := GenerateKey(bytes.NewReader(bytes.Repeat([]byte{3}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	holder := testHolder(t, Retired{Key: old, Until: now.Add(time.Hour)})
	holder.Now = func() time.Time { return now }
	recorder := httptest.NewRecorder()
	holder.ServeDirectory(recorder, httptest.NewRequest(http.MethodGet, DirectoryPath, nil))
	var directory struct{ Keys []JWK }
	if json.Unmarshal(recorder.Body.Bytes(), &directory) != nil || len(directory.Keys) != 2 || directory.Keys[0].X != rfc8037X || directory.Keys[1] != old.JWK() {
		t.Fatalf("body=%s", recorder.Body.String())
	}
	current, _ := base64.RawURLEncoding.DecodeString(rfc8037X)
	values := map[string]string{`"@authority";req`: "board.example.test"}
	verifyLabel(t, recorder.Header(), "sig1", values, current)
	if base := verifyLabel(t, recorder.Header(), "sig2", values, old.Public()); !strings.Contains(base, `keyid="`+old.Thumbprint()+`"`) {
		t.Fatalf("sig2 base=\n%s", base)
	}
	req := httptest.NewRequest(http.MethodGet, "https://shop.example.test/", nil)
	if err := holder.Current().SignRequest(req); err != nil || strings.Contains(req.Header.Get("Signature-Input"), "sig2") ||
		!strings.Contains(req.Header.Get("Signature-Input"), rfc8037Thumbprint) {
		t.Fatalf("request input=%s err=%v", req.Header.Get("Signature-Input"), err)
	}
	// Once the grace passes only the current key is listed.
	now = now.Add(time.Hour)
	after := httptest.NewRecorder()
	holder.ServeDirectory(after, httptest.NewRequest(http.MethodGet, DirectoryPath, nil))
	if strings.Contains(after.Body.String(), old.JWK().X) || strings.Contains(after.Header().Get("Signature"), "sig2") {
		t.Fatalf("after grace: body=%s header=%v", after.Body.String(), after.Header())
	}
}

// Requests and directory reads load the holder while it is swapped.
func TestHolderSwapsTheSignerWhileInUse(t *testing.T) {
	holder := NewHolder()
	if holder.Current() != nil || holder.Serves() {
		t.Fatal("empty holder signs")
	}
	var none *Holder
	if none.Current() != nil || none.Serves() {
		t.Fatal("nil holder signs")
	}
	recorder := httptest.NewRecorder()
	holder.ServeDirectory(recorder, httptest.NewRequest(http.MethodGet, DirectoryPath, nil))
	if recorder.Code != http.StatusNotFound || recorder.Header().Get("Signature") != "" {
		t.Fatalf("empty holder answered %d", recorder.Code)
	}
	first, second := testSigner(t), testSigner(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 200 {
			if current := holder.Current(); current != nil && current != first && current != second {
				t.Error("torn signer")
			}
			holder.Serves()
		}
	}()
	for i := range 200 {
		if i%2 == 0 {
			holder.Set(first, nil)
		} else {
			holder.Set(second, nil)
		}
	}
	<-done
	holder.Set(nil, nil)
	if holder.Current() != nil || holder.Serves() {
		t.Fatal("cleared holder signs")
	}
}

func TestMissingOrInvalidKey(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	if key, err := KeyFromEnv(env(nil)); key != nil || err != nil {
		t.Fatalf("no key: %v %v", key, err)
	}
	for _, raw := range []string{"short", base64.RawURLEncoding.EncodeToString(make([]byte, 31)), base64.RawURLEncoding.EncodeToString(make([]byte, 64)), "not base64 !"} {
		if _, err := KeyFromEnv(env(map[string]string{EnvKey: raw})); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("%q: %v", raw, err)
		}
	}
	// Standard base64 with padding reads like base64url.
	std := base64.StdEncoding.EncodeToString(mustSeed(t))
	key, err := KeyFromEnv(env(map[string]string{EnvKey: " " + std + " "}))
	if err != nil || key.Thumbprint() != rfc8037Thumbprint {
		t.Fatalf("std: %v", err)
	}
	for _, origin := range []string{"", "http://board.example.test", "https://board.example.test/path", "https://board.example.test?x=1"} {
		if _, err := NewSigner(key, origin); !errors.Is(err, ErrInvalidAgent) {
			t.Fatalf("%q: %v", origin, err)
		}
	}
	if signer, err := NewSigner(key, "https://Board.Example.Test/"); err != nil || signer.Agent() != "https://board.example.test" {
		t.Fatalf("normalized: %v %v", signer, err)
	}
	if _, err := NewSigner(nil, "https://board.example.test"); !errors.Is(err, ErrInvalidAgent) {
		t.Fatalf("nil key: %v", err)
	}
}

func TestGenerateKeyRoundTrips(t *testing.T) {
	key, err := GenerateKey(bytes.NewReader(mustSeed(t)))
	if err != nil || key.Thumbprint() != rfc8037Thumbprint {
		t.Fatalf("key=%v err=%v", key, err)
	}
	parsed, err := ParseKey(key.Seed())
	if err != nil || parsed.Thumbprint() != key.Thumbprint() {
		t.Fatalf("parsed=%v err=%v", parsed, err)
	}
}

func mustSeed(t *testing.T) []byte {
	t.Helper()
	seed, err := base64.RawURLEncoding.DecodeString(rfc8037D)
	if err != nil {
		t.Fatal(err)
	}
	return seed
}
