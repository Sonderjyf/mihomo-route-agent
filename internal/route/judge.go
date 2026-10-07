package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Evidence struct {
	DirectTLS string `json:"direct_tls"`
	ProxyTLS  string `json:"proxy_tls"`
}

type State struct {
	Hostname    string   `json:"hostname"`
	Registrable string   `json:"registrable_domain,omitempty"`
	Evidence    Evidence `json:"evidence"`
	Fixture     string   `json:"fixture,omitempty"`
}

type Answer struct {
	Type          string               `json:"type"`
	Choice        Decision             `json:"choice"`
	Probabilities map[Decision]float64 `json:"probabilities"`
	Confidence    float64              `json:"confidence"`
}

type Judge interface {
	Decide(context.Context, State) (Answer, error)
}

// Accept never treats a domain name or a concentrated distribution as a network fact.
func Accept(s State, answer Answer) Decision {
	if answer.Type != "choice" || !answer.Choice.Valid() || len(answer.Probabilities) != 3 {
		return Uncertain
	}
	total := 0.0
	for _, label := range []Decision{Direct, Proxy, Uncertain} {
		p, ok := answer.Probabilities[label]
		if !ok || math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			return Uncertain
		}
		total += p
	}
	if math.Abs(total-1) > .02 {
		return Uncertain
	}
	permitted := Uncertain
	if s.Evidence.DirectTLS == "verified_success" {
		permitted = Direct
	}
	if s.Evidence.DirectTLS == "repeated_failure" && s.Evidence.ProxyTLS == "verified_success" {
		permitted = Proxy
	}
	if answer.Choice == permitted && answer.Probabilities[permitted] >= .8 {
		return permitted
	}
	return Uncertain
}

type Jev struct {
	client *http.Client
	key    string
}

func parseAPIProxy(proxy string) (*url.URL, error) {
	if proxy == "" {
		return nil, nil
	}
	u, err := url.Parse(proxy)
	if err != nil || u.Scheme != "http" || u.User != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, fmt.Errorf("proxy must be a loopback HTTP URL")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("proxy port must be from 1 to 65535")
		}
	}
	return u, nil
}

func NewJev(key, proxy string) (*Jev, error) {
	if key == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is required")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	u, err := parseAPIProxy(proxy)
	if err != nil {
		return nil, err
	}
	if u != nil {
		transport.Proxy = http.ProxyURL(u)
	}
	return &Jev{key: key, client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (j *Jev) Decide(ctx context.Context, state State) (Answer, error) {
	body := map[string]any{"model": "typesafe/jev-1.13", "state": state, "questions": map[string]any{"route": map[string]any{
		"type": "choice", "instructions": "Choose only from supplied evidence. Domain spelling, TLD and model memory are not current reachability evidence. Missing or conflicting evidence requires UNCERTAIN.",
		"criteria": map[string]string{"DIRECT": "Verified direct TLS success without conflicting evidence.", "PROXY": "Repeated direct failure AND verified proxy TLS success.", "UNCERTAIN": "Missing, untested, conflicting or insufficient evidence. Never infer DIRECT from low PROXY probability."}}}}
	wire, err := json.Marshal(body)
	if err != nil {
		return Answer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/alpha/decisions", strings.NewReader(string(wire)))
	if err != nil {
		return Answer{}, err
	}
	req.Header.Set("Authorization", "Bearer "+j.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := j.client.Do(req)
	if err != nil {
		return Answer{}, fmt.Errorf("jev transport failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Answer{}, fmt.Errorf("jev HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 262145))
	if err != nil || len(raw) > 262144 {
		return Answer{}, fmt.Errorf("invalid jev response size")
	}
	var response struct {
		Answers map[string]Answer `json:"answers"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return Answer{}, fmt.Errorf("invalid jev JSON")
	}
	answer, ok := response.Answers["route"]
	if !ok {
		return Answer{}, fmt.Errorf("missing route answer")
	}
	return answer, nil
}

// Stub is used only when the executable is launched with --allow-lab-fixtures.
type Stub struct {
	Answer Answer
	Wait   func(context.Context) error
}

func (s Stub) Decide(ctx context.Context, _ State) (Answer, error) {
	if s.Wait != nil {
		if err := s.Wait(ctx); err != nil {
			return Answer{}, err
		}
	}
	return s.Answer, nil
}
