package route

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// PreviewPlan is an offline artifact. Neither profile is applied by this package.
type PreviewPlan struct {
	SourceSHA256  string                     `json:"source_sha256"`
	Candidate     map[string]json.RawMessage `json:"candidate"`
	AgentConfig   Config                     `json:"agent_config"`
	Changes       []PreviewChange            `json:"changes"`
	ManagedFields []string                   `json:"managed_fields"`
	Limitations   []string                   `json:"limitations"`
	Rollback      string                     `json:"rollback"`
}

type PreviewChange struct {
	Field  string          `json:"field"`
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after"`
}

func equivalentJSON(a, b []byte) bool {
	var left, right any
	if len(a) != 0 && json.Unmarshal(a, &left) != nil {
		return false
	}
	if len(b) != 0 && json.Unmarshal(b, &right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

// Keep the first preview format deliberately small. Unsupported profile fields
// must be reviewed before they can influence DNS, bootstrap or rule precedence.
type previewProfile struct {
	MixedPort          uint16 `json:"mixed-port"`
	Mode               string `json:"mode"`
	AllowLAN           bool   `json:"allow-lan"`
	BindAddress        string `json:"bind-address"`
	ExternalController string `json:"external-controller"`
	Secret             string `json:"secret"`
	IPv6               bool   `json:"ipv6"`
	LogLevel           string `json:"log-level"`
	Tun                struct {
		Enable bool `json:"enable"`
	} `json:"tun"`
	DNS struct {
		Enable         bool     `json:"enable"`
		Listen         string   `json:"listen"`
		EnhancedMode   string   `json:"enhanced-mode"`
		Nameserver     []string `json:"nameserver"`
		IPv6           bool     `json:"ipv6"`
		UseHosts       bool     `json:"use-hosts"`
		UseSystemHosts bool     `json:"use-system-hosts"`
	} `json:"dns"`
	Proxies []struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Server   string `json:"server"`
		Port     uint16 `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		UDP      bool   `json:"udp"`
	} `json:"proxies"`
	Groups []struct {
		Name    string   `json:"name"`
		Type    string   `json:"type"`
		Proxies []string `json:"proxies"`
	} `json:"proxy-groups"`
	Providers map[string]json.RawMessage `json:"rule-providers"`
	Rules     []string                   `json:"rules"`
}

func Preview(c Config, source []byte) (PreviewPlan, error) {
	agentRulesBefore, _ := json.Marshal(c.Rules)
	if len(source) > 4<<20 {
		return PreviewPlan{}, fmt.Errorf("profile exceeds 4 MiB")
	}
	var p previewProfile
	d := json.NewDecoder(bytes.NewReader(source))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return PreviewPlan{}, fmt.Errorf("unsupported JSON profile: %w", err)
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return PreviewPlan{}, fmt.Errorf("trailing profile data")
	}
	if err := validatePreviewTopology(c, p); err != nil {
		return PreviewPlan{}, err
	}

	fragment, err := Render(c)
	if err != nil {
		return PreviewPlan{}, err
	}
	if len(p.Providers) != 0 {
		// A previously generated candidate is accepted only with its companion
		// Agent config and exactly the managed rules/providers we would render.
		want, _ := json.Marshal(fragment["rule-providers"])
		got, _ := json.Marshal(p.Providers)
		if !equivalentJSON(want, got) || !reflect.DeepEqual(p.Rules, fragment["rules"]) {
			return PreviewPlan{}, fmt.Errorf("existing rule-providers or managed rules differ; use the untouched source or matching companion Agent config")
		}
	} else {
		inherited, err := previewRules(p.Rules, c)
		if err != nil {
			return PreviewPlan{}, err
		}
		c.Rules = append(inherited, c.Rules...)
		fragment, err = Render(c)
		if err != nil {
			return PreviewPlan{}, err
		}
	}

	var candidate map[string]json.RawMessage
	if err := json.Unmarshal(source, &candidate); err != nil {
		return PreviewPlan{}, err
	}
	before := map[string]json.RawMessage{"dns": candidate["dns"], "rules": candidate["rules"], "rule-providers": candidate["rule-providers"]}
	var dnsConfig map[string]json.RawMessage
	_ = json.Unmarshal(candidate["dns"], &dnsConfig)
	for key, value := range fragment["dns"].(map[string]any) {
		dnsConfig[key], _ = json.Marshal(value)
	}
	// Host-file answers bypass the Gate, so this preview explicitly disables them.
	dnsConfig["use-hosts"] = json.RawMessage("false")
	dnsConfig["use-system-hosts"] = json.RawMessage("false")
	candidate["dns"], _ = json.Marshal(dnsConfig)
	candidate["rule-providers"], _ = json.Marshal(fragment["rule-providers"])
	candidate["rules"], _ = json.Marshal(fragment["rules"])
	changes := []PreviewChange{}
	for _, field := range []string{"dns", "rule-providers", "rules"} {
		if !equivalentJSON(before[field], candidate[field]) {
			changes = append(changes, PreviewChange{field, before[field], candidate[field]})
		}
	}
	agentRulesAfter, _ := json.Marshal(c.Rules)
	if !equivalentJSON(agentRulesBefore, agentRulesAfter) {
		changes = append(changes, PreviewChange{"agent_config.rules", agentRulesBefore, agentRulesAfter})
	}
	digest := sha256.Sum256(source)
	return PreviewPlan{
		SourceSHA256: hex.EncodeToString(digest[:]), Candidate: candidate, AgentConfig: c,
		Changes: changes,
		ManagedFields: []string{
			"dns.enable=true; dns.enhanced-mode=redir-host; dns.nameserver points only to Agent DNS",
			"dns.use-hosts=false; dns.use-system-hosts=false",
			"rule-providers: add managed learned-direct and learned-proxy",
			"rules: hard protection, manual, inherited explicit, configured explicit/trusted, learned, matching fallback",
			"agent_config.rules: inherit supported source domain rules for matcher/core agreement",
		},
		Limitations: []string{
			"Offline preview only; no ports bound, credentials checked, core launched or profile applied",
			"Candidate can contain source proxy credentials; keep the artifact private",
			"Does not prove port availability, upstream independence beyond visible endpoints, or API/node connectivity",
			"Official core syntax check, FlClash overwrite integration, subscription refresh and actual TUN acceptance remain untested",
		},
		Rollback: "The source file is unchanged. Discard this preview to cancel; if manually testing later, stop only the isolated core/Agent and return to the saved original profile. No active settings were changed by preview.",
	}, nil
}

func previewRules(rules []string, c Config) ([]Rule, error) {
	fallback := string(c.Fallback)
	if c.Fallback == Proxy {
		fallback = c.ProxyGroup
	}
	if len(rules) == 0 || rules[len(rules)-1] != "MATCH,"+fallback {
		return nil, fmt.Errorf("profile must end with MATCH matching the Agent fallback")
	}
	var inherited []Rule
	for i, raw := range rules[:len(rules)-1] {
		parts := strings.Split(raw, ",")
		if len(parts) != 3 || (parts[0] != "DOMAIN" && parts[0] != "DOMAIN-SUFFIX" && parts[0] != "DOMAIN-KEYWORD") {
			return nil, fmt.Errorf("unsupported profile rule at index %d; only DOMAIN, DOMAIN-SUFFIX and DOMAIN-KEYWORD are supported", i)
		}
		decision := Direct
		if parts[2] == c.ProxyGroup {
			decision = Proxy
		} else if parts[2] != "DIRECT" {
			return nil, fmt.Errorf("unsupported rule target at index %d", i)
		}
		inherited = append(inherited, Rule{Type: parts[0], Value: parts[1], Route: decision, Source: "explicit"})
	}
	return inherited, nil
}

func validatePreviewTopology(c Config, p previewProfile) error {
	if p.Mode != "rule" || p.AllowLAN || p.BindAddress != "127.0.0.1" || p.Tun.Enable || p.MixedPort == 0 {
		return fmt.Errorf("preview requires rule mode, loopback binding, a fixed mixed-port, allow-lan=false and TUN disabled")
	}
	if c.Controller == "" || validateController(c.Controller) != nil {
		return fmt.Errorf("preview requires a loopback Agent controller")
	}
	controller, _ := url.Parse(c.Controller)
	if p.ExternalController != controller.Host {
		return fmt.Errorf("profile controller must match Agent controller")
	}
	if p.DNS.EnhancedMode != "redir-host" {
		return fmt.Errorf("preview requires explicit redir-host DNS; Fake-IP is unsupported")
	}
	// The graph contains only explicit IP endpoints. Canonical comparison catches
	// mapped IPv4 and alternate port spellings without resolving any hostname.
	endpoints := map[netip.AddrPort]string{}
	for _, item := range []struct{ name, address string }{
		{"Agent DNS", c.DNSListen}, {"Agent HTTP", c.HTTPListen}, {"controller", p.ExternalController},
		{"core DNS", p.DNS.Listen}, {"core mixed", fmt.Sprintf("127.0.0.1:%d", p.MixedPort)}, {"upstream", c.Upstream},
	} {
		endpoint, err := netip.ParseAddrPort(item.address)
		if err != nil || endpoint.Port() == 0 {
			return fmt.Errorf("%s must use an IP and fixed port", item.name)
		}
		endpoint = netip.AddrPortFrom(endpoint.Addr().Unmap(), endpoint.Port())
		if item.name != "upstream" && endpoint.Addr().String() != "127.0.0.1" {
			return fmt.Errorf("%s must use loopback", item.name)
		}
		if prior, exists := endpoints[endpoint]; exists {
			return fmt.Errorf("topology collision: %s and %s", prior, item.name)
		}
		endpoints[endpoint] = item.name
	}
	if c.Mode != "off" && c.APIProxy == "" {
		return fmt.Errorf("enabled model preview requires an explicit independent API proxy")
	}
	if c.APIProxy != "" {
		u, err := parseAPIProxy(c.APIProxy)
		if err != nil {
			return err
		}
		port := uint64(80)
		if u.Port() != "" {
			port, _ = strconv.ParseUint(u.Port(), 10, 16)
		}
		endpoint := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))
		if prior, exists := endpoints[endpoint]; exists {
			return fmt.Errorf("API proxy must be independent of %s", prior)
		}
	}
	names := map[string]bool{"DIRECT": true}
	for _, proxy := range p.Proxies {
		addr, err := netip.ParseAddr(proxy.Server)
		if proxy.Type != "socks5" || err != nil || proxy.Port == 0 {
			return fmt.Errorf("preview supports only SOCKS5 nodes with literal IP servers and fixed ports")
		}
		if proxy.Name == "" || names[proxy.Name] {
			return fmt.Errorf("proxy names must be nonempty and unique")
		}
		names[proxy.Name] = true
		if _, exists := endpoints[netip.AddrPortFrom(addr.Unmap(), proxy.Port)]; exists {
			return fmt.Errorf("proxy node points back to a managed listener or upstream")
		}
	}
	groupNames := map[string]bool{}
	for _, group := range p.Groups {
		if group.Name == "" || names[group.Name] || groupNames[group.Name] || group.Type != "select" || len(group.Proxies) == 0 {
			return fmt.Errorf("preview requires unique select groups with explicit node members")
		}
		groupNames[group.Name] = true
		for _, member := range group.Proxies {
			if !names[member] {
				return fmt.Errorf("group member must be a defined static node or DIRECT; nested/dynamic groups are unsupported")
			}
		}
	}
	if !groupNames[c.ProxyGroup] {
		return fmt.Errorf("configured proxy group is missing from profile")
	}
	return nil
}
