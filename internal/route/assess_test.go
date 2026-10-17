package route

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestAssessmentCountsWithoutSensitiveValues(t *testing.T) {
	source := []byte(`mode: rule
secret: PRIVATE-SENTINEL
tun: {enable: true}
dns:
  enhanced-mode: fake-ip
  nameserver-policy: {private.example: 'https://PRIVATE-SENTINEL/dns-query'}
  use-hosts: true
proxies:
  - {name: PRIVATE-SENTINEL, type: vless, server: private.example, uuid: PRIVATE-SENTINEL}
  - {name: PRIVATE-SENTINEL, type: PRIVATE-SENTINEL}
proxy-groups:
  - {name: PRIVATE-SENTINEL, type: select, proxies: [PRIVATE-SENTINEL]}
rules:
  - PROCESS-NAME,PRIVATE-SENTINEL,DIRECT
  - IP-CIDR,192.0.2.0/24,DIRECT,no-resolve
  - GEOIP,CN,DIRECT
  - DOMAIN-SUFFIX,private.example,PRIVATE-SENTINEL
  - PRIVATE-SENTINEL,private.example,DIRECT
  - MATCH,PRIVATE-SENTINEL
`)
	before := bytes.Clone(source)
	r, err := Assess(source)
	if err != nil {
		t.Fatal(err)
	}
	if r.RuleCount != 6 || r.ContextRuleCount != 4 || r.ProxyTypes["vless"] != 1 || r.ProxyTypes["OTHER"] != 1 || r.RuleTypes["OTHER"] != 1 || r.GroupCount != 1 || r.DNSPolicyCount != 1 || !r.TunEnabled || r.DNSMode != "fake-ip" || len(r.Findings) != 6 {
		t.Fatalf("unexpected report: %+v", r)
	}
	out, _ := json.Marshal(r)
	for _, secret := range []string{"PRIVATE-SENTINEL", "private.example", "192.0.2.0", "https://"} {
		if bytes.Contains(out, []byte(secret)) {
			t.Fatalf("sensitive value leaked: %s", secret)
		}
	}
	if !bytes.Equal(before, source) {
		t.Fatal("source modified")
	}
	document, err := decodeProfileJSON(source)
	if err != nil {
		t.Fatal(err)
	}
	jsonReport, err := Assess(document)
	if err != nil || !reflect.DeepEqual(r, jsonReport) {
		t.Fatalf("JSON/YAML mismatch: %v", err)
	}
}

func TestAssessmentRejectsInvalidShapeWithoutEchoingInput(t *testing.T) {
	for _, source := range []string{
		"null", "[]", "PRIVATE-SENTINEL", "rules: PRIVATE-SENTINEL", "rules: [PRIVATE-SENTINEL\n",
		"secret: PRIVATE-SENTINEL\nsecret: duplicate", "rules: []\n---\nsecret: PRIVATE-SENTINEL",
		strings.Repeat("X", (4<<20)+1),
	} {
		_, err := Assess([]byte(source))
		if err == nil {
			t.Fatal("accepted invalid profile")
		}
		if strings.Contains(err.Error(), "PRIVATE-SENTINEL") {
			t.Fatal("error leaked input")
		}
	}
}
