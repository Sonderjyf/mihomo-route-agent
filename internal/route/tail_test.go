package route

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const tailFixture = `{"mode":"rule","tun":{"enable":false},"dns":{"enhanced-mode":"fake-ip","nameserver-policy":{"synthetic.test":"127.0.0.1"}},"proxies":[{"name":"lab","type":"vless","uuid":"synthetic-secret","reality-opts":{"public-key":"synthetic-key"}}],"proxy-groups":[{"name":"LAB","type":"select","proxies":["lab"]}],"rule-providers":{"original":{"type":"file","behavior":"domain","path":"./original.yaml"}},"rules":["PROCESS-NAME,synthetic.exe,DIRECT","PROCESS-PATH,C:\\synthetic\\app.exe,DIRECT","IP-CIDR,192.0.2.0/24,DIRECT,no-resolve","GEOIP,LAN,DIRECT,no-resolve","DOMAIN,known.test,LAB","RULE-SET,original,DIRECT","MATCH,LAB"]}`

func TestTailPreservesOriginalSemanticsAndOnlyInsertsAtFallback(t *testing.T) {
	source := []byte(tailFixture)
	before := bytes.Clone(source)
	plan, err := PreviewTail(source, "LAB")
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]json.RawMessage
	_ = json.Unmarshal(source, &original)
	for key, raw := range original {
		if key != "rules" && key != "rule-providers" && !bytes.Equal(raw, plan.Candidate[key]) {
			t.Fatalf("changed opaque field %s", key)
		}
	}
	var oldRules, newRules []string
	_ = json.Unmarshal(original["rules"], &oldRules)
	_ = json.Unmarshal(plan.Candidate["rules"], &newRules)
	want := append(append([]string{}, oldRules[:6]...), "RULE-SET,route-agent-tail-direct,DIRECT", "RULE-SET,route-agent-tail-proxy,LAB", oldRules[6])
	if plan.InsertAt != 6 || !reflect.DeepEqual(want, newRules) {
		t.Fatalf("incorrect insertion: %v", newRules)
	}
	var oldProviders, newProviders map[string]json.RawMessage
	_ = json.Unmarshal(original["rule-providers"], &oldProviders)
	_ = json.Unmarshal(plan.Candidate["rule-providers"], &newProviders)
	if !equivalentJSON(oldProviders["original"], newProviders["original"]) || len(newProviders) != 3 || len(plan.ProviderFiles) != 2 {
		t.Fatal("provider mismatch")
	}
	for _, body := range plan.ProviderFiles {
		if body != "payload: []\n" {
			t.Fatal("nonempty initial provider")
		}
	}
	if !bytes.Equal(source, before) {
		t.Fatal("source changed")
	}
	candidate, _ := json.Marshal(plan.Candidate)
	if _, err := PreviewTail(candidate, "LAB"); err == nil {
		t.Fatal("duplicate injection accepted")
	}
}

func TestTailRejectsAmbiguousControlFlowAndCollisions(t *testing.T) {
	for _, source := range []string{
		strings.Replace(tailFixture, "MATCH,LAB", "MATCH,LAB,no-resolve", 1),
		strings.Replace(tailFixture, "PROCESS-NAME,synthetic.exe,DIRECT", "MATCH,DIRECT", 1),
		strings.Replace(tailFixture, "PROCESS-NAME,synthetic.exe,DIRECT", "SUB-RULE,(NETWORK,tcp),other", 1),
		strings.Replace(tailFixture, `"mode":"rule"`, `"mode":"global"`, 1),
		strings.Replace(tailFixture, `"mode":"rule"`, `"mode":"rule","sub-rules":{}`, 1),
		strings.Replace(tailFixture, "original.yaml", "route-agent-tail-direct.yaml", 1),
		strings.Replace(tailFixture, "original.yaml", `route-agent-tail-\u0064irect.yaml`, 1),
		strings.Replace(strings.Replace(tailFixture, "original.yaml", "route-agent-tail-direct.yaml", 1), `"mode":"rule"`, `"mode":"rule","opaque-number":1e400`, 1),
	} {
		if _, err := PreviewTail([]byte(source), "LAB"); err == nil {
			t.Fatal("accepted unsafe input")
		}
	}
	for _, target := range []string{"", "missing", "DIRECT", "LAB\n", "LAB,DIRECT"} {
		if _, err := PreviewTail([]byte(tailFixture), target); err == nil {
			t.Fatal("accepted invalid target")
		}
	}
}
