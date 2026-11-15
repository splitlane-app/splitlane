// Package install is the commands prepare, install and uninstall of splitlane-helper.exe (spec 3 of
// stage 2; 4.3 of stage 1). The installer puts the files into the program folder; prepare, before it
// touches them, stops the helper of an earlier version and waits for its processes to end; install, run
// from the program folder, makes the data folder with its access list (4.2), writes setup.json and
// registers the scheduled task; uninstall stops the helper, waits and takes the task away (the files are
// the uninstaller's). prepare and install leave their result for the window in export\ (recordResult, spec
// 6 of stage 2 part 2). A helper that runs is stopped first, and nothing is replaced or removed until it
// has given the system exit node back and ended (Review Focus 4 of plan 1B). Every effect on the system
// sits behind Sys, so the tests run on a fake; System is the real one.
package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/singbox"
	"splitlane/helper/internal/store"
)

// Files are what the installer puts into the program folder, in this order (spec 4.2, 14): the helper,
// the unchanged core and its library under their own names, the core's licence, the licence texts of
// SplitLane. install reads them there and checks the core and its library.
var Files = []string{"splitlane-helper.exe", "sing-box.exe", "libcronet.dll", "sing-box-LICENSE", "LICENSE", "THIRD-PARTY.md"}

// CronetSHA256 is the SHA-256 of libcronet.dll from sing-box-1.14.4-windows-amd64.zip: the core loads
// it from its own folder.
const CronetSHA256 = "3217c6260fbca5f16072e0b79735742f40109a63bb0ff88fd6b96dd6b54a2928"

// pinned are the files whose SHA-256 install checks before anything else (spec 4.3, 11); the tests
// replace it.
var pinned = map[string]string{"sing-box.exe": singbox.CoreSHA256, "libcronet.dll": CronetSHA256}

// DataSDDL is the access list of the data folder (spec 4.2): SYSTEM and the administrators only, not
// inherited from ProgramData (P), passed on to everything inside (OICI).
const DataSDDL = "D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// ExportSDDL is the access list of export\ (spec 4.2): that of the data folder and, for the setup
// user, read and execute (0x1200a9): he may read the diagnostics archives, not change them.
func ExportSDDL(userSID string) string { return DataSDDL + "(A;OICI;0x1200a9;;;" + userSID + ")" }

const (
	helperWait    = 120 * time.Second // helper.shutdown to the end of the helper's process (spec 4.3)
	restoreEvery  = 5 * time.Second   // 5.4 step 1: the return of the system exit node is tried this often
	restoreWithin = 30 * time.Second  // ... and is refused when not confirmed within this
	restoreTries  = int(restoreWithin / restoreEvery)
	stopWait      = 5 * time.Second        // 5.4 step 2: the graceful stop of the core, then the forced end
	callTimeout   = 10 * time.Second       // one call of tailscale.exe
	waitEvery     = 500 * time.Millisecond // spec 3 step 2: the processes of the product are looked at this often
	waitWithin    = 10 * time.Second       // ... and the command is refused when some stay this long
	waitTries     = int(waitWithin / waitEvery)
)

// ResultFile is the result of prepare and install for the window, in <DataDir>\export (recordResult).
const ResultFile = "last-install.txt"

// Options are what the command line and the system give install and uninstall.
type Options struct {
	Version     string    // the version of this helper (main.version): the first line of ResultFile
	UserSID     string    // install: the setup user, plain S-1-... (--user-sid); the pipe and export\ give him access
	SessionUser bool      // prepare, install: the setup user is the user of the session of this process (--session-user)
	Purge       bool      // uninstall: remove the data folder and the user's folder too (--purge)
	Exe         string    // the running splitlane-helper.exe: install runs only from ProgramDir
	ProductDir  string    // %ProgramFiles%\SplitLane (place.ProductDir)
	ProgramDir  string    // %ProgramFiles%\SplitLane\helper (place.ProgramDir)
	DataDir     string    // %ProgramData%\SplitLane (place.DataDir)
	UserDataDir string    // %AppData%\SplitLane of the user of this process; only --purge removes it
	Hostname    string    // the computer's name: the device <Hostname>-splitlane of the uninstall note when state.json names none
	Out         io.Writer // the lines for the window: "<PREPARE|INSTALL|UNINSTALL>: " steps, <label>-OK, <label>-REFUSED: <reason>, UNINSTALL-NOTE: ...
}

