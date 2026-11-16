package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"

	"splitlane/helper/internal/serversetup"
)

// maxRequestLine is the longest request line of server-setup, without its newline.
const maxRequestLine = 64 << 10

// serverSetup runs `server-setup` (spec 5): one JSON line from stdin (serversetup.ParseRequest), events and
// the final line to stdout, exit code 0 (done) or 1 (failed). Closing stdin cancels. A last line without
// its newline does not count: the window ends every request with one, so a missing newline is a cut write.
// Nothing else is written: no data folder, no log, no stderr.
func serverSetup(stdin io.Reader, stdout io.Writer, run func(context.Context, serversetup.Request, func(serversetup.Event)) (serversetup.Done, *serversetup.Failure)) int {
	var mu sync.Mutex
	enc := json.NewEncoder(stdout)
	write := func(v any) {
		mu.Lock()
		defer mu.Unlock()
		_ = enc.Encode(v)
	}
	fail := func(f *serversetup.Failure) int {
		write(struct {
			Failed *serversetup.Failure `json:"failed"`
		}{f})
		return 1
	}

	br := bufio.NewReaderSize(stdin, maxRequestLine+2)
	line, err := br.ReadSlice('\n')
	req, bad := serversetup.Request{}, (*serversetup.Failure)(nil)
	switch {
	case err != nil, len(line)-1 > maxRequestLine: // too long, or stdin ended before a newline
		bad = &serversetup.Failure{Code: serversetup.CodeBadInput}
	default:
		req, bad = serversetup.ParseRequest(line[:len(line)-1])
	}
	clear(line)
	if bad != nil {
		return fail(bad)
	}
	defer req.Wipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { // the window closes stdin to cancel
		_, _ = io.Copy(io.Discard, br)
		cancel()
	}()

	done, f := run(ctx, req, func(e serversetup.Event) { write(e) })
	if f != nil {
		return fail(f)
	}
	write(struct {
		Done serversetup.Done `json:"done"`
	}{done})
	return 0
}
