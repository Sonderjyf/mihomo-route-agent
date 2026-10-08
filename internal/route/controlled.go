package route

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// Ownership pins one operator-owned app/core configuration generation. It is
// a cooperative lease, not a claim that Mihomo exposes an atomic reload epoch.
type Ownership struct {
	Version      int       `json:"version"`
	Controller   string    `json:"controller"`
	Listen       string    `json:"http_listen"`
	ProxyGroup   string    `json:"proxy_group"`
	ConfigPath   string    `json:"config_path"`
	ConfigSHA256 string    `json:"config_sha256"`
	RulesSHA256  string    `json:"rules_sha256"`
	Expires      time.Time `json:"expires_at"`
	Token        string    `json:"pause_token"`
	CorePID      int       `json:"core_pid"`
	CoreStarted  string    `json:"core_started_utc_ticks"`
}

type directPathChecker interface {
	Check(context.Context, netip.Addr) error
	CheckOwner(context.Context, Ownership) error
}
type windowsDirectPath struct{ index int }

type coreIdentity struct {
	PID     int    `json:"pid"`
	Started string `json:"started"`
}

func readCoreIdentity(ctx context.Context, controller string) (coreIdentity, error) {
	if runtime.GOOS != "windows" || controller == "" || validateController(controller) != nil {
		return coreIdentity{}, fmt.Errorf("publication blocked: core_process_identity_unavailable")
	}
	u, _ := url.Parse(controller)
	port, _ := strconv.Atoi(u.Port())
	script := `$ErrorActionPreference='Stop'; $ids=@(Get-NetTCPConnection -State Listen -LocalPort ` + strconv.Itoa(port) + ` | Where-Object {$_.LocalAddress -eq '127.0.0.1'} | Select-Object -ExpandProperty OwningProcess -Unique); if($ids.Count -ne 1){exit 7}; $p=Get-Process -Id $ids[0]; @{pid=$p.Id;started=$p.StartTime.ToUniversalTime().Ticks.ToString()} | ConvertTo-Json -Compress`
	check, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	data, err := exec.CommandContext(check, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	var identity coreIdentity
	if err != nil || json.Unmarshal(data, &identity) != nil || identity.PID < 1 || identity.Started == "" {
		return identity, fmt.Errorf("publication blocked: core_process_identity_unavailable")
	}
	return identity, nil
}

func (p windowsDirectPath) CheckOwner(ctx context.Context, lease Ownership) error {
	identity, err := readCoreIdentity(ctx, lease.Controller)
	if err != nil {
		return err
	}
	if identity.PID != lease.CorePID || identity.Started != lease.CoreStarted {
		return fmt.Errorf("publication blocked: core_process_restarted; new ownership required")
	}
	return nil
}

func (p windowsDirectPath) Check(ctx context.Context, target netip.Addr) error {
	if runtime.GOOS != "windows" || p.index < 1 || !target.IsValid() {
		return fmt.Errorf("publication blocked: direct_path_validation_unavailable")
	}
	// Only validated numeric values enter this read-only script. Do not constrain
	// Find-NetRoute to the requested interface: that could hide a preferred TUN.
	script := `$ErrorActionPreference='Stop'; $r=@(Find-NetRoute -RemoteIPAddress '` + target.String() + `'); $a=@(Get-NetAdapter -Physical -InterfaceIndex ` + strconv.Itoa(p.index) + `); if($r.Count -ne 2 -or $a.Count -ne 1 -or $a[0].Status -ne 'Up' -or @($r | Where-Object {$_.InterfaceIndex -ne ` + strconv.Itoa(p.index) + `}).Count -ne 0){exit 7}; 'physical_route_verified'`
	check, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(check, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil || string(bytes.TrimSpace(output)) != "physical_route_verified" {
		return fmt.Errorf("publication blocked: selected_route_not_verified_physical")
	}
	return nil
}

type publicationControl struct {
	path    string
	lease   Ownership
	checker directPathChecker
	pause   chan chan error
	paused  bool                // worker-owned
	targets map[netip.Addr]bool // worker-owned, bounded by the host budget
}

func digest(data []byte) string           { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func rulesDigest(rules []CoreRule) string { data, _ := json.Marshal(rules); return digest(data) }

func readPrivate(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, fmt.Errorf("invalid or oversized private file")
	}
	return data, nil
}

// PrepareOwnership reads the explicitly owned controller; it never changes it.
// The caller must keep this token-bearing artifact private and use a new path.
func PrepareOwnership(ctx context.Context, c Config, profilePath, secret string) (Ownership, error) {
	if profilePath == "" || secret == "" {
		return Ownership{}, fmt.Errorf("ownership requires an effective config file and controller authentication")
	}
	identity, err := readCoreIdentity(ctx, c.Controller)
	if err != nil {
		return Ownership{}, err
	}
	return prepareOwnership(ctx, c, profilePath, secret, identity)
}

func prepareOwnership(ctx context.Context, c Config, profilePath, secret string, identity coreIdentity) (Ownership, error) {
	if profilePath == "" || secret == "" {
		return Ownership{}, fmt.Errorf("ownership requires an effective config file and controller authentication")
	}
	profilePath, err := filepath.Abs(profilePath)
	if err != nil {
		return Ownership{}, err
	}
	data, err := readPrivate(profilePath, 4<<20)
	if err != nil {
		return Ownership{}, err
	}
	o, err := newObserver(c, Stub{})
	if err != nil {
		return Ownership{}, err
	}
	o.Providers.secret = secret
	rules, err := o.rules(ctx)
	if err != nil {
		return Ownership{}, err
	}
	after, err := readPrivate(profilePath, 4<<20)
	if err != nil || !bytes.Equal(data, after) {
		return Ownership{}, fmt.Errorf("effective config changed during ownership capture")
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return Ownership{}, err
	}
	return Ownership{Version: 1, Controller: c.Controller, Listen: c.HTTPListen, ProxyGroup: c.ProxyGroup, ConfigPath: profilePath, ConfigSHA256: digest(data), RulesSHA256: rulesDigest(rules), Expires: time.Now().Add(10 * time.Minute), Token: hex.EncodeToString(token), CorePID: identity.PID, CoreStarted: identity.Started}, nil
}

func NewControlledObserver(c Config, allowed bool, ownershipPath string, directInterface int, judge Judge, secret string) (*LabObserver, error) {
	return newControlledObserver(c, allowed, ownershipPath, windowsDirectPath{directInterface}, judge, secret)
}

func newControlledObserver(c Config, allowed bool, ownershipPath string, checker directPathChecker, judge Judge, secret string) (*LabObserver, error) {
	if !allowed || ownershipPath == "" || secret == "" || checker == nil {
		return nil, fmt.Errorf("controlled apply requires explicit --allow-controlled-apply, private ownership and controller authentication")
	}
	o, err := NewShadowObserver(c, true, judge, secret)
	if err != nil {
		return nil, err
	}
	data, err := readPrivate(ownershipPath, 16384)
	if err != nil {
		return nil, err
	}
	var lease Ownership
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&lease) != nil || decoder.Decode(&struct{}{}) != io.EOF || lease.Version != 1 || lease.Controller != c.Controller || lease.Listen != c.HTTPListen || lease.ProxyGroup != c.ProxyGroup || len(lease.Token) != 64 || len(lease.RulesSHA256) != 64 || len(lease.ConfigSHA256) != 64 {
		return nil, fmt.Errorf("invalid or differently owned publication lease")
	}
	if _, err := hex.DecodeString(lease.Token); err != nil || lease.CorePID < 1 || lease.CoreStarted == "" {
		return nil, fmt.Errorf("invalid pause token")
	}
	o.control = &publicationControl{path: ownershipPath, lease: lease, checker: checker, pause: make(chan chan error, 1), targets: map[netip.Addr]bool{}}
	if err = o.control.check(); err != nil {
		return nil, err
	}
	o.shadow = false
	collector := o.collector.(*TLSCollector)
	collector.pathCheck = func(ctx context.Context, ip netip.Addr) error {
		if err := checker.Check(ctx, ip); err != nil {
			return err
		}
		o.control.targets[ip] = true
		return nil
	}
	// Verify ownership/snapshot before every provider PUT, including rollback.
	o.Providers.beforeWrite = func(ctx context.Context, entries map[string]Entry) error {
		if err := o.unchanged(ctx); err != nil {
			return err
		}
		if len(entries) > 0 {
			if err := o.control.check(); err != nil {
				return err
			}
			return o.control.checkPaths(ctx)
		}
		return nil
	}
	return o, nil
}

func (p *publicationControl) checkPaths(ctx context.Context) error {
	for ip := range p.targets {
		if err := p.checker.Check(ctx, ip); err != nil {
			return err
		}
	}
	return nil
}

func (p *publicationControl) check() error {
	if !time.Now().Before(p.lease.Expires) || time.Until(p.lease.Expires) > 10*time.Minute {
		return fmt.Errorf("publication blocked: ownership_expired_or_invalid")
	}
	return p.checkIdentity()
}

func (p *publicationControl) checkIdentity() error {
	data, err := readPrivate(p.path, 16384)
	var current Ownership
	if err != nil || json.Unmarshal(data, &current) != nil {
		return fmt.Errorf("publication blocked: ownership_unreadable")
	}
	if current.Expires.Equal(p.lease.Expires) {
		current.Expires = p.lease.Expires
	}
	if current != p.lease {
		return fmt.Errorf("publication blocked: ownership_changed; pause before profile refresh")
	}
	config, err := readPrivate(p.lease.ConfigPath, 4<<20)
	if err != nil || digest(config) != p.lease.ConfigSHA256 {
		return fmt.Errorf("publication blocked: effective_config_changed; new ownership required")
	}
	return nil
}

func (o *LabObserver) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+o.control.lease.Token)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ack := make(chan error, 1)
	select {
	case o.control.pause <- ack:
	case <-r.Context().Done():
		return
	}
	select {
	case err := <-ack:
		if err != nil {
			http.Error(w, "pause cleanup failed; refresh is not authorized", http.StatusConflict)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"paused": true, "owned_providers_empty": true, "new_ownership_required": true})
	case <-r.Context().Done():
	}
}

func (o *LabObserver) pauseControlled(ctx context.Context) error {
	if o.control.paused {
		return nil
	}
	if !o.ready.Load() {
		return fmt.Errorf("observer not ready")
	}
	if err := o.unchanged(ctx); err != nil {
		return err
	}
	if err := o.reconcile(ctx); err != nil {
		return err
	}
	if err := o.saveJournal("paused"); err != nil {
		return err
	}
	o.control.paused = true
	o.ready.Store(false)
	return nil
}
