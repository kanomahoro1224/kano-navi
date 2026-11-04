package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/mediavariant"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

func (h *Handler) media(w http.ResponseWriter, r *http.Request, id string) {
	version, present, valid := requestedVersion(r)
	if present && !valid {
		h.writeMediaError(w, sqlite.ErrMediaVersionNotFound, false, r.Method == http.MethodHead)
		return
	}
	width, widthPresent, widthValid := requestedWidth(r)
	if widthPresent && !widthValid {
		h.writeMediaError(w, sqlite.ErrMediaNotFound, false, r.Method == http.MethodHead)
		return
	}
	var file *sqlite.MediaStream
	var err error
	if widthPresent {
		file, err = h.store.OpenMediaVariant(r.Context(), id, version, width)
	} else {
		file, err = h.store.OpenMedia(r.Context(), id, version)
	}
	if err != nil {
		h.writeMediaError(w, err, false, r.Method == http.MethodHead)
		return
	}
	defer file.File.Close()
	serveMediaStream(w, r, file, version != "")
}

// serveMediaStream sends an opened media file with the media headers. It
// streams the file: ServeContent answers ranges, HEAD and both conditional
// validators against the stream's ETag and modification time.
func serveMediaStream(w http.ResponseWriter, r *http.Request, file *sqlite.MediaStream, immutable bool) {
	w.Header().Set("Content-Type", file.MIMEType)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", mediaCacheControl(immutable))
	w.Header().Set("ETag", `"`+file.ETag+`"`)
	serveContent(w, r, "", file.LastModified, file.File)
}

const (
	// mediaWriteTimeout bounds each write of a media body, so a client that
	// stops reading is let go while a slow one that keeps reading is not.
	mediaWriteTimeout = 30 * time.Second
	// mediaResponseLimit bounds a whole media body, about 64 KiB/s for the
	// largest archived video.
	mediaResponseLimit = 30 * time.Minute
)

// serveContent is http.ServeContent with the listener's write timeout
// replaced by per-write progress, since a large video takes longer than a
// bounded reply on a slow connection.
func serveContent(w http.ResponseWriter, r *http.Request, name string, modified time.Time, content io.ReadSeeker) {
	progress := &progressWriter{
		ResponseWriter: w,
		controller:     http.NewResponseController(w),
		end:            time.Now().Add(mediaResponseLimit),
	}
	http.ServeContent(progress, r, name, modified, content)
}

type progressWriter struct {
	http.ResponseWriter
	controller *http.ResponseController
	end        time.Time
}

func (w *progressWriter) Write(body []byte) (int, error) {
	deadline := time.Now().Add(mediaWriteTimeout)
	if deadline.After(w.end) {
		deadline = w.end
	}
	_ = w.controller.SetWriteDeadline(deadline)
	return w.ResponseWriter.Write(body)
}

func (w *progressWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (h *Handler) profileMedia(w http.ResponseWriter, r *http.Request, slot string) {
	slot = strings.ToLower(slot)
	if slot != "avatar" && slot != "banner" {
		h.writeMediaError(w, sqlite.ErrMediaNotFound, true, r.Method == http.MethodHead)
		return
	}
	version, present, valid := requestedVersion(r)
	if present && !valid {
		h.writeMediaError(w, sqlite.ErrMediaVersionNotFound, true, r.Method == http.MethodHead)
		return
	}
	file, err := h.store.ReadProfileMedia(r.Context(), slot, version)
	if err != nil {
		h.writeMediaError(w, err, true, r.Method == http.MethodHead)
		return
	}
	w.Header().Set("Content-Type", file.MIMEType)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", mediaCacheControl(version != ""))
	w.Header().Set("ETag", `"`+file.SHA256+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	if mediaFresh(r, file.SHA256, time.Time{}) {
		w.Header().Del("Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(file.Content)
	}
}

func requestedVersion(r *http.Request) (string, bool, bool) {
	values, present := r.URL.Query()["v"]
	if !present {
		return "", false, true
	}
	if len(values) != 1 || values[0] == "" {
		return "", true, false
	}
	return values[0], true, true
}

// requestedWidth reads w, which must name one of the fixed variant widths.
func requestedWidth(r *http.Request) (int, bool, bool) {
	values, present := r.URL.Query()["w"]
	if !present {
		return 0, false, true
	}
	if len(values) != 1 {
		return 0, true, false
	}
	width, err := strconv.Atoi(values[0])
	if err != nil || !mediavariant.ValidWidth(width) {
		return 0, true, false
	}
	return width, true, true
}

func (h *Handler) writeMediaError(w http.ResponseWriter, err error, profile, head bool) {
	if errors.Is(err, sqlite.ErrUnavailable) || errors.Is(err, sqlite.ErrSchema) {
		if head {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database_unavailable"}, "")
		}
		return
	}
	code := "media_not_found"
	if profile {
		switch {
		case errors.Is(err, sqlite.ErrMediaNotReady):
			code = "profile_media_not_ready"
		case errors.Is(err, sqlite.ErrMediaVersionNotFound):
			code = "profile_media_version_not_found"
		case errors.Is(err, sqlite.ErrMediaNotFound):
			code = "profile_media_not_found"
		}
	} else {
		switch {
		case errors.Is(err, sqlite.ErrMediaNotReady):
			code = "media_not_ready"
		case errors.Is(err, sqlite.ErrMediaVersionNotFound):
			code = "media_version_not_found"
		}
	}
	if head {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": code}, "")
}

func mediaCacheControl(immutable bool) string {
	if immutable {
		return "public, max-age=31536000, immutable"
	}
	return "public, max-age=0, must-revalidate"
}

func mediaFresh(r *http.Request, hash string, modified time.Time) bool {
	if raw := r.Header.Get("If-None-Match"); raw != "" {
		for _, candidate := range strings.Split(raw, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "*" {
				return true
			}
			if strings.HasPrefix(candidate, "W/") {
				candidate = strings.TrimSpace(strings.TrimPrefix(candidate, "W/"))
			}
			if candidate == `"`+hash+`"` {
				return true
			}
		}
	}
	if !modified.IsZero() {
		if raw := r.Header.Get("If-Modified-Since"); raw != "" {
			if since, err := http.ParseTime(raw); err == nil && !modified.Truncate(time.Second).After(since) {
				return true
			}
		}
	}
	return false
}
