package protocol

// Values of tailscale.backend and tailscale.path (spec 5.2). BackendExpired is the engine's own:
// the node says needs_login while the main core runs (spec 7, «Истечение входа»).
const (
	BackendStopped       = "stopped"
	BackendNeedsLogin    = "needs_login"
	BackendStarting      = "starting"
	BackendRunning       = "running"
	BackendNeedsApproval = "needs_approval"
	BackendExpired       = "expired"

	PathDirect = "direct"
	PathRelay  = "relay"
)