// Proc is a process of the product that runs: its id and the full path of its image, empty when the
// process could not be opened (it is then treated as running).
type Proc struct {
	PID   uint32
	Image string
}

// Sys is every effect of install and uninstall on the system; System is the real one.
type Sys interface {
	// Token is the user of this process's token, S-1-..., and whether the token is elevated.
	Token() (sid string, elevated bool, err error)
	// SessionUserSID is the SID of the user logged on in the session of this process; "" without error
	// for a session that has no user (session 0 of services).
	SessionUserSID() (string, error)
	// ProductProcesses are the processes, other than this one, whose image is splitlane-helper.exe or
	// sing-box.exe under productDir, at any depth.
	ProductProcesses(productDir string) ([]Proc, error)
	// AccountName is the account of a SID, DOMAIN\name: the user of the task.
	AccountName(sid string) (string, error)
	// ReadFile reads a file whole: install checks the core and its library with it.
	ReadFile(path string) ([]byte, error)
	// StatFile is nil when the file is there.
	StatFile(path string) error
	// ShutdownHelper: when a helper has the pipe, it sends hello and helper.shutdown and waits at most
	// wait for the helper's process to end. found is false when no helper has the pipe; err tells that
	// one has it and did not end, with its last phase and reason.
	ShutdownHelper(wait time.Duration) (found bool, err error)
	// SystemStatus, SetExitNode and RestoreCommand are the system Tailscale client called by its full
	// path (5.4 step 1, 5.6; package systs).
	SystemStatus(ctx context.Context) (engine.SysStatus, error)
	SetExitNode(ctx context.Context, ip string, allowLAN bool) error
	RestoreCommand(ip string, allowLAN bool) string
	// FindCore opens the helper's core of an earlier run by its identity from state.json, only when
	// its image is exe (singbox.Driver.Find, spec 5.7).
	FindCore(exe string, id store.CoreIdentity) (engine.CoreProc, bool)
	// CheckDir tells whether the folder dir is there; one that is there must be a plain folder, not a
	// link or a junction, made by an administrator (as SecureDir requires), else the error says why.
	CheckDir(dir string) (there bool, err error)
	// WriteResult replaces the file path with data (atomicfile.Write: a temporary file in the same folder,
	// renamed over path, so a link at path is replaced, never followed): recordResult only.
	WriteResult(path string, data []byte) error
	// After is time.After.
	After(d time.Duration) <-chan time.Time
	// SecureDir makes the folder dir with the access list sddl, or gives the list to the folder that is
	// there and to everything in it; a folder that is a link, or that was not made by an administrator,
	// is refused.
	SecureDir(dir, sddl string) error
	// WriteSetup is place.WriteSetup.
	WriteSetup(dataDir string, s place.Setup) error
	// RegisterTask registers TaskName from the XML (schtasks /create /xml /f); the XML file is written
	// in dir, where only administrators write, and removed after.
	RegisterTask(xml, dir string) error
	// DeleteTask removes TaskName; a task that is not there is no error.
	DeleteTask() error
	// RemoveAll removes a folder with everything in it; a folder that is not there is no error.
	RemoveAll(dir string) error
}

// sayFunc prints one line of a step.
type sayFunc func(format string, a ...any)

func output(o Options) io.Writer {
	if o.Out == nil {
		return io.Discard
	}
	return o.Out
}

// progress prints the lines "<label>: ..." of the steps.
func progress(o Options, label string) sayFunc {
	return func(format string, a ...any) { fmt.Fprintf(output(o), label+": "+format+"\n", a...) }
}

