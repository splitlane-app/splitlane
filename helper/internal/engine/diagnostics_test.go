package engine

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// exportData gives the fake store a real data folder for diagnostics.export: logs\ with the helper's log and
// the core's, the core's with the login link of the node, and the core config with the secret of the run.
func exportData(h *harness) string {
	h.t.Helper()
	data := h.t.TempDir()
	h.store.dataDir = data
	for name, text := range map[string]string{
		"logs/helper.log":  "2026/11/14 12:00:00 splitlane-helper 0.1.0 starting\n",
		"logs/core.log":    "INFO sing-box started (0.42s)\nINFO endpoint/tailscale[ts-ep]: Waiting for authentication: https://login.tailscale.com/a/1f2e3d4c\n",
		"core-config.json": `{"experimental":{"clash_api":{"secret":"0123456789abcdef0123456789abcdef"}}}`,
	} {
		path := filepath.Join(data, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			h.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			h.t.Fatal(err)
		}
	}
	return data
}

// export calls diagnostics.export with params and reads the path from its answer as the pipe sends it.
func export(h *harness, params any) (string, *protocol.Error) {
	var raw json.RawMessage
	if params != nil {
		raw, _ = json.Marshal(params)
	}
	res, err := h.e.Handle(context.Background(), protocol.MethodDiagnosticsExport, raw)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(res)
	var out struct {
		Path string `json:"path"`
	}
	json.Unmarshal(b, &out)
	return out.Path, nil
}

// archiveEntries reads an archive: the content of each entry by its name.
func archiveEntries(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(b)
	}
	return out
}

// archiveName is the name of an archive written now: the local time of the engine's clock.
func archiveName(h *harness) string {
	return "splitlane-diagnostics-" + h.clock.Now().Local().Format("20060102-150405") + ".zip"
}

// diagnostics.export (5.1) works in every phase and outside the queue: it answers while a state-changing
// command holds the queue. The archive is export\splitlane-diagnostics-YYYYMMDD-HHMMSS.zip in the local time of
// the engine's clock, and holds the logs with the login link hidden, state.json as it is saved and meta.txt with
// the versions, the phase, its reason, the time and the journal; never the core config.
func TestDiagnosticsExportInAnyPhase(t *testing.T) {
	h := newHarness(t)
	data := exportData(h)
	h.load()
	state, _ := json.MarshalIndent(h.store.saved(), "", "  ")
	h.e.queue.Lock() // a state-changing command under way
	defer h.e.queue.Unlock()
	journal := "absent"
	for _, c := range []struct {
		phase string
		why   *protocol.Reason
	}{
		{protocol.PhaseOff, nil}, {protocol.PhaseStarting, nil}, {protocol.PhaseOn, nil},
		{protocol.PhaseDegraded, reason(protocol.ReasonVPNUnreachable)}, {protocol.PhaseBlocked, nil},
		{protocol.PhaseStopping, reason(protocol.ReasonRestorePending)}, {protocol.PhaseError, reason(protocol.ReasonCoreCrashed)},
	} {
		h.e.mu.Lock()
		h.e.setPhase(c.phase, c.why)
		if c.phase == protocol.PhaseOn {
			h.e.journal, journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}, "present"
		}
		h.e.mu.Unlock()
		answered := make(chan struct{})
		var path string
		var err *protocol.Error
		go func() {
			defer close(answered)
			path, err = export(h, nil)
		}()
		select {
		case <-answered:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: diagnostics.export waits for the queue", c.phase)
		}
		if err != nil || path != filepath.Join(data, "export", archiveName(h)) {
			t.Fatalf("%s: %q %v", c.phase, path, err)
		}
		got := archiveEntries(t, path)
		if names := slices.Sorted(maps.Keys(got)); !slices.Equal(names, []string{"logs/core.log", "logs/helper.log", "meta.txt", "state.json"}) {
			t.Fatalf("%s: entries %v", c.phase, names)
		}
		code := "-"
		if c.why != nil {
			code = c.why.Code
		}
		meta := "core: 1.14.4\nhelper: 0.1.0\njournal: " + journal + "\nphase: " + c.phase + "\nreason: " + code +
			"\nreason params: -\nstate: read\ntime: " + h.clock.Now().Local().Format(time.RFC3339) + "\n"
		if got["meta.txt"] != meta || got["state.json"] != string(state)+"\n" {
			t.Fatalf("%s: meta.txt\n%s\nstate.json\n%s", c.phase, got["meta.txt"], got["state.json"])
		}
		if log := got["logs/core.log"]; strings.Contains(log, "1f2e3d4c") || !strings.Contains(log, "Waiting for authentication: <login link>\n") {
			t.Fatalf("%s: core.log\n%s", c.phase, log)
		}
		<-h.clock.After(time.Second) // the next archive, the next second
	}
}

