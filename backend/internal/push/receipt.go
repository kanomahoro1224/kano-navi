package push

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const (
	// ReceiptPath is the route Huawei's receipt server posts delivery
	// statuses to; AGC's receipt configuration names the same path behind the
	// public origin.
	ReceiptPath = "/api/push/receipt"
	// MaxReceiptBodyBytes bounds one receipt batch: at most 100 statuses,
	// each carrying token-length fields.
	MaxReceiptBodyBytes = 1 << 20
	// receiptHeaderName carries the signature material.
	receiptHeaderName = "X-HUAWEI-CALLBACK-ID"
	// Receipt statuses 2 and 5 mark a token no further send can reach: the
	// app is uninstalled, or Huawei reissued the token for the device.
	receiptStatusUninstalled = 2
	receiptStatusMismatch    = 5
)

// ReceiptHandler receives Huawei Push Kit delivery receipts and retires the
// tokens they report as dead, so a stale token cannot cost one failed send
// per update forever. Every accepted batch is answered with the fixed object
// Huawei's receipt server and AGC's receipt test read as success.
type ReceiptHandler struct {
	store *sqlite.Store
	user  string
	key   []byte
}

// NewReceiptHandler returns nil while the callback secret is empty, leaving
// the route absent; callers hold the value without a branch. secret is the
// AGC receipt configuration's callback key and user its callback user name.
// AGC issues the key Base64-encoded and the documented verification decodes
// it before use; a value that does not decode is taken as the raw key of an
// older configuration.
func NewReceiptHandler(store *sqlite.Store, secret, user string) *ReceiptHandler {
	secret = strings.TrimSpace(secret)
	if store == nil || secret == "" {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		key = []byte(secret)
	}
	return &ReceiptHandler{store: store, user: strings.TrimSpace(user), key: key}
}

// ServeHTTP verifies the callback header's signature, retires the tokens the
// batch reports as uninstalled or mismatched, and answers the fixed success
// object. A rejected request never touches the store.
func (h *ReceiptHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeReceiptResult(w, http.StatusMethodNotAllowed, "405", "method not allowed")
		return
	}
	if !h.verifySignature(r.Header.Get(receiptHeaderName)) {
		writeReceiptResult(w, http.StatusUnauthorized, "401", "invalid signature")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxReceiptBodyBytes))
	if err != nil {
		writeReceiptResult(w, http.StatusBadRequest, "400", "unreadable body")
		return
	}
	// AGC's receipt test posts a fixed test message to prove reachability; an
	// empty body carries nothing to retire and is acknowledged like any
	// receipt without statuses.
	if len(bytes.TrimSpace(body)) == 0 {
		writeReceiptResult(w, http.StatusOK, "0", "success")
		return
	}
	var batch receiptBatch
	if err = json.Unmarshal(body, &batch); err != nil {
		writeReceiptResult(w, http.StatusBadRequest, "400", "invalid json")
		return
	}
	var stale []string
	for _, status := range batch.Statuses {
		token := strings.TrimSpace(status.Token)
		if token == "" {
			continue
		}
		if receiptOutcomeRetires(status.Delivery) || receiptOutcomeRetires(status.Form) {
			stale = append(stale, token)
		}
	}
	if len(stale) > 0 {
		if err = h.store.DeletePushDevices(r.Context(), stale); err != nil {
			writeReceiptResult(w, http.StatusServiceUnavailable, "503", "storage unavailable")
			return
		}
	}
	writeReceiptResult(w, http.StatusOK, "0", "success")
}

// verifySignature recomputes the documented
// Base64(HMAC-SHA256(secret, timestamp+nonce+userName)) from the callback
// header and compares it with the transmitted value in constant time.
func (h *ReceiptHandler) verifySignature(header string) bool {
	timestamp, nonce, value, ok := parseReceiptHeader(header)
	if !ok {
		return false
	}
	mac := hmac.New(sha256.New, h.key)
	_, _ = mac.Write([]byte(timestamp + nonce + h.user))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(value)) == 1
}

// parseReceiptHeader splits the "timestamp=...; nonce=...; value=..." header
// into its three parts.
func parseReceiptHeader(header string) (timestamp, nonce, value string, ok bool) {
	for _, part := range strings.Split(header, ";") {
		name, entry, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(name) {
		case "timestamp":
			timestamp = strings.TrimSpace(entry)
		case "nonce":
			nonce = strings.TrimSpace(entry)
		case "value":
			value = strings.TrimSpace(entry)
		}
	}
	return timestamp, nonce, value, timestamp != "" && nonce != "" && value != ""
}

// receiptBatch is the receipt request body: one batch carries at most 100
// statuses.
type receiptBatch struct {
	Statuses []receiptStatus `json:"statuses"`
}

// receiptStatus is one delivered message's outcome. Only the fields the
// retirement decision reads are decoded.
type receiptStatus struct {
	Token    string          `json:"token"`
	Delivery *receiptOutcome `json:"deliveryStatus"`
	Form     *receiptOutcome `json:"formStatus"`
}

// receiptOutcome is the delivery or form status of one message.
type receiptOutcome struct {
	Result int `json:"result"`
}

// receiptOutcomeRetires reports whether an outcome marks the token dead for
// every future send.
func receiptOutcomeRetires(outcome *receiptOutcome) bool {
	if outcome == nil {
		return false
	}
	return outcome.Result == receiptStatusUninstalled || outcome.Result == receiptStatusMismatch
}

// receiptResult is the fixed response object; anything but code "0" makes
// Huawei's receipt server and AGC's receipt test treat the delivery as
// failed.
type receiptResult struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeReceiptResult answers with the two-field object Huawei's receipt
// server reads.
func writeReceiptResult(w http.ResponseWriter, status int, code, message string) {
	body, err := json.Marshal(receiptResult{Code: code, Message: message})
	if err != nil {
		status = http.StatusInternalServerError
		body = []byte(`{"code":"500","message":"internal error"}`)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