// recordResult writes <DataDir>\export\ResultFile for the window, which reads it when it starts again after an
// update it started (spec 6 of stage 2 part 2): three lines, o.Version, ok or refused, and the reason (the text
// of the -REFUSED line, on one line; empty for ok), each ended by CRLF, UTF-8 without BOM, the file replaced
// whole. Only into a data folder and a folder export that pass CheckDir (plain folders, not links or junctions,
// made by an administrator): a refusal may come from a data folder that a user made, with links of his own in
// it, and this process writes with administrator rights. Neither folder is made here. A result that cannot be
// recorded is a line; the outcome of the command stays as it is.
func recordResult(o Options, s Sys, say sayFunc, cmdErr error) {
	export := filepath.Join(o.DataDir, "export")
	for _, dir := range []string{o.DataDir, export} {
		there, err := s.CheckDir(dir)
		switch {
		case err != nil:
			say("the result for the window is not recorded: %v", err)
			return
		case !there:
			say("the result for the window is not recorded: %s is not there", dir)
			return
		}
	}
	result, reason := "ok", ""
	if cmdErr != nil {
		result, reason = "refused", oneLine.Replace(cmdErr.Error())
	}
	if err := s.WriteResult(filepath.Join(export, ResultFile), []byte(o.Version+"\r\n"+result+"\r\n"+reason+"\r\n")); err != nil {
		say("the result for the window is not recorded: %v", err)
	}
}

// oneLine puts a text of several lines on one.
var oneLine = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")

// finish prints the last line, <label>-OK or <label>-REFUSED: <reason>, and returns err.
func finish(o Options, label string, err error) error {
	if err != nil {
		fmt.Fprintf(output(o), "%s-REFUSED: %v\n", label, err)
		return err
	}
	fmt.Fprintf(output(o), "%s-OK\n", label)
	return nil
}

// Install is `install --user-sid <SID> | --session-user` (spec 3 step 5), in this order: 1. this process is
// elevated and runs as the setup user; 2. it runs from the program folder, and the files there are read,
// the core and its library checked by SHA-256 (the installer put them there; install copies nothing);
// 3. a data folder that is there must be a plain folder made by an administrator (a user may have made it
// before the first install, with a journal of his own in it; step 4 would refuse it anyway), then the
// helper that runs is stopped, or without one what an abnormal end left is undone (stopOld); 4. the data
// folder and export\ get their access lists; 5. setup.json (a product helper without it exits with code
// 6); 6. the scheduled task. Then the result for the window, ok once the task is registered, else refused
// (recordResult). It prints INSTALL-OK, or INSTALL-REFUSED: <reason> and returns the error.
func Install(o Options, s Sys) error {
	say := progress(o, "INSTALL")
	err := install(o, s, say)
	recordResult(o, s, say, err)
	return finish(o, "INSTALL", err)
}

func install(o Options, s Sys, say sayFunc) error {
	account, o, err := checkUser("install", o, s)
	if err != nil {
		return err
	}
	if !sameDir(filepath.Dir(o.Exe), o.ProgramDir) {
		return fmt.Errorf("install runs from %s, where the installer puts the files", o.ProgramDir)
	}
	if err := readSource(s, o.ProgramDir); err != nil {
		return err
	}
	there, err := s.CheckDir(o.DataDir)
	if err != nil {
		return fmt.Errorf("the data folder: %w", err)
	}
	if err := stopOld(o, s, there, say); err != nil {
		return err
	}
	if err := s.SecureDir(o.DataDir, DataSDDL); err != nil {
		return fmt.Errorf("the data folder: %w", err)
	}
	if err := s.SecureDir(filepath.Join(o.DataDir, "export"), ExportSDDL(o.UserSID)); err != nil {
		return fmt.Errorf("the folder export: %w", err)
	}
	say("%s and its folder export have their access lists", o.DataDir)
	if err := s.WriteSetup(o.DataDir, place.Setup{UserSID: o.UserSID}); err != nil {
		return fmt.Errorf("%s: %w", place.SetupFile, err)
	}
	if err := s.RegisterTask(TaskXML(account, o.UserSID, filepath.Join(o.ProgramDir, "splitlane-helper.exe")), o.DataDir); err != nil {
		return fmt.Errorf("the task %s: %w", TaskName, err)
	}
	say("the task %s runs %s as the setup user", TaskName, filepath.Join(o.ProgramDir, "splitlane-helper.exe"))
	return nil
}

