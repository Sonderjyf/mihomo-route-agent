package route

import (
	"fmt"
	"net/netip"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

type Domain struct {
	Host        string `json:"hostname"`
	Registrable string `json:"registrable_domain,omitempty"`
	Local       bool   `json:"local"`
	IP          bool   `json:"ip_literal,omitempty"`
}

func Normalize(raw string) (Domain, error) {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return Domain{Host: addr.String(), Local: ProtectedIP(addr), IP: true}, nil
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || len(ascii) == 0 || len(ascii) > 253 {
		return Domain{}, fmt.Errorf("invalid hostname")
	}
	for _, label := range strings.Split(ascii, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return Domain{}, fmt.Errorf("invalid hostname label")
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return Domain{}, fmt.Errorf("invalid hostname character")
			}
		}
	}
	local := !strings.Contains(ascii, ".")
	for _, suffix := range []string{"localhost", "local", "home.arpa"} {
		local = local || ascii == suffix || strings.HasSuffix(ascii, "."+suffix)
	}
	registered, _ := publicsuffix.EffectiveTLDPlusOne(ascii)
	if local {
		registered = ""
	}
	return Domain{Host: ascii, Registrable: registered, Local: local}, nil
}

func ProtectedIP(addr netip.Addr) bool {
	addr = addr.Unmap()
	cgn := netip.MustParsePrefix("100.64.0.0/10")
	return !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || cgn.Contains(addr)
}
