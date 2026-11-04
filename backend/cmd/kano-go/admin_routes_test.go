package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/adminapi"
	"github.com/yexca/kano-navi/backend/internal/adminauth"
	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/outboundproxy"
	"github.com/yexca/kano-navi/backend/internal/source/youtube/websub"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// Every route the service's admin modules register, with every method,
// answers 401 without a session. A module route on /session would be served
// without the session check, so none may use that path.
func TestEveryModuleRouteRequiresASession(t *testing.T) {
	store, err := sqlite.OpenStandalone(filepath.Join(t.TempDir(), "database", "kano.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	auth, err := adminauth.New(adminauth.Config{Mode: "production", Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	const key = "synthetic-secrets-key-0123456789"
	modules := standaloneModules(store, "production", key, outboundproxy.NewPool(store.DB(), key))
	modules = append(modules, websub.NewAdmin(store, websub.New("UCAAAAAAAAAAAAAAAAAAAAAA", http.DefaultClient, nil), key, true))
	handler := adminapi.New(adminapi.Options{Store: store, Auth: auth, Modules: modules})

	wildcard := regexp.MustCompile(`\{[^}]+\}`)
	checked := 0
	for _, module := range modules {
		for _, route := range module.AdminRoutes() {
			if route.Path == "/session" || strings.HasPrefix(route.Path, "/session/") {
				t.Fatalf("%s %s is served without the session check", route.Method, route.Path)
			}
			request := httptest.NewRequest(route.Method, httpx.AdminPrefix+wildcard.ReplaceAllString(route.Path, "synthetic"), strings.NewReader("{}"))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "admin_auth_required") {
				t.Errorf("%s %s = %d %s", route.Method, route.Path, response.Code, response.Body)
			}
			checked++
		}
	}
	if checked < 30 {
		t.Fatalf("only %d module routes checked", checked)
	}
}
