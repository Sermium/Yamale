package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// Files are stored by their SHA-256 and served under it. Nobody can replace
// one: a different body is a different name. That is what lets the app check
// every photo and record against the fingerprint the chain holds.

const maxFileBytes = 8 << 20

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (s *server) filePath(hash string) string {
	return filepath.Join(s.dir, "files", hash[:2], hash)
}

func fileType(b []byte) string {
	switch t := http.DetectContentType(b); t {
	case "image/jpeg", "image/png", "image/webp":
		return t
	}
	if json.Valid(b) {
		return "application/json"
	}
	return ""
}

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFileBytes))
	if err != nil {
		refuse(w, http.StatusRequestEntityTooLarge, "a file may be at most 8 MiB")
		return
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])

	address, err := s.signer(r, "file", func(string) string { return hash })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	js, err := s.chain.Jewelers(r.Context())
	if err != nil {
		refuse(w, http.StatusBadGateway, err.Error())
		return
	}
	if _, ok := jewelerOf(js, address); !ok {
		refuse(w, http.StatusForbidden, "only the staff of an active jeweler can store files")
		return
	}
	ctype := fileType(body)
	if ctype == "" {
		refuse(w, http.StatusUnsupportedMediaType, "a photo must be JPEG, PNG or WebP, and a record must be JSON")
		return
	}

	path := s.filePath(hash)
	if _, err := os.Stat(path); err == nil {
		writeJSON(w, http.StatusCreated, map[string]string{"hash": hash})
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := writeAtomically(path+".type", []byte(ctype)); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := writeAtomically(path, body); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"hash": hash})
}

func (s *server) handleFile(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if !hashPattern.MatchString(hash) {
		refuse(w, http.StatusBadRequest, "a file is named by 64 lowercase hex characters")
		return
	}
	body, err := os.ReadFile(s.filePath(hash))
	if err != nil {
		refuse(w, http.StatusNotFound, "no such file")
		return
	}
	ctype, err := os.ReadFile(s.filePath(hash) + ".type")
	if err != nil {
		ctype = []byte("application/octet-stream")
	}
	w.Header().Set("Content-Type", string(ctype))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+hash+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}

// writeAtomically writes beside the target and renames, so a reader never
// sees half a file and a crash leaves either the old state or the new one.
func writeAtomically(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
