package coreconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// SetupOptions are the inputs of the setup-mode config (spec 7).
type SetupOptions struct {
	DataDir   string // %ProgramData%\SplitLane or the development data folder
	Hostname  string // <PC>-splitlane
	APIPort   int    // the API service of this run (spec 6.1)
	APISecret string
}

// BuildSetup returns the config of the setup mode (spec 7) as indented JSON, in the shape that
// Plan 0 checked ((б), (д), (з)): the embedded node (no exit node), the local DNS server,
// route.default_domain_resolver and the API service. There is no adapter and no
// route.auto_detect_interface: without an adapter with strict_route the block of the system
// Tailscale would not let the core out through the physical adapter, so the node reaches the
// control server by the ordinary route of the system. No probes and no Clash API either.
func BuildSetup(o SetupOptions) ([]byte, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	p := PathsFor(o.DataDir)
	config := obj{
		"log": obj{"level": "info", "timestamp": true, "output": filepath.ToSlash(p.CoreLog)},
		"dns": obj{"servers": []any{obj{"type": "local", "tag": "dns-local"}}},
		"endpoints": []any{obj{
			"type":            "tailscale",
			"tag":             TagVPN,
			"state_directory": filepath.ToSlash(p.StateDir),
			"hostname":        o.Hostname,
		}},
		"outbounds": []any{obj{"type": "direct", "tag": TagDirect}},
		"route":     obj{"default_domain_resolver": "dns-local"},
		"services": []any{obj{
			"type": "api", "tag": "api", "listen": "127.0.0.1", "listen_port": o.APIPort, "secret": o.APISecret,
		}},
	}
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// validate checks what Build checks for the same fields, with the same words.
func (o SetupOptions) validate() error {
	switch {
	case o.DataDir == "" || !filepath.IsAbs(o.DataDir):
		return fmt.Errorf("coreconfig: data folder must be an absolute path")
	case o.Hostname == "":
		return fmt.Errorf("coreconfig: hostname is empty")
	case o.APIPort <= 0 || o.APIPort > 65535:
		return fmt.Errorf("coreconfig: the API port must be a number 1..65535")
	case len(o.APISecret) < 16:
		return fmt.Errorf("coreconfig: secrets must be at least 16 characters")
	}
	return nil
}
