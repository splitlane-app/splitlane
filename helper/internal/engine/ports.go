// Package engine is the phase machine of the helper: enabling (spec 5.3),
// disabling and rollback (5.4), supervision (5.5), the system Tailscale (5.6),
// the restore journal (5.7) and the commands of 5.1. Everything that touches the
// system sits behind the interfaces of this file, so the tests run on fakes; the core
// sits behind CoreDriver (spec 4), so the engine does not depend on sing-box.
package engine

import (
	"context"
	"time"

	"splitlane/helper/internal/connector"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
	"splitlane/helper/internal/store"
)

// Clock is time; tests use a clock that does not wait.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// Store is the data folder (package store).
type Store interface {
	LoadState() (store.State, error)
	SaveState(store.State) error
	LoadJournal() (*store.Journal, error)
	SaveJournal(store.Journal) error
	DeleteJournal() error
	WriteRules(direct, vpn []byte) error
	WriteConfig(config []byte) (string, error)
	ExportDir() string // the folder of the diagnostics archives (spec 4.2)
	Logs() []string    // the logs that diagnostics.export takes, full paths (spec 5.1)
	// LoadServers and SaveServers are servers.json (stage 3 part 1, section 3): the servers and
	// subscriptions of the proxy connector with their secrets. SaveServers refuses more than the limits
	// (store.ErrTooMany).
	LoadServers() (store.Servers, error)
	SaveServers(store.Servers) error
}

// CoreProc is one running core process.
type CoreProc interface {
	Identity() store.CoreIdentity
	Done() <-chan struct{}
	Interrupt() error // the graceful signal: CTRL_BREAK_EVENT to its process group (spec 5.4)
	Kill() error
}

// Ports are the random local ports of one core run: two probe inbounds, the core's local API
// and its API service (spec 6.1).
type Ports struct {
	ProbeVPN    int
	ProbeDirect int
	Clash       int
	API         int
}

// CoreOptions are the values of one run that the engine decides (spec 6); the driver adds its
// own paths (data folder, rule files, log).
type CoreOptions struct {
	Mode         string // the core's mode at start, from CoreDriver.Mode (spec 6.3)
	HelperPath   string // full path of splitlane-helper.exe (rule 6 of 6.2)
	TailscaleDir string // install folder of the system Tailscale client (rule 5 of 6.2)
	Lane         Lane   // the connector of the VPN lane
	Ports        Ports
	ClashSecret  string
	APISecret    string
	DevPin       bool   // development only (Plan 1, safety rule 4)
	LogLevel     string // the core's log level: "" is info (spec 6.1), debug only from Settings.CoreLogLevel
}

// Lane is the connector of the VPN lane of one run (spec of stage 3 part 1, section 5): exactly one
// field is set, the driver refuses anything else. The driver renders it for its core; the lane is
// always the core's outbound or endpoint of the lane vpn.
type Lane struct {
	Tailscale *TailscaleLane    // the embedded Tailscale node with an exit node
	Proxy     *connector.Server // one server, parsed again from its saved text for this run
}

// TailscaleLane is the embedded Tailscale node of a run.
type TailscaleLane struct {
	Hostname string // <PC>-splitlane
	ExitNode string // IPv4 address of the exit node in the tailnet (spec 7)
}

// CoreEvents is what the core's log says after a byte offset: the connector state (spec 5.3
// step 4, 7) and the reload of a lane file (8.3).
type CoreEvents struct {
	Started      bool // this run of the core wrote its start line
	NeedLogin    bool // the connector asks for a login
	ReloadFailed bool // the core could not read a lane file again
}

// CoreDriver hides the core (spec 4): build the config, apply the lanes (the rule files and the
// mode), get and close connections (ClashAPI), the connector state (Events, NodeAPI), the config
// of the setup mode (spec 7). Stage 1 has one driver, for sing-box 1.14.4 (package singbox). The
// engine knows nothing of the core's config, rule-file format, mode names or log lines: a second
// core (stage 3) is a second driver.
type CoreDriver interface {
	// BuildConfig returns the config of one run (spec 6).
	BuildConfig(o CoreOptions) ([]byte, error)
	// LaneFiles renders the layout as the two rule files the core re-reads (6.5, 8.2).
	LaneFiles(apps []protocol.AppRule, env rules.Env) (direct, vpn []byte, err error)
	// Mode names the core's mode for the main lane, "all direct" and degraded (6.3).
	Mode(defaultLane string, allDirect, degraded bool) string
	Check(ctx context.Context, configPath string) error
	// Start sets the previous log aside and starts the core; the log then holds only this run.
	Start(configPath string) (CoreProc, error)
	// StartSetup starts the core of the setup mode (spec 7) as Start does, with a log of its own:
	// a setup run never sets aside the log of a main core. SetupStarted tells that its start line came.
	StartSetup(configPath string) (CoreProc, error)
	SetupStarted() bool
	// Find returns the helper's own core left from an earlier run, if it still runs (spec 5.7).
	Find(id store.CoreIdentity) (CoreProc, bool)
	// LogSize is the size of the current core log; Events reads the log after a byte offset.
	LogSize() int64
	Events(offset int64) CoreEvents
	Clash(port int, secret string) ClashAPI
	// BuildSetupConfig returns the config of the setup mode (spec 7): the node, local DNS, the API service; no adapter.
	BuildSetupConfig(o SetupOptions) ([]byte, error)
	// Node is the API service of a running core.
	Node(port int, secret string) NodeAPI
}

