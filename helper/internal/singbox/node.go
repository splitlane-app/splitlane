package singbox

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/protocol"
)

// Runner runs exe with args, with env added to the helper's own environment, and returns the
// standard output; a non-zero exit is an error that carries the standard error text.
type Runner func(ctx context.Context, env []string, exe string, args ...string) ([]byte, error)

// Node is the client of the core's API service (spec 7; Plan 0, check (б)) through the core's own
// CLI, `sing-box api tailscale ...`. The URL and the secret go through the environment
// (BOX_API_URL, BOX_API_SECRET): the secret is never on a command line. The caller's ctx bounds
// every call; a ping takes up to about ten seconds.
type Node struct {
	Exe    string
	URL    string // 127.0.0.1:<port> of the API service of this run
	Secret string
	Run    Runner
}

var _ engine.NodeAPI = (*Node)(nil)

// NewNode talks to the API service on 127.0.0.1:port with the run's secret, through exe without a window.
func NewNode(exe string, port int, secret string) *Node {
	return &Node{Exe: exe, URL: fmt.Sprintf("127.0.0.1:%d", port), Secret: secret, Run: runAPI}
}

// colorCode is an ANSI color sequence: the core colors its FATAL line on stderr.
var colorCode = regexp.MustCompile("\x1b\\[[0-9;]*m")

func runAPI(ctx context.Context, env []string, exe string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = hidden(windows.CREATE_NO_WINDOW)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		text := strings.TrimSpace(colorCode.ReplaceAllString(stderr.String(), ""))
		if len(text) > 400 {
			text = text[:400]
		}
		return out, fmt.Errorf("%s %s: %v: %s", filepath.Base(exe), strings.Join(args, " "), err, text)
	}
	return out, nil
}

func (n *Node) tailscale(ctx context.Context, args ...string) ([]byte, error) {
	env := []string{"BOX_API_URL=" + n.URL, "BOX_API_SECRET=" + n.Secret}
	return n.Run(ctx, env, n.Exe, append([]string{"api", "tailscale"}, args...)...)
}

// State reads `status` (the State and Log in lines) and the first row of `peer list`, which is
// the node itself: `status` does not print the node's addresses.
func (n *Node) State(ctx context.Context) (engine.NodeState, error) {
	out, err := n.tailscale(ctx, "status")
	if err != nil {
		return engine.NodeState{}, err
	}
	st := parseStatus(out)
	peers, err := n.tailscale(ctx, "peer", "list")
	if err != nil {
		return engine.NodeState{}, err
	}
	st.HasAddress = selfHasAddress(peers)
	return st, nil
}

// parseStatus reads the block of `status`: "  State:    Running", "  Log in:   https://...".
// An empty value is printed as "-".
func parseStatus(out []byte) engine.NodeState {
	var state, authURL string
	for _, line := range lines(out) {
		label, value, ok := strings.Cut(line, ":")
		if !ok || !strings.HasPrefix(label, "  ") {
			continue
		}
		if value = strings.TrimSpace(value); value == "-" {
			value = ""
		}
		switch strings.TrimSpace(label) {
		case "State":
			if state == "" {
				state = value
			}
		case "Log in":
			if authURL == "" {
				authURL = value
			}
		}
	}
	return engine.NodeState{Backend: backendOf(state), AuthURL: authURL}
}

// backendOf maps the state of the node (ipn.State of Tailscale) to tailscale.backend (spec 5.2).
func backendOf(state string) string {
	switch state {
	case "NeedsLogin":
		return protocol.BackendNeedsLogin
	case "NeedsMachineAuth":
		return protocol.BackendNeedsApproval
	case "Starting":
		return protocol.BackendStarting
	case "Running":
		return protocol.BackendRunning
	default: // Stopped, NoState, InUseOtherUser, nothing
		return protocol.BackendStopped
	}
}

// selfHasAddress reads the first row of `peer list`, the node itself: its IP cell is the first of
// its tailnet addresses, "-" while it has none.
func selfHasAddress(out []byte) bool {
	r := rows(out)
	return len(r) > 0 && len(r[0]) > 1 && isIPv4(r[0][1])
}

// ExitNodes reads `exit-node list`: the peers that offer to be an exit node, one row each (DNS
// name, first tailnet address, online yes/no, selected). The id is the IPv4 address (spec 7), so
// a row without one is left out; the name is the first label of the DNS name.
func (n *Node) ExitNodes(ctx context.Context) ([]protocol.ExitNode, error) {
	out, err := n.tailscale(ctx, "exit-node", "list")
	if err != nil {
		return nil, err
	}
	nodes := []protocol.ExitNode{}
	for _, cells := range rows(out) {
		if len(cells) < 3 || !isIPv4(cells[1]) {
			continue
		}
		name, _, _ := strings.Cut(cells[0], ".")
		nodes = append(nodes, protocol.ExitNode{ID: cells[1], Name: name, Online: cells[2] == "yes"})
	}
	return nodes, nil
}

// Path pings the exit node through the embedded node (`ping <address>`: up to ten pongs, it stops
// at the first direct one). The last pong tells the path: "via DERP(...)" and "via peer relay ..."
// are relays, "via <ip:port>" is direct. No pong: "" (unknown).
func (n *Node) Path(ctx context.Context, exitNode string) (string, error) {
	out, err := n.tailscale(ctx, "ping", exitNode)
	if err != nil {
		return "", err
	}
	path := ""
	for _, line := range lines(out) {
		at := strings.LastIndex(line, " via ")
		if !strings.HasPrefix(line, "pong from ") || at < 0 {
			continue
		}
		via := line[at+len(" via "):]
		if strings.HasPrefix(via, "DERP(") || strings.HasPrefix(via, "peer relay ") {
			path = protocol.PathRelay
		} else {
			path = protocol.PathDirect
		}
	}
	return path, nil
}

// Logout logs the embedded node out of the tailnet (spec 7, tailscale.logout).
func (n *Node) Logout(ctx context.Context) error {
	_, err := n.tailscale(ctx, "logout")
	return err
}

func lines(out []byte) []string {
	return strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
}

// rows are the non-empty lines of a table, split at tabs.
func rows(out []byte) [][]string {
	var r [][]string
	for _, line := range lines(out) {
		if line != "" {
			r = append(r, strings.Split(line, "\t"))
		}
	}
	return r
}

func isIPv4(s string) bool {
	addr, err := netip.ParseAddr(s)
	return err == nil && addr.Is4()
}
