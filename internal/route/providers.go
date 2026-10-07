package route

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Decision Decision  `json:"decision"`
	Expires  time.Time `json:"expires_at"`
}

type Providers struct {
	mu         sync.RWMutex
	writer     chan struct{}
	entries    map[string]Entry
	served     map[Decision][]byte
	dirty      bool
	generation uint64
	controller string
	secret     string
	client     *http.Client
	capacity   int
}

func validateController(controller string) error {
	if controller != "" {
		u, err := url.Parse(controller)
		if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(controller, "#") || u.Path != "" {
			return fmt.Errorf("controller must be an http://127.0.0.1:PORT URL")
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("controller must use a fixed port from 1 to 65535")
		}
	}
	return nil
}

func NewProviders(controller, secret string, capacity int) (*Providers, error) {
	if err := validateController(controller); err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	p := &Providers{writer: make(chan struct{}, 1), entries: map[string]Entry{}, controller: controller, secret: secret, capacity: capacity,
		client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	p.served = compile(p.entries)
	p.dirty = controller != ""
	return p, nil
}

func compile(entries map[string]Entry) map[Decision][]byte {
	result := map[Decision][]byte{}
	for _, route := range []Decision{Direct, Proxy} {
		var hosts []string
		for host, entry := range entries {
			if entry.Decision == route {
				hosts = append(hosts, host)
			}
		}
		sort.Strings(hosts)
		// Mihomo's provider parser scans YAML lines; JSON-as-YAML is not accepted.
		// Values are already normalized hostnames, so this fixed format is sufficient.
		var body strings.Builder
		if len(hosts) == 0 {
			body.WriteString("payload: []\n")
		} else {
			body.WriteString("payload:\n")
			for _, host := range hosts {
				body.WriteString("  - DOMAIN," + host + "\n")
			}
		}
		result[route] = []byte(body.String())
	}
	return result
}

func (p *Providers) Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var route Decision
	switch r.URL.Path {
	case "/rules/learned-direct.yaml":
		route = Direct
	case "/rules/learned-proxy.yaml":
		route = Proxy
	default:
		http.NotFound(w, r)
		return
	}
	p.mu.RLock()
	body := append([]byte(nil), p.served[route]...)
	p.mu.RUnlock()
	digest := sha256.Sum256(body)
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("ETag", `"`+hex.EncodeToString(digest[:])+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func (p *Providers) Lookup(host string) (Entry, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	entry, ok := p.entries[host]
	return entry, ok && !p.dirty && time.Now().Before(entry.Expires)
}

func (p *Providers) Status() (uint64, int, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.generation, len(p.entries), !p.dirty
}

func (p *Providers) updateCore(ctx context.Context, entries map[string]Entry) error {
	if p.controller == "" {
		return fmt.Errorf("controller is not configured")
	}
	for _, name := range []string{"learned-direct", "learned-proxy"} {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPut, p.controller+"/providers/rules/"+name, nil)
		if p.secret != "" {
			req.Header.Set("Authorization", "Bearer "+p.secret)
		}
		res, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("provider reload transport failed")
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			return fmt.Errorf("provider reload HTTP %d", res.StatusCode)
		}
	}
	return p.verifyCore(ctx, entries)
}

func (p *Providers) verifyCore(ctx context.Context, entries map[string]Entry) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.controller+"/providers/rules", nil)
	if p.secret != "" {
		req.Header.Set("Authorization", "Bearer "+p.secret)
	}
	res, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("provider readback failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("provider readback HTTP %d", res.StatusCode)
	}
	var metadata struct {
		Providers map[string]struct {
			Count int `json:"ruleCount"`
		} `json:"providers"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&metadata) != nil {
		return fmt.Errorf("invalid provider metadata")
	}
	counts := map[Decision]int{}
	for _, entry := range entries {
		counts[entry.Decision]++
	}
	for _, route := range []Decision{Direct, Proxy} {
		name := "learned-" + strings.ToLower(string(route))
		item, ok := metadata.Providers[name]
		if !ok || item.Count != counts[route] {
			return fmt.Errorf("provider count mismatch")
		}
	}
	return nil
}

// Check catches missing/reset providers without reloading an unchanged core.
// Matching counts alone do not prove content identity; successful commits still
// require PUT acknowledgements and subsequent connection auditing.
func (p *Providers) Check(ctx context.Context) error {
	select {
	case p.writer <- struct{}{}:
		defer func() { <-p.writer }()
	case <-ctx.Done():
		return ctx.Err()
	}
	p.mu.RLock()
	entries := p.entries
	p.mu.RUnlock()
	err := p.verifyCore(ctx, entries)
	if err != nil {
		p.mu.Lock()
		p.dirty = true
		p.mu.Unlock()
	}
	return err
}

// Change publishes a complete candidate snapshot. Only successful ACK/readback
// makes entries reusable. Failure restores the previous served/core snapshot.
func (p *Providers) Change(ctx context.Context, host string, entry *Entry) error {
	select {
	case p.writer <- struct{}{}:
		defer func() { <-p.writer }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.RLock()
	old := p.entries
	next := make(map[string]Entry, len(old)+1)
	for key, value := range old {
		if time.Now().Before(value.Expires) {
			next[key] = value
		}
	}
	p.mu.RUnlock()
	if host != "" {
		delete(next, host)
		if entry != nil {
			if entry.Decision != Direct && entry.Decision != Proxy {
				return fmt.Errorf("cannot compile UNCERTAIN")
			}
			d, err := Normalize(host)
			if err != nil || d.Host != host || d.Local {
				return fmt.Errorf("invalid learned hostname")
			}
			if len(next) >= p.capacity {
				return fmt.Errorf("learned capacity reached")
			}
			next[host] = *entry
		}
	}
	p.mu.Lock()
	p.served = compile(next)
	p.dirty = true
	p.mu.Unlock()
	err := p.updateCore(ctx, next)
	if err != nil || ctx.Err() != nil {
		p.mu.Lock()
		p.served = compile(old)
		p.mu.Unlock()
		rollbackCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		restored := p.updateCore(rollbackCtx, old) == nil
		p.mu.Lock()
		p.dirty = !restored
		p.mu.Unlock()
		if err != nil {
			return err
		}
		return ctx.Err()
	}
	p.mu.Lock()
	p.entries = next
	p.dirty = false
	p.generation++
	p.mu.Unlock()
	return nil
}

func (p *Providers) Expired() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, entry := range p.entries {
		if !time.Now().Before(entry.Expires) {
			return true
		}
	}
	return false
}
