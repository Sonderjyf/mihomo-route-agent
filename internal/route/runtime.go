package route

import (
	"context"
	"fmt"
	"time"
)

// ObservationConfig is opt-in and always shadow-only. It cannot authorize a
// routing update merely by asserting that a physical network path is direct.
type ObservationConfig struct {
	DNS              string   `json:"dns"`
	Proxy            string   `json:"proxy"`
	Hosts            []string `json:"hosts"`
	AttemptTimeoutMS int      `json:"attempt_timeout_ms"`
}

func (c ObservationConfig) Validate() error {
	if len(c.Hosts) == 0 || len(c.Hosts) > 128 {
		return fmt.Errorf("observation requires 1..128 explicit allowed hosts")
	}
	seen := map[string]bool{}
	for _, host := range c.Hosts {
		d, err := Normalize(host)
		if err != nil || d.Host != host || d.Local || d.IP || seen[host] {
			return fmt.Errorf("observation hosts must be unique normalized nonlocal hostnames")
		}
		seen[host] = true
	}
	_, err := NewTLSCollector(c.DNS, c.Proxy, time.Duration(c.AttemptTimeoutMS)*time.Millisecond)
	return err
}

func (c ObservationConfig) Allows(host string) bool {
	for _, allowed := range c.Hosts {
		if host == allowed {
			return true
		}
	}
	return false
}

func NewShadowObserver(c Config, allowExternal bool, judge Judge, controllerSecret string) (*LabObserver, error) {
	if !allowExternal || c.Mode != "async" || c.Controller == "" || c.Observation == nil || len(c.LabFixtures) != 0 || judge == nil {
		return nil, fmt.Errorf("observe requires explicit probe permission, async mode, observation settings, controller and no fixtures")
	}
	if err := c.Observation.Validate(); err != nil {
		return nil, err
	}
	o, err := newObserver(c, judge)
	if err != nil {
		return nil, err
	}
	o.shadow = true
	o.Providers.secret = controllerSecret
	o.collector, err = NewTLSCollector(c.Observation.DNS, c.Observation.Proxy, time.Duration(c.Observation.AttemptTimeoutMS)*time.Millisecond)
	return o, err
}

type countedJudge struct{ observer *LabObserver }

func (j countedJudge) Decide(ctx context.Context, state State) (Answer, error) {
	j.observer.judgments.Add(1)
	return j.observer.judge.Decide(ctx, state)
}
