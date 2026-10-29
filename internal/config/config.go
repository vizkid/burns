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
// Home is absolute, or relative to the user's home directory. ExtraHomes are
// other homes signed in to the same account, such as the CLI's default home:
// they are one quota, so they are one account, not a second one.
type Account struct {
	Provider   string   `json:"provider"`
	Name       string   `json:"name"`
	Home       string   `json:"home"`
	ExtraHomes []string `json:"extra_homes,omitempty"`
	Kind       string   `json:"kind"`
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
	seen, homes := map[string]bool{}, map[string]bool{}
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
		default:
			problem = c.checkHomes(a, homes)
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

// checkHomes reports an empty extra home, or a home already claimed: a home's
// files can belong to one account only.
func (c Config) checkHomes(a Account, claimed map[string]bool) string {
	for _, h := range a.ExtraHomes {
		if strings.TrimSpace(h) == "" {
			return "extra_homes entries must not be empty"
		}
	}
	for _, d := range c.HomeDirs(a) {
		if claimed[d] {
			return "home " + d + " belongs to another account"
		}
		claimed[d] = true
	}
	return ""
}

// HomeDir is the account's absolute agent home.
func (c Config) HomeDir(a Account) string { return c.abs(a.Home) }

// HomeDirs is the account's home followed by its extra homes, all absolute.
func (c Config) HomeDirs(a Account) []string {
	dirs := []string{c.abs(a.Home)}
	for _, h := range a.ExtraHomes {
		dirs = append(dirs, c.abs(h))
	}
	return dirs
}

func (c Config) abs(home string) string {
	if filepath.IsAbs(home) {
		return filepath.Clean(home)
	}
	return filepath.Join(c.userHome, home)
}

// ForPath returns the account one of whose homes contains path.
func (c Config) ForPath(path string) (Account, bool) {
	clean := filepath.Clean(path)
	for _, a := range c.Accounts {
		for _, d := range c.HomeDirs(a) {
			if strings.HasPrefix(clean, d+string(filepath.Separator)) {
				return a, true
			}
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
