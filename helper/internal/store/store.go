// Package store keeps the helper's files in the data folder: state.json (spec 5.2),
// the restore journal restore.json (spec 5.7), the rule files and the core config.
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

// LoadState reads state.json; a missing file gives Defaults.
func (d Dir) LoadState() (State, error) {
	s := Defaults()
	b, err := os.ReadFile(d.state())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return Defaults(), fmt.Errorf("state.json: %w", err)
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

// maxBadJournals bounds how many set-aside journals are kept side by side.
const maxBadJournals = 100

// LoadJournal reads restore.json; nil without error when there is none.
// An unreadable journal is renamed to restore.json.bad (or the first free restore.json.bad.<n>)
// and reported as an error that gives the cause. If it cannot be renamed, the error says so
// and the journal stays where it is.
func (d Dir) LoadJournal() (*Journal, error) {
	b, err := os.ReadFile(d.journal())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var j Journal
	cause := json.Unmarshal(b, &j)
	if cause == nil && j.ExitNode == "" {
		cause = errors.New("empty exitNode")
	}
	if cause != nil {
		bad, err := d.setAsideJournal()
		if err != nil {
			return nil, fmt.Errorf("restore.json is unreadable (%v) and could not be set aside: %w", cause, err)
		}
		return nil, fmt.Errorf("restore.json is unreadable (%v) and was set aside as %s", cause, filepath.Base(bad))
	}
	return &j, nil
}

// setAsideJournal renames restore.json to the first free name among restore.json.bad,
// restore.json.bad.1, restore.json.bad.2 ... so that an earlier set-aside journal is never replaced.
func (d Dir) setAsideJournal() (string, error) {
	for n := 0; n < maxBadJournals; n++ {
		bad := d.journal() + ".bad"
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
		return bad, renameFile(d.journal(), bad)
	}
	return "", fmt.Errorf("all %d names restore.json.bad ... are taken", maxBadJournals)
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
