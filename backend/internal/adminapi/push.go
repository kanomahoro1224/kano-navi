package adminapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
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
	Configured bool            `json:"configured"`
	ProjectID  string          `json:"projectId"`
	KeyID      string          `json:"keyId"`
	SubAccount string          `json:"subAccount"`
	Category   string          `json:"category"`
	TestMode   bool            `json:"testMode"`
	Devices    []pushDevice    `json:"devices"`
	Web        webPushSnapshot `json:"web"`
}

// webPushRow is one browser in the console list. The endpoint is masked like a
// device token: enough to recognise the row, not enough to deliver to it.
type webPushRow struct {
	Endpoint      string `json:"endpoint"`
	EndpointLabel string `json:"endpointLabel"`
	Locale        string `json:"locale"`
	UserAgent     string `json:"userAgent"`
	Enabled       bool   `json:"enabled"`
	NotifyLive    bool   `json:"notifyLive"`
	NotifyVideo   bool   `json:"notifyVideo"`
	NotifyPost    bool   `json:"notifyPost"`
	FailureCount  int    `json:"failureCount"`
	CreatedAt     string `json:"createdAt"`
	LastSeenAt    string `json:"lastSeenAt"`
}

type webPushSnapshot struct {
	Configured    bool         `json:"configured"`
	Total         int          `json:"total"`
	Enabled       int          `json:"enabled"`
	Subscriptions []webPushRow `json:"subscriptions"`
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
	status.Web = webPushSnapshot{Subscriptions: []webPushRow{}}
	if a.WebPush != nil && a.WebPush.Enabled() {
		status.Web.Configured = true
		total, enabled, statsErr := a.Store.WebPushSubscriptionStats(r.Context())
		if statsErr != nil {
			httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
			return
		}
		status.Web.Total, status.Web.Enabled = total, enabled
		subscriptions, listErr := a.Store.AdminWebPushSubscriptions(r.Context(), 20)
		if listErr != nil {
			httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
			return
		}
		for _, sub := range subscriptions {
			status.Web.Subscriptions = append(status.Web.Subscriptions, webPushRow{
				Endpoint:      sub.Endpoint,
				EndpointLabel: maskPushEndpoint(sub.Endpoint),
				Locale:        sub.Locale,
				UserAgent:     sub.UserAgent,
				Enabled:       sub.Enabled,
				NotifyLive:    sub.NotifyLive,
				NotifyVideo:   sub.NotifyVideo,
				NotifyPost:    sub.NotifyPost,
				FailureCount:  sub.FailureCount,
				CreatedAt:     sub.CreatedAt,
				LastSeenAt:    sub.LastSeenAt,
			})
		}
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
	body, err := httpx.Body(w, r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	// The panel sends the same probe to either transport; the default stays
	// Huawei so an existing caller keeps working.
	var transport string
	if raw, ok := body["transport"]; ok {
		if err = json.Unmarshal(raw, &transport); err != nil {
			httpx.WriteError(w, httpx.Invalid("transport must be a string"))
			return
		}
	}
	if strings.EqualFold(transport, "web") {
		a.pushTestWeb(w, r, body)
		return
	}
	if a.Push == nil || !a.Push.Enabled() {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "push_unavailable")
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

// pushTestWeb sends the sample notification to every browser, or only to the
// one endpoint when given. A gone subscription is retired here as well.
func (a *api) pushTestWeb(w http.ResponseWriter, r *http.Request, body map[string]json.RawMessage) {
	if a.WebPush == nil || !a.WebPush.Enabled() {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "push_unavailable")
		return
	}
	var endpoint string
	if raw, ok := body["endpoint"]; ok {
		if err := json.Unmarshal(raw, &endpoint); err != nil {
			httpx.WriteError(w, httpx.Invalid("endpoint must be a string"))
			return
		}
	}
	subscriptions, err := a.Store.WebPushSubscriptions(r.Context())
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	sent, stale := 0, 0
	firstErr := ""
	for _, sub := range subscriptions {
		if endpoint != "" && sub.Endpoint != endpoint {
			continue
		}
		sent++
		sendErr := a.WebPush.Send(r.Context(), sub, pushkit.TestWebPayload(sub.Locale))
		switch {
		case sendErr == nil:
			_ = a.Store.ClearWebPushFailures(r.Context(), sub.Endpoint)
		case errors.Is(sendErr, pushkit.ErrWebPushGone):
			stale++
			_ = a.Store.DeleteWebPushSubscription(r.Context(), sub.Endpoint)
		default:
			if firstErr == "" {
				firstErr = sendErr.Error()
			}
			_ = a.Store.RecordWebPushFailure(r.Context(), sub.Endpoint, 10)
		}
	}
	if endpoint != "" && sent == 0 {
		httpx.WriteCode(w, http.StatusNotFound, "not_found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":            firstErr == "",
		"transport":     "web",
		"subscriptions": sent,
		"stale":         stale,
		"error":         firstErr,
	})
}

// deleteWebPushSubscription retires one browser the operator no longer wants
// to notify. The endpoint travels as a query value because it contains slashes.
func (a *api) deleteWebPushSubscription(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.TrimSpace(r.URL.Query().Get("endpoint"))
	if endpoint == "" {
		httpx.WriteCode(w, http.StatusBadRequest, "invalid_endpoint")
		return
	}
	if err := a.Store.DeleteWebPushSubscription(r.Context(), endpoint); err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maskPushEndpoint keeps the push service host and the tail of the path, which
// is what an operator recognises a browser by, and drops the rest.
func maskPushEndpoint(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "…"
	}
	tail := parsed.Path
	if len(tail) > 8 {
		tail = tail[len(tail)-8:]
	}
	return parsed.Host + "/…" + tail
}

// maskPushToken keeps the head and tail of a push token, which is what a
// device row is recognised by, and drops the middle.
func maskPushToken(token string) string {
	if len(token) <= 12 {
		return strings.Repeat("•", 4)
	}
	return token[:6] + "…" + token[len(token)-4:]
}
