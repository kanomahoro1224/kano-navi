package adminapi

import (
	"net/http"
	"time"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/variants"
)

// variantMemo is the last copies status and the newest run finish it saw.
type variantMemo struct {
	finish string
	at     time.Time
	groups []variants.GroupStatus
}

// variantStatus reports how complete each group's smaller copies are. A
// listing reads directory names and remembered hashes only, never an
// original. Copies change through runs, so the last listing is answered
// again until another run finishes; refresh=1 lists anew, as after the
// variants command or files changed by hand. One listing runs at a time.
func (a *api) variantStatus(w http.ResponseWriter, r *http.Request) {
	if a.Variants == nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	finish, err := a.Store.LatestWorkflowFinish(r.Context())
	if err != nil {
		httpx.WriteCode(w, http.StatusInternalServerError, "variants_status_failed")
		return
	}
	a.variantsMu.Lock()
	defer a.variantsMu.Unlock()
	memo := a.variantMemo
	if memo.groups == nil || memo.finish != finish || r.URL.Query().Get("refresh") == "1" {
		groups, statusErr := variants.Status(r.Context(), a.Store, *a.Variants)
		if statusErr != nil {
			httpx.WriteCode(w, http.StatusInternalServerError, "variants_status_failed")
			return
		}
		memo = variantMemo{finish: finish, at: a.Now(), groups: groups}
		a.variantMemo = memo
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"groups": memo.groups, "checkedAt": sqlite.ISOTime(memo.at)})
}
