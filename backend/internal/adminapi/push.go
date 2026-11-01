package adminapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	pushkit "github.com/yexca/kano-navi/backend/internal/push"
)

// pushDevice is one row of the console's device list. The token is masked:
// enough to match a device against the App and the logs, not enough to deliver
// to it from outside the server.
type pushDevice struct {
	DeviceID    string `json:"deviceId"`
	Token       string `json:"token"`
	Platform    string `json:"platform"`
	Locale      string `json:"locale"`
	AppVersion  string `json:"appVersion"`
	Enabled     bool   `json:"enabled"`
	NotifyLive  bool   `json:"notifyLive"`
	NotifyVideo bool   `json:"notifyVideo"`
	NotifyPost  bool   `json:"notifyPost"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	LastSeenAt  string `json:"lastSeenAt"`
}

type pushSnapshot struct {
	Configured bool         `json:"configured"`
	ProjectID  string       `json:"projectId"`
	KeyID      string       `json:"keyId"`
	SubAccount string       `json:"subAccount"`
	Category   string       `json:"category"`
	TestMode   bool         `json:"testMode"`
	Devices    []pushDevice `json:"devices"`
}

// pushPanel answers the console's Push Kit panel: whether a service account
// is loaded and which devices registered themselves.
func (a *api) pushPanel(w http.ResponseWriter, r *http.Request) {
	devices, err := a.Store.AdminPushDevices(r.Context())
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	status := pushSnapshot{Devices: make([]pushDevice, 0, len(devices))}
	if a.Push != nil && a.Push.Enabled() {
		status.Configured = true
		status.ProjectID = a.Push.ProjectID()
		status.KeyID = a.Push.KeyID()
		status.SubAccount = a.Push.SubAccount()
		status.Category = a.Push.Category()
		status.TestMode = a.Push.TestMode()
	}
	for _, device := range devices {
		status.Devices = append(status.Devices, pushDevice{
			DeviceID:    device.DeviceID,
			Token:       maskPushToken(device.Token),
			Platform:    device.Platform,
			Locale:      device.Locale,
			AppVersion:  device.AppVersion,
			Enabled:     device.Enabled,
			NotifyLive:  device.NotifyLive,
			NotifyVideo: device.NotifyVideo,
			NotifyPost:  device.NotifyPost,
			CreatedAt:   device.CreatedAt,
			UpdatedAt:   device.UpdatedAt,
			LastSeenAt:  device.LastSeenAt,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, status)
}

// pushTest sends one test notification, to a single device or to every device
// that still accepts updates. A rejected token is retired here as well, so a
// test does the same cleanup a real dispatch would.
func (a *api) pushTest(w http.ResponseWriter, r *http.Request) {
	if a.Push == nil || !a.Push.Enabled() {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "push_unavailable")
		return
	}
	body, err := httpx.Body(w, r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var deviceID string
	if raw, ok := body["deviceId"]; ok {
		if err = json.Unmarshal(raw, &deviceID); err != nil {
			httpx.WriteError(w, httpx.Invalid("deviceId must be a string"))
			return
		}
	}
	devices, err := a.Store.AdminPushDevices(r.Context())
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	byLocale := map[string][]string{}
	total := 0
	for _, device := range devices {
		if !device.Enabled || (deviceID != "" && device.DeviceID != deviceID) {
			continue
		}
		byLocale[device.Locale] = append(byLocale[device.Locale], device.Token)
		total++
	}
	if deviceID != "" && total == 0 {
		httpx.WriteCode(w, http.StatusNotFound, "not_found")
		return
	}
	var stale []string
	var firstErr string
	for locale, tokens := range byLocale {
		rejected, sendErr := a.Push.Send(r.Context(), pushkit.TestNotification(locale), tokens)
		stale = append(stale, rejected...)
		if sendErr != nil && firstErr == "" {
			firstErr = sendErr.Error()
		}
	}
	if len(stale) > 0 {
		_ = a.Store.DeletePushDevices(r.Context(), stale)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":      firstErr == "",
		"devices": total,
		"stale":   len(stale),
		"error":   firstErr,
	})
}

// deletePushDevice retires one device the operator no longer uses.
func (a *api) deletePushDevice(w http.ResponseWriter, r *http.Request) {
	if err := a.Store.DeletePushDevice(r.Context(), r.PathValue("id")); err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maskPushToken keeps the head and tail of a push token, which is what a
// device row is recognised by, and drops the middle.
func maskPushToken(token string) string {
	if len(token) <= 12 {
		return strings.Repeat("•", 4)
	}
	return token[:6] + "…" + token[len(token)-4:]
}
