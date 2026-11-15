package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/atomicfile"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/singbox"
	"splitlane/helper/internal/store"
)

const (
	testSID  = "S-1-5-21-1000-2000-3000-1001"
	otherSID = "S-1-5-21-1000-2000-3000-1002"
	exitNode = "100.64.0.1" // the tailnet range, as in the engine's tests
	account  = `TESTPC\tester`
)

// fakeSys plays the system. It records every call in one ordered list; each entry starts with its
// step: sid, hash, stop, copy, acl, setup, task, remove.
type fakeSys struct {
	log      []string
	user     string            // the user of the token
	elevated bool              // the token is elevated
	files    map[string][]byte // what ReadFile finds, by full path
	helper   string            // "none": no helper has the pipe; "ends": it ends after helper.shutdown; "stays": it does not
	sys      engine.SysStatus  // the system Tailscale
	setTakes bool              // SetExitNode turns the exit node on
	core     *fakeCore         // the core of an earlier run that FindCore finds
	dirOwner string            // the owner CheckDir finds for a folder that is there; "" is the administrators
	xml      string            // the XML of the task registered
	session  string            // the user of the session (SessionUserSID); "" is a session without a user
	sessErr  error             // when set, SessionUserSID fails with it
	procs    [][]Proc          // the answers of ProductProcesses, one per call; none left is "no process"
	stay     []Proc            // when set, every call of ProductProcesses answers this
	data     string            // the data folder: a helper that ends saves its state.json there, "was on" off
	sup      *supervisor       // when set, the supervisor of stage 2 answers ProductProcesses
	taskErr  error             // when set, RegisterTask fails with it
	writeErr error             // when set, WriteResult fails with it; else it writes with atomicfile
}

// supervisor is the supervisor of stage 2 in its pause before a restart (spec 4): it runs, and from the call
// restartAt of ProductProcesses on, a new worker of it has the pipe. That worker and the supervisor end only
// after helper.shutdown.
type supervisor struct {
	restartAt, calls int
	worker, ended    bool
}

func (f *fakeSys) add(format string, a ...any) { f.log = append(f.log, fmt.Sprintf(format, a...)) }

func (f *fakeSys) Token() (string, bool, error) {
	f.add("sid token")
	return f.user, f.elevated, nil
}

func (f *fakeSys) SessionUserSID() (string, error) {
	f.add("sid session")
	if f.sessErr != nil {
		return "", f.sessErr
	}
	return f.session, nil
}

func (f *fakeSys) ProductProcesses(dir string) ([]Proc, error) {
	f.add("wait procs %s", dir)
	if s := f.sup; s != nil {
		s.calls++
		if s.ended {
			return nil, nil
		}
		ps := []Proc{{PID: 20, Image: filepath.Join(dir, "helper", "splitlane-helper.exe")}}
		if s.calls >= s.restartAt {
			s.worker, f.helper = true, "ends"
			ps = append(ps, Proc{PID: 21, Image: filepath.Join(dir, "helper", "splitlane-helper.exe")})
		}
		return ps, nil
	}
	if f.stay != nil {
		return f.stay, nil
	}
	if len(f.procs) == 0 {
		return nil, nil
	}
	p := f.procs[0]
	f.procs = f.procs[1:]
	return p, nil
}

func (f *fakeSys) AccountName(sid string) (string, error) {
	f.add("sid account %s", sid)
	return account, nil
}

func (f *fakeSys) StatFile(path string) error {
	f.add("hash stat %s", filepath.Base(path))
	if _, ok := f.files[path]; !ok {
		return &fs.PathError{Op: "stat", Path: path, Err: fs.ErrNotExist}
	}
	return nil
}

func (f *fakeSys) ReadFile(path string) ([]byte, error) {
	f.add("hash read %s", filepath.Base(path))
	b, ok := f.files[path]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return b, nil
}

// ShutdownHelper: a helper that ends does what cmdShutdown does: it saves state.json with "was on" off
// (when its data folder is there), and it is gone; the supervisor ends with it.
func (f *fakeSys) ShutdownHelper(wait time.Duration) (bool, error) {
	f.add("stop shutdown %s", wait)
	switch f.helper {
	case "ends":
		if _, err := os.Stat(f.data); f.data != "" && err == nil {
			d := store.Dir{Path: f.data}
			st, _ := d.LoadState()
			st.WasOn = false
			if err := d.SaveState(st); err != nil {
				panic(err)
			}
			f.add("stop saved wasOn=false")
		}
		f.helper = "none"
		if f.sup != nil && f.sup.worker {
			f.sup.ended = true
		}
		return true, nil
	case "stays":
		return true, errors.New("the helper (process 4242) did not end within 2m0s; its last state: phase=stopping reason=restore_pending (exit node not confirmed)")
	}
	return false, nil
}

func (f *fakeSys) SystemStatus(context.Context) (engine.SysStatus, error) {
	f.add("stop status")
	return f.sys, nil
}

func (f *fakeSys) SetExitNode(_ context.Context, ip string, allowLAN bool) error {
	f.add("stop set %s %v", ip, allowLAN)
	if !f.setTakes {
		return errors.New("tailscale set: exit status 1: no such node")
	}
	f.sys.ExitNodeOn, f.sys.ExitNodeIP = true, ip
	return nil
}

func (f *fakeSys) RestoreCommand(ip string, allowLAN bool) string {
	return fmt.Sprintf(`"C:\Program Files\Tailscale\tailscale.exe" set --exit-node=%s --exit-node-allow-lan-access=%v`, ip, allowLAN)
}

func (f *fakeSys) FindCore(exe string, id store.CoreIdentity) (engine.CoreProc, bool) {
	f.add("stop find %s %d", exe, id.PID)
	if f.core == nil || f.core.id != id || !strings.EqualFold(exe, id.Path) {
		return nil, false
	}
	return f.core, true
}

// CheckDir finds the temporary folder as it is, with the real check (a link, a junction or a file is
// refused) and the user of the test as an administrator; one that is there belongs to dirOwner when it is set.
func (f *fakeSys) CheckDir(dir string) (bool, error) {
	f.add("stop check %s", dir)
	if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if f.dirOwner != "" {
		return true, fmt.Errorf("%s is there and belongs to %s: it was not made by an administrator; remove it first", dir, f.dirOwner)
	}
	return checkDir(dir, testOwners)
}

// testOwners own the temporary folders: the user of the test, or the administrators when it runs elevated.
var testOwners = func() []string {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		panic(err)
	}
	return append([]string{u.User.Sid.String()}, adminOwners...)
}()

func (f *fakeSys) WriteResult(path string, data []byte) error {
	f.add("result write %s", path)
	if f.writeErr != nil {
		return f.writeErr
	}
	return atomicfile.Write(path, data)
}

