// Package tailnet guards smithersd's tailnet listener (docs/SPEC.md, "Tailnet listener").
// Only the devices of the Tailscale user who owns this host may read.
package tailnet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Identity is who Tailscale says a peer is.
type Identity struct {
	Login  string
	Tagged bool // a tagged node acts for no user
}

// Whois looks up the peer at an IP address.
type Whois interface {
	Whois(ctx context.Context, ip string) (Identity, error)
}

// CLI asks the tailscale command at Path.
type CLI struct{ Path string }

// Whois runs `tailscale whois --json IP`.
func (c CLI) Whois(ctx context.Context, ip string) (Identity, error) {
	out, err := c.run(ctx, "whois", "--json", ip)
	if err != nil {
		return Identity{}, err
	}
	var w struct {
		Node        struct{ Tags []string }
		UserProfile struct{ LoginName string }
	}
	if err := json.Unmarshal(out, &w); err != nil {
		return Identity{}, fmt.Errorf("tailscale whois %s: %w", ip, err)
	}
	return Identity{Login: w.UserProfile.LoginName, Tagged: len(w.Node.Tags) > 0}, nil
}

// Self returns this node's first tailnet IPv4 address and the login that owns it.
func (c CLI) Self(ctx context.Context) (ip, login string, err error) {
	out, err := c.run(ctx, "ip", "-4")
	if err != nil {
		return "", "", err
	}
	ip = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if net.ParseIP(ip) == nil {
		return "", "", fmt.Errorf("tailscale ip -4 printed %q", ip)
	}
	id, err := c.Whois(ctx, ip)
	if err != nil {
		return "", "", err
	}
	if id.Tagged || id.Login == "" {
		return "", "", fmt.Errorf("this node %s belongs to no user", ip)
	}
	return ip, id.Login, nil
}

func (c CLI) run(ctx context.Context, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Path, args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tailscale %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Gate lets a request through only when its caller's login is Login.
// Allowed callers are cached for TTL per IP; refusals are not, so a lookup that
// failed while tailscaled restarted is tried again.
type Gate struct {
	Whois Whois
	Login string
	TTL   time.Duration
	Now   func() time.Time

	mu      sync.Mutex
	allowed map[string]time.Time // IP → when the lookup expires
}

// Wrap guards h.
func (g *Gate) Wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.allow(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (g *Gate) allow(r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(ip) == nil {
		return false
	}
	now := g.Now()
	g.mu.Lock()
	until, ok := g.allowed[ip]
	g.mu.Unlock()
	if ok && now.Before(until) {
		return true
	}
	id, err := g.Whois.Whois(r.Context(), ip)
	if err != nil {
		log.Printf("WARN tailnet caller %s: %v", ip, err)
		return false
	}
	if id.Tagged || id.Login != g.Login {
		log.Printf("WARN tailnet caller %s refused: %q", ip, id.Login)
		return false
	}
	g.mu.Lock()
	if g.allowed == nil {
		g.allowed = map[string]time.Time{}
	}
	g.allowed[ip] = now.Add(g.TTL)
	g.mu.Unlock()
	return true
}
