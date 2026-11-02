package tailnet

import (
	"context"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// LookupTimeout bounds one whois: a request's context has no deadline.
const LookupTimeout = 5 * time.Second

// Gate lets a request through only when its caller's login is Login. Each
// answer is cached for TTL per IP, so another user's requests cost one process.
// A failed lookup is tried again on the next request, in case tailscaled was
// restarting or slow. Until then the last answer for that IP stands for Grace
// past its TTL; with none, the request is refused.
type Gate struct {
	Whois Whois
	Login string
	// Self is this node's own address, which Login owns: no lookup needed.
	Self  string
	TTL   time.Duration
	Grace time.Duration
	Now   func() time.Time

	mu      sync.Mutex
	answers map[string]answer  // by IP
	flights map[string]*flight // lookups running now, by IP
}

type answer struct {
	allowed bool
	until   time.Time
}

// flight is one lookup that concurrent requests from one IP share.
type flight struct {
	done    chan struct{}
	allowed bool
	err     error
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
	if g.Self != "" && ip == g.Self {
		return true
	}
	now := g.Now()
	g.mu.Lock()
	last, known := g.answers[ip]
	if known && now.Before(last.until) {
		g.mu.Unlock()
		return last.allowed
	}
	f, running := g.flights[ip]
	if !running {
		f = &flight{done: make(chan struct{})}
		if g.flights == nil {
			g.flights = map[string]*flight{}
		}
		g.flights[ip] = f
	}
	g.mu.Unlock()
	if running {
		select {
		case <-f.done:
		case <-r.Context().Done():
			return false
		}
	} else {
		g.lookup(ip, f, now)
	}
	if f.err == nil {
		return f.allowed
	}
	// A slow tailscaled is not a new peer: keep the last answer for a while.
	return known && now.Before(last.until.Add(g.Grace)) && last.allowed
}

// lookup runs one whois for ip, stores a real answer, and releases the waiters.
// It is bounded by LookupTimeout, not by the request that started it, so a
// caller that gives up does not fail the others.
func (g *Gate) lookup(ip string, f *flight, now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), LookupTimeout)
	defer cancel()
	id, err := g.Whois.Whois(ctx, ip)
	f.err = err
	if err != nil {
		log.Printf("WARN tailnet caller %s: %v", ip, err)
	} else {
		f.allowed = !id.Tagged && id.Login == g.Login
		if !f.allowed {
			log.Printf("WARN tailnet caller %s refused: %q", ip, id.Login)
		}
	}
	g.mu.Lock()
	if err == nil {
		if g.answers == nil {
			g.answers = map[string]answer{}
		}
		g.answers[ip] = answer{allowed: f.allowed, until: now.Add(g.TTL)}
	}
	delete(g.flights, ip)
	g.mu.Unlock()
	close(f.done)
}
