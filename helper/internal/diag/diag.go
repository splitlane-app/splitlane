// Package diag builds the archive of diagnostics.export (spec 5.1): the logs of the helper and of the
// core, the saved state and a few facts, as one zip. It takes only what it is given, never the core
// config, the node's state folder or a secret (spec 11), and it hides the node's login link in the logs.
package diag

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"splitlane/helper/internal/atomicfile"
	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/protocol"
)

// maxLog bounds what the archive takes of one log: its last 8 MiB. The logs themselves are never cut
// while they are written (plan 1B).
const maxLog = 8 << 20

// Input is what one archive holds.
type Input struct {
	Logs  []string          // full paths; each goes in as logs/<base name>, one that is not there is named in meta.txt
	State []byte            // the saved state, as state.json
	Meta  map[string]string // meta.txt: one "key: value" line each, sorted by key
}

// loginLink takes the place of a login link in the logs.
const loginLink = "<login link>"

var (
	// loginRe is a link of the Tailscale login server, or a link https://<host>/a/<token> of any control server.
	loginRe = regexp.MustCompile(`https?://(?:login\.tailscale\.com|[^\s"'<>]*/a/)[^\s"'<>]*`)
	// urlRe is any link: in the line of coreconfig.LoginLine and the line after it every link is the login link.
	urlRe = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

// Build writes the archive to zipPath: logs/<base name> for each log that is there (its tail, the login
// links hidden), state.json and meta.txt; the logs that are not there are named in meta.txt under
// "missing logs". The folder is created when it is not there; the archive is written to a temp file in
// it and renamed (atomicfile), so a reader never meets half an archive.
func Build(zipPath string, in Input) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	now := time.Now()
	meta := maps.Clone(in.Meta)
	if meta == nil {
		meta = map[string]string{}
	}
	var missing []string
	for _, path := range in.Logs {
		name := filepath.Base(path)
		data, ok, err := tail(path)
		switch {
		case err != nil:
			return fmt.Errorf("diag: log %s: %w", name, err)
		case !ok:
			missing = append(missing, name)
			continue
		}
		if err := add(zw, "logs/"+name, redact(data), now); err != nil {
			return err
		}
	}
	if len(missing) > 0 {
		meta["missing logs"] = strings.Join(missing, ", ")
	}
	if err := add(zw, "state.json", in.State, now); err != nil {
		return err
	}
	var text strings.Builder
	for _, k := range slices.Sorted(maps.Keys(meta)) {
		fmt.Fprintf(&text, "%s: %s\n", k, meta[k])
	}
	if err := add(zw, "meta.txt", []byte(text.String()), now); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o755); err != nil {
		return err
	}
	return atomicfile.Write(zipPath, buf.Bytes())
}

func add(zw *zip.Writer, name string, data []byte, at time.Time) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: at})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// tail reads a log as far as it was written when it was opened: a log of up to maxLog bytes whole; of a
// longer one the whole lines in its last maxLog bytes, after a first line that says how many bytes were
// left out. ok is false when the file is not there.
func tail(path string) (data []byte, ok bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	size := fi.Size()
	if size <= maxLog {
		data, err = io.ReadAll(io.LimitReader(f, size))
		return data, err == nil, err
	}
	// One byte more than maxLog: when the cut falls right after a line end, that end is in buf and the
	// line after it is kept whole.
	from := size - maxLog - 1
	buf := make([]byte, maxLog+1)
	if _, err := f.ReadAt(buf, from); err != nil {
		return nil, false, err
	}
	i := bytes.IndexByte(buf, '\n')
	if i < 0 {
		i = len(buf) - 1 // one line longer than maxLog: nothing of it is taken
	}
	head := fmt.Sprintf("[splitlane: the first %d bytes were left out]\n", from+int64(i)+1)
	return append([]byte(head), buf[i+1:]...), true, nil
}

// ReasonParams writes the params of a phase's reason on one line, for the state line of helper.log and for
// meta.txt (review F12-1): key="value" sorted by key, each value quoted (the output of sing-box check has many
// lines), its login links hidden as in the logs and its IPv4 addresses masked (protocol.MaskAddresses); ""
// when there are none.
func ReasonParams(params map[string]string) string {
	parts := make([]string, 0, len(params))
	for _, k := range slices.Sorted(maps.Keys(params)) {
		v := protocol.MaskAddresses(string(redact([]byte(params[k]))))
		parts = append(parts, k+"="+strconv.Quote(v))
	}
	return strings.Join(parts, " ")
}

// redact hides the login links of a log: every link of the login server or of the form
// https://<host>/a/<token>, and every link in the line where the core reports the login link
// (coreconfig.LoginLine) and in the line after it.
func redact(text []byte) []byte {
	lines := bytes.SplitAfter(text, []byte("\n"))
	after := false
	for i, line := range lines {
		wait := bytes.Contains(line, []byte(coreconfig.LoginLine))
		if wait || after {
			lines[i] = urlRe.ReplaceAll(line, []byte(loginLink))
		} else {
			lines[i] = loginRe.ReplaceAll(line, []byte(loginLink))
		}
		after = wait
	}
	return bytes.Join(lines, nil)
}
