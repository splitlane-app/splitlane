// Command slguard is the development dead-man switch of Plan 1 (safety rule 2). It runs elevated,
// is started before the helper and does not depend on the helper being right: when deadline.txt
// is not refreshed in time, when stop.flag appears or after the time cap, it ends the helper, gives
// the system Tailscale its exit node back and waits for the confirmation, ends sing-box.exe and
// checks that the adapter is gone. It is a development tool and never ships.
//
//	slguard run -dir DIR [-cap 60m] [-- HELPER COMMAND LINE]
package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "run" {
		fmt.Fprintln(os.Stderr, "usage: slguard run -dir DIR [-cap 60m] [-- HELPER COMMAND LINE]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dir := fs.String("dir", "", "folder of deadline.txt, stop.flag, cmd.json, status.json")
	hardCap := fs.Duration("cap", 60*time.Minute, "trip after this long whatever happens")
	fs.Parse(os.Args[2:])
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}
	id := make([]byte, 4)
	rand.Read(id)
	g := &Guard{
		Dir: *dir, Sys: newSystem(), Now: time.Now, Sleep: time.Sleep, Launch: fs.Args(),
		HardCap: *hardCap, MaxLead: 900 * time.Second, Tick: 2 * time.Second, RunID: hex.EncodeToString(id),
	}
	switch g.Run() {
	case "finished":
		os.Exit(0)
	case "refused":
		os.Exit(3)
	default:
		os.Exit(1)
	}
}