// After fires at once: the tests do not wait.
func (f *fakeSys) After(d time.Duration) <-chan time.Time {
	f.add("stop wait %s", d)
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

func (f *fakeSys) SecureDir(dir, sddl string) error {
	f.add("acl %s %s", dir, sddl)
	return nil
}

func (f *fakeSys) WriteSetup(dataDir string, s place.Setup) error {
	f.add("setup %s %s", dataDir, s.UserSID)
	return nil
}

func (f *fakeSys) RegisterTask(xml, dir string) error {
	f.add("task register %s", dir)
	f.xml = xml
	return f.taskErr
}

func (f *fakeSys) DeleteTask() error {
	f.add("task delete")
	return nil
}

func (f *fakeSys) RemoveAll(dir string) error {
	f.add("remove %s", dir)
	return nil
}

// fakeCore is the core of an earlier run: it ends on the graceful signal, or only when it is killed.
type fakeCore struct {
	f        *fakeSys
	id       store.CoreIdentity
	done     chan struct{}
	graceful bool
	ended    bool
}

func (c *fakeCore) Identity() store.CoreIdentity { return c.id }
func (c *fakeCore) Done() <-chan struct{}        { return c.done }

func (c *fakeCore) Interrupt() error {
	c.f.add("stop interrupt %d", c.id.PID)
	if c.graceful {
		c.end()
	}
	return nil
}

func (c *fakeCore) Kill() error {
	c.f.add("stop kill %d", c.id.PID)
	c.end()
	return nil
}

func (c *fakeCore) end() {
	if !c.ended {
		c.ended = true
		close(c.done)
	}
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// setup is an elevated setup user with the complete files in the program folder, no helper on the pipe, the system
// Tailscale installed without an exit node, and the options of install: the program and data folders
// in a temporary folder. The pins are those of the fake core and library.
func setup(t *testing.T) (*fakeSys, Options, *strings.Builder) {
	t.Helper()
	root := t.TempDir()
	out := &strings.Builder{}
	product := filepath.Join(root, "Program Files", "SplitLane")
	o := Options{
		Version:     "1.2.3",
		UserSID:     testSID,
		ProductDir:  product,
		ProgramDir:  filepath.Join(product, "helper"),
		Exe:         filepath.Join(product, "helper", "splitlane-helper.exe"),
		DataDir:     filepath.Join(root, "ProgramData", "SplitLane"),
		UserDataDir: filepath.Join(root, "Users", "tester", "AppData", "Roaming", "SplitLane"),
		Hostname:    "testpc",
		Out:         out,
	}
	f := &fakeSys{user: testSID, session: testSID, elevated: true, files: map[string][]byte{}, helper: "none",
		sys: engine.SysStatus{Installed: true}, data: o.DataDir}
	for _, name := range Files {
		f.files[filepath.Join(o.ProgramDir, name)] = []byte("content of " + name)
	}
	old := pinned
	pinned = map[string]string{"sing-box.exe": sum("content of sing-box.exe"), "libcronet.dll": sum("content of libcronet.dll")}
	t.Cleanup(func() { pinned = old })
	return f, o, out
}

// steps is the log as its steps, in order, a step repeated in a row once.
func steps(log []string) string {
	var s []string
	for _, e := range log {
		k, _, _ := strings.Cut(e, " ")
		if len(s) == 0 || s[len(s)-1] != k {
			s = append(s, k)
		}
	}
	return strings.Join(s, ",")
}

// inOrder: every entry of want is in the log, each after the one before.
func inOrder(log, want []string) error {
	i := 0
	for _, e := range log {
		if i < len(want) && e == want[i] {
			i++
		}
	}
	if i < len(want) {
		return fmt.Errorf("%q not found in order; log:\n%s", want[i], strings.Join(log, "\n"))
	}
	return nil
}

func count(log []string, prefix string) int {
	n := 0
	for _, e := range log {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

// afterLast is what the log holds after the last entry of the step (all of it without one).
func afterLast(log []string, step string) string {
	for i := len(log) - 1; i >= 0; i-- {
		if strings.HasPrefix(log[i], step+" ") {
			return strings.Join(log[i+1:], ",")
		}
	}
	return strings.Join(log, ",")
}

// Install goes in the order of spec 4.3: the user of the elevated process, the SHA-256 of the core and
// its library, the helper that runs stopped, the files copied, the access lists of the data folder and
// of export\, setup.json, the task. The bytes copied are the ones that were hashed.
func TestInstallOrder(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "ends"
	if err := Install(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// the last stop: the result for the window checks the data folder (the fake makes none: nothing recorded)
	if got, want := steps(f.log), "sid,hash,stop,acl,setup,task,stop"; got != want {
		t.Fatalf("steps %s, want %s; log:\n%s", got, want, strings.Join(f.log, "\n"))
	}
	for _, name := range Files {
		if !slices.Contains(f.log, "hash read "+name) && !slices.Contains(f.log, "hash stat "+name) {
			t.Errorf("%s was not read from %s", name, o.ProgramDir)
		}
	}
	for _, w := range []string{
		"sid account " + testSID,
		"stop shutdown 2m0s",
		"acl " + o.DataDir + " D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)",
		"acl " + filepath.Join(o.DataDir, "export") + " D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;" + testSID + ")",
		"setup " + o.DataDir + " " + testSID,
		"task register " + o.DataDir,
	} {
		if !slices.Contains(f.log, w) {
			t.Errorf("no %q in the log:\n%s", w, strings.Join(f.log, "\n"))
		}
	}
	if !strings.Contains(f.xml, "<Command>"+esc(filepath.Join(o.ProgramDir, "splitlane-helper.exe"))+"</Command>") ||
		!strings.Contains(f.xml, "<UserId>"+esc(account)+"</UserId>") || !strings.Contains(f.xml, "(A;;FRFX;;;"+testSID+")") {
		t.Errorf("task XML:\n%s", f.xml)
	}
	if lastLine(out.String()) != "INSTALL-OK" {
		t.Fatalf("output:\n%s", out)
	}
}

// UAC passed under another account (spec 4.3): the user of the elevated process is not the one the
// window names. Install refuses before it reads anything else (then only the result for the window checks
// the data folder). A process that is not elevated and a SID that is not plain S-1-... (it goes into access
// lists) are refused as well.
func TestInstallRefusesOtherUser(t *testing.T) {
	f, o, out := setup(t)
	f.user = otherSID
	if err := Install(o, f); err == nil ||
		!strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: UAC was passed under another account: this process runs as "+otherSID+", the setup user is "+testSID) {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := strings.Join(f.log, ","); got != "sid token,stop check "+o.DataDir {
		t.Fatalf("log %s", got)
	}

	f, o, out = setup(t)
	f.elevated = false
	if err := Install(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: install runs elevated") || len(f.log) != 2 {
		t.Fatalf("%v\n%s\n%v", err, out, f.log)
	}

	for _, bad := range []string{"", "WD", "s-1-5-21-1000-2000-3000-1001", testSID + ")(A;;GA;;;WD", " " + testSID} {
		f, o, out = setup(t)
		o.UserSID, f.user = bad, bad
		if err := Install(o, f); err == nil || strings.Join(f.log, ",") != "stop check "+o.DataDir || !strings.Contains(out.String(), "is not a SID in the form S-1-...") {
			t.Errorf("%q: %v\n%s\n%v", bad, err, out, f.log)
		}
	}
}

// Only the unchanged core is installed (spec 4.3, 11): sing-box.exe and libcronet.dll in the folder of
// the running exe must have the pinned SHA-256, and every file of the layout must be there; otherwise
// install refuses before it stops a helper or changes anything.
func TestInstallRefusesWrongCoreHash(t *testing.T) {
	if len(pinned) != 2 || pinned["sing-box.exe"] != singbox.CoreSHA256 || pinned["libcronet.dll"] != CronetSHA256 {
		t.Fatalf("pins %v", pinned)
	}
	for _, c := range []struct{ name, content, want string }{
		{"sing-box.exe", "another core", `\sing-box.exe has SHA-256 ` + sum("another core") + ", want " + sum("content of sing-box.exe")},
		{"libcronet.dll", "another library", `\libcronet.dll has SHA-256 ` + sum("another library")},
		{"THIRD-PARTY.md", "", "INSTALL-REFUSED: the files to install are not complete in "},
	} {
		f, o, out := setup(t)
		path := filepath.Join(filepath.Dir(o.Exe), c.name)
		if c.content == "" {
			delete(f.files, path)
		} else {
			f.files[path] = []byte(c.content)
		}
		if err := Install(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: ") || !strings.Contains(out.String(), c.want) {
			t.Errorf("%s: %v\n%s", c.name, err, out)
		}
		if got := steps(f.log); got != "sid,hash,stop" { // stop: the result's check of the data folder
			t.Errorf("%s: steps %s", c.name, got)
		}
		// no helper stopped: after the files only the result for the window checks the data folder
		if got := afterLast(f.log, "hash"); got != "stop check "+o.DataDir {
			t.Errorf("%s: after the files %q", c.name, got)
		}
	}
}

// Review Focus 4: install over an installation that runs, with routing on and the return of the system
// exit node not confirmed. The disabling of the helper (5.4) keeps it and its core while the exit node
// is not back, so it does not end within 120 s: install refuses with the reason and replaces neither the
// files nor the task; the helper keeps running and keeps trying, the journal is its own.
func TestReinstallWaitsForRestore(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "stays"
	if err := Install(o, f); err == nil ||
		!strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: the helper that runs did not end: ") ||
		!strings.Contains(out.String(), "reason=restore_pending") || !strings.Contains(out.String(), "nothing was changed") {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := steps(f.log); got != "sid,hash,stop" {
		t.Fatalf("steps %s; log:\n%s", got, strings.Join(f.log, "\n"))
	}
	// after the shutdown only the result for the window checks the data folder (not there: nothing recorded)
	if got := strings.Join(f.log[len(f.log)-2:], ","); got != "stop shutdown 2m0s,stop check "+o.DataDir {
		t.Fatalf("last %q", got)
	}
}

// No helper on the pipe, but a journal and a core left by one that ended abnormally (spec 4.3, the
// paragraph on a repeated install; 5.7): install gives the exit node back first, confirmed by the
// status (5.4 step 1), deletes the journal, then stops the core that state.json names, found only with
// the image <ProgramDir>\sing-box.exe, and copies only after that. An exit node not confirmed within
// 30 s refuses the install with the manual command; the journal and the core stay. Without the
// system client there is nothing to give back (5.6): the journal goes.
func TestInstallWithoutPipeRestoresByJournal(t *testing.T) {
	prepare := func() (*fakeSys, Options, *strings.Builder, store.Dir) {
		f, o, out := setup(t)
		d := store.Dir{Path: o.DataDir}
		id := store.CoreIdentity{PID: 4242, Created: 134000000000000000, Path: filepath.Join(o.ProgramDir, "sing-box.exe")}
		st := store.Defaults()
		st.Core = &id
		if err := d.SaveState(st); err != nil {
			t.Fatal(err)
		}
		if err := d.SaveJournal(store.Journal{ExitNode: exitNode, AllowLAN: true}); err != nil {
			t.Fatal(err)
		}
		f.core = &fakeCore{f: f, id: id, done: make(chan struct{}), graceful: true}
		return f, o, out, d
	}

	f, o, out, d := prepare()
	f.setTakes = true
	if err := Install(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	core := filepath.Join(o.ProgramDir, "sing-box.exe")
	if err := inOrder(f.log, []string{"stop shutdown 2m0s", "stop status", "stop set " + exitNode + " true", "stop status",
		"stop find " + core + " 4242", "stop interrupt 4242", "acl " + o.DataDir + " " + DataSDDL}); err != nil {
		t.Fatal(err)
	}
	if j, err := d.LoadJournal(); j != nil || err != nil {
		t.Fatalf("the journal after the confirmed return: %v %v", j, err)
	}
	if count(f.log, "stop kill") != 0 || lastLine(out.String()) != "INSTALL-OK" {
		t.Fatalf("log:\n%s\noutput:\n%s", strings.Join(f.log, "\n"), out)
	}

	f, o, out, d = prepare()
	err := Install(o, f)
	if err == nil || !strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: the system exit node was not given back within 30s (tailscale set: exit status 1: no such node)") ||
		!strings.Contains(out.String(), `give it back by hand: "C:\Program Files\Tailscale\tailscale.exe" set --exit-node=100.64.0.1 --exit-node-allow-lan-access=true`) {
		t.Fatalf("%v\n%s", err, out)
	}
	if count(f.log, "stop set ") != 7 || count(f.log, "stop wait 5s") != 6 || count(f.log, "stop find") != 0 || steps(f.log) != "sid,hash,stop" {
		t.Fatalf("log:\n%s", strings.Join(f.log, "\n"))
	}
	if j, _ := d.LoadJournal(); j == nil {
		t.Fatal("the journal went without a confirmed return")
	}

	f, o, out, d = prepare()
	f.sys = engine.SysStatus{}
	if err := Install(o, f); err != nil || !strings.Contains(out.String(), "the system Tailscale is not installed") || count(f.log, "stop set ") != 0 {
		t.Fatalf("%v\n%s", err, out)
	}
	if j, _ := d.LoadJournal(); j != nil {
		t.Fatal("the journal stays without a system client")
	}
}

// ProgramData lets users make folders: a user can make the data folder before the first install, with
// a restore.json and a state.json of his own in it (review F14-1). install and uninstall read them with
// administrator rights, and act on them (tailscale set with the journal's exit node, the journal set
// aside or deleted, the process of state.json stopped), only from a plain folder of the administrators
// or SYSTEM, checked before the stop. install refuses such a folder before it stops anything (step 5
// would refuse it anyway). uninstall stops the helper and removes the task and the program folder as
// usual, without the journal, the core or the device name of that folder.
func TestDataFolderOfAUserIsNotRead(t *testing.T) {
	prepare := func() (*fakeSys, Options, *strings.Builder, string) {
		f, o, out := setup(t)
		d := store.Dir{Path: o.DataDir}
		id := store.CoreIdentity{PID: 4242, Created: 134000000000000000, Path: filepath.Join(o.ProgramDir, "sing-box.exe")}
		st := store.Defaults()
		st.Core, st.DeviceName = &id, "planted-splitlane"
		if err := d.SaveState(st); err != nil {
			t.Fatal(err)
		}
		if err := d.SaveJournal(store.Journal{ExitNode: exitNode, AllowLAN: true}); err != nil {
			t.Fatal(err)
		}
		f.core = &fakeCore{f: f, id: id, done: make(chan struct{}), graceful: true}
		f.setTakes, f.dirOwner = true, otherSID
		return f, o, out, filepath.Join(o.DataDir, "restore.json")
	}
	notRead := func(f *fakeSys, journal string) {
		t.Helper()
		for _, call := range []string{"stop status", "stop set ", "stop find ", "stop interrupt "} {
			if n := count(f.log, call); n != 0 {
				t.Errorf("%q %d times: the files of a user's folder were acted on; log:\n%s", call, n, strings.Join(f.log, "\n"))
			}
		}
		if _, err := os.Stat(journal); err != nil {
			t.Errorf("the planted journal: %v", err)
		}
	}

	f, o, out, journal := prepare()
	if err := Install(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()),
		"INSTALL-REFUSED: the data folder: "+o.DataDir+" is there and belongs to "+otherSID+": it was not made by an administrator") {
		t.Errorf("%v\n%s", err, out)
	}
	notRead(f, journal)
	if got := steps(f.log); got != "sid,hash,stop" || count(f.log, "stop shutdown") != 0 {
		t.Errorf("steps %s; log:\n%s", got, strings.Join(f.log, "\n"))
	}

	f, o, out, journal = prepare()
	o.Purge = true
	if err := Uninstall(o, f); err != nil || !strings.Contains(out.String(), "restore.json and state.json in it are not read") ||
		!strings.Contains(out.String(), "UNINSTALL-NOTE: the device testpc-splitlane stays") || lastLine(out.String()) != "UNINSTALL-OK" {
		t.Errorf("%v\n%s", err, out)
	}
	notRead(f, journal)
	if err := inOrder(f.log, []string{"stop check " + o.DataDir, "stop shutdown 2m0s", "wait procs " + o.ProductDir, "task delete", "remove " + o.DataDir}); err != nil {
		t.Error(err)
	}

	// The folder missing: nothing to read, install goes on and makes it.
	f, o, out = setup(t)
	if err := Install(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := inOrder(f.log, []string{"stop check " + o.DataDir, "stop shutdown 2m0s", "acl " + o.DataDir + " " + DataSDDL}); err != nil {
		t.Error(err)
	}
}

// uninstall (spec 4.3 point 4) stops first, as install does: a helper on the pipe gets helper.shutdown
// and must end; one that does not refuses the uninstall, and nothing is removed. Without a helper on the
// pipe the core left by one that ended abnormally is stopped, by force when the signal is not enough.
// Then it waits for the processes of the product to end, and only then the task goes. A process that is
// not elevated is refused before anything.
func TestUninstallStopsFirst(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "ends"
	if err := Uninstall(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := strings.Join(f.log, ","); got != "sid token,stop check "+o.DataDir+",stop shutdown 2m0s,wait procs "+o.ProductDir+",task delete" {
		t.Fatalf("log %s", got)
	}
	if lastLine(out.String()) != "UNINSTALL-OK" {
		t.Fatalf("output:\n%s", out)
	}

	f, o, out = setup(t)
	f.helper = "stays"
	if err := Uninstall(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "UNINSTALL-REFUSED: the helper that runs did not end: ") {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := strings.Join(f.log, ","); got != "sid token,stop check "+o.DataDir+",stop shutdown 2m0s" {
		t.Fatalf("log %s", got)
	}

	f, o, out = setup(t)
	id := store.CoreIdentity{PID: 4343, Created: 134000000000000000, Path: filepath.Join(o.ProgramDir, "sing-box.exe")}
	st := store.Defaults()
	st.Core = &id
	if err := (store.Dir{Path: o.DataDir}).SaveState(st); err != nil {
		t.Fatal(err)
	}
	f.core = &fakeCore{f: f, id: id, done: make(chan struct{})}
	if err := Uninstall(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := inOrder(f.log, []string{"stop shutdown 2m0s", "stop find " + id.Path + " 4343", "stop interrupt 4343", "stop wait 5s",
		"stop kill 4343", "wait procs " + o.ProductDir, "task delete"}); err != nil {
		t.Fatal(err)
	}

	f, o, out = setup(t)
	f.elevated = false
	if err := Uninstall(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "UNINSTALL-REFUSED: uninstall runs elevated") || len(f.log) != 1 {
		t.Fatalf("%v\n%s\n%v", err, out, f.log)
	}
}

// uninstall does not remove the program folder (the uninstaller does), from wherever it runs, even from
// the program folder itself; the data folder stays without --purge and there is no note.
func TestUninstallKeepsTheProgramFiles(t *testing.T) {
	f, o, out := setup(t)
	if err := Uninstall(o, f); err != nil || lastLine(out.String()) != "UNINSTALL-OK" {
		t.Fatalf("%v\n%s", err, out)
	}
	if count(f.log, "remove") != 0 || count(f.log, "copy") != 0 || strings.Contains(out.String(), "UNINSTALL-NOTE") {
		t.Fatalf("log:\n%s", strings.Join(f.log, "\n"))
	}
	if count(f.log, "task delete") != 1 {
		t.Fatalf("the task was not removed:\n%s", strings.Join(f.log, "\n"))
	}
}

// Review Focus 1 for uninstall: the helper's process and the core end a moment after the pipe's owner, and
// their files stay busy until then. The task is removed only when no process of the product is left.
func TestUninstallWaitsForProcesses(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "ends"
	f.procs = [][]Proc{{{PID: 10, Image: filepath.Join(o.ProgramDir, "splitlane-helper.exe")}}, {{PID: 11, Image: filepath.Join(o.ProgramDir, "sing-box.exe")}}}
	if err := Uninstall(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := count(f.log, "wait procs"); got != 3 {
		t.Fatalf("%d looks at the processes:\n%s", got, strings.Join(f.log, "\n"))
	}
	if err := inOrder(f.log, []string{"wait procs " + o.ProductDir, "stop wait 500ms", "wait procs " + o.ProductDir, "stop wait 500ms",
		"wait procs " + o.ProductDir, "task delete"}); err != nil {
		t.Fatal(err)
	}

	f, o, out = setup(t)
	f.stay = []Proc{{PID: 12, Image: filepath.Join(o.ProgramDir, "sing-box.exe")}}
	if err := Uninstall(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "UNINSTALL-REFUSED: sing-box.exe still running (process 12)") {
		t.Fatalf("%v\n%s", err, out)
	}
	if count(f.log, "task delete") != 0 {
		t.Fatalf("the task was removed while a process stayed:\n%s", strings.Join(f.log, "\n"))
	}
}

// uninstall --purge removes the data folder, then the folder of the user who ran it, and says that the
// embedded node stays a device of the Tailscale network (spec 4.3 point 4): the name state.json keeps,
// else <PC>-splitlane. The line of the user's folder has no path of the user.
func TestPurgeRemovesTheUserFolder(t *testing.T) {
	note := "UNINSTALL-NOTE: the device %s stays in your Tailscale network; remove it in the Tailscale admin console\nUNINSTALL-OK\n"

	f, o, out := setup(t)
	o.Purge = true
	st := store.Defaults()
	st.DeviceName = "desk-7-splitlane"
	if err := (store.Dir{Path: o.DataDir}).SaveState(st); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	want := "sid token,sid session,stop check " + o.DataDir + ",stop shutdown 2m0s,wait procs " + o.ProductDir + ",task delete,remove " + o.DataDir + ",remove " + o.UserDataDir
	if got := strings.Join(f.log, ","); got != want {
		t.Fatalf("log %s", got)
	}
	if !strings.HasSuffix(out.String(), fmt.Sprintf(note, "desk-7-splitlane")) || strings.Contains(out.String(), o.UserDataDir) || strings.Contains(out.String(), "tester") {
		t.Fatalf("output:\n%s", out)
	}

	f, o, out = setup(t)
	o.Purge = true
	if err := Uninstall(o, f); err != nil || !strings.HasSuffix(out.String(), fmt.Sprintf(note, "testpc-splitlane")) {
		t.Fatalf("%v\n%s", err, out)
	}

	f, o, out = setup(t)
	if err := Uninstall(o, f); err != nil || slices.Contains(f.log, "remove "+o.DataDir) || slices.Contains(f.log, "remove "+o.UserDataDir) {
		t.Fatalf("%v\n%s\n%v", err, out, f.log)
	}
}

// install runs only from the program folder, where the installer put the files (spec 3): from another
// folder (the installer's temporary one, the window's own copy) it refuses before it reads a file.
func TestInstallRunsOnlyFromTheProgramFolder(t *testing.T) {
	for _, exe := range []func(o Options) string{
		func(o Options) string { return filepath.Join(o.ProductDir, "dist", "splitlane-helper.exe") },
		func(o Options) string { return filepath.Join(o.ProductDir, "splitlane-helper.exe") },
		func(o Options) string { return filepath.Join(o.ProgramDir, "old", "splitlane-helper.exe") },
		func(o Options) string { return filepath.Join(o.ProgramDir+"2", "splitlane-helper.exe") },
	} {
		f, o, out := setup(t)
		o.Exe = exe(o)
		// no file read and no helper stopped: after the user only the result for the window checks the data folder
		if err := Install(o, f); err == nil || steps(f.log) != "sid,stop" || afterLast(f.log, "sid") != "stop check "+o.DataDir ||
			lastLine(out.String()) != "INSTALL-REFUSED: install runs from "+o.ProgramDir+", where the installer puts the files" {
			t.Errorf("%s: %v\n%s\n%v", o.Exe, err, out, f.log)
		}
	}
	// The case of the name does not matter on Windows.
	f, o, out := setup(t)
	o.Exe = strings.ToUpper(o.Exe)
	if err := Install(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// install no longer copies (Sys writes no file of the program, only setup.json and the result for the
// window): the task runs <ProgramDir>\splitlane-helper.exe.
func TestInstallDoesNotCopy(t *testing.T) {
	f, o, out := setup(t)
	if err := Install(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(f.xml, "<Command>"+esc(filepath.Join(o.ProgramDir, "splitlane-helper.exe"))+"</Command>") {
		t.Fatalf("task XML:\n%s", f.xml)
	}
	if strings.Contains(out.String(), "copied") {
		t.Fatalf("output:\n%s", out)
	}
}

// install still checks the core and the library of the program folder by SHA-256 before it changes
// anything.
func TestInstallStillChecksTheCore(t *testing.T) {
	f, o, out := setup(t)
	f.files[filepath.Join(o.ProgramDir, "sing-box.exe")] = []byte("another core")
	if err := Install(o, f); err == nil || !strings.Contains(out.String(), "INSTALL-REFUSED: ") || !strings.Contains(out.String(), "has SHA-256 "+sum("another core")) {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := steps(f.log); got != "sid,hash,stop" {
		t.Fatalf("steps %s", got)
	}
	// no helper stopped: after the files only the result for the window checks the data folder
	if got := afterLast(f.log, "hash"); got != "stop check "+o.DataDir {
		t.Fatalf("after the files %q", got)
	}
}

// --session-user: the user of the session of this process is the setup user (spec 3); a session without
// one (session 0 of services, the runner of GitHub Actions) takes the user of the process.
func TestSessionUserTakesTheSessionUser(t *testing.T) {
	f, o, out := setup(t)
	o.UserSID, o.SessionUser = "", true
	f.session = testSID
	if err := Install(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !slices.Contains(f.log, "sid session") || !slices.Contains(f.log, "sid account "+testSID) ||
		!slices.Contains(f.log, "setup "+o.DataDir+" "+testSID) {
		t.Fatalf("log:\n%s", strings.Join(f.log, "\n"))
	}
}

func TestSessionWithoutUserTakesTheProcessUser(t *testing.T) {
	f, o, out := setup(t)
	o.UserSID, o.SessionUser = "", true
	f.session = ""
	if err := Prepare(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !slices.Contains(f.log, "sid session") || !slices.Contains(f.log, "sid account "+testSID) {
		t.Fatalf("log:\n%s", strings.Join(f.log, "\n"))
	}
}

// mutated counts the calls of the log that change the system.
func mutated(log []string) int {
	n := 0
	for _, p := range []string{"copy", "acl", "setup", "task", "remove", "stop set", "stop interrupt", "stop kill"} {
		n += count(log, p)
	}
	return n
}

// Review Focus 3: UAC passed by another administrator, the session user is not that administrator.
// prepare and install refuse before any change.
func TestSessionUserMismatchRefusesBeforeChanges(t *testing.T) {
	for _, cmd := range []string{"prepare", "install"} {
		f, o, out := setup(t)
		o.UserSID, o.SessionUser = "", true
		f.session = otherSID // the process runs as testSID
		f.helper = "ends"
		run := Prepare
		if cmd == "install" {
			run = Install
		}
		if err := run(o, f); err == nil ||
			!strings.HasPrefix(lastLine(out.String()), strings.ToUpper(cmd)+"-REFUSED: UAC was passed under another account: this process runs as "+testSID+", the setup user is "+otherSID) {
			t.Fatalf("%s: %v\n%s", cmd, err, out)
		}
		if mutated(f.log) != 0 || count(f.log, "stop shutdown") != 0 || count(f.log, "hash") != 0 {
			t.Fatalf("%s: log:\n%s", cmd, strings.Join(f.log, "\n"))
		}
	}
}

func notChanged(t *testing.T, f *fakeSys) {
	t.Helper()
	if n := mutated(f.log); n != 0 {
		t.Fatalf("prepare changed the system:\n%s", strings.Join(f.log, "\n"))
	}
}

// Review Focus 1: the helper of stage 1 runs with routing on. prepare stops it by helper.shutdown and then
// waits until no process of the product is left (the supervisor ends a moment after the worker), every
// 500 ms. It changes nothing and runs from any folder.
func TestPrepareStopsTheRunningHelperAndWaits(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "ends"
	o.Exe = filepath.Join(t.TempDir(), "splitlane-helper.exe")
	f.procs = [][]Proc{{{PID: 10, Image: filepath.Join(o.ProgramDir, "splitlane-helper.exe")}}, {{PID: 11, Image: filepath.Join(o.ProgramDir, "sing-box.exe")}}}
	if err := Prepare(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := inOrder(f.log, []string{"sid token", "sid account " + testSID, "stop check " + o.DataDir, "stop shutdown 2m0s",
		"wait procs " + o.ProductDir, "stop wait 500ms", "wait procs " + o.ProductDir, "stop wait 500ms", "wait procs " + o.ProductDir}); err != nil {
		t.Fatal(err)
	}
	notChanged(t, f)
	if lastLine(out.String()) != "PREPARE-OK" {
		t.Fatalf("output:\n%s", out)
	}
}

// Processes that stay for 10 s refuse the preparation with their image names and ids.
func TestPrepareRefusesWhileProcessesStay(t *testing.T) {
	f, o, out := setup(t)
	f.stay = []Proc{{PID: 10, Image: filepath.Join(o.ProgramDir, "splitlane-helper.exe")}, {PID: 11, Image: filepath.Join(o.ProductDir, "sing-box.exe")}}
	if err := Prepare(o, f); err == nil ||
		lastLine(out.String()) != "PREPARE-REFUSED: splitlane-helper.exe, sing-box.exe still running (process 10, 11); the files cannot be replaced" {
		t.Fatalf("%v\n%s", err, out)
	}
	if n := count(f.log, "stop wait 500ms"); n != 20 {
		t.Fatalf("%d waits of 500 ms, want 20 (10 s)", n)
	}
	notChanged(t, f)

	// A process that could not be opened has no image: it counts as still running.
	f, o, out = setup(t)
	f.stay = []Proc{{PID: 13}}
	if err := Prepare(o, f); err == nil || !strings.Contains(lastLine(out.String()), "still running (process 13)") {
		t.Fatalf("%v\n%s", err, out)
	}
}

// Review Focus 1: a core left by the helper of stage 1 lies at the root of the product folder; it is
// looked for by that image when the one in the program folder is not found.
func TestPrepareFindsTheLegacyCore(t *testing.T) {
	f, o, out := setup(t)
	legacy := filepath.Join(o.ProductDir, "sing-box.exe")
	id := store.CoreIdentity{PID: 4444, Created: 134000000000000000, Path: legacy}
	st := store.Defaults()
	st.Core = &id
	if err := (store.Dir{Path: o.DataDir}).SaveState(st); err != nil {
		t.Fatal(err)
	}
	f.core = &fakeCore{f: f, id: id, done: make(chan struct{}), graceful: true}
	if err := Prepare(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := inOrder(f.log, []string{"stop find " + filepath.Join(o.ProgramDir, "sing-box.exe") + " 4444", "stop find " + legacy + " 4444",
		"stop interrupt 4444", "wait procs " + o.ProductDir}); err != nil {
		t.Fatal(err)
	}
	if lastLine(out.String()) != "PREPARE-OK" {
		t.Fatalf("output:\n%s", out)
	}
}

// The return of the exit node not confirmed: prepare refuses with the manual command, before it waits,
// and changes nothing (the journal stays).
func TestPrepareRefusesAnUnconfirmedRestore(t *testing.T) {
	f, o, out := setup(t)
	d := store.Dir{Path: o.DataDir}
	if err := d.SaveJournal(store.Journal{ExitNode: exitNode, AllowLAN: true}); err != nil {
		t.Fatal(err)
	}
	err := Prepare(o, f)
	if err == nil || !strings.HasPrefix(lastLine(out.String()), "PREPARE-REFUSED: the system exit node was not given back within 30s") ||
		!strings.Contains(out.String(), "give it back by hand: ") {
		t.Fatalf("%v\n%s", err, out)
	}
	if count(f.log, "wait procs") != 0 {
		t.Fatalf("log:\n%s", strings.Join(f.log, "\n"))
	}
	if j, _ := d.LoadJournal(); j == nil {
		t.Fatal("the journal went without a confirmed return")
	}
}

// wasOnState saves a state.json with "was on" and a device name in the data folder.
func wasOnState(t *testing.T, o Options, on bool) store.Dir {
	t.Helper()
	d := store.Dir{Path: o.DataDir}
	st := store.Defaults()
	st.WasOn, st.DeviceName = on, "desk-7-splitlane"
	if err := d.SaveState(st); err != nil {
		t.Fatal(err)
	}
	return d
}

func loadState(t *testing.T, d store.Dir) store.State {
	t.Helper()
	st, err := d.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// An update keeps splitting as it was (final review, item 1): helper.shutdown writes "was on" off, so
// prepare reads state.json before it and writes "was on" back after the helper and its processes are
// gone; the new worker then switches splitting on at its start (5.7). The rest of state.json stays.
func TestPrepareKeepsSplittingOn(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "ends"
	d := wasOnState(t, o, true)
	f.procs = [][]Proc{{{PID: 10, Image: filepath.Join(o.ProgramDir, "splitlane-helper.exe")}}}
	if err := Prepare(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := inOrder(f.log, []string{"stop check " + o.DataDir, "stop shutdown 2m0s", "stop saved wasOn=false",
		"wait procs " + o.ProductDir, "wait procs " + o.ProductDir}); err != nil {
		t.Fatal(err)
	}
	if st := loadState(t, d); !st.WasOn || st.DeviceName != "desk-7-splitlane" {
		t.Fatalf("state.json after prepare: wasOn %v, device %q", st.WasOn, st.DeviceName)
	}
	if !strings.Contains(out.String(), "PREPARE: splitting was on: state.json says so again; the helper of this version switches it on after its start\n") ||
		lastLine(out.String()) != "PREPARE-OK" {
		t.Fatalf("output:\n%s", out)
	}

	// install and uninstall leave "was on" as helper.shutdown wrote it.
	for _, run := range []func(Options, Sys) error{Install, Uninstall} {
		f, o, out := setup(t)
		f.helper = "ends"
		d := wasOnState(t, o, true)
		if err := run(o, f); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if loadState(t, d).WasOn {
			t.Fatalf("only prepare writes \"was on\" back:\n%s", out)
		}
	}
}

// The helper that ran with splitting on was stopped (helper.shutdown wrote "was on" off), then prepare refuses
// because processes stay: "was on" is written back before the refusal, so the window that the installer starts
// again and its helper return to splitting as it was. Splitting that was off stays off.
func TestPrepareRefusalAfterTheStopKeepsSplittingOn(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "ends"
	d := wasOnState(t, o, true)
	f.stay = []Proc{{PID: 10, Image: filepath.Join(o.ProductDir, "sing-box.exe")}}
	if err := Prepare(o, f); err == nil ||
		lastLine(out.String()) != "PREPARE-REFUSED: sing-box.exe still running (process 10); the files cannot be replaced" {
		t.Fatalf("%v\n%s", err, out)
	}
	if !slices.Contains(f.log, "stop saved wasOn=false") {
		t.Fatalf("the helper did not end; log:\n%s", strings.Join(f.log, "\n"))
	}
	if st := loadState(t, d); !st.WasOn || st.DeviceName != "desk-7-splitlane" {
		t.Fatalf("state.json after the refusal: wasOn %v, device %q", st.WasOn, st.DeviceName)
	}
	if !strings.Contains(out.String(), "PREPARE: splitting was on: state.json says so again; the helper of this version switches it on after its start\n") {
		t.Fatalf("output:\n%s", out)
	}

	f, o, out = setup(t)
	f.helper = "ends"
	d = wasOnState(t, o, false)
	f.stay = []Proc{{PID: 10}}
	if err := Prepare(o, f); err == nil || loadState(t, d).WasOn || strings.Contains(out.String(), "splitting was on") {
		t.Fatalf("splitting was off: %v\n%s", err, out)
	}
}

// Splitting was off: it stays off, and prepare writes nothing.
func TestPrepareLeavesSplittingOff(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "ends"
	d := wasOnState(t, o, false)
	if err := Prepare(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if loadState(t, d).WasOn || strings.Contains(out.String(), "splitting was on") {
		t.Fatalf("splitting was off and is on after prepare:\n%s", out)
	}
}

// state.json is read only from a data folder that passed CheckDir, and written back only into it: a folder
// that is not there is not made, one of a user refuses prepare before anything (its file stays as it is).
func TestPrepareDoesNotWriteStateWithoutCheckedFolder(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "ends"
	if err := Prepare(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := os.Lstat(o.DataDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("prepare made the data folder: %v", err)
	}

	f, o, out = setup(t)
	f.helper, f.dirOwner = "ends", otherSID
	wasOnState(t, o, true)
	file := filepath.Join(o.DataDir, "state.json")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := Prepare(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "PREPARE-REFUSED: the data folder: ") {
		t.Fatalf("%v\n%s", err, out)
	}
	if after, err := os.ReadFile(file); err != nil || string(after) != string(before) || count(f.log, "stop shutdown") != 0 {
		t.Fatalf("state.json of a user's folder: %v\n%s\nlog:\n%s", err, after, strings.Join(f.log, "\n"))
	}
}

// state.json that cannot be read before the shutdown: "was on" is not known, nothing is written back, and
// a line says so.
func TestPrepareDoesNotWriteStateAfterAReadError(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "ends"
	d := store.Dir{Path: o.DataDir}
	if err := os.MkdirAll(o.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.DataDir, "state.json"), []byte(`{"wasOn":true,`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out.String(), "PREPARE: state.json: ") || !strings.Contains(out.String(), "; splitting is not switched on again after the update\n") ||
		strings.Contains(out.String(), "splitting was on") {
		t.Fatalf("output:\n%s", out)
	}
	if loadState(t, d).WasOn {
		t.Fatal("\"was on\" was written back without being read")
	}
}

// The supervisor of stage 2 may start a new worker in its 5 s pause while prepare or uninstall waits for
// the processes to end (final review, item 4): that worker takes the pipe and gets helper.shutdown too,
// within the same 10 s, instead of making the command refuse.
func TestWaitGoneStopsAWorkerStartedDuringTheWait(t *testing.T) {
	for _, run := range []func(Options, Sys) error{Prepare, Uninstall} {
		f, o, out := setup(t)
		f.helper = "ends"
		f.sup = &supervisor{restartAt: 3}
		if err := run(o, f); err != nil {
			t.Fatalf("%v\n%s\nlog:\n%s", err, out, strings.Join(f.log, "\n"))
		}
		if err := inOrder(f.log, []string{"stop shutdown 2m0s", "wait procs " + o.ProductDir, "stop shutdown 10s", "stop wait 500ms",
			"wait procs " + o.ProductDir, "stop shutdown 9.5s", "stop wait 500ms", "wait procs " + o.ProductDir, "stop shutdown 9s",
			"stop wait 500ms", "wait procs " + o.ProductDir}); err != nil {
			t.Fatal(err)
		}
		if count(f.log, "stop shutdown") != 4 || count(f.log, "wait procs") != 4 ||
			!strings.Contains(out.String(), ": a helper that took the pipe during the wait has ended\n") {
			t.Fatalf("output:\n%s\nlog:\n%s", out, strings.Join(f.log, "\n"))
		}
	}

	// That worker does not end: the command refuses with its reason, as for the first helper.
	f, o, out := setup(t)
	f.stay = []Proc{{PID: 10, Image: filepath.Join(o.ProgramDir, "splitlane-helper.exe")}}
	f.helper = "stays"
	err := waitGone(o, f, progress(o, "PREPARE"))
	if err == nil || !strings.HasPrefix(err.Error(), "the helper that took the pipe during the wait did not end: ") ||
		!strings.Contains(err.Error(), "reason=restore_pending") || count(f.log, "stop shutdown") != 1 {
		t.Fatalf("%v\n%s\nlog:\n%s", err, out, strings.Join(f.log, "\n"))
	}
}

// uninstall --purge under a UAC passed by another administrator (final review, item 10): %AppData% of this
// process is that administrator's, not the signed-in user's, so it stays; the data folder goes. A session
// without a user takes the user of the process (the folder goes); a session that cannot be found out keeps
// the folder too.
func TestPurgeUnderAnotherAdminKeepsTheUserFolder(t *testing.T) {
	f, o, out := setup(t)
	o.Purge = true
	f.session = otherSID // the process runs as testSID
	if err := Uninstall(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !slices.Contains(f.log, "remove "+o.DataDir) || slices.Contains(f.log, "remove "+o.UserDataDir) {
		t.Fatalf("log:\n%s", strings.Join(f.log, "\n"))
	}
	if !strings.Contains(out.String(), "UNINSTALL: the window's settings of the signed-in user stay: UAC was passed under another account\n") ||
		!strings.Contains(out.String(), "UNINSTALL-NOTE: ") || lastLine(out.String()) != "UNINSTALL-OK" {
		t.Fatalf("output:\n%s", out)
	}
	if err := inOrder(f.log, []string{"sid token", "sid session", "stop check " + o.DataDir}); err != nil {
		t.Fatal(err)
	}

	f, o, out = setup(t)
	o.Purge = true
	f.session = ""
	if err := Uninstall(o, f); err != nil || !slices.Contains(f.log, "remove "+o.UserDataDir) {
		t.Fatalf("a session without a user: %v\n%s\n%v", err, out, f.log)
	}

	f, o, out = setup(t)
	o.Purge = true
	f.sessErr = errors.New("WTSQuerySessionInformationW: access denied")
	if err := Uninstall(o, f); err != nil || slices.Contains(f.log, "remove "+o.UserDataDir) || !slices.Contains(f.log, "remove "+o.DataDir) ||
		!strings.Contains(out.String(), "UNINSTALL: the user of the session: WTSQuerySessionInformationW: access denied; the window's settings stay\n") {
		t.Fatalf("%v\n%s\n%v", err, out, f.log)
	}

	// Without --purge the session is not looked at.
	f, o, out = setup(t)
	if err := Uninstall(o, f); err != nil || slices.Contains(f.log, "sid session") {
		t.Fatalf("%v\n%s\n%v", err, out, f.log)
	}
}

// mkExport makes the data folder and export\ as an earlier install left them (the fake's SecureDir makes no
// folder) and returns the path of the result file.
func mkExport(t *testing.T, o Options) string {
	t.Helper()
	dir := filepath.Join(o.DataDir, "export")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, ResultFile)
}

// readResult is the result file, and whether it is there.
func readResult(t *testing.T, file string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

// refusedResult is the result file of a refusal: the reason is that of the -REFUSED line.
func refusedResult(out, label string) string {
	return "1.2.3\r\nrefused\r\n" + strings.TrimPrefix(lastLine(out), label+"-REFUSED: ") + "\r\n"
}

// Spec 6 of stage 2 part 2: install records ok for the window once the task is registered: the version of
// this helper, ok and an empty reason, CRLF, UTF-8 without BOM; an older result is replaced whole, and no
// temporary file is left.
func TestInstallRecordsOk(t *testing.T) {
	f, o, out := setup(t)
	file := mkExport(t, o)
	if err := os.WriteFile(file, []byte(strings.Repeat("0.9.9\r\nrefused\r\nan older result\r\n", 10)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(o, f); err != nil || lastLine(out.String()) != "INSTALL-OK" {
		t.Fatalf("%v\n%s", err, out)
	}
	if got, ok := readResult(t, file); !ok || got != "1.2.3\r\nok\r\n\r\n" {
		t.Fatalf("result %q %v", got, ok)
	}
	if err := inOrder(f.log, []string{"task register " + o.DataDir, "stop check " + o.DataDir, "stop check " + filepath.Dir(file), "result write " + file}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil || len(entries) != 1 {
		t.Fatalf("export holds %v (%v), want only %s", entries, err, ResultFile)
	}
}

// install refuses: the result says refused with the reason of the INSTALL-REFUSED line, on one line; a task
// that could not be registered is a refusal, never ok.
func TestInstallRecordsARefusal(t *testing.T) {
	f, o, out := setup(t)
	file := mkExport(t, o)
	f.helper = "stays"
	if err := Install(o, f); err == nil {
		t.Fatalf("install did not refuse:\n%s", out)
	}
	if got, _ := readResult(t, file); got != refusedResult(out.String(), "INSTALL") || !strings.Contains(got, "reason=restore_pending") {
		t.Fatalf("result %q\noutput:\n%s", got, out)
	}

	f, o, out = setup(t)
	file = mkExport(t, o)
	f.taskErr = errors.New("schtasks /create: exit status 1: ERROR: the first line\r\nthe second line")
	if err := Install(o, f); err == nil {
		t.Fatalf("install did not refuse:\n%s", out)
	}
	if got, _ := readResult(t, file); got != "1.2.3\r\nrefused\r\nthe task "+TaskName+": schtasks /create: exit status 1: ERROR: the first line the second line\r\n" {
		t.Fatalf("result %q", got)
	}
}

// prepare records a refusal with the reason of its PREPARE-REFUSED line, and nothing when it succeeds
// (install records the outcome of the setup).
func TestPrepareRecordsARefusal(t *testing.T) {
	f, o, out := setup(t)
	file := mkExport(t, o)
	f.stay = []Proc{{PID: 10, Image: filepath.Join(o.ProgramDir, "splitlane-helper.exe")}}
	if err := Prepare(o, f); err == nil {
		t.Fatalf("prepare did not refuse:\n%s", out)
	}
	want := "1.2.3\r\nrefused\r\nsplitlane-helper.exe still running (process 10); the files cannot be replaced\r\n"
	if got, _ := readResult(t, file); got != want || got != refusedResult(out.String(), "PREPARE") {
		t.Fatalf("result %q, want %q", got, want)
	}

	f, o, out = setup(t)
	file = mkExport(t, o)
	f.helper = "ends"
	if err := Prepare(o, f); err != nil || lastLine(out.String()) != "PREPARE-OK" {
		t.Fatalf("%v\n%s", err, out)
	}
	if got, ok := readResult(t, file); ok || count(f.log, "result") != 0 {
		t.Fatalf("prepare recorded a success: %q", got)
	}
}

// A data folder that does not pass CheckDir (a user made it before the first install, with links of his own
// in it): neither prepare nor install writes the result there, and a line says why.
func TestNoRecordInAnUntrustedDataFolder(t *testing.T) {
	for _, run := range []func(Options, Sys) error{Prepare, Install} {
		f, o, out := setup(t)
		file := mkExport(t, o)
		f.dirOwner = otherSID
		if err := run(o, f); err == nil || !strings.Contains(lastLine(out.String()), "-REFUSED: the data folder: ") {
			t.Fatalf("%v\n%s", err, out)
		}
		if _, ok := readResult(t, file); ok || count(f.log, "result") != 0 {
			t.Fatalf("a result was written into a user's folder; log:\n%s", strings.Join(f.log, "\n"))
		}
		if !strings.Contains(out.String(), ": the result for the window is not recorded: "+o.DataDir+" is there and belongs to "+otherSID) {
			t.Fatalf("output:\n%s", out)
		}
	}
}

// export\ is a junction to another folder (mklink /J inside a temporary folder needs no administrator):
// nothing is written through it, after a success or a refusal; the outcome stays.
func TestNoRecordThroughALinkedExportFolder(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		f, o, out := setup(t)
		target := t.TempDir()
		if err := os.MkdirAll(o.DataDir, 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(o.DataDir, "export")
		if b, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Fatalf("mklink /J: %v: %s", err, b)
		}
		want := "INSTALL-OK"
		if refuse {
			f.helper, want = "stays", "INSTALL-REFUSED: the helper that runs did not end: "
		}
		if err := Install(o, f); (err != nil) != refuse || !strings.HasPrefix(lastLine(out.String()), want) {
			t.Fatalf("%v\n%s", err, out)
		}
		if entries, err := os.ReadDir(target); err != nil || len(entries) != 0 || count(f.log, "result") != 0 {
			t.Fatalf("written through the junction: %v %v; log:\n%s", entries, err, strings.Join(f.log, "\n"))
		}
		if !strings.Contains(out.String(), "INSTALL: the result for the window is not recorded: "+link+" is not a plain folder") {
			t.Fatalf("output:\n%s", out)
		}
	}
}

// A result that cannot be written is a line: install still succeeds or refuses as it did, with its own reason.
func TestRecordFailureDoesNotChangeTheOutcome(t *testing.T) {
	f, o, out := setup(t)
	mkExport(t, o)
	f.writeErr = errors.New("access is denied")
	if err := Install(o, f); err != nil || lastLine(out.String()) != "INSTALL-OK" ||
		!strings.Contains(out.String(), "INSTALL: the result for the window is not recorded: access is denied\n") {
		t.Fatalf("%v\n%s", err, out)
	}

	f, o, out = setup(t)
	mkExport(t, o)
	f.writeErr = errors.New("access is denied")
	f.stay = []Proc{{PID: 10}}
	if err := Prepare(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "PREPARE-REFUSED: splitlane-helper.exe or sing-box.exe still running (process 10)") {
		t.Fatalf("%v\n%s", err, out)
	}

	// No data folder (a first install that refuses): nothing to write into, and the outcome stays.
	f, o, out = setup(t)
	f.helper = "stays"
	if err := Install(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: the helper that runs did not end: ") ||
		!strings.Contains(out.String(), "INSTALL: the result for the window is not recorded: "+o.DataDir+" is not there\n") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := os.Lstat(o.DataDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the data folder was made: %v", err)
	}
}
