package httpapi

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/webbotauth"
)

func TestWebBotAuthDirectoryIsServedOnlyWithAKey(t *testing.T) {
	store, _ := openFixture(t)
	without := NewStandaloneHandler(store, nil, Standalone{Static: http.NotFoundHandler()})
	if got := request(t, without, http.MethodGet, webbotauth.DirectoryPath); got.Code != http.StatusNotFound || got.Header().Get("Signature") != "" {
		t.Fatalf("without a key: status=%d header=%v", got.Code, got.Header())
	}
	key, err := webbotauth.ParseKey(base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatal(err)
	}
	signer, err := webbotauth.NewSigner(key, "https://board.example.test")
	if err != nil {
		t.Fatal(err)
	}
	with := NewStandaloneHandler(store, nil, Standalone{Signer: signer})
	got := request(t, with, http.MethodGet, webbotauth.DirectoryPath)
	if got.Code != http.StatusOK || got.Header().Get("Content-Type") != webbotauth.DirectoryContentType ||
		!strings.HasPrefix(got.Header().Get("Signature-Input"), `sig1=("@authority";req);alg="ed25519";keyid="`+key.Thumbprint()+`";tag="http-message-signatures-directory";`) ||
		!strings.Contains(got.Body.String(), `"x":"`+key.JWK().X+`"`) || strings.Contains(got.Body.String(), `"d"`) {
		t.Fatalf("with a key: status=%d header=%v body=%s", got.Code, got.Header(), got.Body.String())
	}
	// Compression leaves the directory alone.
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, webbotauth.DirectoryPath, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	Compress(with).ServeHTTP(recorder, req)
	if recorder.Header().Get("Content-Encoding") != "" || recorder.Body.String() != got.Body.String() {
		t.Fatalf("compressed: header=%v", recorder.Header())
	}
}
