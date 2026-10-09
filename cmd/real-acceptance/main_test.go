package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Sonderjyf/mihomo-route-agent/internal/route"
)

func TestWorkerInputFailureEmitsSafeResult(t *testing.T) {
	const canary = "SYNTHETIC_SECRET_CANARY_7dc161af"
	for _, enabled := range []string{"0", "1"} {
		t.Run(enabled, func(t *testing.T) {
			t.Setenv("REAL_ACCEPTANCE_WORKER", enabled)
			t.Setenv("REAL_CORE_PID", canary)
			t.Setenv("OPENROUTER_API_KEY", canary)
			t.Setenv("REAL_CONTROLLER_SECRET", canary)
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			previous := os.Stdout
			os.Stdout = writer
			defer func() { os.Stdout = previous; reader.Close(); writer.Close() }()
			code := run()
			writer.Close()
			os.Stdout = previous
			raw, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			var out route.RealAcceptanceResult
			if code != 2 || json.Unmarshal(raw, &out) != nil || out.Stage != "worker_inputs" || out.ModelAttempts != 0 || out.Reason != "guard_rejected" {
				t.Fatal("missing failure protocol")
			}
			if strings.Contains(string(raw), canary) {
				t.Fatal("input disclosed")
			}
		})
	}
}
