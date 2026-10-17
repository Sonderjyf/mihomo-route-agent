package route

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJournalPersistsTTLWithoutTrustingItOnRestart(t *testing.T) {
	c := testConfig()
	c.HTTPListen = "127.0.0.1:1"
	c.Mode = "async"
	c.Judge = "stub"
	c.LabFixtures = map[string]Evidence{"one.route-lab.test": {}}
	o, err := NewLabObserver(c, true)
	if err != nil {
		t.Fatal(err)
	}
	o.statePath = filepath.Join(t.TempDir(), "observer.json")
	expires := time.Now().Add(time.Minute).UTC()
	o.Providers.entries["one.route-lab.test"] = Entry{Proxy, expires}
	if err = o.saveJournal("active"); err != nil {
		t.Fatal(err)
	}
	if err = o.checkJournal(); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(o.statePath)
	var state observerJournal
	if json.Unmarshal(body, &state) != nil || state.Phase != "active" || !state.Entries["one.route-lab.test"].Expires.Equal(expires) {
		t.Fatal("journal lost expiry/state")
	}
	fresh, _ := NewLabObserver(c, true)
	fresh.statePath = o.statePath
	if err = fresh.checkJournal(); err != nil {
		t.Fatal(err)
	}
	if _, ok := fresh.Providers.Lookup("one.route-lab.test"); ok {
		t.Fatal("trusted persisted entry without reconciliation")
	}
	fresh.c.Controller = "http://127.0.0.1:2"
	if err = fresh.checkJournal(); err == nil {
		t.Fatal("accepted another owner")
	}
	if err = os.WriteFile(o.statePath, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = o.checkJournal(); err == nil {
		t.Fatal("accepted corrupt state")
	}
	if err = o.saveJournal("recovery_required"); err != nil {
		t.Fatal("could not replace existing journal", err)
	}
}

func TestStrictProviderRequiresFreshFetchNotMatchingCount(t *testing.T) {
	for _, fetch := range []bool{false, true} {
		t.Run(map[bool]string{false: "wrong_source", true: "owned_source"}[fetch], func(t *testing.T) {
			p, _ := newProviders("", "", 8, "route-agent-tail-")
			p.strictFetch = true
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					if fetch {
						name := strings.TrimPrefix(r.URL.Path, "/providers/rules/")
						p.Handler(httptest.NewRecorder(), httptest.NewRequest("GET", "/rules/"+name+".yaml", nil))
					}
					w.WriteHeader(204)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"providers": map[string]any{
					"route-agent-tail-direct": map[string]any{"name": "route-agent-tail-direct", "ruleCount": 0, "behavior": "Classical", "vehicleType": "HTTP"},
					"route-agent-tail-proxy":  map[string]any{"name": "route-agent-tail-proxy", "ruleCount": 0, "behavior": "Classical", "vehicleType": "HTTP"},
				}})
			}))
			defer core.Close()
			p.controller = core.URL
			err := p.Change(context.Background(), "", nil)
			_, _, ready := p.Status()
			if (err == nil) != fetch || ready != fetch {
				t.Fatalf("fetch=%v err=%v ready=%v", fetch, err, ready)
			}
		})
	}
}
