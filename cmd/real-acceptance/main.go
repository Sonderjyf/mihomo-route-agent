package main

import (
	"context"
	"encoding/json"
	"github.com/Sonderjyf/mihomo-route-agent/internal/route"
	"os"
	"strconv"
	"time"
)

func main() { os.Exit(run()) }

func emit(out route.RealAcceptanceResult, code int) int {
	if json.NewEncoder(os.Stdout).Encode(out) != nil {
		return 4
	}
	return code
}

func run() int {
	out := route.RealAcceptanceResult{Stage: "worker_inputs", Reason: "guard_rejected", Hosts: []route.RealHostResult{}, APIRequests: []route.RealAPIRequest{}}
	if os.Getenv("REAL_ACCEPTANCE_WORKER") != "1" {
		return emit(out, 2)
	}
	pid, e := strconv.Atoi(os.Getenv("REAL_CORE_PID"))
	if e != nil {
		return emit(out, 2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	// No deferred output on panic: an incomplete worker must report UNKNOWN in
	// the wrapper, not emit the pre-call zero-attempt result.
	out, e = route.RunRealAcceptance(ctx, pid, os.Getenv("OPENROUTER_API_KEY"), os.Getenv("REAL_CONTROLLER_SECRET"))
	if e != nil {
		return emit(out, 3)
	}
	return emit(out, 0)
}
