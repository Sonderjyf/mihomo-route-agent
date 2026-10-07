package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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
