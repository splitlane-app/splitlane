// Package store keeps the helper's files in the data folder: state.json (spec 5.2),
// the restore journal restore.json (spec 5.7), the rule files and the core config; it names
// the logs and the folder of the diagnostics archives (spec 5.1, 4.2).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"splitlane/helper/internal/atomicfile"
	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/protocol"
)

// DefaultProbeURL answers with the external address (spec 11); it is kept in state.json, not sent over the pipe.
const DefaultProbeURL = "https://api.ipify.org"

// CoreIdentity tells the helper's own core from any other sing-box.exe (spec 5.7).
type CoreIdentity struct {
	PID     int    `json:"pid"`
	Created int64  `json:"created"` // process creation time, FILETIME in 100 ns units
	Path    string `json:"path"`
}

// State is state.json (spec 5.2).
type State struct {
	Version          int                 `json:"version"`
	Apps             []protocol.AppRule  `json:"apps"`
	DefaultLane      string              `json:"defaultLane"`
	SelectedExitNode string              `json:"selectedExitNode"`
	LoggedIn         bool                `json:"loggedIn"`
	ExitNodes        []protocol.ExitNode `json:"exitNodes"`
	DeviceName       string              `json:"deviceName,omitempty"`
	WasOn            bool                `json:"wasOn"`
	Core             *CoreIdentity       `json:"core,omitempty"`
	ProbeURL         string              `json:"probeUrl"`
}

// Journal is restore.json: what to give back to the system Tailscale (spec 5.7).
type Journal struct {
	ExitNode string `json:"exitNode"`
	AllowLAN bool   `json:"allowLan"`
}

// Defaults is the state of a fresh install.
func Defaults() State {
	return State{Version: 1, Apps: []protocol.AppRule{}, DefaultLane: protocol.LaneVPN,
		ExitNodes: []protocol.ExitNode{}, ProbeURL: DefaultProbeURL}
}

// Dir is the data folder: %ProgramData%\SplitLane, or the development data folder.
type Dir struct {
	Path string
}

func (d Dir) state() string   { return filepath.Join(d.Path, "state.json") }
func (d Dir) journal() string { return filepath.Join(d.Path, "restore.json") }

// ErrIO marks a file that is there but could not be read (access denied, a sharing violation): its
// content is unknown, so it stays where it is and must not be taken for missing or written over.
// Unreadable content is another error: that file is set aside.
var ErrIO = errors.New("store: read failed")

// readFile is os.ReadFile; the tests replace it to simulate a read that fails.
var readFile = os.ReadFile

// LoadState reads state.json; a missing file gives Defaults. A file that cannot be read gives Defaults
// and an error that wraps ErrIO: the file stays, and the caller must not save over it. Unreadable
// content is set aside as state.json.bad (or the first free state.json.bad.<n>) and gives Defaults
// with an error that gives the cause; if it cannot be set aside, the error says so.
func (d Dir) LoadState() (State, error) {
	s := Defaults()
	b, err := readFile(d.state())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("%w: %w", ErrIO, err)
	}
	if cause := json.Unmarshal(b, &s); cause != nil {
		return Defaults(), setAside(d.state(), cause)
	}
	if s.Apps == nil {
		s.Apps = []protocol.AppRule{}
	}
	if s.ExitNodes == nil {
		s.ExitNodes = []protocol.ExitNode{}
	}
	if s.DefaultLane != protocol.LaneDirect {
		s.DefaultLane = protocol.LaneVPN
	}
	if s.ProbeURL == "" {
		s.ProbeURL = DefaultProbeURL
	}
	return s, nil
}

// SaveState writes state.json atomically.
func (d Dir) SaveState(s State) error {
	return d.writeJSON(d.state(), s)
}

// renameFile is os.Rename; the tests replace it to simulate a rename that fails.
var renameFile = os.Rename

// maxBadFiles bounds how many set-aside copies of one file are kept side by side.
const maxBadFiles = 100

