package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"splitlane/helper/internal/diag"
	"splitlane/helper/internal/protocol"
)

// The archives of diagnostics.export (5.1, 4.2): export\splitlane-diagnostics-YYYYMMDD-HHMMSS.zip, the
// local time of the engine's clock; so many are kept, the new one among them.
const (
	archivePrefix = "splitlane-diagnostics-"
	archiveSuffix = ".zip"
	keepArchives  = 5
)

// cmdExport is diagnostics.export (5.1): in any phase, outside the queue of the state-changing commands;
// the answer comes when the archive is written. The archive holds the logs of the helper and of the core,
// the saved state (state.json as this run holds it; never the core config, the rule files, the node's
// state folder, the journal or a secret: spec 11) and meta.txt: the versions, the phase and its reason,
// the time, and whether a restore journal is there (one that could not be read counts as there, as for
// restoreOnDisable). The state is read under e.mu; the archive is built without it.
func (e *Engine) cmdExport(params json.RawMessage) (any, *protocol.Error) {
	if err := noParams(params); err != nil {
		return nil, err
	}
	now := e.d.Clock.Now().Local()
	e.mu.Lock()
	state, err := json.MarshalIndent(e.saved, "", "  ")
	why := "-"
	if e.st.Reason != nil {
		why = e.st.Reason.Code
	}
	journal := "absent"
	if e.journal != nil || e.journalErr != nil {
		journal = "present"
	}
	meta := map[string]string{"helper": e.d.Settings.HelperVersion, "core": e.d.Settings.CoreVersion,
		"phase": e.st.Phase, "reason": why, "time": now.Format(time.RFC3339), "journal": journal}
	e.mu.Unlock()
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeInternal, "diagnostics.export: state: %v", err)
	}
	dir := e.d.Store.ExportDir()
	name := archivePrefix + now.Format("20060102-150405") + archiveSuffix
	path := filepath.Join(dir, name)
	in := diag.Input{Logs: e.d.Store.Logs(), State: append(state, '\n'), Meta: meta}
	if err := diag.Build(path, in); err != nil {
		e.d.Log("diagnostics.export failed: %v", err)
		return nil, protocol.Errorf(protocol.CodeInternal, "diagnostics.export: %v", err)
	}
	e.d.Log("diagnostics.export: %s written", name)
	e.pruneArchives(dir, name)
	return protocol.ExportResult{Path: path}, nil
}

// pruneArchives deletes the older archives in dir: the one just written (keep) stays whatever its name,
// and so do the newest of the others by name (the name carries the time; os.ReadDir sorts by name),
// keepArchives in all. Other files stay. A failure is logged: the archive is written, the export is done.
func (e *Engine) pruneArchives(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		e.d.Log("diagnostics.export: the older archives not listed: %v", err)
		return
	}
	var others []string
	for _, en := range entries {
		n := en.Name()
		if n != keep && en.Type().IsRegular() && strings.HasPrefix(n, archivePrefix) && strings.HasSuffix(n, archiveSuffix) {
			others = append(others, n)
		}
	}
	for _, n := range others[:max(0, len(others)-(keepArchives-1))] {
		if err := os.Remove(filepath.Join(dir, n)); err != nil && !errors.Is(err, os.ErrNotExist) {
			e.d.Log("diagnostics.export: the older archive %s not deleted: %v", n, err)
		}
	}
}
