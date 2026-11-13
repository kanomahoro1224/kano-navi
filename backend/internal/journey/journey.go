// Package journey holds the body of GET /api/journey: the Journey page's
// branches and milestones with their images. scripts/export-journey-api.ts
// writes journey.json from the page's tracked data; it ships with each
// release and the server never changes it.
package journey

import _ "embed"

//go:embed journey.json
var packaged []byte

// Packaged is the body this build ships with, as exported.
func Packaged() []byte { return packaged }
