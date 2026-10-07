package route

import (
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strings"
)

type Config struct {
	DNSListen         string              `json:"dns_listen"`
	HTTPListen        string              `json:"http_listen"`
	Upstream          string              `json:"upstream"`
	Controller        string              `json:"controller"`
	ProxyGroup        string              `json:"proxy_group"`
	Mode              string              `json:"mode"`
	Judge             string              `json:"judge"`
	APIProxy          string              `json:"api_proxy"`
	Fallback          Decision            `json:"fallback"`
	PreflightMS       int                 `json:"preflight_ms"`
	DNSDeadlineMS     int                 `json:"dns_deadline_ms"`
	LearnedTTLSeconds int                 `json:"learned_ttl_seconds"`
	Capacity          int                 `json:"capacity"`
	MaxAPIRequests    int                 `json:"max_api_requests"`
	Rules             []Rule              `json:"rules"`
	LabFixtures       map[string]Evidence `json:"lab_fixtures,omitempty"`
}

func LoadConfig(path string, allowLab bool) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	c := Config{DNSListen: "127.0.0.1:15355", HTTPListen: "127.0.0.1:18765", Upstream: "127.0.0.1:15356", ProxyGroup: "PROXY", Mode: "off", Judge: "jev", Fallback: Direct, PreflightMS: 1500, DNSDeadlineMS: 2500, LearnedTTLSeconds: 604800, Capacity: 1024, MaxAPIRequests: 100}
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return Config{}, err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return Config{}, fmt.Errorf("trailing configuration data")
	}
	if c.Mode != "off" && c.Mode != "async" && c.Mode != "bounded-preflight" {
		return Config{}, fmt.Errorf("invalid mode")
	}
	if c.Judge != "jev" && c.Judge != "stub" {
		return Config{}, fmt.Errorf("invalid judge")
	}
	if (len(c.LabFixtures) > 0 || c.Judge == "stub") && !allowLab {
		return Config{}, fmt.Errorf("lab fixtures/stub require --allow-lab-fixtures")
	}
	if c.Fallback != Direct && c.Fallback != Proxy {
		return Config{}, fmt.Errorf("fallback must be DIRECT or PROXY")
	}
	if c.PreflightMS < 50 || c.PreflightMS > 10000 || c.DNSDeadlineMS < c.PreflightMS+100 || c.DNSDeadlineMS > 15000 || c.LearnedTTLSeconds < 1 || c.Capacity < 1 || c.Capacity > 100000 || c.MaxAPIRequests < 1 {
		return Config{}, fmt.Errorf("invalid resource/deadline limits")
	}
	for _, address := range []*string{&c.DNSListen, &c.HTTPListen} {
		endpoint, e := netip.ParseAddrPort(*address)
		if e != nil || endpoint.Addr().String() != "127.0.0.1" || endpoint.Port() == 0 {
			return Config{}, fmt.Errorf("listeners must use 127.0.0.1 and a fixed port")
		}
		*address = endpoint.String()
	}
	if c.DNSListen == c.HTTPListen {
		return Config{}, fmt.Errorf("DNS and HTTP listeners must use different ports")
	}
	upstream, err := netip.ParseAddrPort(c.Upstream)
	if err != nil || upstream.Port() == 0 {
		return Config{}, fmt.Errorf("upstream must use an IP and a fixed port")
	}
	c.Upstream = netip.AddrPortFrom(upstream.Addr().Unmap(), upstream.Port()).String()
	if c.Upstream == c.DNSListen || c.Upstream == c.HTTPListen {
		return Config{}, fmt.Errorf("upstream must be an IP:port independent of Gate")
	}
	if err = validateController(c.Controller); err != nil {
		return Config{}, err
	}
	if _, err = parseAPIProxy(c.APIProxy); err != nil {
		return Config{}, err
	}
	if c.ProxyGroup == "" || strings.ContainsAny(c.ProxyGroup, ",\r\n") {
		return Config{}, fmt.Errorf("invalid proxy group")
	}
	if _, err = NewMatcher(c.Rules); err != nil {
		return Config{}, err
	}
	for host := range c.LabFixtures {
		d, err := Normalize(host)
		if err != nil || d.Host != host || !strings.HasSuffix(host, ".test") {
			return Config{}, fmt.Errorf("fixtures require normalized .test hostnames")
		}
	}
	return c, nil
}

func Render(c Config) (map[string]any, error) {
	m, err := NewMatcher(c.Rules)
	if err != nil {
		return nil, err
	}
	providers := map[string]any{}
	for _, name := range []string{"learned-direct", "learned-proxy"} {
		providers[name] = map[string]any{"type": "http", "behavior": "classical", "format": "yaml", "url": "http://" + c.HTTPListen + "/rules/" + name + ".yaml", "path": "./route-agent/" + name + ".yaml", "interval": 3600}
	}
	return map[string]any{"dns": map[string]any{"enable": true, "enhanced-mode": "redir-host", "nameserver": []string{"udp://" + c.DNSListen}}, "rule-providers": providers, "rules": m.MihomoRules(c.ProxyGroup, c.Fallback)}, nil
}
