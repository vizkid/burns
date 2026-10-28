package tail

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// probe is how many bytes the head and tail hashes cover.
const probe = 256

// Read returns the complete lines after c.Offset, without their newlines,
// and the cursor after them. reset is true when the file was replaced or
// rewritten and reading restarted at 0.
func Read(c Cursor) (lines [][]byte, next Cursor, reset bool, err error) {
	f, err := os.Open(c.Path)
	if err != nil {
		return nil, c, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, c, false, err
	}
	if c.Inode != 0 && rewritten(f, c, info) {
		c = Cursor{Path: c.Path}
		reset = true
	}
	if _, err := f.Seek(c.Offset, io.SeekStart); err != nil {
		return nil, c, reset, err
	}
	lines, consumed, full, err := completeLines(f)
	if err != nil {
		return nil, c, reset, err
	}
	next = Cursor{Path: c.Path, Inode: inode(info), Offset: c.Offset + consumed, Size: info.Size(), ModTime: info.ModTime()}
	if full {
		next.Size = -1 // more lines wait: keep Changed true
	}
	if next.HeadHash, err = hashAt(f, 0, min(probe, next.Offset)); err != nil {
		return nil, c, reset, err
	}
	if next.TailHash, err = hashAt(f, max(0, next.Offset-probe), next.Offset); err != nil {
		return nil, c, reset, err
	}
	return lines, next, reset, nil
}

// completeLines reads to EOF, or until BatchBytes are consumed (full), and
// returns every newline-terminated line.
// A line longer than MaxLine is consumed but not returned. Bytes after the
// last newline are left for the next read.
func completeLines(r io.Reader) (lines [][]byte, consumed int64, full bool, err error) {
	br := bufio.NewReaderSize(r, 64<<10)
	var read int64
	var cur []byte
	var skipping bool
	for {
		chunk, err := br.ReadSlice('\n')
		read += int64(len(chunk))
		switch {
		case err == nil:
			if !skipping && len(cur)+len(chunk)-1 <= MaxLine {
				lines = append(lines, append(append([]byte(nil), cur...), chunk[:len(chunk)-1]...))
			}
			consumed, cur, skipping = read, cur[:0], false
			if consumed >= BatchBytes {
				return lines, consumed, true, nil
			}
		case err == bufio.ErrBufferFull:
			if !skipping {
				cur = append(cur, chunk...)
				if len(cur) > MaxLine {
					cur, skipping = cur[:0], true
				}
			}
		case err == io.EOF:
			return lines, consumed, false, nil
		default:
			return nil, 0, false, err
		}
	}
}

// rewritten reports whether the bytes the cursor already read have changed.
func rewritten(f *os.File, c Cursor, info fs.FileInfo) bool {
	if inode(info) != c.Inode || info.Size() < c.Offset {
		return true
	}
	head, err := hashAt(f, 0, min(probe, c.Offset))
	if err != nil || head != c.HeadHash {
		return true
	}
	tailHash, err := hashAt(f, max(0, c.Offset-probe), c.Offset)
	return err != nil || tailHash != c.TailHash
}

func hashAt(f *os.File, from, to int64) (string, error) {
	buf := make([]byte, to-from)
	if _, err := f.ReadAt(buf, from); err != nil && err != io.EOF {
		return "", err
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:8]), nil
}

func inode(info fs.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Ino)
	}
	return 0
}

// Changed reports whether the file may hold bytes the cursor has not read.
func Changed(c Cursor) bool {
	info, err := os.Stat(c.Path)
	return err == nil && ChangedInfo(c, info)
}

// ChangedInfo is Changed for a stat the caller already has.
func ChangedInfo(c Cursor, info fs.FileInfo) bool {
	return inode(info) != c.Inode || info.Size() != c.Size || !info.ModTime().Equal(c.ModTime)
}

// Find lists files under root whose names match the glob pattern, at any depth.
func Find(root, pattern string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if ok, _ := filepath.Match(pattern, d.Name()); ok {
				out = append(out, p)
			}
		}
		return nil
	})
	return out, err
}
