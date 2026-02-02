package crypto

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashString calculates SHA256 hash of a string.
func HashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))[:16]
}
