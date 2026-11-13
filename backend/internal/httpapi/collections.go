package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/yexca/kano-navi/backend/internal/journey"
	"github.com/yexca/kano-navi/backend/internal/works"
)

// preparedJSON is a fixed public body that ships with the build: compacted
// and given its validator once, then served unchanged on every read.
type preparedJSON struct {
	source func() []byte
	once   sync.Once
	body   []byte
	etag   string
}

func (p *preparedJSON) load() ([]byte, string) {
	p.once.Do(func() {
		var compact bytes.Buffer
		if json.Compact(&compact, p.source()) != nil {
			return
		}
		p.body = compact.Bytes()
		p.etag = revalidatedETag(p.body)
	})
	return p.body, p.etag
}

func (p *preparedJSON) serve(w http.ResponseWriter, r *http.Request) {
	body, etag := p.load()
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
