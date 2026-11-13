package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

// AppRelease is the newest published build of one client platform. The
// download URL points at a distribution page rather than at a file, so a new
// build replaces the file behind the link and existing clients keep working.
type AppRelease struct {
	// VersionCode is the Android versionCode (the integer a client compares
	// against its own); versionName is display text only.
	VersionCode      int    `json:"versionCode"`
	VersionName      string `json:"versionName"`
	MinSupportedCode int    `json:"minSupportedCode"`
	DownloadURL      string `json:"downloadUrl"`
	Notes            string `json:"notes,omitempty"`
	PublishedAt      string `json:"publishedAt,omitempty"`
}

// AppReleases answers version checks per platform. A platform is present only
// while it has a configured release; a client unknown to it gets 404 and keeps
// running silently.
type AppReleases map[string]AppRelease

// appReleasePlatforms lists the clients this repository publishes.
var appReleasePlatforms = []string{"android", "harmony"}

// DefaultAppReleases is the build the app shipped with when this repository
// was cut. Android is the delivered client; harmony is added by configuration
// alone, so an unset platform never answers a half-empty entry.
func DefaultAppReleases() AppReleases {
	return AppReleases{
		"android": {
			VersionCode:      1,
			VersionName:      "0.1.0",
			MinSupportedCode: 1,
			DownloadURL:      "https://pan.xiaolubao.com/@s/1224",
		},
	}
}

// AppReleasesFromEnv layers APP_<PLATFORM>_* variables over the defaults, so
// publishing a build edits configuration instead of code. A value that cannot
// be parsed keeps the default rather than publishing a bogus version, and a
// platform with no variables at all stays absent.
func AppReleasesFromEnv(getenv func(string) string) AppReleases {
	releases := DefaultAppReleases()
	for _, platform := range appReleasePlatforms {
		applyAppReleaseEnv(releases, platform, getenv)
	}
	return releases
}

func applyAppReleaseEnv(releases AppReleases, platform string, getenv func(string) string) {
	prefix := "APP_" + strings.ToUpper(platform) + "_"
	versionCode := strings.TrimSpace(getenv(prefix + "VERSION_CODE"))
	versionName := strings.TrimSpace(getenv(prefix + "VERSION_NAME"))
	minSupported := strings.TrimSpace(getenv(prefix + "MIN_SUPPORTED_CODE"))
	downloadURL := strings.TrimSpace(getenv(prefix + "DOWNLOAD_URL"))
	notes := strings.TrimSpace(getenv(prefix + "NOTES"))
	publishedAt := strings.TrimSpace(getenv(prefix + "PUBLISHED_AT"))
	if versionCode == "" && versionName == "" && minSupported == "" &&
		downloadURL == "" && notes == "" && publishedAt == "" {
		return
	}

	entry := releases[platform]
	if parsed, ok := parseNonNegative(versionCode); ok {
		entry.VersionCode = parsed
	}
	if parsed, ok := parseNonNegative(minSupported); ok {
		entry.MinSupportedCode = parsed
	}
	if versionName != "" {
		entry.VersionName = versionName
	}
	if downloadURL != "" {
		entry.DownloadURL = downloadURL
	}
	entry.Notes = notes
	entry.PublishedAt = publishedAt
	if entry.MinSupportedCode <= 0 {
		entry.MinSupportedCode = entry.VersionCode
	}
	releases[platform] = entry
}

func parseNonNegative(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

// appRelease answers a client's version check. Without a platform it reports
// every configured platform, which keeps one cached response usable by all
// clients; with one it reports that entry alone.
func (h *Handler) appRelease(w http.ResponseWriter, r *http.Request) {
	platform := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("platform")))
	if platform == "" {
		writeJSON(w, http.StatusOK, h.releases, "no-store")
		return
	}
	entry, ok := h.releases[platform]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown_platform"}, "no-store")
		return
	}
	writeJSON(w, http.StatusOK, entry, "no-store")
}
