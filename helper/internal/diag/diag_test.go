package diag

import (
	"archive/zip"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// write puts text into path, creating its folder.
func write(t *testing.T, path, text string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// entries reads the archive: the content of each entry by its name.
func entries(t *testing.T, path string) map[string]string {
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

// The decision of plan 1B: the core's log is never cut while it is written; the archive takes at most the
// last 8 MiB of each log. A longer log goes in by the whole lines of its last 8 MiB, after a first line that
// says how many bytes were left out: a cut in the middle of a line leaves that line out too (half a login
// link would not be found and hidden), a cut right after a line end keeps the next line. A shorter log goes
// in whole, without that line.
func TestBuildTakesTheTail(t *testing.T) {
	dir := t.TempDir()
	var middle strings.Builder // lines of 100 bytes: the cut falls in the middle of the first line
	for n := 0; middle.Len() < maxLog+50; n++ {
		fmt.Fprintf(&middle, "%099d\n", n)
	}
	var edge strings.Builder // one line of 18 bytes, then exactly 8 MiB of lines of 64 bytes
	edge.WriteString("this line is left\n")
	for n := 0; n < maxLog/64; n++ {
		fmt.Fprintf(&edge, "%063d\n", n)
	}
	logs := []string{
		write(t, filepath.Join(dir, "logs", "core.log"), middle.String()),
		write(t, filepath.Join(dir, "logs", "core.log.prev"), edge.String()),
		write(t, filepath.Join(dir, "logs", "helper.log"), "short\n"),
	}
	zipPath := filepath.Join(dir, "export", "a.zip")
	if err := Build(zipPath, Input{Logs: logs, State: []byte("{}\n")}); err != nil {
		t.Fatal(err)
	}
	if middle.Len() != maxLog+92 {
		t.Fatalf("core.log has %d bytes", middle.Len())
	}
	got := entries(t, zipPath)
	// the cut is 92 bytes into the first line of 100 bytes: that whole line is left out
	if want := "[splitlane: the first 100 bytes were left out]\n" + middle.String()[100:]; got["logs/core.log"] != want {
		t.Errorf("core.log: %d bytes, starts %q", len(got["logs/core.log"]), got["logs/core.log"][:min(80, len(got["logs/core.log"]))])
	}
	if want := "[splitlane: the first 18 bytes were left out]\n" + edge.String()[18:]; got["logs/core.log.prev"] != want {
		t.Errorf("core.log.prev: %d bytes, starts %q", len(got["logs/core.log.prev"]), got["logs/core.log.prev"][:min(80, len(got["logs/core.log.prev"]))])
	}
	if got["logs/helper.log"] != "short\n" {
		t.Errorf("helper.log: %q", got["logs/helper.log"])
	}
}

// The login link of the node never goes into the archive (Global Constraints of plan 1B): the core prints it
// in the line of "Waiting for authentication", Tailscale may print it in a line of its own. Every link of the
// login server and every https://<host>/a/<token> becomes <login link>, and so does every link in the line of
// "Waiting for authentication" and in the line after it. Other links and lines stay as they are.
func TestBuildRedactsLoginLinks(t *testing.T) {
	dir := t.TempDir()
	core := write(t, filepath.Join(dir, "logs", "core.log"), strings.Join([]string{
		"+0000 2026-11-14 12:00:00 INFO endpoint/tailscale[ts-ep]: Waiting for authentication: https://login.tailscale.com/a/1f2e3d4c5b",
		"+0000 2026-11-14 12:00:01 DEBUG endpoint/tailscale[ts-ep]: control: AuthURL is https://login.tailscale.com/a/6a7b8c9d0e",
		"+0000 2026-11-14 12:00:02 INFO endpoint/tailscale[ts-ep]: Waiting for authentication:",
		"https://control.example/register/nodekey:abcdef0123",
		"+0000 2026-11-14 12:00:03 INFO endpoint/tailscale[ts-ep]: visit https://control.example/a/77aa88bb",
		"+0000 2026-11-14 12:00:04 INFO outbound/direct[direct]: outbound connection to api.ipify.org:443",
		"",
	}, "\n"))
	helper := write(t, filepath.Join(dir, "logs", "helper.log"),
		"2026/11/14 12:00:05 probe https://api.ipify.org: ok\n2026/11/14 12:00:06 {\"authUrl\":\"https://login.tailscale.com/a/99ff\"}\n")
	zipPath := filepath.Join(dir, "export", "b.zip")
	if err := Build(zipPath, Input{Logs: []string{helper, core}, State: []byte("{}\n")}); err != nil {
		t.Fatal(err)
	}
	got := entries(t, zipPath)
	all := got["logs/core.log"] + got["logs/helper.log"]
	for _, token := range []string{"1f2e3d4c5b", "6a7b8c9d0e", "nodekey:abcdef0123", "77aa88bb", "99ff", "login.tailscale.com"} {
		if strings.Contains(all, token) {
			t.Errorf("%q is in the archive", token)
		}
	}
	if n := strings.Count(all, "<login link>"); n != 5 {
		t.Errorf("%d login links hidden, want 5", n)
	}
	for _, kept := range []string{"Waiting for authentication: <login link>\n", "AuthURL is <login link>\n", "visit <login link>\n",
		"outbound connection to api.ipify.org:443\n", "probe https://api.ipify.org: ok\n", `{"authUrl":"<login link>"}`} {
		if !strings.Contains(all, kept) {
			t.Errorf("missing %q", kept)
		}
	}
	if t.Failed() {
		t.Fatalf("archive:\n%s", all)
	}
}

// A log that is not there (a first run has no .prev) is left out of the archive and named in meta.txt.
// meta.txt is one "key: value" line per key, sorted by key; state.json holds the bytes given.
func TestBuildSkipsMissingLogs(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	helper := write(t, filepath.Join(logs, "helper.log"), "helper line\n")
	zipPath := filepath.Join(dir, "export", "c.zip")
	state := "{\n  \"version\": 1\n}\n"
	err := Build(zipPath, Input{
		Logs:  []string{helper, filepath.Join(logs, "helper.log.prev"), filepath.Join(logs, "core.log"), filepath.Join(logs, "core.log.prev")},
		State: []byte(state),
		Meta:  map[string]string{"phase": "off", "helper": "0.1.0", "core": "1.14.4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := entries(t, zipPath)
	if names := slices.Sorted(maps.Keys(got)); !slices.Equal(names, []string{"logs/helper.log", "meta.txt", "state.json"}) {
		t.Fatalf("entries %v", names)
	}
	if got["state.json"] != state || got["logs/helper.log"] != "helper line\n" {
		t.Fatalf("state.json %q, helper.log %q", got["state.json"], got["logs/helper.log"])
	}
	if want := "core: 1.14.4\nhelper: 0.1.0\nmissing logs: helper.log.prev, core.log, core.log.prev\nphase: off\n"; got["meta.txt"] != want {
		t.Fatalf("meta.txt:\n%s", got["meta.txt"])
	}
}

// The archive holds the logs it is given, state.json and meta.txt, nothing else of the data folder (spec 5.1,
// 11): not the core config with the secrets of the run, not the node's state folder with its key, not the
// restore journal, not the rule files. It is written to a temp file in the same folder and renamed: no temp
// file is left beside it.
func TestBuildNeverAddsOtherFiles(t *testing.T) {
	data := t.TempDir()
	const secret = "s3cr3t-0123456789abcdef"
	write(t, filepath.Join(data, "core-config.json"), `{"experimental":{"clash_api":{"secret":"`+secret+`"}}}`)
	write(t, filepath.Join(data, "tailscale", "tailscaled.state"), `{"_machinekey":"privkey:`+secret+`"}`)
	write(t, filepath.Join(data, "restore.json"), `{"exitNode":"100.64.0.1","allowLan":true}`)
	write(t, filepath.Join(data, "rules", "lane-direct.json"), `{"version":3,"rules":[]}`)
	helper := write(t, filepath.Join(data, "logs", "helper.log"), "helper line\n")
	core := write(t, filepath.Join(data, "logs", "core.log"), "core line\n")
	export := filepath.Join(data, "export")
	zipPath := filepath.Join(export, "splitlane-diagnostics-20261114-120000.zip")
	if err := Build(zipPath, Input{Logs: []string{helper, core}, State: []byte("{}\n"), Meta: map[string]string{"phase": "on"}}); err != nil {
		t.Fatal(err)
	}
	got := entries(t, zipPath)
	if names := slices.Sorted(maps.Keys(got)); !slices.Equal(names, []string{"logs/core.log", "logs/helper.log", "meta.txt", "state.json"}) {
		t.Fatalf("entries %v", names)
	}
	for name, text := range got {
		if strings.Contains(text, secret) {
			t.Fatalf("%s holds the secret", name)
		}
	}
	left, err := os.ReadDir(export)
	if err != nil || len(left) != 1 || left[0].Name() != filepath.Base(zipPath) {
		t.Fatalf("the export folder holds %v (%v)", left, err)
	}
}
