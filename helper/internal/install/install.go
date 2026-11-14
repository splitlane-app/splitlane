// Package install is the commands install and uninstall of splitlane-helper.exe (spec 4.3, points 2
// and 4); stage 1 has no installer (spec 13): the window runs them with UAC. install puts the helper,
// the unchanged core with its library and the licence texts into the program folder, makes the data
// folder with its access list (4.2), writes setup.json and registers the scheduled task; uninstall
// takes them away again. A helper that runs is stopped first, and nothing is replaced or removed
// until it has given the system exit node back and ended (Review Focus 4 of plan 1B). Every effect on
// the system sits behind Sys, so the tests run on a fake; System is the real one.
package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/singbox"
	"splitlane/helper/internal/store"
)

// Files are what install copies from the folder of the running exe (dist\helper) into the program
// folder, in this order (spec 4.2, 14): the helper, the unchanged core and its library under their
// own names, the core's licence, the licence texts of SplitLane.
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
	stopWait      = 5 * time.Second  // 5.4 step 2: the graceful stop of the core, then the forced end
	callTimeout   = 10 * time.Second // one call of tailscale.exe
)

// Options are what the command line and the system give install and uninstall.
type Options struct {
	UserSID    string    // install: the setup user, plain S-1-... (--user-sid); the pipe and export\ give him access
	Purge      bool      // uninstall: remove the data folder too (--purge)
	Exe        string    // the running splitlane-helper.exe: install copies the files of its folder
	ProgramDir string    // %ProgramFiles%\SplitLane (place.ProgramDir)
	DataDir    string    // %ProgramData%\SplitLane (place.DataDir)
	Hostname   string    // the computer's name: the device <Hostname>-splitlane of the uninstall note when state.json names none
	Out        io.Writer // the lines for the window: "<INSTALL|UNINSTALL>: " steps, INSTALL-OK, INSTALL-REFUSED: <reason>, UNINSTALL-NOTE: ..., UNINSTALL-OK
}

