package route

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// TailPlan is a private offline artifact, not an Agent runtime configuration.
// File providers start empty and are not connected to the Agent HTTP service.
type TailPlan struct {
	Candidate     map[string]json.RawMessage `json:"candidate"`
	ProviderFiles map[string]string          `json:"provider_files"`
	InsertAt      int                        `json:"insert_at"`
	Limitations   []string                   `json:"limitations"`
}

func PreviewTail(source []byte, proxyTarget string) (TailPlan, error) {
	document, err := decodeProfileJSON(source)
	if err != nil {
		return TailPlan{}, fmt.Errorf("invalid YAML/JSON profile")
	}
	var candidate map[string]json.RawMessage
	if json.Unmarshal(document, &candidate) != nil || candidate == nil {
		return TailPlan{}, fmt.Errorf("profile must be an object")
	}
	var mode string
	var rules []string
	if json.Unmarshal(candidate["mode"], &mode) != nil || mode != "rule" {
		return TailPlan{}, fmt.Errorf("tail preview requires explicit rule mode")
	}
	if json.Unmarshal(candidate["rules"], &rules) != nil || len(rules) == 0 {
		return TailPlan{}, fmt.Errorf("rules must be a nonempty string list")
	}
	// No guessing about control flow, implicit fallback or conditional MATCH.
	last := strings.Split(rules[len(rules)-1], ",")
	if len(last) != 2 || last[0] != "MATCH" || strings.TrimSpace(last[1]) == "" {
		return TailPlan{}, fmt.Errorf("rules must end with an unconditional MATCH,target")
	}
	if _, exists := candidate["sub-rules"]; exists {
		return TailPlan{}, fmt.Errorf("sub-rules require separate control-flow review")
	}
	for i, rule := range rules[:len(rules)-1] {
		kind, _, found := strings.Cut(rule, ",")
		if !found || safeCategory(kind, "DOMAIN DOMAIN-SUFFIX DOMAIN-KEYWORD DOMAIN-REGEX IP-CIDR IP-CIDR6 GEOIP GEOSITE PROCESS-NAME PROCESS-PATH PROCESS-NAME-REGEX PROCESS-PATH-REGEX RULE-SET NETWORK DST-PORT SRC-PORT SRC-IP-CIDR IN-TYPE IN-NAME UID") == "OTHER" {
			return TailPlan{}, fmt.Errorf("rule %d requires separate control-flow review", i)
		}
	}
	if strings.TrimSpace(proxyTarget) != proxyTarget || proxyTarget == "" || strings.ContainsAny(proxyTarget, ",\r\n") || safeCategory(proxyTarget, "DIRECT REJECT REJECT-DROP PASS COMPATIBLE GLOBAL") != "OTHER" {
		return TailPlan{}, fmt.Errorf("proxy target must name a profile group or node")
	}
	matches := 0
	for _, key := range []string{"proxies", "proxy-groups"} {
		var named []struct {
			Name string `json:"name"`
		}
		if raw, ok := candidate[key]; ok {
			if json.Unmarshal(raw, &named) != nil {
				return TailPlan{}, fmt.Errorf("invalid named node/group list")
			}
			for _, item := range named {
				if item.Name == proxyTarget {
					matches++
				}
			}
		}
	}
	if matches != 1 {
		return TailPlan{}, fmt.Errorf("proxy target must resolve to exactly one declared group or node")
	}
	providers := map[string]json.RawMessage{}
	if raw, ok := candidate["rule-providers"]; ok && string(raw) != "null" {
		if json.Unmarshal(raw, &providers) != nil || providers == nil {
			return TailPlan{}, fmt.Errorf("invalid rule-providers object")
		}
	}
	// Reserved names must be absent everywhere in the source, including opaque
	// provider paths and rule references. Repeated application is rejected.
	var semantic any
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	if decoder.Decode(&semantic) != nil {
		return TailPlan{}, fmt.Errorf("invalid JSON profile")
	}
	canonical, _ := json.Marshal(semantic)
	for _, name := range []string{"route-agent-tail-direct", "route-agent-tail-proxy"} {
		if strings.Contains(string(canonical), name) {
			return TailPlan{}, fmt.Errorf("reserved tail provider name/path already present")
		}
	}
	files := map[string]string{}
	for _, name := range []string{"route-agent-tail-direct", "route-agent-tail-proxy"} {
		path := "./route-agent-tail/" + name + ".yaml"
		providers[name], _ = json.Marshal(map[string]string{"type": "file", "behavior": "classical", "format": "yaml", "path": path})
		files[path] = "payload: []\n"
	}
	// Preserve every original rule string and all unrelated raw JSON fields.
	next := append([]string{}, rules[:len(rules)-1]...)
	next = append(next, "RULE-SET,route-agent-tail-direct,DIRECT", "RULE-SET,route-agent-tail-proxy,"+proxyTarget, rules[len(rules)-1])
	candidate["rules"], _ = json.Marshal(next)
	candidate["rule-providers"], _ = json.Marshal(providers)
	return TailPlan{Candidate: candidate, ProviderFiles: files, InsertAt: len(rules) - 1, Limitations: []string{
		"Offline private candidate only; contains original credentials/endpoints. No profile, provider file or live state is applied.",
		"Original rules retain order and precedence; DNS, Fake-IP, TUN and opaque node fields remain unchanged. Core syntax is not validated here.",
		"Providers start empty and use local files, not the Agent HTTP endpoint. No observer, learning, DNS gating or first-connection behavior is enabled.",
		"Do not launch a real-profile candidate. Use only a separate synthetic execution copy with TUN disabled, isolated ports and local endpoints/providers.",
	}}, nil
}
