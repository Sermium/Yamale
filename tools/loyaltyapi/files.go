package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
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

const (
	maxUploads       = 4
	uploadDeadline   = 5 * time.Minute
	maxJSONFileBytes = 1 << 20 // a record is small; only images may be large
)

// acquire takes one of the upload slots, waiting briefly. The caller must
// release it.
func (s *server) acquire() bool {
	s.slotsOnce.Do(func() {
		if s.slots == nil {
			s.slots = make(chan struct{}, maxUploads)
		}
	})
	wait := s.slotWait
	if wait == 0 {
		wait = 2 * time.Second
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case s.slots <- struct{}{}:
		return true
	case <-t.C:
		return false
	}
}

func (s *server) release() { <-s.slots }

// readFailed answers a body that could not be read: 413 only if it was over
// the limit, otherwise the client's upload simply did not complete.
func readFailed(w http.ResponseWriter, err error, tooBig string) {
	var big *http.MaxBytesError
	if errors.As(err, &big) {
		refuse(w, http.StatusRequestEntityTooLarge, tooBig)
		return
	}
	refuse(w, http.StatusBadRequest, "the upload did not complete")
}

// handleUpload refuses strangers from the headers and the chain before it
// reads a byte of the body, streams the body to disk while hashing it, and
// moves it into place only once the signature over that hash verifies.
func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if !s.acquire() {
		w.Header().Set("Retry-After", "5")
		refuse(w, http.StatusServiceUnavailable, "the server is busy storing other files; try again shortly")
		return
	}
	defer s.release()
	_ = http.NewResponseController(w).SetReadDeadline(s.now().Add(uploadDeadline))

	c, err := s.claimed(r)
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	js, err := s.jewelers(r.Context())
	if err != nil {
		refuse(w, http.StatusBadGateway, err.Error())
		return
	}
	if _, ok := jewelerOf(js, c.address); !ok {
		refuse(w, http.StatusForbidden, "only the staff of an active jeweler can store files")
		return
	}

	filesDir := filepath.Join(s.dir, "files")
	if err := os.MkdirAll(filesDir, 0o750); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	tmp, err := os.CreateTemp(filesDir, ".upload-*")
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.Remove(tmp.Name()) // a no-op once renamed into place
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, h), http.MaxBytesReader(w, r.Body, maxFileBytes))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		readFailed(w, err, "a file may be at most 8 MiB")
		return
	}
	hash := hex.EncodeToString(h.Sum(nil))
	if err := s.verify(c, "file", hash); err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}

	ctype, err := sniffFile(tmp.Name(), size)
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
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
	if err := os.Rename(tmp.Name(), path); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"hash": hash})
}

// sniffFile reads the first 512 bytes for an image type, and otherwise
// accepts only a small file that is valid JSON.
func sniffFile(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	if t := fileType(head[:n]); t != "" && t != "application/json" {
		return t, nil
	}
	if size > maxJSONFileBytes {
		return "", nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if json.Valid(body) {
		return "application/json", nil
	}
	return "", nil
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
