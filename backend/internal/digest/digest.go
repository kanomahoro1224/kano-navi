// Package digest holds the content hashes that stored identifiers are built
// from. Use it instead of a package-local sha256 helper.
package digest

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256Hex is the lower-case hex SHA-256 of the bytes of value, the form
// createHash("sha256").update(value).digest("hex") gave the JavaScript
// server for a UTF-8 string.
func SHA256Hex[T ~string | ~[]byte](value T) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
