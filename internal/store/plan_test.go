package store

import (
	"strings"
	"testing"
)

// plan returns SQLite's query plan text for a statement.
func plan(t *testing.T, s *Store, q string, args ...any) string {
	t.Helper()
	rows, err := s.db.Query("EXPLAIN QUERY PLAN "+q, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		out = append(out, detail)
	}
	return strings.Join(out, "\n")
}

// Hot-path lookups must use an exact index, not a scan of a provider's rows.
func TestHotLookupsUseExactIndexes(t *testing.T) {
	s := open(t)
	for name, c := range map[string]struct{ q, want string }{
		"flag by request key": {flagQuery, "compaction_request"},
		"keyed compaction":    {keyedCompactionQuery, "compaction_key"},
	} {
		if got := plan(t, s, c.q, "codex", "k"); !strings.Contains(got, c.want) {
			t.Errorf("%s: plan %q does not use %s", name, got, c.want)
		}
	}
}
