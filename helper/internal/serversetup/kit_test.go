package serversetup

import (
	"bytes"
	"os"
	"testing"
)

// TestKitIsTheServerKit: the embedded copy is the kit of scripts/server byte for byte, so a change of
// the kit that is not copied here fails the tests instead of shipping an old kit.
func TestKitIsTheServerKit(t *testing.T) {
	want, err := os.ReadFile("../../../scripts/server/install-singbox-server.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(Kit, want) {
		t.Fatalf("helper/internal/serversetup/kit/install-singbox-server.sh (%d bytes) differs from scripts/server/install-singbox-server.sh (%d bytes): copy it", len(Kit), len(want))
	}
	if !bytes.HasPrefix(Kit, []byte("#!/usr/bin/env bash\n")) && !bytes.HasPrefix(Kit, []byte("#!/bin/bash\n")) {
		t.Fatalf("the kit starts with %q", Kit[:min(len(Kit), 32)])
	}
	// bash on the server reads CRLF as part of the commands.
	if bytes.Contains(Kit, []byte("\r\n")) {
		t.Fatal("the kit has CRLF line ends")
	}
}
