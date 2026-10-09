package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testPathChecker struct {
	blocked   atomic.Bool
	calls     atomic.Int64
	restarted atomic.Bool
}

func (g *testPathChecker) CheckOwner(context.Context, Ownership) error {
	if g.restarted.Load() {
		return fmt.Errorf("publication blocked: core process restarted")
	}
	return nil
}

func (g *testPathChecker) Check(context.Context, netip.Addr) error {
	g.calls.Add(1)
	if g.blocked.Load() {
		return fmt.Errorf("publication blocked: synthetic path unavailable")
	}
	return nil
}

func controlledFixture(t *testing.T) (*LabObserver, *testPathChecker, *atomic.Int64) {
	t.Helper()
	c := shadowConfig()
	c.ProxyGroup = "LEARNED"
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c.HTTPListen = listener.Addr().String()
	listener.Close()
	rules := []CoreRule{{Index: 0, Type: "Domain", Payload: "known.test", Proxy: "BASE"},
		{Index: 1, Type: "AND", Payload: tailCorePayload("route-agent-tail-direct"), Proxy: "DIRECT"},
		{Index: 2, Type: "AND", Payload: tailCorePayload("route-agent-tail-proxy"), Proxy: "LEARNED"},
		{Index: 3, Type: "Match", Proxy: "BASE"}}
	var o *LabObserver
	puts := &atomic.Int64{}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-controller" {
			t.Error("controller auth missing")
		}
		switch {
		case r.URL.Path == "/configs":
			json.NewEncoder(w).Encode(map[string]string{"mode": "rule"})
		case r.URL.Path == "/rules":
			json.NewEncoder(w).Encode(map[string]any{"rules": rules})
		case r.URL.Path == "/connections":
			other := observation("example.com", "Match")
			other.Metadata.DestinationPort = "8443"
			json.NewEncoder(w).Encode(map[string]any{"connections": []CoreConnection{other, observation("not-allowed.test", "Match"), observation("example.com", "Match")}})
		case r.Method == "PUT":
			puts.Add(1)
			name := strings.TrimPrefix(r.URL.Path, "/providers/rules/")
			o.Providers.Handler(httptest.NewRecorder(), httptest.NewRequest("GET", "/rules/"+name+".yaml", nil))
			w.WriteHeader(204)
		case r.URL.Path == "/providers/rules":
			providers := map[string]any{}
			o.Providers.mu.RLock()
			for _, d := range []Decision{Direct, Proxy} {
				name := "route-agent-tail-" + strings.ToLower(string(d))
				providers[name] = map[string]any{"name": name, "behavior": "Classical", "vehicleType": "HTTP", "ruleCount": strings.Count(string(o.Providers.served[d]), "  - DOMAIN,")}
			}
			o.Providers.mu.RUnlock()
			json.NewEncoder(w).Encode(map[string]any{"providers": providers})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(core.Close)
	c.Controller = core.URL
	root := t.TempDir()
	configPath := filepath.Join(root, "effective.yaml")
	if err := os.WriteFile(configPath, []byte("synthetic effective config"), 0600); err != nil {
		t.Fatal(err)
	}
	lease, err := prepareOwnership(context.Background(), c, configPath, "synthetic-controller", coreIdentity{123, "synthetic-start"})
	if err != nil {
		t.Fatal(err)
	}
	ownershipPath := filepath.Join(root, "ownership.json")
	data, _ := json.Marshal(lease)
	if err := os.WriteFile(ownershipPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	guard := &testPathChecker{}
	o, err = newControlledObserver(c, true, ownershipPath, guard, &recordingJudge{}, "synthetic-controller")
	if err != nil {
		t.Fatal(err)
	}
	collector, _ := localProbe(t, false, 200)
	collector.pathCheck = o.collector.(*TLSCollector).pathCheck
	o.collector = collector
	o.baseline = rules
	o.started = time.Now().Add(-time.Second)
	o.statePath = filepath.Join(root, "state.json")
	o.Providers.strictFetch = true
	return o, guard, puts
}

func TestControlledRealCollectorPublishesOnlyAllowedTCP443AndDrains(t *testing.T) {
	o, guard, puts := controlledFixture(t)
	if err := o.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if puts.Load() != 2 || o.commits.Load() != 1 || o.attempts.Load() != 1 || guard.calls.Load() < 4 {
		t.Fatalf("wrong bounded publication: puts=%d commits=%d paths=%d", puts.Load(), o.commits.Load(), guard.calls.Load())
	}
	entry, ok := o.Providers.Lookup("example.com")
	if !ok || entry.Decision != Proxy || time.Until(entry.Expires) > time.Minute {
		t.Fatal("missing bounded learned entry")
	}
	o.ready.Store(true)
	if err := o.pauseControlled(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := o.Providers.Lookup("example.com"); ok || !o.control.paused || o.ready.Load() {
		t.Fatal("pause did not drain")
	}
	if err := o.pauseControlled(context.Background()); err != nil || puts.Load() != 4 {
		t.Fatal("pause not idempotent", err)
	}
	// The app may now refresh. This observer cannot adopt the new generation.
	os.WriteFile(o.control.lease.ConfigPath, []byte("refreshed"), 0600)
	if err := o.unchanged(context.Background()); err == nil {
		t.Fatal("accepted refreshed profile under old ownership")
	}
}

func TestControlledBlocksUnavailablePathBeforeAnyControllerWrite(t *testing.T) {
	o, guard, puts := controlledFixture(t)
	guard.blocked.Store(true)
	if err := o.Run(context.Background(), o.statePath); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatal("missing blocked reason", err)
	}
	if puts.Load() != 0 || o.attempts.Load() != 0 {
		t.Fatal("activity before path validation")
	}
	if err := (windowsDirectPath{}).Check(context.Background(), netip.MustParseAddr("1.1.1.1")); err == nil {
		t.Fatal("accepted missing physical interface")
	}
}

func TestControlledConfigChangeDuringJudgmentPreventsCommit(t *testing.T) {
	o, _, puts := controlledFixture(t)
	o.judge = Stub{Answer: validAnswer(Proxy), Wait: func(context.Context) error {
		return os.WriteFile(o.control.lease.ConfigPath, []byte("profile B"), 0600)
	}}
	if err := o.poll(context.Background()); err == nil {
		t.Fatal("accepted stale evidence")
	}
	if puts.Load() != 0 {
		t.Fatal("wrote after config changed")
	}
}

func TestControlledCoreRestartDuringJudgmentPreventsCommit(t *testing.T) {
	o, guard, puts := controlledFixture(t)
	o.judge = Stub{Answer: validAnswer(Proxy), Wait: func(context.Context) error { guard.restarted.Store(true); return nil }}
	if err := o.poll(context.Background()); err == nil {
		t.Fatal("accepted a replacement core")
	}
	if puts.Load() != 0 {
		t.Fatal("wrote to replacement core")
	}
}

func TestControlledPathLossDuringCollectionCannotBecomeProxyEvidence(t *testing.T) {
	o, _, puts := controlledFixture(t)
	collector := o.collector.(*TLSCollector)
	calls := 0
	collector.pathCheck = func(context.Context, netip.Addr) error {
		calls++
		if calls > 1 {
			return fmt.Errorf("route changed")
		}
		return nil
	}
	if err := o.poll(context.Background()); err == nil {
		t.Fatal("missing path-loss blocked reason")
	}
	if puts.Load() != 0 || o.judgments.Load() != 0 {
		t.Fatal("path uncertainty turned into proxy failure evidence")
	}
}

func TestControlledPauseAPIAndNormalRunCleanup(t *testing.T) {
	o, _, puts := controlledFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- o.Run(ctx, o.statePath) }()
	deadline := time.Now().Add(4 * time.Second)
	for o.commits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if o.commits.Load() != 1 {
		cancel()
		<-done
		t.Fatal("run did not publish")
	}
	bad := httptest.NewRecorder()
	o.HTTPHandler().ServeHTTP(bad, httptest.NewRequest("POST", "/control/pause", nil))
	if bad.Code != 401 {
		t.Fatal("pause without token")
	}
	req := httptest.NewRequest("POST", "/control/pause", nil)
	req.Header.Set("Authorization", "Bearer "+o.control.lease.Token)
	good := httptest.NewRecorder()
	o.HTTPHandler().ServeHTTP(good, req)
	if good.Code != 200 || !strings.Contains(good.Body.String(), `"owned_providers_empty":true`) {
		t.Fatal("pause not acknowledged", good.Body.String())
	}
	before := puts.Load()
	time.Sleep(150 * time.Millisecond)
	if puts.Load() != before || o.ready.Load() {
		t.Fatal("resumed after pause")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(o.control.path + ".lock"); !os.IsNotExist(err) {
		t.Fatal("ownership lock retained after clean exit")
	}
}

func TestControlledLeaseExpiryAllowsOnlyOwnedCleanup(t *testing.T) {
	o, _, puts := controlledFixture(t)
	if err := o.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	o.control.lease.Expires = time.Now().Add(-time.Second)
	data, _ := json.Marshal(o.control.lease)
	os.WriteFile(o.control.path, data, 0600)
	if err := o.poll(context.Background()); err == nil {
		t.Fatal("accepted expired lease")
	}
	o.ready.Store(true)
	if err := o.pauseControlled(context.Background()); err != nil {
		t.Fatal("expiry prevented cleanup", err)
	}
	if puts.Load() != 4 {
		t.Fatal("unexpected writes")
	}
}
