package httpapi

import (
	"net/http"
	"testing"
	"time"
)

func TestAppReleaseServesEveryPlatformByDefault(t *testing.T) {
	store, _ := openFixture(t)
	handler := NewHandlerWithClock(store, func() time.Time { return time.Unix(0, 0) })

	recorder := request(t, handler, http.MethodGet, "/api/app/release")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control = %q", got)
	}
	payload := decodeObject(t, recorder)
	android, ok := payload["android"].(map[string]any)
	if !ok {
		t.Fatalf("android entry missing; body=%s", recorder.Body)
	}
	if got := android["versionCode"]; got != float64(1) {
		t.Fatalf("versionCode = %v", got)
	}
	if got := android["downloadUrl"]; got != "https://pan.xiaolubao.com/@s/1224" {
		t.Fatalf("downloadUrl = %v", got)
	}
}

func TestAppReleaseFiltersOnePlatform(t *testing.T) {
	store, _ := openFixture(t)
	handler := NewHandlerWithClock(store, func() time.Time { return time.Unix(0, 0) })

	recorder := request(t, handler, http.MethodGet, "/api/app/release?platform=android")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body)
	}
	payload := decodeObject(t, recorder)
	if _, nested := payload["android"]; nested {
		t.Fatalf("platform filter returned the whole map: %s", recorder.Body)
	}
	if got := payload["versionCode"]; got != float64(1) {
		t.Fatalf("versionCode = %v", got)
	}

	recorder = request(t, handler, http.MethodGet, "/api/app/release?platform=ios")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown platform status = %d", recorder.Code)
	}
	if got := decodeObject(t, recorder)["error"]; got != "unknown_platform" {
		t.Fatalf("error = %v", got)
	}
}

func TestAppReleaseRejectsOtherMethods(t *testing.T) {
	store, _ := openFixture(t)
	handler := NewHandlerWithClock(store, func() time.Time { return time.Unix(0, 0) })

	recorder := request(t, handler, http.MethodPost, "/api/app/release")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("allow = %q", got)
	}
}

func TestAppReleasesFromEnvOverridesDefaults(t *testing.T) {
	values := map[string]string{
		"APP_ANDROID_VERSION_CODE": "4",
		"APP_ANDROID_VERSION_NAME": "0.2.0",
		"APP_ANDROID_DOWNLOAD_URL": "https://example.test/app",
		"APP_ANDROID_NOTES":        "Fixes the widget avatar",
	}
	releases := AppReleasesFromEnv(func(key string) string { return values[key] })

	android := releases["android"]
	if android.VersionCode != 4 || android.VersionName != "0.2.0" {
		t.Fatalf("entry = %+v", android)
	}
	if android.DownloadURL != "https://example.test/app" || android.Notes != "Fixes the widget avatar" {
		t.Fatalf("entry = %+v", android)
	}
	// The floor is independent of the published version: leaving it alone keeps
	// every older build updateable but not forced.
	if android.MinSupportedCode != 1 {
		t.Fatalf("minSupportedCode = %d", android.MinSupportedCode)
	}
	if _, present := releases["harmony"]; present {
		t.Fatalf("harmony published without configuration: %+v", releases)
	}
}

func TestAppReleasesFromEnvKeepsDefaultsForUnparsableValues(t *testing.T) {
	values := map[string]string{
		"APP_ANDROID_VERSION_CODE":       "latest",
		"APP_ANDROID_MIN_SUPPORTED_CODE": "0",
	}
	releases := AppReleasesFromEnv(func(key string) string { return values[key] })

	android := releases["android"]
	if android.VersionCode != 1 {
		t.Fatalf("versionCode = %d", android.VersionCode)
	}
	if android.MinSupportedCode != 1 {
		t.Fatalf("minSupportedCode = %d", android.MinSupportedCode)
	}
}

func TestAppReleasesFromEnvAddsConfiguredPlatform(t *testing.T) {
	values := map[string]string{
		"APP_HARMONY_VERSION_CODE": "3",
		"APP_HARMONY_VERSION_NAME": "0.1.2",
		"APP_HARMONY_DOWNLOAD_URL": "https://example.test/harmony",
	}
	releases := AppReleasesFromEnv(func(key string) string { return values[key] })

	harmony, present := releases["harmony"]
	if !present {
		t.Fatalf("harmony missing: %+v", releases)
	}
	if harmony.VersionCode != 3 || harmony.MinSupportedCode != 3 {
		t.Fatalf("harmony = %+v", harmony)
	}
}
