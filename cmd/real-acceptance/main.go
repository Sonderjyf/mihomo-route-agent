package main

import (
	"context"
	"encoding/json"
	"github.com/Sonderjyf/mihomo-route-agent/internal/route"
	"os"
	"strconv"
	"time"
)

func main() {
	// Only the reviewed wrapper enables this separate executable. No product CLI
	// flag, fixtures, arbitrary endpoint, secret argv, or network-policy override.
	if os.Getenv("REAL_ACCEPTANCE_WORKER") != "1" {
		os.Exit(2)
	}
	pid, e := strconv.Atoi(os.Getenv("REAL_CORE_PID"))
	if e != nil {
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	out, e := route.RunRealAcceptance(ctx, pid, os.Getenv("OPENROUTER_API_KEY"), os.Getenv("REAL_CONTROLLER_SECRET"))
	if e != nil {
		os.Exit(3)
	}
	if json.NewEncoder(os.Stdout).Encode(out) != nil {
		os.Exit(4)
	}
}