// Prepare is `prepare --session-user` (spec 3 step 2), run by the installer from its temporary folder before
// any file is touched: the user as for install, the data folder, the stop of the helper that runs
// (stopOld), then the wait until no process of the product is left. It changes no file of the program, no
// access list and no task; stopOld may give the system exit node back, drop restore.json after a confirmed
// return and stop a core left from an earlier run. An update keeps splitting as it was: helper.shutdown
// writes "was on" off in state.json, so prepare reads it before and writes it back after (keepOn). A refusal
// is recorded for the window (recordResult); a success is not: install records the outcome of the setup. It
// prints PREPARE-OK, or PREPARE-REFUSED: <reason> and returns the error.
func Prepare(o Options, s Sys) error {
	say := progress(o, "PREPARE")
	err := prepare(o, s, say)
	if err != nil {
		recordResult(o, s, say, err)
	}
	return finish(o, "PREPARE", err)
}

func prepare(o Options, s Sys, say sayFunc) error {
	_, o, err := checkUser("prepare", o, s)
	if err != nil {
		return err
	}
	there, err := s.CheckDir(o.DataDir)
	if err != nil {
		return fmt.Errorf("the data folder: %w", err)
	}
	d := store.Dir{Path: o.DataDir}
	wasOn := false
	if there { // read and written only in a data folder that passed CheckDir
		st, err := d.LoadState()
		if err != nil {
			say("state.json: %v; splitting is not switched on again after the update", err)
		}
		wasOn = err == nil && st.WasOn
	}
	if err := stopOld(o, s, there, say); err != nil {
		return err
	}
	if err := waitGone(o, s, say); err != nil {
		return err
	}
	if wasOn {
		keepOn(d, say)
	}
	return nil
}

// keepOn writes "was on" back into state.json after the helper that ran with splitting on has ended:
// helper.shutdown wrote it off (cmdShutdown), and the helper of this version switches splitting on again
// at its start, as after an abnormal end (5.2, 5.7 of spec 1). A state.json that cannot be read or written
// now is a line, not a refusal: the old helper is stopped already, and splitting then stays off.
func keepOn(d store.Dir, say sayFunc) {
	st, err := d.LoadState()
	if err == nil && st.WasOn {
		return
	}
	if err == nil {
		st.WasOn = true
		err = d.SaveState(st)
	}
	if err != nil {
		say("state.json: %v; splitting is not switched on again after the update", err)
		return
	}
	say("splitting was on: state.json says so again; the helper of this version switches it on after its start")
}

// waitGone looks every 500 ms for processes of the product (spec 3 step 2): the supervisor and the worker
// of this version, the helper and the core of stage 1. helper.shutdown waits only for the owner of the
// pipe, so the supervisor and the core may still run, with their files busy. A supervisor of stage 2 may
// start a new worker in its pause before a restart (spec 4): while processes are left, the pipe is asked
// once per look, and a helper that took it gets helper.shutdown too, with the rest of the 10 s to end.
// Some left after 10 s refuse the command. A process that cannot be looked at counts as running.
func waitGone(o Options, s Sys, say sayFunc) error {
	for attempt := 0; ; attempt++ {
		ps, err := s.ProductProcesses(o.ProductDir)
		if err != nil {
			return fmt.Errorf("the processes of %s: %w", o.ProductDir, err)
		}
		if len(ps) == 0 {
			return nil
		}
		if attempt == waitTries {
			return fmt.Errorf("%s still running (process %s); the files cannot be replaced", imageNames(ps), pids(ps))
		}
		if attempt == 0 {
			say("waiting for %s to end", imageNames(ps))
		}
		switch found, err := s.ShutdownHelper(waitWithin - time.Duration(attempt)*waitEvery); {
		case found && err != nil:
			return fmt.Errorf("the helper that took the pipe during the wait did not end: %v; nothing was changed", err)
		case found:
			say("a helper that took the pipe during the wait has ended")
		}
		<-s.After(waitEvery)
	}
}

