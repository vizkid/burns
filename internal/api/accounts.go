package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"

	"github.com/vizkid/burns/internal/config"
)

// MaxAccounts bounds a PUT /accounts body.
const MaxAccounts = 64 << 10

// etag is the quoted hash of the accounts file's bytes.
func etag(b []byte) string {
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// getAccounts answers the accounts file as it is on disk, with its hash as the
// ETag, so an app can write it back only if nothing changed in between.
func (s *Server) getAccounts(w http.ResponseWriter, r *http.Request) {
	s.accountsMu.Lock()
	defer s.accountsMu.Unlock()
	b, err := os.ReadFile(s.ConfigPath)
	if err != nil {
		fail(w, "read accounts", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", etag(b))
	w.Write(b)
}

// putAccounts replaces the accounts file (docs/SPEC.md, "Accounts in the apps"):
// If-Match must carry the file's current ETag, the body must load as startup
// loads it, and the write is atomic. Then smithersd restarts to take it.
func (s *Server) putAccounts(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r, MaxAccounts)
	if !ok {
		return
	}
	match := r.Header.Get("If-Match")
	if match == "" {
		http.Error(w, "If-Match required: read GET /accounts first", http.StatusPreconditionRequired)
		return
	}
	s.accountsMu.Lock()
	defer s.accountsMu.Unlock()
	current, err := os.ReadFile(s.ConfigPath)
	if err != nil {
		fail(w, "read accounts", err)
		return
	}
	if match != etag(current) {
		http.Error(w, "accounts changed since read", http.StatusPreconditionFailed)
		return
	}
	if _, err := config.Parse(body, s.Config.UserHome()); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := writeAtomic(s.ConfigPath, body); err != nil {
		fail(w, "write accounts", err)
		return
	}
	tag := etag(body)
	w.Header().Set("ETag", tag)
	writeJSON(w, http.StatusOK, map[string]string{"etag": tag})
	if s.Restart != nil {
		s.Restart()
	}
}

// writeAtomic writes a temporary file beside path, mode 0600, and renames it
// over path: a crash mid-write leaves the old file whole.
func writeAtomic(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".accounts-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// homeExists reports whether an account's home is on this host.
func (s *Server) homeExists(dir string) bool {
	if s.HomeExists != nil {
		return s.HomeExists(dir)
	}
	fi, err := os.Stat(dir)
	return err == nil && fi.IsDir()
}
