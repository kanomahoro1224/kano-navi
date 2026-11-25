package contentadmin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/mediaformat"
	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

type accountAvatarCandidateItem struct {
	ID        string  `json:"id"`
	Origin    string  `json:"origin"`
	SourceURL *string `json:"sourceUrl"`
	Status    string  `json:"status"`
	IsActive  bool    `json:"isActive"`
	MIMEType  *string `json:"mimeType"`
	ByteSize  *int64  `json:"byteSize"`
	SHA256    *string `json:"sha256"`
	Width     *int    `json:"width"`
	Height    *int    `json:"height"`
	LastError *string `json:"lastError"`
	Preview   *string `json:"previewUrl"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

type accountAvatarAdminItem struct {
	Source            string                       `json:"source"`
	AccountKey        string                       `json:"accountKey"`
	AccountHandle     string                       `json:"accountHandle"`
	Role              string                       `json:"role"`
	Keywords          []string                     `json:"keywords"`
	Mode              string                       `json:"mode"`
	ActiveCandidateID *string                      `json:"activeCandidateId"`
	PublicURL         *string                      `json:"publicUrl"`
	UpdatedAt         *string                      `json:"updatedAt"`
	Candidates        []accountAvatarCandidateItem `json:"candidates"`
}

type accountAvatarsPayload struct {
	Items []accountAvatarAdminItem `json:"items"`
}

func (m *Module) listAccountAvatarsRoute(w http.ResponseWriter, r *http.Request) {
	payload, err := m.buildAccountAvatarsPayload(r.Context())
	if err != nil {
		writeFailure(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, payload)
}

func (m *Module) discoverAccountAvatarRoute(w http.ResponseWriter, r *http.Request, source, handle string) {
	src, key, display, ok := sqlite.NormalizeAccountAvatarKey(source, handle)
	if !ok {
		httpx.WriteError(w, httpx.Invalid("account source or handle is invalid"))
		return
	}
	urls, fetchErr := m.discoverAccountAvatarURLs(r.Context(), display)
	if len(urls) == 0 {
		if fetchErr != nil {
			writeFailure(w, fetchErr)
			return
		}
		httpx.WriteCode(w, http.StatusUnprocessableEntity, "account_avatar_not_discovered")
		return
	}
	best := urls[0]
	_, cand, err := m.store.RecordAccountAvatarURL(r.Context(), src, display, sqlite.AccountAvatarOriginDiscover, best, false)
	if err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	stored, err := m.downloadAndVerifyAvatarURL(r.Context(), src, best)
	if err != nil {
		_ = m.store.MarkAccountAvatarCandidateFailed(r.Context(), cand.ID, failureCode())
		writeFailure(w, err)
		return
	}
	if _, err = m.store.SaveDownloadedAccountAvatarCandidate(
		r.Context(), cand.ID, stored.relativePath, stored.mimeType, stored.byteSize,
		stored.sha256, intOrZero(stored.width), intOrZero(stored.height), "", "", true,
	); err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	m.writeAccountAvatarState(w, r, http.StatusOK, src, key)
}

func (m *Module) addAccountAvatarURLRoute(w http.ResponseWriter, r *http.Request, source, handle string) {
	src, key, display, ok := sqlite.NormalizeAccountAvatarKey(source, handle)
	if !ok {
		httpx.WriteError(w, httpx.Invalid("account source or handle is invalid"))
		return
	}
	var body struct {
		SourceURL *string `json:"sourceUrl"`
		Activate  *bool   `json:"activate"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxProfileImageBytes)).Decode(&body); err != nil {
		httpx.WriteCode(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if body.SourceURL == nil || strings.TrimSpace(*body.SourceURL) == "" {
		httpx.WriteError(w, httpx.Invalid("sourceUrl is required"))
		return
	}
	normalized := xsource.NormalizeAvatarURL(*body.SourceURL)
	if normalized == "" {
		normalized = strings.TrimSpace(*body.SourceURL)
	}
	validatedURL, valid := profileImageURL(normalized)
	if !valid {
		httpx.WriteError(w, httpx.Invalid("sourceUrl must be an allowed HTTPS avatar URL"))
		return
	}
	activate := body.Activate == nil || *body.Activate

	_, cand, err := m.store.RecordAccountAvatarURL(r.Context(), src, display, sqlite.AccountAvatarOriginURL, validatedURL, false)
	if err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	stored, err := m.downloadAndVerifyAvatarURL(r.Context(), src, validatedURL)
	if err != nil {
		_ = m.store.MarkAccountAvatarCandidateFailed(r.Context(), cand.ID, failureCode())
		writeFailure(w, err)
		return
	}
	if _, err = m.store.SaveDownloadedAccountAvatarCandidate(
		r.Context(), cand.ID, stored.relativePath, stored.mimeType, stored.byteSize,
		stored.sha256, intOrZero(stored.width), intOrZero(stored.height), "", "", !activate,
	); err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	if activate {
		if _, err = m.store.SelectAccountAvatarCandidate(r.Context(), src, display, cand.ID); err != nil {
			m.writeAccountAvatarError(w, err)
			return
		}
	}
	m.writeAccountAvatarState(w, r, http.StatusCreated, src, key)
}

func (m *Module) uploadAccountAvatarRoute(w http.ResponseWriter, r *http.Request, source, handle string) {
	src, key, display, ok := sqlite.NormalizeAccountAvatarKey(source, handle)
	if !ok {
		httpx.WriteError(w, httpx.Invalid("account source or handle is invalid"))
		return
	}
	image, uploaded := m.readUploadedImage(w, r)
	if !uploaded {
		return
	}
	if _, _, err := m.store.SaveUploadedAccountAvatar(
		r.Context(), src, display, image.relativePath, image.mimeType,
		image.byteSize, image.sha256, intOrZero(image.width), intOrZero(image.height), true,
	); err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	m.writeAccountAvatarState(w, r, http.StatusCreated, src, key)
}

func (m *Module) setAccountAvatarModeRoute(w http.ResponseWriter, r *http.Request, source, handle string) {
	src, key, display, ok := sqlite.NormalizeAccountAvatarKey(source, handle)
	if !ok {
		httpx.WriteError(w, httpx.Invalid("account source or handle is invalid"))
		return
	}
	var body struct {
		Mode *string `json:"mode"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteCode(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if body.Mode == nil {
		httpx.WriteError(w, httpx.Invalid("mode is required"))
		return
	}
	if _, err := m.store.SetAccountAvatarMode(r.Context(), src, display, *body.Mode); err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	m.writeAccountAvatarState(w, r, http.StatusOK, src, key)
}

func (m *Module) selectAccountAvatarCandidateRoute(w http.ResponseWriter, r *http.Request, source, handle, candidateID string) {
	src, key, display, ok := sqlite.NormalizeAccountAvatarKey(source, handle)
	if !ok {
		httpx.WriteError(w, httpx.Invalid("account source or handle is invalid"))
		return
	}
	if _, err := m.store.SelectAccountAvatarCandidate(r.Context(), src, display, candidateID); err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	m.writeAccountAvatarState(w, r, http.StatusOK, src, key)
}

func (m *Module) downloadAccountAvatarCandidateRoute(w http.ResponseWriter, r *http.Request, source, handle, candidateID string) {
	src, key, _, ok := sqlite.NormalizeAccountAvatarKey(source, handle)
	if !ok {
		httpx.WriteError(w, httpx.Invalid("account source or handle is invalid"))
		return
	}
	cand, err := m.store.GetAccountAvatarCandidate(r.Context(), src, key, candidateID)
	if err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	if cand.SourceURL == "" {
		httpx.WriteError(w, httpx.Invalid("candidate has no remote source URL"))
		return
	}
	validatedURL, valid := profileImageURL(cand.SourceURL)
	if !valid {
		httpx.WriteError(w, httpx.Invalid("candidate sourceUrl host is not allowed"))
		return
	}
	stored, err := m.downloadAndVerifyAvatarURL(r.Context(), src, validatedURL)
	if err != nil {
		_ = m.store.MarkAccountAvatarCandidateFailed(r.Context(), cand.ID, failureCode())
		writeFailure(w, err)
		return
	}
	if _, err = m.store.SaveDownloadedAccountAvatarCandidate(
		r.Context(), cand.ID, stored.relativePath, stored.mimeType, stored.byteSize,
		stored.sha256, intOrZero(stored.width), intOrZero(stored.height), "", "", true,
	); err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	m.writeAccountAvatarState(w, r, http.StatusOK, src, key)
}

func (m *Module) deleteAccountAvatarCandidateRoute(w http.ResponseWriter, r *http.Request, source, handle, candidateID string) {
	src, key, display, ok := sqlite.NormalizeAccountAvatarKey(source, handle)
	if !ok {
		httpx.WriteError(w, httpx.Invalid("account source or handle is invalid"))
		return
	}
	if _, err := m.store.DeleteAccountAvatarCandidate(r.Context(), src, display, candidateID); err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	m.writeAccountAvatarState(w, r, http.StatusOK, src, key)
}

func (m *Module) previewAccountAvatarCandidateRoute(w http.ResponseWriter, r *http.Request, source, handle, candidateID string) {
	src, key, _, ok := sqlite.NormalizeAccountAvatarKey(source, handle)
	if !ok {
		httpx.WriteCode(w, http.StatusNotFound, "account_avatar_not_found")
		return
	}
	if _, err := m.store.GetAccountAvatarCandidate(r.Context(), src, key, candidateID); err != nil {
		if errors.Is(err, sqlite.ErrAccountAvatarNotFound) || errors.Is(err, sqlite.ErrAccountAvatarInvalid) {
			httpx.WriteCode(w, http.StatusNotFound, "account_avatar_not_found")
			return
		}
		writeFailure(w, err)
		return
	}
	file, err := m.store.ReadAccountAvatarCandidate(r.Context(), candidateID)
	if err != nil {
		if errors.Is(err, sqlite.ErrMediaNotReady) {
			httpx.WriteCode(w, http.StatusNotFound, "account_avatar_not_ready")
			return
		}
		httpx.WriteCode(w, http.StatusNotFound, "account_avatar_not_found")
		return
	}
	if mediaformat.Sniff(file.Content) != file.MIMEType {
		httpx.WriteCode(w, http.StatusNotFound, "account_avatar_not_found")
		return
	}
	writeImagePreview(w, r, file.MIMEType, file.Content)
}

func (m *Module) applyAccountAvatarToProfileRoute(w http.ResponseWriter, r *http.Request, source, handle string) {
	src, key, display, ok := sqlite.NormalizeAccountAvatarKey(source, handle)
	if !ok {
		httpx.WriteError(w, httpx.Invalid("account source or handle is invalid"))
		return
	}
	var body struct {
		CandidateID *string `json:"candidateId"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil && !strings.Contains(err.Error(), "EOF") {
			httpx.WriteCode(w, http.StatusBadRequest, "invalid_json")
			return
		}
	}
	avatarState, err := m.store.GetAccountAvatar(r.Context(), src, key)
	if err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	candID := avatarState.ActiveCandidateID
	if body.CandidateID != nil && strings.TrimSpace(*body.CandidateID) != "" {
		candID = strings.TrimSpace(*body.CandidateID)
	}
	if candID == "" {
		httpx.WriteError(w, httpx.Invalid("account has no active ready avatar candidate"))
		return
	}
	cand, err := m.store.GetAccountAvatarCandidate(r.Context(), src, key, candID)
	if err != nil {
		m.writeAccountAvatarError(w, err)
		return
	}
	if cand.Status != "ready" {
		httpx.WriteError(w, httpx.Invalid("avatar candidate must be ready first"))
		return
	}
	file, err := m.store.ReadAccountAvatarCandidate(r.Context(), cand.ID)
	if err != nil {
		writeFailure(w, err)
		return
	}
	profileSource := "x"
	var sourceURLPtr *string
	sourceRef := "account-avatar:" + display
	if cand.Origin == sqlite.AccountAvatarOriginUpload {
		profileSource = "upload"
	} else if cand.SourceURL != "" {
		u := cand.SourceURL
		sourceURLPtr = &u
	}
	stored, err := m.storeProfileImage(profileSource, file.Content, file.MIMEType, "account avatar is not a supported image")
	if err != nil {
		writeFailure(w, err)
		return
	}
	now := m.timestamp()
	var profileID string
	err = m.inTx(r.Context(), func(tx *sql.Tx) error {
		var upsertErr error
		if profileID, upsertErr = upsertProfileCandidate(r.Context(), tx, profileCandidate{
			Slot: "avatar", Source: profileSource, SourceRef: &sourceRef, SourceURL: sourceURLPtr,
		}, now); upsertErr != nil {
			return upsertErr
		}
		return markProfileMediaReady(r.Context(), tx, profileID, stored, now)
	})
	if err != nil {
		writeFailure(w, err)
		return
	}
	if err = m.selectProfileMedia(r.Context(), profileID, "avatar"); err != nil {
		writeFailure(w, err)
		return
	}
	profilePayload, err := m.profilePayload(r.Context())
	if err != nil {
		writeFailure(w, err)
		return
	}
	avatarsPayload, err := m.buildAccountAvatarsPayload(r.Context())
	if err != nil {
		writeFailure(w, err)
		return
	}
	var current *accountAvatarAdminItem
	for i := range avatarsPayload.Items {
		if avatarsPayload.Items[i].Source == src && avatarsPayload.Items[i].AccountKey == key {
			current = &avatarsPayload.Items[i]
			break
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"item":    current,
		"items":   avatarsPayload.Items,
		"profile": profilePayload,
	})
}

func (m *Module) writeAccountAvatarState(w http.ResponseWriter, r *http.Request, status int, src, key string) {
	payload, err := m.buildAccountAvatarsPayload(r.Context())
	if err != nil {
		writeFailure(w, err)
		return
	}
	var current *accountAvatarAdminItem
	for i := range payload.Items {
		if payload.Items[i].Source == src && payload.Items[i].AccountKey == key {
			current = &payload.Items[i]
			break
		}
	}
	httpx.WriteJSON(w, status, map[string]any{
		"item":  current,
		"items": payload.Items,
	})
}

func (m *Module) writeAccountAvatarError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sqlite.ErrReadOnly):
		httpx.WriteCode(w, http.StatusServiceUnavailable, "read_only")
	case errors.Is(err, sqlite.ErrAccountAvatarInvalid):
		httpx.WriteError(w, httpx.Invalid("invalid account avatar request"))
	case errors.Is(err, sqlite.ErrAccountAvatarNotFound):
		httpx.WriteCode(w, http.StatusNotFound, "account_avatar_not_found")
	case errors.Is(err, sqlite.ErrAccountAvatarNotReady):
		httpx.WriteError(w, httpx.Invalid("avatar candidate is not ready"))
	default:
		writeFailure(w, err)
	}
}

