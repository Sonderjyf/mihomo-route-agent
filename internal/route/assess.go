package route

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Assessment contains counts and fixed vocabulary only. It is not a candidate
// profile or a claim that the core accepts or currently uses the source.
type Assessment struct {
	Scope              string         `json:"scope"`
	RuleCount          int            `json:"rule_count"`
	RuleTypes          map[string]int `json:"rule_types"`
	ContextRuleCount   int            `json:"context_rule_count"`
	ProxyTypes         map[string]int `json:"proxy_types"`
	GroupCount         int            `json:"group_count"`
	RuleProviderCount  int            `json:"rule_provider_count"`
	ProxyProviderCount int            `json:"proxy_provider_count"`
	DNSMode            string         `json:"dns_mode"`
	DNSPolicyCount     int            `json:"dns_policy_count"`
	TunEnabled         bool           `json:"tun_enabled_in_file"`
	Findings           []string       `json:"findings"`
}

func safeCategory(value, allowed string) string {
	for _, item := range strings.Fields(allowed) {
		if value == item {
			return item
		}
	}
	return "OTHER"
}

func Assess(source []byte) (Assessment, error) {
	// Parser errors can include source scalars. Do not expose them in a report
	// intended to be safe to share.
	invalid := fmt.Errorf("invalid profile structure (expected one YAML/JSON object, at most 4 MiB)")
	document, err := decodeProfileJSON(source)
	if err != nil {
		return Assessment{}, invalid
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(document, &root) != nil || root == nil {
		return Assessment{}, invalid
	}
	var p struct {
		Rules   []string `json:"rules"`
		Proxies []struct {
			Type string `json:"type"`
		} `json:"proxies"`
		Groups         []json.RawMessage          `json:"proxy-groups"`
		RuleProviders  map[string]json.RawMessage `json:"rule-providers"`
		ProxyProviders map[string]json.RawMessage `json:"proxy-providers"`
		DNS            struct {
			Mode           string                     `json:"enhanced-mode"`
			Policy         map[string]json.RawMessage `json:"nameserver-policy"`
			UseHosts       bool                       `json:"use-hosts"`
			UseSystemHosts bool                       `json:"use-system-hosts"`
		} `json:"dns"`
		Tun struct {
			Enable bool `json:"enable"`
		} `json:"tun"`
	}
	if json.Unmarshal(document, &p) != nil {
		return Assessment{}, invalid
	}
	r := Assessment{
		Scope:     "offline structural assessment only; no candidate, syntax validation, runtime inspection or changes",
		RuleCount: len(p.Rules), RuleTypes: map[string]int{}, ProxyTypes: map[string]int{},
		GroupCount: len(p.Groups), RuleProviderCount: len(p.RuleProviders), ProxyProviderCount: len(p.ProxyProviders),
		DNSMode: safeCategory(p.DNS.Mode, "fake-ip redir-host"), DNSPolicyCount: len(p.DNS.Policy), TunEnabled: p.Tun.Enable,
		Findings: []string{"Structural counts do not establish preview compatibility. Unknown fields are not validated; use preview only on a separate supported fixture."},
	}
	for _, rule := range p.Rules {
		kind, _, _ := strings.Cut(rule, ",")
		kind = safeCategory(strings.TrimSpace(kind), "DOMAIN DOMAIN-SUFFIX DOMAIN-KEYWORD DOMAIN-REGEX IP-CIDR IP-CIDR6 GEOIP GEOSITE PROCESS-NAME PROCESS-PATH PROCESS-NAME-REGEX PROCESS-PATH-REGEX RULE-SET MATCH AND OR NOT NETWORK DST-PORT SRC-PORT SRC-IP-CIDR IN-TYPE IN-NAME UID")
		r.RuleTypes[kind]++
		switch kind {
		case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "MATCH":
		default:
			r.ContextRuleCount++
		}
	}
	for _, proxy := range p.Proxies {
		r.ProxyTypes[safeCategory(proxy.Type, "socks5 http ss ssr vmess vless trojan hysteria hysteria2 tuic wireguard direct reject")]++
	}
	if r.ContextRuleCount > 0 {
		r.Findings = append(r.Findings, "Rules outside the prototype domain matcher require the core's ordered evaluation and connection context. Keep the complete original rules; do not convert these rules to UNKNOWN or insert learned rules ahead of them.")
	}
	if r.DNSMode == "fake-ip" {
		r.Findings = append(r.Findings, "Fake-IP may answer before the upstream DNS Gate runs. DNS-first preflight cannot claim coverage; retain Fake-IP for a connection-observation prototype, or explicitly choose an isolated redir-host experiment.")
	}
	if r.DNSPolicyCount > 0 || p.DNS.UseHosts || p.DNS.UseSystemHosts {
		r.Findings = append(r.Findings, "DNS policies or host answers have independent semantics and may bypass the Gate. Preserve them until a separate DNS design is accepted.")
	}
	if len(p.Proxies) != r.ProxyTypes["socks5"] || len(p.ProxyProviders) > 0 {
		r.Findings = append(r.Findings, "Proxy protocols/providers exceed the static SOCKS5 preview subset. A future adapter must preserve opaque node/provider fields and references.")
	}
	if p.Tun.Enable {
		r.Findings = append(r.Findings, "The file enables TUN. This says nothing about runtime TUN; do not launch this profile for an offline assessment.")
	}
	return r, nil
}
