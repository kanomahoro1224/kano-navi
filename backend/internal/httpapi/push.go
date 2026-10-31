package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const (
	// MaxPushBodyBytes bounds a registration: a token, a device id and four
	// flags, never media.
	MaxPushBodyBytes = 8 << 10
	// maxPushTokenRunes leaves room for Huawei's longest tokens, which grow
	// with the app's scope set.
	maxPushTokenRunes = 4096
	maxPushFieldRunes = 128
)

// pushDeviceRequest is the App's registration payload. The four switches are
// pointers so an omitted field keeps its default (on) instead of reading as
// "the user turned it off".
type pushDeviceRequest struct {
	Token      string `json:"token"`
	DeviceID   string `json:"deviceId"`
	Platform   string `json:"platform"`
	Locale     string `json:"locale"`
	AppVersion string `json:"appVersion"`
	Enabled    *bool  `json:"enabled"`
	Live       *bool  `json:"live"`
	Video      *bool  `json:"video"`
	Post       *bool  `json:"post"`
}

// pushDevice registers or updates one device's push token and preferences.
// The App calls it on launch, after a token rotation and whenever a switch in
// settings changes, so this path must stay idempotent.
func (h *Handler) pushDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if h.store == nil || !h.store.Writable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "push_unavailable"}, "no-store")
		return
	}
	var request pushDeviceRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxPushBodyBytes))
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"}, "")
		return
	}

	token := strings.TrimSpace(request.Token)
	if token == "" || utf8.RuneCountInString(token) > maxPushTokenRunes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_token"}, "")
		return
	}
	deviceID := strings.TrimSpace(request.DeviceID)
	if deviceID == "" || utf8.RuneCountInString(deviceID) > maxPushFieldRunes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_device"}, "")
		return
	}

	device := sqlite.PushDevice{
		Token:       token,
		DeviceID:    deviceID,
		Platform:    clipRunes(request.Platform, maxPushFieldRunes),
		Locale:      clipRunes(request.Locale, 32),
		AppVersion:  clipRunes(request.AppVersion, 32),
		Enabled:     booleanOr(request.Enabled, true),
		NotifyLive:  booleanOr(request.Live, true),
		NotifyVideo: booleanOr(request.Video, true),
		NotifyPost:  booleanOr(request.Post, true),
	}
	if err := h.store.RegisterPushDevice(r.Context(), device); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "storage_unavailable"}, "no-store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true}, "no-store")
}

// booleanOr applies the default for a switch the App did not send.
func booleanOr(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

// clipRunes trims a free-form metadata field to the column's practical size.
func clipRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