func (m *Module) buildAccountAvatarsPayload(ctx context.Context) (*accountAvatarsPayload, error) {
	stored, err := m.store.ListAccountAvatars(ctx)
	if err != nil {
		return nil, err
	}
	storedByKey := map[string]sqlite.AccountAvatar{}
	for _, s := range stored {
		storedByKey[s.Source+":"+s.AccountKey] = s
	}
	related, err := m.store.XRelatedAccounts(ctx)
	if err != nil {
		return nil, err
	}
	// Archive-only accounts are Kano's own too, though never synchronized.
	archived, err := m.store.XArchiveAccounts(ctx)
	if err != nil {
		return nil, err
	}
	own := append(append([]string{}, m.xHandles...), archived...)

	items := make([]accountAvatarAdminItem, 0, len(own)+len(related)+len(stored))
	seen := map[string]bool{}

	appendItem := func(src, key, display, role string, keywords []string) {
		mapKey := src + ":" + key
		if seen[mapKey] {
			return
		}
		seen[mapKey] = true
		rec, hasRec := storedByKey[mapKey]
		mode := sqlite.AccountAvatarModeAuto
		var activeIDPtr, publicURLPtr, updatedAtPtr *string
		if hasRec {
			if rec.AccountHandle != "" {
				display = rec.AccountHandle
			}
			mode = rec.Mode
			if rec.ActiveCandidateID != "" {
				idCopy := rec.ActiveCandidateID
				activeIDPtr = &idCopy
			}
			if rec.UpdatedAt != "" {
				uCopy := rec.UpdatedAt
				updatedAtPtr = &uCopy
			}
			if rec.Mode != sqlite.AccountAvatarModeHidden && rec.Active != nil {
				if pub := rec.Active.PublicURL(); pub != "" {
					pubCopy := pub
					publicURLPtr = &pubCopy
				}
			}
		}
		cands := make([]accountAvatarCandidateItem, 0, len(rec.Candidates))
		for _, c := range rec.Candidates {
			var srcURLPtr, mimePtr, shaPtr, errPtr, previewPtr *string
			var sizePtr *int64
			var wPtr, hPtr *int
			if c.SourceURL != "" {
				v := c.SourceURL
				srcURLPtr = &v
			}
			if c.MIMEType != "" {
				v := c.MIMEType
				mimePtr = &v
			}
			if c.ByteSize > 0 {
				v := c.ByteSize
				sizePtr = &v
			}
			if c.SHA256 != "" {
				v := c.SHA256
				shaPtr = &v
			}
			if c.Width > 0 {
				v := c.Width
				wPtr = &v
			}
			if c.Height > 0 {
				v := c.Height
				hPtr = &v
			}
			if c.LastError != "" {
				v := c.LastError
				errPtr = &v
			}
			if c.Status == "ready" {
				v := "/api/admin/account-avatars/" + encodeURIComponent(src) + "/" + encodeURIComponent(key) + "/candidates/" + encodeURIComponent(c.ID) + "/preview"
				previewPtr = &v
			}
			cands = append(cands, accountAvatarCandidateItem{
				ID:        c.ID,
				Origin:    c.Origin,
				SourceURL: srcURLPtr,
				Status:    c.Status,
				IsActive:  rec.ActiveCandidateID != "" && rec.ActiveCandidateID == c.ID,
				MIMEType:  mimePtr,
				ByteSize:  sizePtr,
				SHA256:    shaPtr,
				Width:     wPtr,
				Height:    hPtr,
				LastError: errPtr,
				Preview:   previewPtr,
				CreatedAt: c.CreatedAt,
				UpdatedAt: c.UpdatedAt,
			})
		}
		if keywords == nil {
			keywords = []string{}
		}
		items = append(items, accountAvatarAdminItem{
			Source:            src,
			AccountKey:        key,
			AccountHandle:     display,
			Role:              role,
			Keywords:          append([]string{}, keywords...),
			Mode:              mode,
			ActiveCandidateID: activeIDPtr,
			PublicURL:         publicURLPtr,
			UpdatedAt:         updatedAtPtr,
			Candidates:        cands,
		})
	}

	for _, rawHandle := range own {
		src, key, display, ok := sqlite.NormalizeAccountAvatarKey("x", rawHandle)
		if ok {
			appendItem(src, key, display, "own", nil)
		}
	}
	for _, rel := range related {
		src, key, display, ok := sqlite.NormalizeAccountAvatarKey("x", rel.Handle)
		if ok {
			appendItem(src, key, display, "related", rel.Keywords)
		}
	}
	for _, s := range stored {
		appendItem(s.Source, s.AccountKey, s.AccountHandle, "other", nil)
	}
	return &accountAvatarsPayload{Items: items}, nil
}

