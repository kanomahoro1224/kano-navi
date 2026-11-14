package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/yexca/kano-navi/backend/internal/cdn"
	"github.com/yexca/kano-navi/backend/internal/journey"
	"github.com/yexca/kano-navi/backend/internal/works"
)

// preparedJSON is a fixed public body that ships with the build: compacted,
// moved onto the CDN base when one is set, and given its validator once per
// base, then served unchanged on every read.
type preparedJSON struct {
	source func() []byte
	mu     sync.Mutex
	bodies map[cdn.Base]preparedBody
}

// preparedBody is a prepared JSON body and its ETag; a nil body means the
// source did not parse.
type preparedBody struct {
	body []byte
	etag string
}

func (p *preparedJSON) load(base cdn.Base) ([]byte, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if prepared, ok := p.bodies[base]; ok {
		return prepared.body, prepared.etag
	}
	var prepared preparedBody
	var compact bytes.Buffer
	if json.Compact(&compact, p.source()) == nil {
		// The ETag covers the rewritten body, so a CDN deployment and an
		// origin deployment never validate each other's copies.
		if body, err := base.JSON(compact.Bytes()); err == nil {
			prepared = preparedBody{body: body, etag: revalidatedETag(body)}
		}
	}
	if p.bodies == nil {
		p.bodies = map[cdn.Base]preparedBody{}
	}
	p.bodies[base] = prepared
	return prepared.body, prepared.etag
}

func (p *preparedJSON) serve(w http.ResponseWriter, r *http.Request, base cdn.Base) {
	body, etag := p.load(base)
	if body == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"}, "")
		return
	}
	writePreparedJSON(w, r, body, etag)
}

// The Journey and Works collections: their tracked page data as exported
// into the build, never joined with the snapshot, so they carry no revision.
var (
	journeyCollection         = &preparedJSON{source: journey.Packaged}
	worksCollection           = &preparedJSON{source: works.Catalog}
	worksUploadListCollection = &preparedJSON{source: works.UploadList}
)