// Conn is one connection of the core. Lane is the lane it runs on (the driver names its outbounds
// after the lanes); ByLanes tells that rules 8–11 of 6.2 (the mode or a lane file) chose it. Host,
// Upload and Download are for the live activity (8.4).
type Conn struct {
	ID          string
	ProcessPath string
	Lane        string
	ByLanes     bool
	Host        string // the name from the reverse mapping of DNS answers (6.4), else the destination address
	Upload      int64  // bytes sent so far
	Download    int64  // bytes received so far
}

// ClashAPI is the core's local API (spec 3).
type ClashAPI interface {
	// SetMode switches the clash mode and reads it back; an error when the core did not take it (spec 6.3).
	SetMode(ctx context.Context, mode string) error
	Connections(ctx context.Context) ([]Conn, error)
	CloseConn(ctx context.Context, id string) error
}

// SetupOptions are the values of one run of the setup mode (spec 7); the driver adds its paths.
type SetupOptions struct {
	Hostname  string
	APIPort   int
	APISecret string
	LogLevel  string // as CoreOptions.LogLevel
}

// NodeState is what the API service says about the embedded node (spec 7, 5.2).
type NodeState struct {
	Backend    string // protocol.Backend*: stopped, needs_login, starting, running, needs_approval (never expired: the engine derives it)
	AuthURL    string // the login link, "" when none is offered
	HasAddress bool   // the node has its IPv4 address in the tailnet
}

// NodeAPI is the core's API service (spec 3, 7), one run of the core.
type NodeAPI interface {
	State(ctx context.Context) (NodeState, error)
	ExitNodes(ctx context.Context) ([]protocol.ExitNode, error) // id = IPv4 address in the tailnet (spec 7)
	Path(ctx context.Context, exitNode string) (string, error)  // protocol.PathDirect, protocol.PathRelay or "" when unknown
	Logout(ctx context.Context) error
}

// SysStatus is what the system Tailscale client says (spec 5.6).
type SysStatus struct {
	Installed  bool
	ExitNodeOn bool
	ExitNodeIP string // IPv4 address of the selected exit node
}

// SystemTailscale is the official client, called by its full path (package systs).
type SystemTailscale interface {
	Status(ctx context.Context) (SysStatus, error)
	AllowLAN(ctx context.Context) (bool, error)
	SetExitNode(ctx context.Context, ip string, allowLAN bool) error
	ClearExitNode(ctx context.Context) error
	RestoreCommand(ip string, allowLAN bool) string
	Dir() string
}

// ProbeResult: Reached is false on a timeout or a connection error; IP is empty when the
// service answered without an address (an HTTP error is not a failure of the link, spec 5.5).
type ProbeResult struct {
	Reached bool
	IP      string
}

// Prober asks the address service which address a request comes from (spec 5.3, 5.5).
type Prober interface {
	ViaProxy(ctx context.Context, url string, port int) ProbeResult // through a probe inbound of the core
	Direct(ctx context.Context, url string) ProbeResult             // the helper's own request: the path through the adapter
}

// Host is the rest of Windows (package winhost).
type Host interface {
	HasDefaultRoute() bool
	IPForwarding() bool
	DNSOverride() bool
	CoreRunning() bool
	AdapterPresent() bool
	FlushDNS() error
	FreePorts(n int) ([]int, error)
	Secret() string
	// RemoveStaleAdapter removes the Wintun device of the core's adapter that a forced end of an
	// earlier core left behind, non-present (5.3 step 4); removed tells that there was one.
	RemoveStaleAdapter() (removed bool, err error)
}

// Settings are fixed for the life of the process.
type Settings struct {
	HelperPath    string
	Hostname      string // <PC>-splitlane; state.json deviceName overrides it
	DevPin        bool   // development only, see coreconfig.DevPinProcesses
	HelperVersion string
	CoreVersion   string
	CoreLogLevel  string // development only: debug from --core-log-level; "" or info in the product (spec 6.1)
}

// Deps are everything the engine uses.
type Deps struct {
	Clock    Clock
	Store    Store
	Core     CoreDriver
	Sys      SystemTailscale
	Probe    Prober
	Host     Host
	Fetch    connector.Fetcher // downloads subscriptions (stage 3 part 1, section 3); nil refuses every download
	Rules    rules.Env
	Settings Settings
	Log      func(format string, args ...any)
	OnState  func(protocol.State) // every change of the state (event "state")
	OnExit   func()               // after helper.shutdown has finished
}
