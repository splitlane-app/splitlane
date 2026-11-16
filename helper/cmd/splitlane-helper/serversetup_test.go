package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/serversetup"
)

const ssRequest = `{"host":"127.0.0.1","port":22,"user":"root","password":"invented-pw-7391"}` + "\n"

type runFn = func(context.Context, serversetup.Request, func(serversetup.Event)) (serversetup.Done, *serversetup.Failure)

func okRun(t *testing.T) runFn {
	return func(ctx context.Context, r serversetup.Request, emit func(serversetup.Event)) (serversetup.Done, *serversetup.Failure) {
		if r.Host != "127.0.0.1" || r.Port != 22 || r.User != "root" || string(r.Password) != "invented-pw-7391" {
			t.Errorf("request: %+v", r)
		}
		emit(serversetup.Event{Step: "connect", State: "start"})
		emit(serversetup.Event{Step: "connect", State: "done", Detail: "ok"})
		sub := "https://x.example/sub"
		return serversetup.Done{Subscription: &sub, Links: "vless://a\n", Warnings: []string{}}, nil
	}
}

func neverRun(t *testing.T) runFn {
	return func(context.Context, serversetup.Request, func(serversetup.Event)) (serversetup.Done, *serversetup.Failure) {
		t.Error("run must not be called")
		return serversetup.Done{}, nil
	}
}

func TestServerSetupDone(t *testing.T) {
	var out bytes.Buffer
	if code := serverSetup(strings.NewReader(ssRequest), &out, okRun(t)); code != 0 {
		t.Fatalf("code %d", code)
	}
	want := `{"step":"connect","state":"start","detail":""}` + "\n" +
		`{"step":"connect","state":"done","detail":"ok"}` + "\n" +
		`{"done":{"subscription":"https://x.example/sub","links":"vless://a\n","warnings":[]}}` + "\n"
	if out.String() != want {
		t.Fatalf("stdout:\n%s\nwant:\n%s", out.String(), want)
	}
	if strings.Contains(out.String(), "invented-pw") {
		t.Fatal("the password is in the output")
	}
}

func TestServerSetupFailed(t *testing.T) {
	var out bytes.Buffer
	r := func(context.Context, serversetup.Request, func(serversetup.Event)) (serversetup.Done, *serversetup.Failure) {
		return serversetup.Done{}, &serversetup.Failure{Code: serversetup.CodeSSHAuth, Detail: "refused"}
	}
	if code := serverSetup(strings.NewReader(ssRequest), &out, r); code != 1 {
		t.Fatalf("code %d", code)
	}
	if want := `{"failed":{"code":"ssh_auth","detail":"refused"}}` + "\n"; out.String() != want {
		t.Fatalf("stdout %q", out.String())
	}
}

func TestServerSetupBadInput(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"only newline":     "\n",
		"garbage":          "not json\n",
		"no final newline": strings.TrimSuffix(ssRequest, "\n"),
		"too long":         `{"host":"` + strings.Repeat("a", 70000) + `"}` + "\n",
		"unknown key":      `{"host":"127.0.0.1","port":22,"user":"root","password":"invented-pw-7391","x":1}` + "\n",
		"both secrets":     `{"host":"127.0.0.1","port":22,"user":"root","password":"invented-pw-7391","key":"k"}` + "\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if code := serverSetup(strings.NewReader(in), &out, neverRun(t)); code != 1 {
				t.Fatalf("code %d", code)
			}
			if s := out.String(); !strings.HasPrefix(s, `{"failed":{"code":"bad_input"`) || strings.Count(s, "\n") != 1 {
				t.Fatalf("stdout %q", out.String())
			}
			if strings.Contains(out.String(), "invented-pw") {
				t.Fatal("the password is in the output")
			}
		})
	}
}

func TestServerSetupLineAtTheLimit(t *testing.T) {
	// A line of exactly 64 KiB reaches the parser (which refuses it as bad_input for its content, not length).
	line := `{"host":"127.0.0.1","port":22,"user":"root","password":"` + strings.Repeat("p", maxRequestLine-len(`{"host":"127.0.0.1","port":22,"user":"root","password":""}`)) + `"}`
	if len(line) != maxRequestLine {
		t.Fatalf("test line is %d", len(line))
	}
	called := false
	r := func(_ context.Context, req serversetup.Request, _ func(serversetup.Event)) (serversetup.Done, *serversetup.Failure) {
		called = len(req.Password) > 60000
		return serversetup.Done{Warnings: []string{}}, nil
	}
	var out bytes.Buffer
	serverSetup(strings.NewReader(line+"\n"), &out, r)
	if !called {
		t.Fatalf("a line of the limit was refused: %s", out.String())
	}
}

func TestServerSetupClosingStdinCancels(t *testing.T) {
	pr, pw := io.Pipe()
	started := make(chan struct{})
	r := func(ctx context.Context, _ serversetup.Request, _ func(serversetup.Event)) (serversetup.Done, *serversetup.Failure) {
		close(started)
		select {
		case <-ctx.Done():
			return serversetup.Done{}, &serversetup.Failure{Code: serversetup.CodeCancelled}
		case <-time.After(5 * time.Second):
			return serversetup.Done{Warnings: []string{}}, nil
		}
	}
	var out bytes.Buffer
	res := make(chan int)
	go func() { res <- serverSetup(pr, &out, r) }()
	pw.Write([]byte(ssRequest))
	<-started
	pw.Close()
	select {
	case code := <-res:
		if code != 1 || out.String() != `{"failed":{"code":"cancelled"}}`+"\n" {
			t.Fatalf("code %d, stdout %q", code, out.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("closing stdin did not cancel")
	}
}

func TestServerSetupNeedsNeitherDevNorDataFolder(t *testing.T) {
	prog := t.TempDir()
	oldData, oldDir, oldIn, oldOut := productData, programDir, os.Stdin, os.Stdout
	t.Cleanup(func() { productData, programDir, os.Stdin, os.Stdout = oldData, oldDir, oldIn, oldOut })
	productData = func() (string, error) {
		t.Error("the data folder was asked for")
		return filepath.Join(prog, "SplitLane"), nil
	}
	programDir = func() (string, error) { t.Error("the program folder was asked for"); return prog, nil }
	in, outF := filepath.Join(t.TempDir(), "in"), filepath.Join(t.TempDir(), "out")
	if err := os.WriteFile(in, []byte("garbage\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Open(in)
	fo, _ := os.Create(outF)
	defer fi.Close()
	defer fo.Close()
	os.Stdin, os.Stdout = fi, fo
	if code := run([]string{"server-setup"}); code != 1 {
		t.Fatalf("code %d", code)
	}
	got, _ := os.ReadFile(outF)
	if !strings.HasPrefix(string(got), `{"failed":{"code":"bad_input"`) {
		t.Fatalf("stdout %q", got)
	}
	if left, _ := os.ReadDir(prog); len(left) != 0 {
		t.Fatalf("the folder is not empty: %v", left)
	}
}
