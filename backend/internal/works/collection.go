package works

import _ "embed"

// scripts/export-works-api.ts writes these bodies from the Works page's
// tracked data; they ship with each release and the server never changes
// them.
var (
	//go:embed catalog.json
	catalog []byte
	//go:embed upload-list.json
	uploadList []byte
)

// Catalog is the body of GET /api/works, as exported.
func Catalog() []byte { return catalog }

// UploadList is the body of GET /api/works/upload-list, as exported.
func UploadList() []byte { return uploadList }