func (m *Module) discoverAccountAvatarURLs(ctx context.Context, handle string) ([]string, error) {
	seen := map[string]bool{}
	found := []string{}
	add := func(raw string) {
		norm := xsource.NormalizeAvatarURL(raw)
		if norm == "" {
			norm = strings.TrimSpace(raw)
		}
		clean, ok := profileImageURL(norm)
		if ok && !seen[clean] {
			seen[clean] = true
			found = append(found, clean)
		}
	}

	fxAvatars, _, _, lastErr := m.discoverFxTwitterMedia(ctx, handle)
	for _, candidate := range fxAvatars {
		add(candidate)
	}
	if len(found) > 0 {
		return found, nil
	}

	pageURL := "https://x.com/" + url.PathEscape(handle)
	if res, err := m.fetchAllowed(ctx, pageURL, pageFetch); err != nil {
		if lastErr == nil {
			lastErr = err
		}
	} else if res.ok() {
		for _, candidate := range discoveredImageURLs(string(res.body), "avatar") {
			add(candidate)
		}
	} else if lastErr == nil {
		lastErr = httpStatusFailure(pageFetch, res.status)
	}

	return found, lastErr
}

func (m *Module) downloadAndVerifyAvatarURL(ctx context.Context, source, targetURL string) (*storedImage, error) {
	res, err := m.fetchAllowed(ctx, targetURL, imageFetch)
	if err != nil {
		return nil, err
	}
	if !res.ok() {
		return nil, httpStatusFailure(imageFetch, res.status)
	}
	return m.storeProfileImage(source, res.body, res.contentType, "downloaded account avatar is not a valid image")
}

func intOrZero(v *int64) int {
	if v == nil || *v <= 0 {
		return 0
	}
	return int(*v)
}

func failureCode() string {
	return "account_avatar_download_failed"
}