// Sys is every effect of install and uninstall on the system; System is the real one.
type Sys interface {
	// Token is the user of this process's token, S-1-..., and whether the token is elevated.
	Token() (sid string, elevated bool, err error)
	// AccountName is the account of a SID, DOMAIN\name: the user of the task.
	AccountName(sid string) (string, error)
	// ReadFile reads a file whole: install checks and copies the same bytes.
	ReadFile(path string) ([]byte, error)
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
	// After is time.After.
	After(d time.Duration) <-chan time.Time
	// WriteFile replaces path with data, making its folder when it is missing; a file in use is an error.
	WriteFile(path string, data []byte) error
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

// finish prints the last line, <label>-OK or <label>-REFUSED: <reason>, and returns err.
func finish(o Options, label string, err error) error {
	if err != nil {
		fmt.Fprintf(output(o), "%s-REFUSED: %v\n", label, err)
		return err
	}
	fmt.Fprintf(output(o), "%s-OK\n", label)
	return nil
}

// Install is `install --user-sid <SID>` (spec 4.3 point 2 and the paragraph on a repeated install), in
// this order: 1. this process is elevated and runs as the setup user; 2. the files of the folder of the
// running exe are read, the core and its library checked by SHA-256; 3. a data folder that is there
// must be a plain folder made by an administrator (a user may have made it before the first install,
// with a journal of his own in it; step 5 would refuse it anyway), then the helper that runs is
// stopped, or without one what an abnormal end left is undone (stopOld); 4. the files go into the
// program folder; 5. the data folder and export\ get their access lists; 6. setup.json (a product
// helper without it exits with code 6); 7. the scheduled task. It prints INSTALL-OK, or
// INSTALL-REFUSED: <reason> and returns the error.
func Install(o Options, s Sys) error {
	return finish(o, "INSTALL", install(o, s, progress(o, "INSTALL")))
}

func install(o Options, s Sys, say sayFunc) error {
	account, err := checkUser(o, s)
	if err != nil {
		return err
	}
	src := filepath.Dir(o.Exe)
	if inside(src, o.ProgramDir) {
		return fmt.Errorf("install copies the application's own copy into %s; it does not run from there", o.ProgramDir)
	}
	files, err := readSource(s, src)
	if err != nil {
		return err
	}
	there, err := s.CheckDir(o.DataDir)
	if err != nil {
		return fmt.Errorf("the data folder: %w", err)
	}
	if err := stopOld(o, s, there, say); err != nil {
		return err
	}
	for _, name := range Files {
		if err := s.WriteFile(filepath.Join(o.ProgramDir, name), files[name]); err != nil {
			return fmt.Errorf("the program folder: %w", err)
		}
	}
	say("%d files copied to %s", len(Files), o.ProgramDir)
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

// checkUser is step 1 (spec 4.3): the SID is plain S-1-... (it goes into access lists; place takes no
// other form), this process is elevated and runs as that user. A UAC passed under another account
// would give the task an account without administrator rights, which stage 1 does not support. It
// returns the user's account name for the task.
func checkUser(o Options, s Sys) (string, error) {
	if sid, err := windows.StringToSid(o.UserSID); err != nil || sid.String() != o.UserSID {
		return "", fmt.Errorf("--user-sid %q is not a SID in the form S-1-...", o.UserSID)
	}
	user, elevated, err := s.Token()
	switch {
	case err != nil:
		return "", fmt.Errorf("the user of this process: %w", err)
	case !elevated:
		return "", errors.New("install runs elevated: the window starts it with UAC (spec 4.3)")
	case user != o.UserSID:
		return "", fmt.Errorf("UAC was passed under another account: this process runs as %s, the setup user is %s; stage 1 needs the setup user to be an administrator (spec 4.3, 13)", user, o.UserSID)
	}
	account, err := s.AccountName(o.UserSID)
	if err != nil {
		return "", fmt.Errorf("the account of %s: %w", o.UserSID, err)
	}
	return account, nil
}

// readSource is step 2: every file of Files from the folder src, the pinned ones checked.
func readSource(s Sys, src string) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, name := range Files {
		b, err := s.ReadFile(filepath.Join(src, name))
		if err != nil {
			return nil, fmt.Errorf("the files to install are not complete in %s: %w", src, err)
		}
		if want, ok := pinned[name]; ok {
			sum := sha256.Sum256(b)
			if got := hex.EncodeToString(sum[:]); got != want {
				return nil, fmt.Errorf("%s has SHA-256 %s, want %s: only the unchanged sing-box %s is installed",
					filepath.Join(src, name), got, want, singbox.CoreVersion)
			}
		}
		files[name] = b
	}
	return files, nil
}

// stopOld is step 3 of install and the first step of uninstall (spec 4.3). A helper that has the pipe
// gets helper.shutdown and must end within helperWait: its disabling gives the system exit node back
// first (5.4), so a helper that stays (restore_pending, for example) refuses the command, nothing is
// changed, and that helper keeps trying. Without a helper on the pipe the command does the start-up's
// work of 5.7 itself: the journal first (5.4 step 1), then the helper's own core left from an earlier
// run, found by the identity in state.json and the image <ProgramDir>\sing-box.exe. "Was on" stays in
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
	return stopCore(s, filepath.Join(o.ProgramDir, "sing-box.exe"), *st.Core, say)
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
func stopCore(s Sys, exe string, id store.CoreIdentity, say sayFunc) error {
	p, ok := s.FindCore(exe, id)
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

// Uninstall is `uninstall [--purge]` (spec 4.3 point 4), in this order: the same stop as step 3 of
// install (refused while it is not confirmed; the files of a data folder that does not pass CheckDir
// are not read, and the rest goes on); the task; the program folder; with Purge the data
// folder, and a note that the embedded node stays a device of the Tailscale network. It runs from the
// application's own copy: an exe inside the program folder is refused before anything, it would be
// removed under itself. It prints UNINSTALL-OK, or UNINSTALL-REFUSED: <reason> and returns the error.
func Uninstall(o Options, s Sys) error {
	return finish(o, "UNINSTALL", uninstall(o, s, progress(o, "UNINSTALL")))
}

func uninstall(o Options, s Sys, say sayFunc) error {
	if inside(o.Exe, o.ProgramDir) {
		return fmt.Errorf("uninstall runs from the application's own copy, not from %s", o.ProgramDir)
	}
	switch _, elevated, err := s.Token(); {
	case err != nil:
		return fmt.Errorf("the user of this process: %w", err)
	case !elevated:
		return errors.New("uninstall runs elevated: the window starts it with UAC (spec 4.3)")
	}
	read, err := s.CheckDir(o.DataDir)
	if err != nil {
		say("the data folder: %v; restore.json and state.json in it are not read", err)
		read = false
	}
	if err := stopOld(o, s, read, say); err != nil {
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
	if err := s.RemoveAll(o.ProgramDir); err != nil {
		return fmt.Errorf("the program folder: %w", err)
	}
	say("%s is removed", o.ProgramDir)
	if !o.Purge {
		return nil
	}
	if err := s.RemoveAll(o.DataDir); err != nil {
		return fmt.Errorf("the data folder: %w", err)
	}
	say("%s is removed", o.DataDir)
	fmt.Fprintf(output(o), "UNINSTALL-NOTE: the device %s stays in your Tailscale network; remove it in the Tailscale admin console\n", device)
	return nil
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

// inside tells that path is dir or below it; names compare without case, as on Windows (filepath.Rel).
func inside(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