// Review F12-1: meta.txt holds the params of the reason, as the state line of helper.log does: sorted by key, each
// value quoted on the one line, the IPv4 addresses masked and the login links hidden. So the error of
// restore_pending and its manual command, the output of sing-box check of config_invalid, the cause of
// core_start_timeout and of switch_check_failed can be read from the archive.
func TestExportHasReasonParams(t *testing.T) {
	h := newHarness(t)
	exportData(h)
	h.load()
	h.e.mu.Lock()
	h.e.setPhase(protocol.PhaseStopping, reason(protocol.ReasonRestorePending,
		protocol.ParamCommand, "tailscale set --exit-node=100.101.102.103 --exit-node-allow-lan-access",
		protocol.ParamError, "tailscale set: exit status 1\nvisit https://login.tailscale.com/a/1f2e3d4c"))
	h.e.mu.Unlock()
	path, err := export(h, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "reason: restore_pending\nreason params: " +
		`command="tailscale set --exit-node=<ip> --exit-node-allow-lan-access" error="tailscale set: exit status 1\nvisit <login link>"` + "\n"
	if meta := archiveEntries(t, path)["meta.txt"]; !strings.Contains(meta, want) {
		t.Fatalf("meta.txt:\n%s", meta)
	}
}

// While state.json cannot be read, the state.json of the archive is the defaults in memory; meta.txt says so,
// with the read error, so that support does not take the defaults for the user's state (review of plan 1B,
// final, minor 3). Once the file reads, meta.txt says read.
func TestExportSaysStateUnreadable(t *testing.T) {
	h := newHarness(t)
	exportData(h)
	h.store.stateReadFails = 1000
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	path, err := export(h, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "state: unreadable (store: read failed: open C:\\data\\state.json: The process cannot access the file because it is being used by another process.)\n"
	if meta := archiveEntries(t, path)["meta.txt"]; !strings.Contains(meta, want) {
		t.Fatalf("meta.txt:\n%s", meta)
	}
	h.store.mu.Lock()
	h.store.stateReadFails = 0
	h.store.mu.Unlock()
	h.e.mu.Lock()
	h.e.save() // the next save reads the file first
	h.e.mu.Unlock()
	<-h.clock.After(time.Second) // the next archive, the next second
	if path, err = export(h, nil); err != nil {
		t.Fatal(err)
	}
	if meta := archiveEntries(t, path)["meta.txt"]; !strings.Contains(meta, "state: read\n") {
		t.Fatalf("meta.txt once state.json reads:\n%s", meta)
	}
}

// diagnostics.export keeps 5 archives in export\: the new one and the 4 newest others by name (the name carries
// the time); older archives are deleted, other files stay. The new archive stays also when the clock has gone
// back and its name is the oldest.
func TestExportKeepsFiveArchives(t *testing.T) {
	h := newHarness(t)
	data := exportData(h)
	h.load()
	dir := filepath.Join(data, "export")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var old []string
	for n := range 6 {
		old = append(old, fmt.Sprintf("splitlane-diagnostics-20261113-10000%d.zip", n))
	}
	for _, name := range append(slices.Clone(old), "notes.txt") {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files := func() []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return names
	}
	first, err := export(h, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := files(), append(append([]string{"notes.txt"}, old[2:]...), filepath.Base(first)); !slices.Equal(got, want) {
		t.Fatalf("after the first export: %v, want %v", got, want)
	}
	h.clock.mu.Lock()
	h.clock.now = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) // the clock has gone back
	h.clock.mu.Unlock()
	second, err := export(h, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]string{"notes.txt", filepath.Base(second)}, old[3:]...), filepath.Base(first))
	if got := files(); !slices.Equal(got, want) {
		t.Fatalf("after the clock went back: %v", got)
	}
}

// diagnostics.export takes no params (5.1): {} and none are fine; anything else is bad_request and writes
// nothing. An archive that cannot be written is internal, with the cause in the helper's log.
func TestExportBadParams(t *testing.T) {
	h := newHarness(t)
	data := exportData(h)
	h.load()
	for _, params := range []any{map[string]any{"x": 1}, []int{1}, "text", 1} {
		if _, err := export(h, params); err == nil || err.Code != protocol.CodeBadRequest {
			t.Fatalf("%v: %v", params, err)
		}
	}
	dir := filepath.Join(data, "export")
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused export writes nothing: %v", err)
	}
	if path, err := export(h, map[string]any{}); err != nil || filepath.Dir(path) != dir {
		t.Fatalf("{}: %q %v", path, err)
	}
	logEvents(h)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, nil, 0o644); err != nil { // export\ is a file: no folder can be made there
		t.Fatal(err)
	}
	if _, err := export(h, nil); err == nil || err.Code != protocol.CodeInternal {
		t.Fatalf("an archive that cannot be written: %v", err)
	}
	if h.rec.index("log:diagnostics.export failed: ") < 0 {
		t.Fatalf("not logged: %v", h.rec.list())
	}
}

// The pipe answers diagnostics.export beside other requests: a second one while one is written is busy
// instead of waiting behind it, and writes nothing.
func TestExportOneAtATime(t *testing.T) {
	h := newHarness(t)
	data := exportData(h)
	h.load()
	h.e.exporting.Lock() // an export under way
	_, err := export(h, nil)
	h.e.exporting.Unlock()
	if err == nil || err.Code != protocol.CodeBusy {
		t.Fatalf("a second export: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(data, "export")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a busy export writes nothing: %v", statErr)
	}
	if _, err := export(h, nil); err != nil {
		t.Fatalf("once the first is done: %v", err)
	}
}

// Spec 3, 11: servers.json holds links and subscription addresses; the archive never has it.
func TestExportLeavesServersOut(t *testing.T) {
	h := newHarness(t)
	data := exportData(h)
	h.load()
	if err := os.WriteFile(filepath.Join(data, "servers.json"), []byte("trojan://hunter2secretpass@h.example:443"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := export(h, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range archiveEntries(t, path) {
		if name == "servers.json" || strings.Contains(text, "hunter2secretpass") {
			t.Fatalf("%s leaks servers.json", name)
		}
	}
}
