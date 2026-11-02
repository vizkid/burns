package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// readAll reads a bounded body.
func readAll(res *http.Response) ([]byte, error) {
	return io.ReadAll(io.LimitReader(res.Body, maxBody))
}

// credentials is the part of Claude Code's stored login that Burns reads.
type credentials struct {
	OAuth struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   int64  `json:"expiresAt"` // Milliseconds.
	} `json:"claudeAiOauth"`
}

// accessToken takes the access token from stored credentials. It returns an expired token as
// missing and never refreshes one: a refresh rotates the login under Claude Code's feet.
func accessToken(raw []byte, now time.Time) (string, error) {
	var c credentials
	if err := json.Unmarshal(raw, &c); err != nil || c.OAuth.AccessToken == "" {
		return "", ErrNoToken
	}
	if c.OAuth.ExpiresAt > 0 && now.After(time.UnixMilli(c.OAuth.ExpiresAt)) {
		return "", fmt.Errorf("%w: expired", ErrNoToken)
	}
	return c.OAuth.AccessToken, nil
}

// keychainService names a home's Claude Code login in the macOS keychain. The default home has no suffix.
func keychainService(home, userHome string) string {
	const base = "Claude Code-credentials"
	if home == filepath.Join(userHome, ".claude") {
		return base
	}
	sum := sha256.Sum256([]byte(home))
	return base + "-" + hex.EncodeToString(sum[:])[:8]
}

// StoredToken reads a home's Claude Code login where that CLI keeps it: the keychain on macOS,
// and .credentials.json in the home elsewhere.
func StoredToken(userHome string, now func() time.Time) func(home string) (string, error) {
	return func(home string) (string, error) {
		var raw []byte
		var err error
		if runtime.GOOS == "darwin" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			raw, err = exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", keychainService(home, userHome), "-w").Output()
		} else {
			raw, err = os.ReadFile(filepath.Join(home, ".credentials.json"))
		}
		if err != nil {
			return "", ErrNoToken
		}
		return accessToken(raw, now())
	}
}
