package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

// writePrivateJSON sets a private cache validator for stable GET payloads.
// Authentication responses stay no-store at their own handlers.
func writePrivateJSON(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
