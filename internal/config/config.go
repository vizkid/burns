// Package config loads the accounts smithersd collects for.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Account kinds.
const (
	Subscription = "subscription"
	Azure        = "azure"
)

// maxFile bounds the accounts file.
const maxFile = 1 << 20

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// Account is one provider and account name with its agent home.
// Home is absolute, or relative to the user's home directory.
type Account struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Home     string `json:"home"`
	Kind     string `json:"kind"`
}

// Config is the accounts file. Order is the settings order used for ties.
type Config struct {
	Accounts []Account `json:"accounts"`
	userHome string
}

// Load reads and validates the accounts file. Unknown fields are errors.
func Load(path, userHome string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return Config{}, err
	}
	if len(data) > maxFile {
		return Config{}, fmt.Errorf("%s: larger than %d bytes", path, maxFile)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if dec.More() {
		return Config{}, fmt.Errorf("%s: more than one JSON document", path)
	}
	c.userHome = userHome
	return c, c.validate()
}

func (c Config) validate() error {
	seen := map[string]bool{}
	for i, a := range c.Accounts {
		var problem string
		switch {
		case a.Provider != "claude" && a.Provider != "codex":
			problem = "provider must be claude or codex"
		case !namePattern.MatchString(a.Name):
			problem = "name must be lowercase letters, digits, and hyphens"
		case strings.TrimSpace(a.Home) == "":
			problem = "home is required"
		case a.Kind != Subscription && a.Kind != Azure:
			problem = "kind must be subscription or azure"
		case a.Kind == Azure && a.Provider != "codex":
			problem = "azure accounts exist only for codex"
		case seen[a.Provider+"/"+a.Name]:
			problem = "duplicate account"
		}
		if problem != "" {
			return fmt.Errorf("account %d (%s %s): %s", i+1, a.Provider, a.Name, problem)
		}
		seen[a.Provider+"/"+a.Name] = true
	}
	if len(c.Accounts) == 0 && c.Accounts == nil {
		return errors.New("accounts is required")
	}
	return nil
}

// HomeDir is the account's absolute agent home.
func (c Config) HomeDir(a Account) string {
	if filepath.IsAbs(a.Home) {
		return filepath.Clean(a.Home)
	}
	return filepath.Join(c.userHome, a.Home)
}

// ForPath returns the account whose home contains path.
func (c Config) ForPath(path string) (Account, bool) {
	clean := filepath.Clean(path)
	for _, a := range c.Accounts {
		if strings.HasPrefix(clean, c.HomeDir(a)+string(filepath.Separator)) {
			return a, true
		}
	}
	return Account{}, false
}

// Find returns the named account of a provider.
func (c Config) Find(provider, name string) (Account, bool) {
	for _, a := range c.Accounts {
		if a.Provider == provider && a.Name == name {
			return a, true
		}
	}
	return Account{}, false
}