// imageNames are the file names of the images of ps, in the order of the first appearance; a process
// without an image is named by the two images.
func imageNames(ps []Proc) string {
	var names []string
	for _, p := range ps {
		n := "splitlane-helper.exe or sing-box.exe"
		if p.Image != "" {
			n = filepath.Base(p.Image)
		}
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	return strings.Join(names, ", ")
}

func pids(ps []Proc) string {
	var ids []string
	for _, p := range ps {
		ids = append(ids, strconv.FormatUint(uint64(p.PID), 10))
	}
	return strings.Join(ids, ", ")
}

// resolveUser is the setup user: --user-sid as given; --session-user the user logged on in the session of
// this process, and the user of the process when the session has none (session 0, the runner of GitHub
// Actions).
func resolveUser(o Options, s Sys) (string, error) {
	if !o.SessionUser {
		return o.UserSID, nil
	}
	sid, err := s.SessionUserSID()
	if err != nil {
		return "", fmt.Errorf("the user of the session: %w", err)
	}
	if sid != "" {
		return sid, nil
	}
	user, _, err := s.Token()
	if err != nil {
		return "", fmt.Errorf("the user of this process: %w", err)
	}
	return user, nil
}

// checkUser is step 1 (spec 3): the setup user (resolveUser) is a plain S-1-... SID (it goes into access
// lists; place takes no other form), and this process is elevated and runs as that user. A UAC passed
// under another account would give the task an account without administrator rights, which is not
// supported. It returns the user's account name for the task and the options with UserSID filled.
func checkUser(cmd string, o Options, s Sys) (string, Options, error) {
	sid, err := resolveUser(o, s)
	if err != nil {
		return "", o, err
	}
	o.UserSID = sid
	if p, err := windows.StringToSid(o.UserSID); err != nil || p.String() != o.UserSID {
		src := "--user-sid"
		if o.SessionUser {
			src = "the session user"
		}
		return "", o, fmt.Errorf("%s %q is not a SID in the form S-1-...", src, o.UserSID)
	}
	user, elevated, err := s.Token()
	switch {
	case err != nil:
		return "", o, fmt.Errorf("the user of this process: %w", err)
	case !elevated:
		return "", o, fmt.Errorf("%s runs elevated: the installer or the window starts it with UAC (spec 4.3)", cmd)
	case user != o.UserSID:
		return "", o, fmt.Errorf("UAC was passed under another account: this process runs as %s, the setup user is %s; the setup user must be an administrator (spec 3)", user, o.UserSID)
	}
	account, err := s.AccountName(o.UserSID)
	if err != nil {
		return "", o, fmt.Errorf("the account of %s: %w", o.UserSID, err)
	}
	return account, o, nil
}

// readSource is step 2: every file of Files is there in the folder src, and the pinned ones (the core and
// its library) have their SHA-256.
func readSource(s Sys, src string) error {
	for _, name := range Files {
		want, ok := pinned[name]
		if !ok {
			if err := s.StatFile(filepath.Join(src, name)); err != nil {
				return fmt.Errorf("the files to install are not complete in %s: %w", src, err)
			}
			continue
		}
		b, err := s.ReadFile(filepath.Join(src, name))
		if err != nil {
			return fmt.Errorf("the files to install are not complete in %s: %w", src, err)
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != want {
			return fmt.Errorf("%s has SHA-256 %s, want %s: only the unchanged sing-box %s is installed",
				filepath.Join(src, name), got, want, singbox.CoreVersion)
		}
	}
	return nil
}

// stopOld is step 3 of install, the stop of prepare and the first step of uninstall (spec 4.3). A helper that has the pipe
// gets helper.shutdown and must end within helperWait: its disabling gives the system exit node back
// first (5.4), so a helper that stays (restore_pending, for example) refuses the command, nothing is
// changed, and that helper keeps trying. Without a helper on the pipe the command does the start-up's
// work of 5.7 itself: the journal first (5.4 step 1), then the helper's own core left from an earlier
// run, found by the identity in state.json and the image <ProgramDir>\sing-box.exe, else (a core of
// stage 1) <ProductDir>\sing-box.exe. "Was on" stays in
// state.json: the next helper enables again after its start-up, as after any abnormal end (5.2).
// read tells that the data folder is there and passed CheckDir: only then are its files read, with
// administrator rights, and acted on.
func stopOld(o Options, s Sys, read bool, say sayFunc) error {
	if found, err := s.ShutdownHelper(helperWait); found {
		if err != nil {
			return fmt.Errorf("the helper that runs did not end: %v; nothing was changed: it ends once the system exit node is back (spec 5.4)", err)
		}
		say("the helper that ran has ended")
		return nil
	}
	if !read {
		return nil
	}
	d := store.Dir{Path: o.DataDir}
	j, err := d.LoadJournal()
	switch {
	case errors.Is(err, store.ErrIO):
		return fmt.Errorf("restore.json is there but cannot be read: %v; nothing was changed", err)
	case err != nil:
		say("restore.json: %v; there is no exit node to give back", err)
	case j != nil:
		if err := restore(s, d, *j, say); err != nil {
			return err
		}
	}
	st, err := d.LoadState()
	if err != nil {
		say("state.json: %v; the core of an earlier run is not looked for", err)
	}
	if st.Core == nil {
		return nil
	}
	return stopCore(s, o, *st.Core, say)
}

// restore is 5.4 step 1 for install and uninstall: the exit node of the journal back, confirmed by
// `tailscale status --json`, tried every 5 s for 30 s; then the journal goes. An exit node that the
// user selected in the meantime stays. Not confirmed: the command is refused with the manual command.
// Without the system client there is nothing to give back (5.6) and the journal goes too.
func restore(s Sys, d store.Dir, j store.Journal, say sayFunc) error {
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		st, err := s.SystemStatus(ctx)
		why := err
		if err == nil && !st.Installed {
			cancel()
			say("the system Tailscale is not installed: there is no exit node to give back (spec 5.6)")
			return dropJournal(d)
		}
		if err != nil || !st.ExitNodeOn {
			why = s.SetExitNode(ctx, j.ExitNode, j.AllowLAN)
			st, err = s.SystemStatus(ctx)
			if why == nil {
				why = err
			}
		}
		cancel()
		if err == nil && st.ExitNodeOn {
			if st.ExitNodeIP != j.ExitNode {
				say("the system exit node is on with another node than the journal's: the user's choice stays")
			} else {
				say("the system exit node is back")
			}
			return dropJournal(d)
		}
		if attempt == restoreTries {
			if why == nil {
				why = errors.New("the exit node is not on")
			}
			return fmt.Errorf("the system exit node was not given back within %s (%v); give it back by hand: %s",
				restoreWithin, why, s.RestoreCommand(j.ExitNode, j.AllowLAN))
		}
		<-s.After(restoreEvery)
	}
}

func dropJournal(d store.Dir) error {
	if err := d.DeleteJournal(); err != nil {
		return fmt.Errorf("restore.json: %w", err)
	}
	return nil
}

// stopCore is 5.4 step 2 for the core left from an earlier run: the graceful signal, at most 5 s, then
// the forced end; at once when the signal fails. A core that does not end refuses the command.
func stopCore(s Sys, o Options, id store.CoreIdentity, say sayFunc) error {
	p, ok := s.FindCore(filepath.Join(o.ProgramDir, "sing-box.exe"), id)
	if !ok && o.ProductDir != "" {
		p, ok = s.FindCore(filepath.Join(o.ProductDir, "sing-box.exe"), id)
	}
	if !ok {
		return nil
	}
	say("stopping the core left from an earlier run (process %d)", id.PID)
	if err := p.Interrupt(); err != nil {
		say("the graceful stop signal failed: %v; ending the core", err)
		_ = p.Kill()
	} else if !ended(p) {
		select {
		case <-p.Done():
		case <-s.After(stopWait):
			say("the core did not stop within %s; ending it", stopWait)
			_ = p.Kill()
		}
	}
	if ended(p) {
		return nil
	}
	select {
	case <-p.Done():
		return nil
	case <-s.After(stopWait):
		return fmt.Errorf("the core left from an earlier run (process %d) does not end; nothing else was changed", id.PID)
	}
}

func ended(p engine.CoreProc) bool {
	select {
	case <-p.Done():
		return true
	default:
		return false
	}
}

// Uninstall is `uninstall [--purge]` (spec 3, Removal), in this order: this process is elevated; the same
// stop as step 3 of install (refused while it is not confirmed; the files of a data folder that does not
// pass CheckDir are not read, and the rest goes on); the wait until no process of the product is left;
// the task; with Purge the data folder, the folder %AppData%\SplitLane of the user of this process (only
// when he is the signed-in user: ownSession) and a note that the embedded node stays a device of the
// Tailscale network. The files of the program folder are the uninstaller's: it runs from there and removes
// nothing of it. It prints UNINSTALL-OK, or UNINSTALL-REFUSED: <reason> and returns the error.
func Uninstall(o Options, s Sys) error {
	return finish(o, "UNINSTALL", uninstall(o, s, progress(o, "UNINSTALL")))
}

func uninstall(o Options, s Sys, say sayFunc) error {
	user, elevated, err := s.Token()
	switch {
	case err != nil:
		return fmt.Errorf("the user of this process: %w", err)
	case !elevated:
		return errors.New("uninstall runs elevated: the uninstaller or the window starts it with UAC (spec 4.3)")
	}
	if o.Purge && o.UserDataDir != "" && !ownSession(s, user, say) {
		o.UserDataDir = "" // %AppData% of this process is not the signed-in user's: it stays
	}
	read, err := s.CheckDir(o.DataDir)
	if err != nil {
		say("the data folder: %v; restore.json and state.json in it are not read", err)
		read = false
	}
	if err := stopOld(o, s, read, say); err != nil {
		return err
	}
	if err := waitGone(o, s, say); err != nil {
		return err
	}
	device := ""
	if o.Purge {
		device = deviceName(o, read)
	}
	if err := s.DeleteTask(); err != nil {
		return fmt.Errorf("the task %s: %w", TaskName, err)
	}
	say("the task %s is removed", TaskName)
	if !o.Purge {
		return nil
	}
	if err := s.RemoveAll(o.DataDir); err != nil {
		return fmt.Errorf("the data folder: %w", err)
	}
	say("%s is removed", o.DataDir)
	if o.UserDataDir != "" {
		if err := s.RemoveAll(o.UserDataDir); err != nil {
			var pe *fs.PathError
			if errors.As(err, &pe) {
				err = pe.Err // the path is the user's: it stays out of the line
			}
			return fmt.Errorf("the window's settings folder: %v", err)
		}
		say("the settings folder of the user is removed")
	}
	fmt.Fprintf(output(o), "UNINSTALL-NOTE: the device %s stays in your Tailscale network; remove it in the Tailscale admin console\n", device)
	return nil
}

// ownSession tells that the user of this process, user, is the user of its session, as --session-user
// finds him (resolveUser: a session without a user is the process's): only then is %AppData%\SplitLane of
// this process the signed-in user's, the one --purge removes. A UAC passed under another account, or a
// session that cannot be found out, says so in a line.
func ownSession(s Sys, user string, say sayFunc) bool {
	sid, err := resolveUser(Options{SessionUser: true}, s)
	switch {
	case err != nil:
		say("%v; the window's settings stay", err)
		return false
	case sid != user:
		say("the window's settings of the signed-in user stay: UAC was passed under another account")
		return false
	}
	return true
}

// deviceName is the embedded node's device in the Tailscale network: the name state.json keeps (read
// only from a data folder that passed CheckDir), else <Hostname>-splitlane, as the helper names it.
func deviceName(o Options, read bool) string {
	if read {
		if st, err := (store.Dir{Path: o.DataDir}).LoadState(); err == nil && st.DeviceName != "" {
			return st.DeviceName
		}
	}
	return o.Hostname + "-splitlane"
}

// sameDir tells that a and b are the same folder; names compare with ASCII-only case folding (place.PathEqual).
func sameDir(a, b string) bool { return place.PathEqual(a, b) }
