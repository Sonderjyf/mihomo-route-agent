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
		return fmt.Errorf("usage: route-agent assess|check|preview|tail-preview|probe|observe|observe-lab|serve|render|status|explain|version [options]")
	}
	command := os.Args[1]
	if command == "version" {
		fmt.Println(version)
		return nil
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	configPath := flags.String("config", "config.example.json", "JSON configuration")
	allowLab := flags.Bool("allow-lab-fixtures", false, "enable synthetic .test fixtures and stub judge")
	profilePath := flags.String("profile", "", "explicit YAML/JSON profile copy for offline preview or assessment")
	outputPath := flags.String("output", "", "new JSON artifact (default stdout; existing files are never overwritten)")
	proxyTarget := flags.String("proxy-target", "", "declared group/node for offline tail-preview")
	stateFile := flags.String("state-file", "", "private lifecycle journal required for observe-lab")
	runFor := flags.Duration("run-for", 0, "optional bounded observer lifetime")
	allowExternal := flags.Bool("allow-external-probes", false, "explicitly permit only configured allowlisted probe targets; shadow only")
	allowModel := flags.Bool("allow-model-api", false, "explicitly permit Jev requests; does not enforce a monetary cap")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if command == "preview" || command == "assess" || command == "tail-preview" {
		if *profilePath == "" {
			return fmt.Errorf("%s requires --profile pointing to a separate YAML/JSON profile copy", command)
		}
		f, err := os.Open(*profilePath)
		if err != nil {
			return err
		}
		source, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
		f.Close()
		if err != nil {
			return err
		}
		var plan any
		if command == "assess" {
			plan, err = route.Assess(source)
		} else if command == "tail-preview" {
			plan, err = route.PreviewTail(source, *proxyTarget)
		} else {
			var c route.Config
			c, err = route.LoadConfig(*configPath, *allowLab)
			if err == nil {
				plan, err = route.Preview(c, source)
			}
		}
		if err != nil {
			return err
		}
		var output io.Writer = os.Stdout
		if *outputPath != "" {
			file, err := os.OpenFile(*outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return err
			}
			defer file.Close()
			output = file
		}
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	}
	c, err := route.LoadConfig(*configPath, *allowLab)
	if err != nil {
		return err
	}
	switch command {
	case "probe", "observe":
		if !*allowExternal || c.Observation == nil || c.Mode != "async" || len(c.LabFixtures) != 0 {
			return fmt.Errorf("explicit --allow-external-probes, async observation config and no synthetic fixtures required")
		}
		if *runFor < 0 || *runFor > 10*time.Minute {
			return fmt.Errorf("run-for must be between zero and ten minutes")
		}
		if command == "probe" && (flags.NArg() != 1 || !c.Observation.Allows(flags.Arg(0))) {
			return fmt.Errorf("probe requires one explicitly allowlisted hostname")
		}
		if command == "observe" && (flags.NArg() != 0 || *stateFile != "") {
			return fmt.Errorf("shadow observe takes no hostname or state-file")
		}
		var judge route.Judge
		if c.Judge == "jev" {
			if !*allowModel {
				return fmt.Errorf("Jev requires explicit --allow-model-api before reading credentials")
			}
			judge, err = route.NewJev(os.Getenv("OPENROUTER_API_KEY"), c.APIProxy)
			if err != nil {
				return err
			}
		} else {
			judge = route.Stub{Answer: route.Answer{Type: "choice", Choice: route.Uncertain, Probabilities: map[route.Decision]float64{route.Direct: 0, route.Proxy: 0, route.Uncertain: 1}}}
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if command == "probe" {
			collector, e := route.NewTLSCollector(c.Observation.DNS, c.Observation.Proxy, time.Duration(c.Observation.AttemptTimeoutMS)*time.Millisecond)
			if e != nil {
				return e
			}
			state, decision, e := route.EvaluateEvidence(ctx, flags.Arg(0), collector, judge)
			if e != nil {
				return e
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"shadow": true, "state": state, "decision": decision, "routing_updated": false})
		}
		observer, e := route.NewShadowObserver(c, true, judge, os.Getenv("MIHOMO_SECRET"))
		if e != nil {
			return e
		}
		if *runFor > 0 {
			var stop context.CancelFunc
			ctx, stop = context.WithTimeout(ctx, *runFor)
			defer stop()
		}
		return observer.Run(ctx, "")
	case "observe-lab":
		if *runFor < 0 || *runFor > 10*time.Minute {
			return fmt.Errorf("run-for must be between zero and ten minutes")
		}
		observer, err := route.NewLabObserver(c, *allowLab)
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if *runFor > 0 {
			var timeoutCancel context.CancelFunc
			ctx, timeoutCancel = context.WithTimeout(ctx, *runFor)
			defer timeoutCancel()
		}
		return observer.Run(ctx, *stateFile)
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
