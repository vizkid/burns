package tail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func lines(t *testing.T, c Cursor) ([]string, Cursor, bool) {
	t.Helper()
	ls, next, reset, err := Read(c)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = string(l)
	}
	return out, next, reset
}

func TestReadsOnlyNewCompleteLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.jsonl")
	write(t, p, "a\nb\npart")
	got, c, _ := lines(t, Cursor{Path: p})
	if strings.Join(got, ",") != "a,b" || c.Offset != 4 {
		t.Fatalf("first read = %v at %d", got, c.Offset)
	}
	appendTo(t, p, "ial\nc\n")
	got, c, reset := lines(t, c)
	if strings.Join(got, ",") != "partial,c" || reset || c.Offset != 14 {
		t.Fatalf("second read = %v at %d reset=%v", got, c.Offset, reset)
	}
	if Changed(c) {
		t.Fatal("nothing new: Changed must be false")
	}
	appendTo(t, p, "d\n")
	if !Changed(c) {
		t.Fatal("appended: Changed must be true")
	}
}

func TestShorterFileRestarts(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.jsonl")
	write(t, p, "aaaa\nbbbb\n")
	_, c, _ := lines(t, Cursor{Path: p})
	write(t, p, "x\n")
	got, _, reset := lines(t, c)
	if !reset || strings.Join(got, ",") != "x" {
		t.Fatalf("got %v reset=%v", got, reset)
	}
}

func TestReplacedFileRestarts(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "r.jsonl")
	write(t, p, "a\n")
	_, c, _ := lines(t, Cursor{Path: p})
	q := filepath.Join(dir, "new")
	write(t, q, "a\nb\nc\n")
	if err := os.Rename(q, p); err != nil {
		t.Fatal(err)
	}
	got, _, reset := lines(t, c)
	if !reset || strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("got %v reset=%v", got, reset)
	}
}

func TestRewriteBehindStablePrefixRestarts(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.jsonl")
	write(t, p, "header\nold-1\n")
	_, c, _ := lines(t, Cursor{Path: p})
	f, _ := os.OpenFile(p, os.O_WRONLY, 0)
	f.WriteAt([]byte("header\nnew-1\nnew-2\n"), 0)
	f.Close()
	got, _, reset := lines(t, c)
	if !reset || strings.Join(got, ",") != "header,new-1,new-2" {
		t.Fatalf("got %v reset=%v", got, reset)
	}
}

func TestLongLineSkipped(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.jsonl")
	write(t, p, "a\n"+strings.Repeat("x", MaxLine+10)+"\nb\n")
	got, c, _ := lines(t, Cursor{Path: p})
	if strings.Join(got, ",") != "a,b" || c.Offset != int64(MaxLine+15) {
		t.Fatalf("got %d lines at %d", len(got), c.Offset)
	}
}

func TestCursorSurvivesRestart(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.jsonl")
	write(t, p, "a\n")
	_, c, _ := lines(t, Cursor{Path: p})
	stored := c // what the store would persist and reload
	appendTo(t, p, "b\n")
	got, _, reset := lines(t, stored)
	if reset || strings.Join(got, ",") != "b" {
		t.Fatalf("got %v reset=%v", got, reset)
	}
}

func TestMissingFileIsAnError(t *testing.T) {
	if _, _, _, err := Read(Cursor{Path: filepath.Join(t.TempDir(), "gone")}); err == nil {
		t.Fatal("missing file must be an error")
	}
}

func TestFindWalksTree(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "2026", "10", "20"), 0o700)
	write(t, filepath.Join(root, "2026", "10", "20", "rollout-a.jsonl"), "")
	write(t, filepath.Join(root, "2026", "10", "20", "other.txt"), "")
	write(t, filepath.Join(root, "rollout-b.jsonl"), "")
	got, err := Find(root, "rollout-*.jsonl")
	if err != nil || len(got) != 2 {
		t.Fatalf("find = %v, %v", got, err)
	}
}

// A large backlog is returned in batches; Changed stays true until it is drained.
func TestBatchesLargeBacklog(t *testing.T) {
	old := BatchBytes
	BatchBytes = 4
	defer func() { BatchBytes = old }()
	p := filepath.Join(t.TempDir(), "r.jsonl")
	write(t, p, "aa\nbb\ncc\n")
	var all []string
	c := Cursor{Path: p}
	for i := 0; i < 5; i++ {
		var got []string
		got, c, _ = lines(t, c)
		all = append(all, got...)
		if !Changed(c) {
			break
		}
	}
	if strings.Join(all, ",") != "aa,bb,cc" || Changed(c) {
		t.Fatalf("batched = %v changed=%v", all, Changed(c))
	}
}
