package api

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// maxDirs caps one answer: the panel shows a short list, and a huge folder must not stall it.
const maxDirs = 50

// dirs completes a folder the launch panel's operator types (docs/SPEC.md, "Beside, typed folders
// and new sessions"). It lists names of folders under the home folder, never contents, and
// refuses any path that could leave it.
func (s *Server) dirs(w http.ResponseWriter, r *http.Request) {
	home := s.Home
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			fail(w, "home folder", err)
			return
		}
		home = h
	}
	prefix := r.URL.Query().Get("prefix")
	if strings.HasPrefix(prefix, "/") || slices.Contains(strings.Split(prefix, "/"), "..") {
		http.Error(w, "prefix must be relative to the home folder", http.StatusBadRequest)
		return
	}
	parent, part := "", prefix
	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		parent, part = prefix[:i], prefix[i+1:]
	}
	found := []string{}
	// The lexical checks above do not see links: a link in home can point anywhere. Only a
	// folder whose real path is inside home's real path is read or offered.
	root, err := filepath.EvalSymlinks(home)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"host": s.Host, "home": home, "dirs": found})
		return
	}
	var entries []os.DirEntry
	if inside(root, filepath.Join(home, parent)) {
		entries, _ = os.ReadDir(filepath.Join(home, parent)) // a missing folder has nothing to offer
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(part)) || (strings.HasPrefix(name, ".") && !strings.HasPrefix(part, ".")) {
			continue
		}
		// Stat follows a link, so a link to a folder inside home counts.
		path := filepath.Join(home, parent, name)
		if info, err := os.Stat(path); err != nil || !info.IsDir() || !inside(root, path) {
			continue
		}
		found = append(found, strings.TrimPrefix(parent+"/"+name, "/"))
	}
	slices.SortFunc(found, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	writeJSON(w, http.StatusOK, map[string]any{"host": s.Host, "home": home, "dirs": found[:min(len(found), maxDirs)]})
}

// inside reports whether path's real location is root or under it.
func inside(root, path string) bool {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, real)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}
