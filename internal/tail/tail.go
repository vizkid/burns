// Package tail reads complete lines appended to a file since a stored cursor.
// It detects a replaced or rewritten file and starts again from the beginning.
// Callers must make re-reading harmless; smithersd does it with dedupe keys.
package tail

import "time"

// MaxLine is the longest line kept. Longer lines are skipped whole.
const MaxLine = 4 << 20

// BatchBytes bounds how many bytes one Read consumes. A read that stops at the
// bound leaves Changed true, so the caller reads again.
var BatchBytes int64 = 32 << 20

// Cursor is the stored read position of one file.
type Cursor struct {
	Path     string    `json:"path"`
	Inode    uint64    `json:"inode"`
	Offset   int64     `json:"offset"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
	HeadHash string    `json:"head_hash"`
	TailHash string    `json:"tail_hash"`
}
