package serversetup

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"splitlane/helper/internal/connector"
)

// The kit's --json output (scripts/server/install-singbox-server.sh): every line of stdout is one JSON
// object, a step {"step","state","detail"}, then one result {"result":{"links","wireguard",
// "subscription","ports","warnings"}} or one error {"error":{"code","message"}}. Its text for people
// goes to stderr. Whatever else reaches stdout (it should be nothing, but apt or a shell can say
// something) is skipped.

// kitStep maps the kit's steps to the window's. The kit's own system step is not shown: SplitLane
// checked the system before it uploaded the kit.
var kitStep = map[string]string{
	"singbox":      "install",
	"config":       "install",
	"service":      "install",
	"harden":       "harden",
	"subscription": "subscription",
}

var kitStates = map[string]bool{"start": true, "done": true, "skip": true, "warn": true}

// kitCodes are the kit's error codes the window knows; any other is kit_failed.
var kitCodes = map[string]bool{CodeOSUnsupported: true, CodeDownloadFailed: true, CodePortsBusy: true}

// warningCode is the form of a warning code; a warning of another form is dropped.
var warningCode = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

// kitLine is one line of the kit as Run uses it: a step for the window, the result, the error, or
// nothing (all nil).
type kitLine struct {
	event  *Event
	result *kitResult
	fail   *Failure
}

// kitResult is the part of the kit's result SplitLane uses.
type kitResult struct {
	Links        []string // non-empty, trimmed, one line each
	Subscription string   // an https:// address, or "" when the kit set up none
	Warnings     []string // warning codes, each once
}

// parseKitLine reads one line of the kit's stdout. A line is used only when it is a JSON object with
// exactly one of step, result and error, of the right types; a step only when the window shows it
// and its state is known. Details and messages go through connector.Mask: the kit puts no secret in
// them, and the mask is there in case one slips in.
func parseKitLine(line []byte) kitLine {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return kitLine{}
	}
	var l struct {
		Step   *string `json:"step"`
		State  string  `json:"state"`
		Detail string  `json:"detail"`
		Result *struct {
			Links        []string `json:"links"`
			Subscription *string  `json:"subscription"`
			Warnings     []string `json:"warnings"`
		} `json:"result"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(line, &l) != nil {
		return kitLine{}
	}
	kinds := 0
	for _, has := range []bool{l.Step != nil, l.Result != nil, l.Error != nil} {
		if has {
			kinds++
		}
	}
	switch {
	case kinds != 1:
		return kitLine{}
	case l.Step != nil:
		step, ok := kitStep[*l.Step]
		if !ok || !kitStates[l.State] {
			return kitLine{}
		}
		return kitLine{event: &Event{Step: step, State: l.State, Detail: connector.Mask(l.Detail)}}
	case l.Error != nil:
		code := l.Error.Code
		if !kitCodes[code] {
			code = CodeKitFailed
		}
		return kitLine{fail: &Failure{Code: code, Detail: connector.Mask(l.Error.Message)}}
	}
	res := &kitResult{}
	for _, link := range l.Result.Links {
		// Done.Links has a link a line: one with a line break inside cannot be one.
		if link = strings.TrimSpace(link); link != "" && !strings.ContainsAny(link, "\r\n") {
			res.Links = append(res.Links, link)
		}
	}
	if sub := l.Result.Subscription; sub != nil {
		if u := strings.TrimSpace(*sub); strings.HasPrefix(u, "https://") && !strings.ContainsAny(u, " \t\r\n") {
			res.Subscription = u
		}
	}
	for _, w := range l.Result.Warnings {
		if warningCode.MatchString(w) && !contains(res.Warnings, w) {
			res.Warnings = append(res.Warnings, w)
		}
	}
	return kitLine{result: res}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// eachLine calls fn with every line of r, its '\n' included (the last line may have none). A line
// longer than max bytes is skipped whole, so no line holds more than max bytes in memory. fn must not
// keep the slice.
func eachLine(r io.Reader, max int, fn func(line []byte)) error {
	br := bufio.NewReaderSize(r, min(max, 64<<10))
	var line []byte
	skip := false
	for {
		chunk, err := br.ReadSlice('\n')
		if !skip {
			if len(line)+len(chunk) > max {
				line, skip = line[:0], true
			} else {
				line = append(line, chunk...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if !skip && len(line) > 0 {
			fn(line)
		}
		line, skip = line[:0], false
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// tail keeps the last max bytes written to it.
type tail struct {
	max int
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.max; over > 0 {
		t.buf = t.buf[:copy(t.buf, t.buf[over:])]
	}
	return len(p), nil
}

func (t *tail) bytes() []byte { return t.buf }

// lastLines is the last n non-blank lines of b, each made one line of text, joined with " | ": what
// a command said last on stderr, for the detail of a failure.
func lastLines(b []byte, n int) string {
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = oneLine(l); l != "" {
			out = append(out, l)
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return strings.Join(out, " | ")
}
