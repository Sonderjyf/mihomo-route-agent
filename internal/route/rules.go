package route

import (
	"fmt"
	"sort"
	"strings"
)

type Decision string

const (
	Direct    Decision = "DIRECT"
	Proxy     Decision = "PROXY"
	Uncertain Decision = "UNCERTAIN"
)

func (d Decision) Valid() bool { return d == Direct || d == Proxy || d == Uncertain }

type Rule struct {
	Type   string   `json:"type"`
	Value  string   `json:"value"`
	Route  Decision `json:"route"`
	Source string   `json:"source"`
}

type Match struct {
	Decision Decision `json:"decision"`
	Source   string   `json:"source"`
}

type Matcher struct{ rules []Rule }

func NewMatcher(rules []Rule) (*Matcher, error) {
	copyRules := append([]Rule(nil), rules...)
	priority := map[string]int{"manual": 0, "explicit": 1, "trusted": 2}
	for i := range copyRules {
		r := &copyRules[i]
		if _, ok := priority[r.Source]; !ok || (r.Route != Direct && r.Route != Proxy) {
			return nil, fmt.Errorf("invalid rule source or route")
		}
		switch r.Type {
		case "DOMAIN", "DOMAIN-SUFFIX":
			d, err := Normalize(r.Value)
			if err != nil || d.IP {
				if err == nil {
					err = fmt.Errorf("IP literals are not domain rules")
				}
				return nil, err
			}
			r.Value = d.Host
		case "DOMAIN-KEYWORD":
			r.Value = strings.ToLower(r.Value)
			if r.Value == "" || strings.ContainsAny(r.Value, ",\r\n") {
				return nil, fmt.Errorf("invalid keyword")
			}
		default:
			return nil, fmt.Errorf("unsupported DNS rule type: %s", r.Type)
		}
	}
	sort.SliceStable(copyRules, func(i, j int) bool { return priority[copyRules[i].Source] < priority[copyRules[j].Source] })
	return &Matcher{rules: copyRules}, nil
}

func (m *Matcher) Match(d Domain) Match {
	if d.Local {
		return Match{Direct, "hard"}
	}
	if d.IP {
		return Match{Uncertain, "ip-literal"}
	}
	for _, r := range m.rules {
		matched := r.Type == "DOMAIN" && d.Host == r.Value || r.Type == "DOMAIN-SUFFIX" && (d.Host == r.Value || strings.HasSuffix(d.Host, "."+r.Value)) || r.Type == "DOMAIN-KEYWORD" && strings.Contains(d.Host, r.Value)
		if matched {
			return Match{r.Route, r.Source}
		}
	}
	return Match{Uncertain, "unknown"}
}

// MihomoRules uses the exact same ordered static snapshot as Match.
func (m *Matcher) MihomoRules(proxyGroup string, fallback Decision) []string {
	result := []string{"DOMAIN,localhost,DIRECT", "DOMAIN-SUFFIX,localhost,DIRECT", "DOMAIN-SUFFIX,local,DIRECT", "DOMAIN-SUFFIX,home.arpa,DIRECT",
		"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve", "IP-CIDR,10.0.0.0/8,DIRECT,no-resolve", "IP-CIDR,172.16.0.0/12,DIRECT,no-resolve", "IP-CIDR,192.168.0.0/16,DIRECT,no-resolve", "IP-CIDR,169.254.0.0/16,DIRECT,no-resolve", "IP-CIDR6,::1/128,DIRECT,no-resolve", "IP-CIDR6,fc00::/7,DIRECT,no-resolve", "IP-CIDR6,fe80::/10,DIRECT,no-resolve"}
	result = append(result, "IP-CIDR,100.64.0.0/10,DIRECT,no-resolve")
	result = append(result, "DOMAIN-REGEX,^[^.]+$,DIRECT", "IP-CIDR,0.0.0.0/8,DIRECT,no-resolve", "IP-CIDR,224.0.0.0/4,DIRECT,no-resolve", "IP-CIDR,255.255.255.255/32,DIRECT,no-resolve", "IP-CIDR6,::/128,DIRECT,no-resolve", "IP-CIDR6,ff00::/8,DIRECT,no-resolve")
	for _, r := range m.rules {
		target := string(r.Route)
		if r.Route == Proxy {
			target = proxyGroup
		}
		result = append(result, r.Type+","+r.Value+","+target)
	}
	result = append(result, "RULE-SET,learned-direct,DIRECT", "RULE-SET,learned-proxy,"+proxyGroup)
	target := string(fallback)
	if fallback == Proxy {
		target = proxyGroup
	}
	// Resolve private-IP hard rules for hostname proxy entries as well as TUN IPs.
	// no-resolve would let an unresolved private hostname reach a learned rule.
	for i := range result {
		if strings.HasPrefix(result[i], "IP-CIDR") {
			result[i] = strings.TrimSuffix(result[i], ",no-resolve")
		}
	}
	return append(result, "MATCH,"+target)
}
