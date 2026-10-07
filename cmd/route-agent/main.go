package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Sonderjyf/mihomo-route-agent/internal/route"
)

const version = "0.1.0-dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: route-agent check|serve|render|status|explain|version [options]")
	}
	command := os.Args[1]
	if command == "version" {
		fmt.Println(version)
		return nil
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	configPath := flags.String("config", "config.example.json", "JSON configuration")
	allowLab := flags.Bool("allow-lab-fixtures", false, "enable synthetic .test fixtures and stub judge")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	c, err := route.LoadConfig(*configPath, *allowLab)
	if err != nil {
		return err
	}
	switch command {
	case "check":
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"configuration": "valid", "scope": "offline only; connectivity and routing not tested", "mode": c.Mode, "judge": c.Judge, "fallback": c.Fallback})
	case "render":
		fragment, err := route.Render(c)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(fragment)
	case "explain":
		if flags.NArg() != 1 {
			return fmt.Errorf("explain requires one hostname")
		}
		d, err := route.Normalize(flags.Arg(0))
		if err != nil {
			return err
		}
		m, err := route.NewMatcher(c.Rules)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"domain": d, "static": m.Match(d), "scope": "configuration only; no API or live core query"})
	case "status":
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		client := &http.Client{Transport: transport, Timeout: time.Second}
		res, err := client.Get("http://" + c.HTTPListen + "/status")
		if err != nil {
			return fmt.Errorf("agent status unavailable")
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("status HTTP %d", res.StatusCode)
		}
		_, err = io.Copy(os.Stdout, io.LimitReader(res.Body, 65536))
		return err
	case "serve":
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
		var judge route.Judge
		if c.Mode != "off" {
			if c.Judge == "stub" {
				judge = route.Stub{Answer: route.Answer{Type: "choice", Choice: route.Proxy, Probabilities: map[route.Decision]float64{route.Direct: 0, route.Proxy: 1, route.Uncertain: 0}}, Wait: func(ctx context.Context) error {
					timer := time.NewTimer(120 * time.Millisecond)
					defer timer.Stop()
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-timer.C:
						return nil
					}
				}}
			} else {
				judge, err = route.NewJev(os.Getenv("OPENROUTER_API_KEY"), c.APIProxy)
				if err != nil {
					return err
				}
			}
		}
		a, err := route.New(ctx, c, judge, os.Getenv("MIHOMO_SECRET"))
		if err != nil {
			return err
		}
		return a.Run(ctx)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}
