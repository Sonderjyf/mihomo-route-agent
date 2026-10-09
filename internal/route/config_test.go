package route

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigEndpointValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"defaults", `{}`, true},
		{"async without runtime credentials", `{"mode":"async","controller":"http://127.0.0.1:19091","api_proxy":"http://localhost:7888"}`, true},
		{"IPv6 upstream", `{"upstream":"[::1]:15356"}`, true},
		{"empty listener port", `{"dns_listen":"127.0.0.1:"}`, false},
		{"service name port", `{"dns_listen":"127.0.0.1:domain"}`, false},
		{"zero port", `{"http_listen":"127.0.0.1:00"}`, false},
		{"out of range port", `{"http_listen":"127.0.0.1:65536"}`, false},
		{"non-loopback listener", `{"dns_listen":"0.0.0.0:15355"}`, false},
		{"colliding listeners", `{"http_listen":"127.0.0.1:015355"}`, false},
		{"same upstream", `{"upstream":"127.0.0.1:015355"}`, false},
		{"mapped same upstream", `{"upstream":"[::ffff:127.0.0.1]:15355"}`, false},
		{"HTTP upstream", `{"upstream":"127.0.0.1:18765"}`, false},
		{"zero upstream port", `{"upstream":"127.0.0.1:0"}`, false},
		{"non-loopback controller", `{"controller":"http://192.0.2.1:19091"}`, false},
		{"controller missing port", `{"controller":"http://127.0.0.1"}`, false},
		{"controller fragment", `{"controller":"http://127.0.0.1:19091#ignored"}`, false},
		{"controller empty fragment", `{"controller":"http://127.0.0.1:19091#"}`, false},
		{"proxy credentials", `{"api_proxy":"http://user:password@127.0.0.1:7888"}`, false},
		{"proxy path", `{"api_proxy":"http://127.0.0.1:7888/path"}`, false},
		{"proxy port", `{"api_proxy":"http://127.0.0.1:65536"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadConfig(path, false)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}
