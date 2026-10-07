package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeAndObserverRequireExplicitPermissionBeforeNetwork(t *testing.T) {
	previousArgs := os.Args
	t.Cleanup(func() { os.Args = previousArgs })
	for _, command := range []string{"probe", "observe"} {
		os.Args = []string{"route-agent", command, "--config", "../../examples/observation.shadow.json", "--allow-lab-fixtures"}
		if err := run(); err == nil || !strings.Contains(err.Error(), "--allow-external-probes") {
			t.Fatalf("missing probe permission: %v", err)
		}
	}
	c, err := os.ReadFile("../../examples/observation.shadow.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "jev.json")
	if err = os.WriteFile(path, []byte(strings.Replace(string(c), `"stub"`, `"jev"`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	os.Args = []string{"route-agent", "probe", "--config", path, "--allow-external-probes", "example.com"}
	if err = run(); err == nil || !strings.Contains(err.Error(), "--allow-model-api") {
		t.Fatalf("missing model permission: %v", err)
	}
	os.Args = []string{"route-agent", "probe", "--config", path, "--allow-external-probes", "not-allowed.test"}
	if err = run(); err == nil || !strings.Contains(err.Error(), "allowlisted") {
		t.Fatalf("missing host allowlist: %v", err)
	}
}

func TestAssessNeedsNoAgentConfigAndProtectsOutput(t *testing.T) {
	dir := t.TempDir()
	profile, output := filepath.Join(dir, "profile.yaml"), filepath.Join(dir, "assessment.json")
	source := []byte("rules: ['MATCH,DIRECT']\ndns: {enhanced-mode: fake-ip}\n")
	if err := os.WriteFile(profile, source, 0600); err != nil {
		t.Fatal(err)
	}
	previousArgs := os.Args
	t.Cleanup(func() { os.Args = previousArgs })
	os.Args = []string{"route-agent", "assess", "--config", filepath.Join(dir, "does-not-exist.json"), "--profile", profile, "--output", output}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	artifact, err := os.ReadFile(output)
	if err != nil || !json.Valid(artifact) {
		t.Fatalf("missing report: %v", err)
	}
	if err := run(); err == nil {
		t.Fatal("overwrote report")
	}
	os.Args[len(os.Args)-1] = profile
	if err := run(); err == nil {
		t.Fatal("overwrote source")
	}
	after, _ := os.ReadFile(profile)
	if !bytes.Equal(source, after) {
		t.Fatal("source changed")
	}
}

func TestTailPreviewIsOfflineAndProtectsOutput(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "tail.json")
	previousArgs := os.Args
	t.Cleanup(func() { os.Args = previousArgs })
	os.Args = []string{"route-agent", "tail-preview", "--config", filepath.Join(dir, "missing.json"), "--profile", "../../examples/isolated-profile.yaml", "--proxy-target", "PROXY", "--output", output}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	artifact, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		ProviderFiles map[string]string `json:"provider_files"`
	}
	if json.Unmarshal(artifact, &plan) != nil || len(plan.ProviderFiles) != 2 {
		t.Fatal("missing offline provider artifact")
	}
	if err := run(); err == nil {
		t.Fatal("overwrote artifact")
	}
	after, _ := os.ReadFile(output)
	if !bytes.Equal(artifact, after) {
		t.Fatal("artifact changed")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatal("created files besides the plan")
	}
}

func TestPreviewWritesOnlyNewArtifact(t *testing.T) {
	source, err := os.ReadFile("../../examples/isolated-profile.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	profile := filepath.Join(dir, "profile.json")
	output := filepath.Join(dir, "preview.json")
	if err := os.WriteFile(profile, source, 0600); err != nil {
		t.Fatal(err)
	}
	previousArgs := os.Args
	t.Cleanup(func() { os.Args = previousArgs })
	os.Args = []string{"route-agent", "preview", "--config", "../../config.example.json", "--profile", profile, "--output", output}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	artifact, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var plan map[string]json.RawMessage
	if err := json.Unmarshal(artifact, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan["candidate"]) == 0 || len(plan["agent_config"]) == 0 {
		t.Fatal("missing candidate or companion config")
	}
	if err := run(); err == nil {
		t.Fatal("overwrote an existing preview")
	}
	after, _ := os.ReadFile(output)
	if !bytes.Equal(artifact, after) {
		t.Fatal("existing artifact changed")
	}
	os.Args[len(os.Args)-1] = profile
	if err := run(); err == nil {
		t.Fatal("allowed overwriting the source profile")
	}
	after, _ = os.ReadFile(profile)
	if !bytes.Equal(source, after) {
		t.Fatal("source profile changed")
	}
}
