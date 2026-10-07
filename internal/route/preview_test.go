package route

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func previewFixture(t *testing.T) (Config, []byte) {
	t.Helper()
	c, err := LoadConfig("../../config.example.json", false)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../examples/isolated-profile.json")
	if err != nil {
		t.Fatal(err)
	}
	return c, source
}

func TestPreviewPreservesInputAndMatchesAgent(t *testing.T) {
	c, source := previewFixture(t)
	before := append([]byte(nil), source...)
	plan, err := Preview(c, source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, before) || len(c.Rules) != 2 {
		t.Fatal("preview mutated caller input")
	}
	var original map[string]json.RawMessage
	_ = json.Unmarshal(source, &original)
	for key, value := range original {
		if key != "dns" && key != "rules" && key != "rule-providers" && !bytes.Equal(value, plan.Candidate[key]) {
			t.Errorf("unmanaged field changed: %s", key)
		}
	}
	matcher, err := NewMatcher(plan.AgentConfig.Rules)
	if err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]Decision{"known.route-lab.test": Direct, "x.remote.route-lab.test": Proxy, "direct.route-lab.test": Direct, "x.proxy.route-lab.test": Proxy} {
		d, _ := Normalize(host)
		if got := matcher.Match(d); got.Decision != want {
			t.Errorf("%s: %+v", host, got)
		}
	}
	fragment, _ := Render(plan.AgentConfig)
	wantRules, _ := json.Marshal(fragment["rules"])
	if !bytes.Equal(wantRules, plan.Candidate["rules"]) {
		t.Fatal("candidate differs from companion Agent rules")
	}
	candidate, _ := json.Marshal(plan.Candidate)
	again, err := Preview(plan.AgentConfig, candidate)
	if err != nil {
		t.Fatal(err)
	}
	againCandidate, _ := json.Marshal(again.Candidate)
	if !bytes.Equal(againCandidate, candidate) || !reflect.DeepEqual(again.AgentConfig, plan.AgentConfig) {
		t.Fatal("repeated preview is not idempotent")
	}
	if len(plan.Changes) != 4 || len(again.Changes) != 0 {
		t.Fatalf("incorrect change list: first=%d again=%d", len(plan.Changes), len(again.Changes))
	}
}

func TestPreviewRejectsUnsupportedSemanticsAndLoops(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any, *Config)
	}{
		{"process rule", func(p map[string]any, c *Config) {
			p["rules"] = []string{"PROCESS-NAME,app.exe,DIRECT", "MATCH,DIRECT"}
		}},
		{"different fallback", func(p map[string]any, c *Config) { p["rules"] = []string{"MATCH,PROXY"} }},
		{"unrecognized field", func(p map[string]any, c *Config) { p["listeners"] = []any{} }},
		{"uppercase DNS field", func(p map[string]any, c *Config) { p["DNS"] = p["dns"]; delete(p, "dns") }},
		{"dynamic provider", func(p map[string]any, c *Config) { p["proxy-providers"] = map[string]any{} }},
		{"TUN enabled", func(p map[string]any, c *Config) { p["tun"] = map[string]any{"enable": true} }},
		{"Fake-IP", func(p map[string]any, c *Config) { p["dns"].(map[string]any)["enhanced-mode"] = "fake-ip" }},
		{"DNS bypass", func(p map[string]any, c *Config) { p["dns"].(map[string]any)["nameserver-policy"] = map[string]any{} }},
		{"core upstream loop", func(p map[string]any, c *Config) { c.Upstream = "127.0.0.1:15354" }},
		{"mapped upstream loop", func(p map[string]any, c *Config) { c.Upstream = "[::ffff:127.0.0.1]:15354" }},
		{"mixed collision", func(p map[string]any, c *Config) { p["mixed-port"] = float64(15355) }},
		{"controller mismatch", func(p map[string]any, c *Config) { p["external-controller"] = "127.0.0.1:19092" }},
		{"API bootstrap loop", func(p map[string]any, c *Config) { c.Mode = "async"; c.APIProxy = "http://localhost:17891" }},
		{"API direct unspecified", func(p map[string]any, c *Config) { c.Mode = "async"; c.APIProxy = "" }},
		{"node hostname", func(p map[string]any, c *Config) {
			p["proxies"].([]any)[0].(map[string]any)["server"] = "proxy.example.test"
		}},
		{"node loop", func(p map[string]any, c *Config) { p["proxies"].([]any)[0].(map[string]any)["port"] = float64(17891) }},
		{"nested group", func(p map[string]any, c *Config) {
			p["proxy-groups"].([]any)[0].(map[string]any)["proxies"] = []string{"PROXY"}
		}},
		{"provider collision", func(p map[string]any, c *Config) {
			p["rule-providers"] = map[string]any{"learned-direct": map[string]any{"type": "file"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, source := previewFixture(t)
			var p map[string]any
			_ = json.Unmarshal(source, &p)
			tc.mutate(p, &c)
			changed, _ := json.Marshal(p)
			plan, err := Preview(c, changed)
			if err == nil || plan.Candidate != nil {
				t.Fatal("unsafe or unsupported profile produced a candidate")
			}
		})
	}
}

func TestPreviewYAMLMatchesJSON(t *testing.T) {
	c, source := previewFixture(t)
	jsonPlan, err := Preview(c, source)
	if err != nil {
		t.Fatal(err)
	}
	yamlSource, err := os.ReadFile("../../examples/isolated-profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	yamlPlan, err := Preview(c, yamlSource)
	if err != nil {
		t.Fatal(err)
	}
	jsonCandidate, _ := json.Marshal(jsonPlan.Candidate)
	yamlCandidate, _ := json.Marshal(yamlPlan.Candidate)
	if !equivalentJSON(jsonCandidate, yamlCandidate) || !reflect.DeepEqual(jsonPlan.AgentConfig, yamlPlan.AgentConfig) {
		t.Fatal("YAML and JSON produced different routing semantics")
	}
	for _, suffix := range []string{"\n---\nmode: global\n", "\nmode: global\n"} {
		if _, err := Preview(c, append(append([]byte(nil), yamlSource...), []byte(suffix)...)); err == nil {
			t.Fatal("accepted multiple documents or duplicate keys")
		}
	}
	if _, err := Preview(c, bytes.Replace(yamlSource, []byte("dns:"), []byte("DNS:"), 1)); err == nil {
		t.Fatal("accepted noncanonical DNS field")
	}
	if _, err := Preview(c, bytes.Replace(yamlSource, []byte("port: 17892"), []byte("port: 17892\n    password: 2026-10-07"), 1)); err == nil {
		t.Fatal("silently converted a timestamp-shaped password")
	}
}
