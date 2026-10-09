package route

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Restart deliberately evicts, never restores, these entries. The journal is
// durable lifecycle evidence, not authority to trust cached core rules.
type observerJournal struct {
	Version int              `json:"version"`
	Owner   string           `json:"owner"`
	Phase   string           `json:"phase"`
	Entries map[string]Entry `json:"entries"`
}

func (o *LabObserver) owner() string {
	body, _ := json.Marshal(struct {
		Controller, Listen string
		Rules              []CoreRule
	}{o.c.Controller, o.c.HTTPListen, o.baseline})
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func (o *LabObserver) checkJournal() error {
	f, err := os.Open(o.statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot open observer state")
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	var state observerJournal
	if err != nil || len(body) > 1<<20 || json.Unmarshal(body, &state) != nil || state.Version != 1 || state.Owner != o.owner() {
		return fmt.Errorf("invalid or differently owned observer state; recovery required")
	}
	return nil
}

func (o *LabObserver) saveJournal(phase string) error {
	if o.statePath == "" {
		return nil
	} // poll-only unit tests have no filesystem state
	o.Providers.mu.RLock()
	entries := make(map[string]Entry, len(o.Providers.entries))
	for host, entry := range o.Providers.entries {
		entries[host] = entry
	}
	o.Providers.mu.RUnlock()
	body, err := json.Marshal(observerJournal{1, o.owner(), phase, entries})
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(o.statePath), ".observer-state-*")
	if err != nil {
		return fmt.Errorf("cannot create observer state temporary file")
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(body); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, o.statePath)
	}
	if err != nil {
		return fmt.Errorf("cannot replace observer state")
	}
	return nil
}

func (o *LabObserver) reconcile(ctx context.Context) error {
	if err := o.saveJournal("reconciling"); err != nil {
		return err
	}
	o.Providers.mu.Lock()
	o.Providers.entries = map[string]Entry{}
	o.Providers.dirty = true
	o.Providers.mu.Unlock()
	if err := o.Providers.Change(ctx, "", nil); err != nil {
		return err
	}
	if err := o.unchanged(ctx); err != nil {
		return err
	}
	return o.saveJournal("active")
}