// LoadJournal reads restore.json; nil without error when there is none. A journal that is there
// but cannot be read gives an error that wraps ErrIO: it stays where it is, and the caller must not
// take it for "no journal". An unreadable journal is renamed to restore.json.bad (or the first free
// restore.json.bad.<n>) and reported as an error that gives the cause. If it cannot be renamed, the
// error says so and the journal stays where it is.
func (d Dir) LoadJournal() (*Journal, error) {
	b, err := readFile(d.journal())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIO, err)
	}
	var j Journal
	cause := json.Unmarshal(b, &j)
	if cause == nil && j.ExitNode == "" {
		cause = errors.New("empty exitNode")
	}
	if cause != nil {
		return nil, setAside(d.journal(), cause)
	}
	return &j, nil
}

// setAside renames a file with unreadable content to the first free name among <name>.bad,
// <name>.bad.1, <name>.bad.2 ... so that an earlier set-aside copy is never replaced, and returns
// the error that reports it with its cause. If it cannot be renamed, the error says so and the file
// stays where it is.
func setAside(path string, cause error) error {
	name := filepath.Base(path)
	bad, err := badName(path)
	if err == nil {
		err = renameFile(path, bad)
	}
	if err != nil {
		return fmt.Errorf("%s is unreadable (%v) and could not be set aside: %w", name, cause, err)
	}
	return fmt.Errorf("%s is unreadable (%v) and was set aside as %s", name, cause, filepath.Base(bad))
}

// badName is the first name among <path>.bad, <path>.bad.1, <path>.bad.2 ... that is not taken.
func badName(path string) (string, error) {
	for n := 0; n < maxBadFiles; n++ {
		bad := path + ".bad"
		if n > 0 {
			bad += "." + strconv.Itoa(n)
		}
		_, err := os.Lstat(bad)
		if err == nil {
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return bad, nil
	}
	return "", fmt.Errorf("all %d names %s.bad ... are taken", maxBadFiles, filepath.Base(path))
}

// SaveJournal writes restore.json atomically and flushed to disk, before `tailscale set` runs (spec 5.7).
func (d Dir) SaveJournal(j Journal) error {
	return d.writeJSON(d.journal(), j)
}

// DeleteJournal removes restore.json; a missing file is not an error.
func (d Dir) DeleteJournal() error {
	err := os.Remove(d.journal())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// WriteRules writes both rule files (spec 6.5). The rules folder is created once and never recreated.
func (d Dir) WriteRules(direct, vpn []byte) error {
	p := coreconfig.PathsFor(d.Path)
	if err := os.MkdirAll(p.RulesDir, 0o755); err != nil {
		return err
	}
	if err := atomicfile.Write(p.RulesDirect, direct); err != nil {
		return err
	}
	return atomicfile.Write(p.RulesVPN, vpn)
}

// WriteConfig writes the core config and returns its path.
func (d Dir) WriteConfig(config []byte) (string, error) {
	p := coreconfig.PathsFor(d.Path)
	if err := os.MkdirAll(p.LogsDir, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(p.StateDir, 0o755); err != nil {
		return "", err
	}
	return p.Config, atomicfile.Write(p.Config, config)
}

// ExportDir is the folder of the diagnostics archives, export\ of the data folder (spec 4.2): the one
// folder that the user who set SplitLane up may read.
func (d Dir) ExportDir() string { return filepath.Join(d.Path, "export") }

// Logs are the logs that diagnostics.export takes (spec 5.1): the helper's, the main core's and the
// setup core's in logs\, each with the one of the run before (.prev).
func (d Dir) Logs() []string {
	p := coreconfig.PathsFor(d.Path)
	helper := filepath.Join(p.LogsDir, "helper.log")
	return []string{helper, helper + ".prev", p.CoreLog, p.CoreLog + ".prev", p.SetupLog, p.SetupLog + ".prev"}
}

func (d Dir) writeJSON(path string, v any) error {
	if err := os.MkdirAll(d.Path, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(path, append(b, '\n'))
}
